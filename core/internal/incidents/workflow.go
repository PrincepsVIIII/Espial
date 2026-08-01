package incidents

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/PrincepsVIIII/Espial/core/internal/audit"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var workflowUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

type Workflow struct {
	pool  *pgxpool.Pool
	hub   *events.Hub
	clock health.Clock
}

func NewWorkflow(pool *pgxpool.Pool, hub *events.Hub, clock health.Clock) *Workflow {
	if clock == nil {
		clock = health.SystemClock{}
	}
	return &Workflow{pool: pool, hub: hub, clock: clock}
}

func (workflow *Workflow) Assignees(ctx context.Context, limit int, cursorValue string) (AssigneeList, error) {
	limit = normalizedLimit(limit)
	page, err := decodeCursor(cursorValue, "incident_assignees", fingerprint("enabled-operators"))
	if err != nil {
		return AssigneeList{}, err
	}
	snapshot := page.Snapshot
	if snapshot.IsZero() {
		if err := workflow.pool.QueryRow(ctx, "SELECT now()").Scan(&snapshot); err != nil {
			return AssigneeList{}, fmt.Errorf("read incident assignee snapshot: %w", err)
		}
	}
	rows, err := workflow.pool.Query(ctx, `
		SELECT user_account.id::text, user_account.display_name, user_account.updated_at
		FROM users user_account
		WHERE user_account.enabled
		  AND length(user_account.display_name) BETWEEN 1 AND 128
		  AND user_account.updated_at <= $1
		  AND ($3 = '' OR (user_account.updated_at, user_account.id) < ($2, NULLIF($3, '')::uuid))
		  AND EXISTS (
			SELECT 1 FROM user_roles membership
			JOIN roles role ON role.id = membership.role_id
			WHERE membership.user_id = user_account.id
			  AND role.permissions ? 'incidents:operate'
		  )
		ORDER BY user_account.updated_at DESC, user_account.id DESC LIMIT $4
	`, snapshot.UTC(), nullableTime(page.OrderedAt), page.ID, limit+1)
	if err != nil {
		return AssigneeList{}, fmt.Errorf("list incident assignees: %w", err)
	}
	defer rows.Close()
	result := AssigneeList{Items: make([]Assignee, 0, limit+1)}
	updatedAt := make([]time.Time, 0, limit+1)
	for rows.Next() {
		var item Assignee
		var itemUpdatedAt time.Time
		if err := rows.Scan(&item.ID, &item.DisplayName, &itemUpdatedAt); err != nil {
			return AssigneeList{}, fmt.Errorf("scan incident assignee: %w", err)
		}
		result.Items = append(result.Items, item)
		updatedAt = append(updatedAt, itemUpdatedAt.UTC())
	}
	if err := rows.Err(); err != nil {
		return AssigneeList{}, fmt.Errorf("read incident assignees: %w", err)
	}
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		updatedAt = updatedAt[:limit]
		last := len(result.Items) - 1
		result.NextCursor, err = encodeCursor(cursor{
			Kind: "incident_assignees", Fingerprint: fingerprint("enabled-operators"),
			Snapshot: snapshot.UTC(), OrderedAt: updatedAt[last], ID: result.Items[last].ID,
		})
		if err != nil {
			return AssigneeList{}, fmt.Errorf("encode incident assignee cursor: %w", err)
		}
	}
	return result, nil
}

