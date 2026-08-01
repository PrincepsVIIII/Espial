package incidents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	workflowActorID    = "60000000-0000-4000-8000-000000000021"
	workflowAssigneeID = "60000000-0000-4000-8000-000000000022"
)

func TestWorkflowTransitionTable(t *testing.T) {
	pool := incidentTestPool(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	seedWorkflowUsers(t, pool)
	workflow := NewWorkflow(pool, nil, health.FixedClock{Time: now})
	statuses := []Status{StatusOpen, StatusAcknowledged, StatusInvestigating, StatusRecovered, StatusResolved}
	actions := []Action{ActionAcknowledge, ActionInvestigate, ActionAssign, ActionNote, ActionResolve}

	for _, status := range statuses {
		for _, action := range actions {
			t.Run(string(status)+"_"+string(action), func(t *testing.T) {
				id := seedWorkflowIncident(t, pool, status, now)
				mutation := workflowMutation(id, action, 1, fmt.Sprintf("table-%s-%s", status, action))
				result, err := workflow.Mutate(context.Background(), mutation)
				valid := action == ActionNote ||
					action == ActionAssign && status != StatusResolved ||
					action == ActionAcknowledge && status == StatusOpen ||
					action == ActionInvestigate && status == StatusAcknowledged ||
					action == ActionResolve && status == StatusRecovered
				if valid && err != nil {
					t.Fatalf("valid transition failed: %v", err)
				}
				if !valid && !errors.Is(err, ErrInvalidTransition) {
					t.Fatalf("invalid transition error = %v", err)
				}
				if valid && (result.Version != 2 || result.TimelineEventID == "") {
					t.Fatalf("mutation result = %#v", result)
				}
			})
		}
	}
}

func TestWorkflowConcurrencyIdempotencyAuditAndIdentitySnapshots(t *testing.T) {
	pool := incidentTestPool(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	seedWorkflowUsers(t, pool)
	hub := events.NewHub(8)
	workflow := NewWorkflow(pool, hub, health.FixedClock{Time: now})
	id := seedWorkflowIncident(t, pool, StatusOpen, now)

	var wait sync.WaitGroup
	wait.Add(2)
	errorsSeen := make(chan error, 2)
	for _, key := range []string{"concurrent-a", "concurrent-b"} {
		go func(idempotencyKey string) {
			defer wait.Done()
			_, err := workflow.Mutate(context.Background(), workflowMutation(id, ActionAcknowledge, 1, idempotencyKey))
			errorsSeen <- err
		}(key)
	}
	wait.Wait()
	close(errorsSeen)
	succeeded, conflicted := 0, 0
	for err := range errorsSeen {
		if err == nil {
			succeeded++
		} else if errors.Is(err, ErrVersionConflict) {
			conflicted++
		} else {
			t.Fatalf("concurrent mutation error = %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent outcomes succeeded=%d conflicted=%d", succeeded, conflicted)
	}

	note := "Investigating switch uplink; <script>alert('not markup')</script>"
	first, err := workflow.Mutate(context.Background(), Mutation{
		IncidentID: id, Action: ActionNote, Note: note, ExpectedVersion: 2,
		IdempotencyKey: "durable-note", ActorUserID: workflowActorID,
		ActorName: "Original Operator", SourceAddress: "192.0.2.10", CorrelationID: "request-note-original",
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := workflow.Mutate(context.Background(), Mutation{
		IncidentID: id, Action: ActionNote, Note: note, ExpectedVersion: 2,
		IdempotencyKey: "durable-note", ActorUserID: workflowActorID,
		ActorName: "Original Operator", SourceAddress: "192.0.2.10", CorrelationID: "request-note-retry",
	})
	if err != nil || !replay.Replayed || replay.TimelineEventID != first.TimelineEventID || replay.CorrelationID != first.CorrelationID {
		t.Fatalf("idempotent replay = %#v, %v", replay, err)
	}
	if _, err := workflow.Mutate(context.Background(), Mutation{
		IncidentID: id, Action: ActionNote, Note: "different", ExpectedVersion: 2,
		IdempotencyKey: "durable-note", ActorUserID: workflowActorID,
		ActorName: "Original Operator", SourceAddress: "192.0.2.10", CorrelationID: "request-note-conflict",
	}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("idempotency conflict = %v", err)
	}

	assigned, err := workflow.Mutate(context.Background(), Mutation{
		IncidentID: id, Action: ActionAssign, OwnerUserID: workflowAssigneeID, ExpectedVersion: 3,
		IdempotencyKey: "assign-owner", ActorUserID: workflowActorID,
		ActorName: "Original Operator", SourceAddress: "192.0.2.10", CorrelationID: "request-assign",
	})
	if err != nil || assigned.Version != 4 {
		t.Fatalf("assignment = %#v, %v", assigned, err)
	}
	if _, err := pool.Exec(context.Background(), `
		UPDATE users SET display_name = CASE id
			WHEN $1 THEN 'Renamed Operator' WHEN $2 THEN 'Renamed Assignee' END
		WHERE id IN ($1, $2)
	`, workflowActorID, workflowAssigneeID); err != nil {
		t.Fatal(err)
	}
	timeline, err := NewReader(pool).Timeline(context.Background(), id, TimelineFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	var noteEvents, assignmentEvents int
	for _, event := range timeline.Items {
		switch event.Kind {
		case "note":
			noteEvents++
			if event.Summary != note || event.ActorName != "Original Operator" {
				t.Fatalf("note evidence = %#v", event)
			}
		case "assigned":
			assignmentEvents++
			if event.ActorName != "Original Operator" || event.SubjectName != "Original Assignee" {
				t.Fatalf("assignment identity snapshot = %#v", event)
			}
		}
	}
	if noteEvents != 1 || assignmentEvents != 1 {
		t.Fatalf("timeline event counts note=%d assignment=%d", noteEvents, assignmentEvents)
	}
	var noteAudits int
	var auditText string
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*), COALESCE(string_agg(COALESCE(before_summary::text, '') || COALESCE(after_summary::text, ''), ''), '')
		FROM audit_events WHERE target_id = $1 AND action = 'incident.note.added'
	`, id).Scan(&noteAudits, &auditText); err != nil {
		t.Fatal(err)
	}
	if noteAudits != 1 || strings.Contains(auditText, note) || !strings.Contains(auditText, "note_characters") {
		t.Fatalf("redacted audit count=%d text=%q", noteAudits, auditText)
	}
}

func TestWorkflowAssigneeEligibilityAndValidationBounds(t *testing.T) {
	pool := incidentTestPool(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	seedWorkflowUsers(t, pool)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, username, display_name, identity_provider, enabled)
		VALUES
		('60000000-0000-4000-8000-000000000023', 'disabled-op', 'Disabled Operator', 'local', false),
		('60000000-0000-4000-8000-000000000024', 'viewer-only', 'Viewer Only', 'local', true)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO user_roles (user_id, role_id) VALUES
		('60000000-0000-4000-8000-000000000023', '10000000-0000-4000-8000-000000000002'),
		('60000000-0000-4000-8000-000000000024', '10000000-0000-4000-8000-000000000001')
	`); err != nil {
		t.Fatal(err)
	}
	workflow := NewWorkflow(pool, nil, health.FixedClock{Time: now})
	assignees, err := workflow.Assignees(context.Background(), 100, "")
	if err != nil || len(assignees.Items) != 2 {
		t.Fatalf("eligible assignees = %#v, %v", assignees, err)
	}
	firstPage, err := workflow.Assignees(context.Background(), 1, "")
	if err != nil || len(firstPage.Items) != 1 || firstPage.NextCursor == "" {
		t.Fatalf("first assignee page = %#v, %v", firstPage, err)
	}
	secondPage, err := workflow.Assignees(context.Background(), 1, firstPage.NextCursor)
	if err != nil || len(secondPage.Items) != 1 || secondPage.Items[0].ID == firstPage.Items[0].ID {
		t.Fatalf("second assignee page = %#v, %v", secondPage, err)
	}
	if _, err := workflow.Assignees(context.Background(), 1, "not-a-cursor"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("invalid assignee cursor = %v", err)
	}
	id := seedWorkflowIncident(t, pool, StatusOpen, now)
	for _, owner := range []string{"60000000-0000-4000-8000-000000000023", "60000000-0000-4000-8000-000000000024"} {
		mutation := workflowMutation(id, ActionAssign, 1, "invalid-owner-"+owner)
		mutation.OwnerUserID = owner
		if _, err := workflow.Mutate(context.Background(), mutation); !errors.Is(err, ErrOwnerNotEligible) {
			t.Fatalf("ineligible owner %s error = %v", owner, err)
		}
	}
	mutation := workflowMutation(id, ActionNote, 1, "long-note")
	mutation.Note = strings.Repeat("x", MaxNoteRunes+1)
	if _, err := workflow.Mutate(context.Background(), mutation); !errors.Is(err, ErrInvalidNote) {
		t.Fatalf("long note error = %v", err)
	}
	mutation = workflowMutation(id, ActionResolve, 1, "empty-resolution")
	mutation.Note = "   "
	if _, err := workflow.Mutate(context.Background(), mutation); !errors.Is(err, ErrInvalidNote) {
		t.Fatalf("empty resolution error = %v", err)
	}

	recoveredID := seedWorkflowIncident(t, pool, StatusRecovered, now)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO incident_rule_states (
			rule_id, resource_id, check_type, active_incident_id
		)
		SELECT rule_id, resource_id, check_type, id FROM incidents WHERE id = $1
	`, recoveredID); err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.Mutate(context.Background(), workflowMutation(recoveredID, ActionResolve, 1, "release-fingerprint")); err != nil {
		t.Fatal(err)
	}
	var released bool
	if err := pool.QueryRow(context.Background(), `
		SELECT active_incident_id IS NULL FROM incident_rule_states
		WHERE active_incident_id IS NULL AND resource_id = (
			SELECT resource_id FROM incidents WHERE id = $1
		)
	`, recoveredID).Scan(&released); err != nil || !released {
		t.Fatalf("resolved fingerprint released = %v, %v", released, err)
	}
}

func workflowMutation(id string, action Action, version int64, key string) Mutation {
	mutation := Mutation{
		IncidentID: id, Action: action, ExpectedVersion: version,
		IdempotencyKey: key, ActorUserID: workflowActorID, ActorName: "Original Operator",
		SourceAddress: "192.0.2.10", CorrelationID: "request-" + key,
	}
	if action == ActionAssign {
		mutation.OwnerUserID = workflowAssigneeID
	}
	if action == ActionNote || action == ActionResolve {
		mutation.Note = "Bounded plain-text operator note."
	}
	return mutation
}

func seedWorkflowUsers(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, username, display_name, identity_provider, enabled) VALUES
		($1, 'workflow-operator', 'Original Operator', 'local', true),
		($2, 'workflow-assignee', 'Original Assignee', 'local', true)
	`, workflowActorID, workflowAssigneeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO user_roles (user_id, role_id) VALUES
		($1, '10000000-0000-4000-8000-000000000002'),
		($2, '10000000-0000-4000-8000-000000000003')
	`, workflowActorID, workflowAssigneeID); err != nil {
		t.Fatal(err)
	}
}

func seedWorkflowIncident(t *testing.T, pool *pgxpool.Pool, status Status, now time.Time) string {
	t.Helper()
	var resourceID, incidentID string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO resources (
			id, integration_id, external_id, kind, display_name, first_seen_at, last_seen_at
		) VALUES (
			gen_random_uuid(), $1, gen_random_uuid()::text, 'host', 'Workflow resource', $2, $2
		) RETURNING id::text
	`, incidentIntegrationID, now).Scan(&resourceID); err != nil {
		t.Fatal(err)
	}
	var recoveredAt, resolvedAt any
	if status == StatusRecovered || status == StatusResolved {
		recoveredAt = now
	}
	if status == StatusResolved {
		resolvedAt = now
	}
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO incidents (
			rule_id, integration_id, resource_id, check_type, fingerprint, title,
			summary, severity, status, detected_at, latest_signal_at, recovered_at,
			resolved_at, version, updated_at
		) VALUES (
			'20000000-0000-4000-8000-000000000001', $1, $2, 'availability',
			gen_random_uuid()::text, 'Workflow incident', 'Workflow summary',
			'critical', $3, $4, $4, $5, $6, 1, $4
		) RETURNING id::text
	`, incidentIntegrationID, resourceID, status, now, recoveredAt, resolvedAt).Scan(&incidentID); err != nil {
		t.Fatal(err)
	}
	return incidentID
}