func (workflow *Workflow) Mutate(ctx context.Context, mutation Mutation) (MutationResult, error) {
	mutation.ActorName = strings.TrimSpace(mutation.ActorName)
	mutation.Note = strings.TrimSpace(mutation.Note)
	if err := validateMutation(mutation); err != nil {
		return MutationResult{}, err
	}
	requestHash, err := mutationHash(mutation)
	if err != nil {
		return MutationResult{}, err
	}
	tx, err := workflow.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return MutationResult{}, fmt.Errorf("begin incident mutation: %w", err)
	}
	defer tx.Rollback(ctx)

	lockKey := mutation.ActorUserID + "\n" + mutation.IncidentID + "\n" + string(mutation.Action) + "\n" + mutation.IdempotencyKey
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return MutationResult{}, fmt.Errorf("lock incident idempotency key: %w", err)
	}
	result, found, err := replayedMutation(ctx, tx, mutation, requestHash)
	if err != nil || found {
		return result, err
	}
	if mutation.Action == ActionResolve {
		rows, err := tx.Query(ctx, `
			SELECT state.rule_id
			FROM incidents incident
			JOIN incident_rule_states state
			  ON state.rule_id = incident.rule_id
			 AND state.resource_id = incident.resource_id
			 AND state.check_type = incident.check_type
			WHERE incident.id = $1
			FOR UPDATE OF state
		`, mutation.IncidentID)
		if err != nil {
			return MutationResult{}, fmt.Errorf("lock incident rule state: %w", err)
		}
		rows.Close()
	}

	var currentStatus Status
	var currentSeverity Severity
	var currentOwnerID string
	var currentVersion int64
	var currentUpdatedAt time.Time
	timelineKind := map[Action]string{
		ActionAcknowledge: "acknowledged", ActionInvestigate: "investigating",
		ActionAssign: "assigned", ActionNote: "note", ActionResolve: "resolved",
	}[mutation.Action]
	if err := tx.QueryRow(ctx, `
		SELECT status, severity, COALESCE(owner_user_id::text, ''), version, updated_at
		FROM incidents WHERE id = $1 FOR UPDATE
	`, mutation.IncidentID).Scan(&currentStatus, &currentSeverity, &currentOwnerID, &currentVersion, &currentUpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MutationResult{}, ErrNotFound
		}
		return MutationResult{}, fmt.Errorf("load incident for mutation: %w", err)
	}
	if currentVersion != mutation.ExpectedVersion {
		return MutationResult{}, &VersionConflictError{CurrentVersion: currentVersion}
	}

	toStatus := currentStatus
	subjectID, subjectName := "", ""
	summary := ""
	auditAction := ""
	afterSummary := map[string]any{"status": currentStatus, "version": currentVersion + 1}
	switch mutation.Action {
	case ActionAcknowledge:
		if currentStatus != StatusOpen {
			return MutationResult{}, ErrInvalidTransition
		}
		toStatus, summary, auditAction = StatusAcknowledged, "Acknowledged by "+mutation.ActorName+".", "incident.acknowledged"
	case ActionInvestigate:
		if currentStatus != StatusAcknowledged {
			return MutationResult{}, ErrInvalidTransition
		}
		toStatus, summary, auditAction = StatusInvestigating, "Investigation started by "+mutation.ActorName+".", "incident.investigation.started"
	case ActionAssign:
		if currentStatus == StatusResolved {
			return MutationResult{}, ErrInvalidTransition
		}
		if err := tx.QueryRow(ctx, `
			SELECT user_account.display_name
			FROM users user_account
			WHERE user_account.id = $1 AND user_account.enabled
			  AND length(user_account.display_name) BETWEEN 1 AND 128
			  AND EXISTS (
				SELECT 1 FROM user_roles membership
				JOIN roles role ON role.id = membership.role_id
				WHERE membership.user_id = user_account.id
				  AND role.permissions ? 'incidents:operate'
			  )
			FOR KEY SHARE
		`, mutation.OwnerUserID).Scan(&subjectName); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return MutationResult{}, ErrOwnerNotEligible
			}
			return MutationResult{}, fmt.Errorf("validate incident owner: %w", err)
		}
		subjectID = mutation.OwnerUserID
		summary, auditAction = "Assigned to "+subjectName+".", "incident.assigned"
		afterSummary["owner_user_id"] = subjectID
	case ActionNote:
		summary, auditAction = mutation.Note, "incident.note.added"
		afterSummary["note_characters"] = utf8.RuneCountInString(mutation.Note)
	case ActionResolve:
		if currentStatus != StatusRecovered {
			return MutationResult{}, ErrInvalidTransition
		}
		toStatus, summary, auditAction = StatusResolved, mutation.Note, "incident.resolved"
		afterSummary["note_characters"] = utf8.RuneCountInString(mutation.Note)
	default:
		return MutationResult{}, ErrInvalidMutation
	}
	afterSummary["status"] = toStatus

	now := workflow.clock.Now().UTC().Truncate(time.Microsecond)
	if !now.After(currentUpdatedAt) {
		now = currentUpdatedAt.UTC().Add(time.Microsecond)
	}
	if err := updateIncident(ctx, tx, mutation, toStatus, now); err != nil {
		return MutationResult{}, err
	}
	if mutation.Action == ActionResolve {
		if _, err := tx.Exec(ctx, `
			UPDATE incident_rule_states
			SET active_incident_id = NULL, updated_at = $2
			WHERE active_incident_id = $1
		`, mutation.IncidentID, now); err != nil {
			return MutationResult{}, fmt.Errorf("release resolved incident fingerprint: %w", err)
		}
	}
	var timelineID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO incident_timeline (
			id, incident_id, actor_user_id, actor_display_name,
			subject_user_id, subject_display_name, kind, from_status, to_status,
			from_severity, to_severity, summary, occurred_at
		) VALUES (
			gen_random_uuid(), $1, $2, $3, NULLIF($4, '')::uuid, NULLIF($5, ''),
			$6, $7, $8, $9, $9, $10, $11
		) RETURNING id::text
	`, mutation.IncidentID, mutation.ActorUserID, mutation.ActorName, subjectID,
		subjectName, timelineKind, currentStatus, toStatus, currentSeverity,
		summary, now).Scan(&timelineID); err != nil {
		return MutationResult{}, fmt.Errorf("append incident timeline: %w", err)
	}
	if err := audit.Append(ctx, tx, audit.Event{
		ActorUserID: mutation.ActorUserID, Action: auditAction,
		TargetType: "incident", TargetID: mutation.IncidentID, Result: "succeeded",
		SourceAddress: mutation.SourceAddress, CorrelationID: mutation.CorrelationID,
		BeforeSummary: map[string]any{
			"status": currentStatus, "owner_user_id": currentOwnerID, "version": currentVersion,
		},
		AfterSummary: afterSummary, OccurredAt: now,
	}); err != nil {
		return MutationResult{}, err
	}
	result = MutationResult{
		IncidentID: mutation.IncidentID, Status: toStatus, Version: currentVersion + 1,
		TimelineEventID: timelineID, CorrelationID: mutation.CorrelationID,
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO incident_action_idempotency (
			actor_user_id, incident_id, action, idempotency_key, request_hash,
			result_version, result_status, timeline_event_id, correlation_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, mutation.ActorUserID, mutation.IncidentID, mutation.Action,
		mutation.IdempotencyKey, requestHash[:], result.Version, result.Status,
		result.TimelineEventID, result.CorrelationID, now); err != nil {
		return MutationResult{}, fmt.Errorf("store incident idempotency receipt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return MutationResult{}, fmt.Errorf("commit incident mutation: %w", err)
	}
	if workflow.hub != nil {
		workflow.hub.Publish(events.Event{
			Kind: events.IncidentChanged, IncidentID: mutation.IncidentID,
			Result: string(toStatus), ChangedAt: now,
		})
	}
	return result, nil
}

func replayedMutation(ctx context.Context, tx pgx.Tx, mutation Mutation, requestHash [32]byte) (MutationResult, bool, error) {
	var result MutationResult
	var storedHash []byte
	err := tx.QueryRow(ctx, `
		SELECT request_hash, result_version, result_status, timeline_event_id::text,
			correlation_id
		FROM incident_action_idempotency
		WHERE actor_user_id = $1 AND incident_id = $2 AND action = $3
		  AND idempotency_key = $4
	`, mutation.ActorUserID, mutation.IncidentID, mutation.Action,
		mutation.IdempotencyKey).Scan(&storedHash, &result.Version, &result.Status,
		&result.TimelineEventID, &result.CorrelationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return MutationResult{}, false, nil
	}
	if err != nil {
		return MutationResult{}, false, fmt.Errorf("read incident idempotency receipt: %w", err)
	}
	if string(storedHash) != string(requestHash[:]) {
		return MutationResult{}, false, ErrIdempotencyConflict
	}
	result.IncidentID, result.Replayed = mutation.IncidentID, true
	return result, true, nil
}

func updateIncident(ctx context.Context, tx pgx.Tx, mutation Mutation, status Status, now time.Time) error {
	var err error
	switch mutation.Action {
	case ActionAcknowledge:
		_, err = tx.Exec(ctx, `UPDATE incidents SET status = $2, acknowledged_at = $3,
			version = version + 1, updated_at = $3 WHERE id = $1`, mutation.IncidentID, status, now)
	case ActionAssign:
		_, err = tx.Exec(ctx, `UPDATE incidents SET owner_user_id = $2,
			version = version + 1, updated_at = $3 WHERE id = $1`, mutation.IncidentID, mutation.OwnerUserID, now)
	case ActionResolve:
		_, err = tx.Exec(ctx, `UPDATE incidents SET status = $2, resolved_at = $3,
			version = version + 1, updated_at = $3 WHERE id = $1`, mutation.IncidentID, status, now)
	default:
		_, err = tx.Exec(ctx, `UPDATE incidents SET status = $2,
			version = version + 1, updated_at = $3 WHERE id = $1`, mutation.IncidentID, status, now)
	}
	if err != nil {
		return fmt.Errorf("update incident: %w", err)
	}
	return nil
}

func validateMutation(mutation Mutation) error {
	if !workflowUUIDPattern.MatchString(mutation.IncidentID) ||
		!workflowUUIDPattern.MatchString(mutation.ActorUserID) ||
		mutation.ExpectedVersion < 1 || mutation.ActorName == "" ||
		utf8.RuneCountInString(mutation.ActorName) > 128 ||
		mutation.CorrelationID == "" || len(mutation.CorrelationID) > 128 ||
		!validIdempotencyKey(mutation.IdempotencyKey) {
		return ErrInvalidMutation
	}
	switch mutation.Action {
	case ActionAssign:
		if !workflowUUIDPattern.MatchString(mutation.OwnerUserID) || mutation.Note != "" {
			return ErrInvalidMutation
		}
	case ActionNote, ActionResolve:
		if mutation.OwnerUserID != "" || mutation.Note == "" || utf8.RuneCountInString(mutation.Note) > MaxNoteRunes {
			return ErrInvalidNote
		}
	case ActionAcknowledge, ActionInvestigate:
		if mutation.OwnerUserID != "" || mutation.Note != "" {
			return ErrInvalidMutation
		}
	default:
		return ErrInvalidMutation
	}
	return nil
}

func validIdempotencyKey(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) {
			return false
		}
	}
	return true
}

func mutationHash(mutation Mutation) ([32]byte, error) {
	payload, err := json.Marshal(struct {
		Action          Action `json:"action"`
		ExpectedVersion int64  `json:"expected_version"`
		OwnerUserID     string `json:"owner_user_id,omitempty"`
		Note            string `json:"note,omitempty"`
	}{mutation.Action, mutation.ExpectedVersion, mutation.OwnerUserID, mutation.Note})
	if err != nil {
		return [32]byte{}, fmt.Errorf("encode incident mutation hash: %w", err)
	}
	return sha256.Sum256(payload), nil
}
