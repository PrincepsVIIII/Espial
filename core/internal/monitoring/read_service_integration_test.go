package monitoring

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/audit"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/observations"
)

const readIntegrationB = "60000000-0000-4000-8000-000000000002"

func TestReadServiceOverviewFiltersDetailsAndStablePagination(t *testing.T) {
	pool := monitoringTestPool(t)
	insertPipelineIntegration(t, pool)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO integrations (id, adapter_id, display_name, enabled, interval_seconds, updated_at)
		VALUES ($1, 'org.ubnetdef.espial.sample', 'Second integration', false, 300, $2)
	`, readIntegrationB, time.Date(2026, 7, 31, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: base}
	ingestor := observations.NewService(pool, observations.Options{Clock: clock})
	states := []health.State{health.Healthy, health.Warning, health.Critical, health.Healthy, health.Warning}
	for index, state := range states {
		observedAt := base.Add(time.Duration(index) * time.Second)
		refresh := 300
		if index == 3 {
			observedAt = base.Add(-2 * time.Minute)
			refresh = 60
		}
		if index == 4 {
			observedAt = base.Add(-4 * time.Minute)
			refresh = 60
		}
		batch := observations.Batch{
			Resources: []observations.ResourceInput{{
				ExternalID: fmt.Sprintf("read-node-%d", index), Kind: []string{"host", "server"}[index%2],
				DisplayName: fmt.Sprintf("Read node %d", index), ObservedAt: observedAt,
				Attributes: map[string]any{"ordinal": index},
			}},
			Observations: []observations.ObservationInput{{
				ExternalResourceID: fmt.Sprintf("read-node-%d", index), CheckType: "availability",
				State: state, Summary: fmt.Sprintf("state %s", state), ObservedAt: observedAt,
				ExpectedRefreshSeconds: refresh, Measurements: map[string]any{}, Metadata: map[string]any{},
			}},
		}
		if _, err := ingestor.Ingest(context.Background(), pipelineIntegrationID, batch); err != nil {
			t.Fatal(err)
		}
	}
	service := NewReadService(pool)
	overview, err := service.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if overview.StaleCount != 1 || overview.UnknownCount != 1 || len(overview.RecentChanges) != 5 {
		t.Fatalf("overview = %#v", overview)
	}
	integrationCounts := map[string]int64{}
	for _, count := range overview.IntegrationCounts {
		integrationCounts[count.State] = count.Count
	}
	if integrationCounts["not_started"] != 1 || integrationCounts["disabled"] != 1 {
		t.Fatalf("integration counts = %#v", integrationCounts)
	}

	seen := map[string]bool{}
	cursor := ""
	for {
		page, err := service.Resources(context.Background(), ResourceFilter{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, resource := range page.Items {
			if seen[resource.ID] {
				t.Fatalf("duplicate resource %s", resource.ID)
			}
			seen[resource.ID] = true
			if resource.LatestObservation == nil || len(resource.Attributes) == 0 {
				t.Fatalf("incomplete resource = %#v", resource)
			}
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(seen) != 5 {
		t.Fatalf("resource count across pages = %d", len(seen))
	}
	stale := true
	filtered, err := service.Resources(context.Background(), ResourceFilter{
		Limit: 10, States: []health.State{health.Stale}, Kinds: []string{"server"},
		IntegrationIDs: []string{pipelineIntegrationID}, Stale: &stale,
	})
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].Health.State != health.Stale {
		t.Fatalf("filtered resources = %#v, %v", filtered, err)
	}
	staleOnly, err := service.Resources(context.Background(), ResourceFilter{Limit: 10, Stale: &stale})
	if err != nil || len(staleOnly.Items) != 1 || staleOnly.Items[0].Health.State != health.Stale {
		t.Fatalf("stale-only resources = %#v, %v", staleOnly, err)
	}
	notStale := false
	nonStale, err := service.Resources(context.Background(), ResourceFilter{Limit: 10, Stale: &notStale})
	if err != nil || len(nonStale.Items) != 4 {
		t.Fatalf("non-stale resources = %#v, %v", nonStale, err)
	}
	detail, err := service.Resource(context.Background(), filtered.Items[0].ID)
	if err != nil || detail.ID != filtered.Items[0].ID {
		t.Fatalf("resource detail = %#v, %v", detail, err)
	}
	if _, err := service.Resource(context.Background(), "60000000-0000-4000-8000-000000000099"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing resource error = %v", err)
	}
	if _, err := service.Resources(context.Background(), ResourceFilter{Cursor: "not-a-cursor"}); safeErrorCode(err) != "invalid_cursor" {
		t.Fatalf("invalid cursor error = %v", err)
	}

	integrations, err := service.Integrations(context.Background(), IntegrationFilter{Limit: 1})
	if err != nil || len(integrations.Items) != 1 || integrations.NextCursor == "" {
		t.Fatalf("first integration page = %#v, %v", integrations, err)
	}
	second, err := service.Integrations(context.Background(), IntegrationFilter{Limit: 1, Cursor: integrations.NextCursor})
	if err != nil || len(second.Items) != 1 || second.Items[0].ID == integrations.Items[0].ID {
		t.Fatalf("second integration page = %#v, %v", second, err)
	}
	disabled := false
	if _, err := service.Integrations(context.Background(), IntegrationFilter{Limit: 1, Cursor: integrations.NextCursor, Enabled: &disabled}); safeErrorCode(err) != "invalid_cursor" {
		t.Fatalf("changed-filter cursor error = %v", err)
	}
	integration, err := service.Integration(context.Background(), pipelineIntegrationID)
	if err != nil || integration.ResourceCount != 5 || len(integration.ConfigKeys) != 0 {
		t.Fatalf("integration detail = %#v, %v", integration, err)
	}
}

func TestAuditReadUsesBoundedStableCursorAndRecordsAdministrativeRead(t *testing.T) {
	pool := monitoringTestPool(t)
	insertPipelineIntegration(t, pool)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, username, display_name, identity_provider)
		VALUES ('70000000-0000-4000-8000-000000000011', 'audit-admin', 'Audit admin', 'local')
	`); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	writer := audit.NewWriter(pool)
	for index := 0; index < 5; index++ {
		if err := writer.Append(context.Background(), audit.Event{
			ActorUserID: "70000000-0000-4000-8000-000000000011",
			Action:      "test.audit.event", TargetType: "integration", TargetID: pipelineIntegrationID,
			Result: "succeeded", SourceAddress: "127.0.0.1",
			CorrelationID: fmt.Sprintf("audit-%d", index),
			AfterSummary:  map[string]any{"safe_count": index}, OccurredAt: base.Add(time.Duration(index) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	service := NewReadService(pool)
	filter := AuditFilter{
		Limit: 2, From: base.Add(-time.Minute), To: base.Add(10 * time.Minute),
		FromExplicit: true, ToExplicit: true, Actions: []string{"test.audit.event"},
	}
	seen := map[string]bool{}
	for {
		page, err := service.Audit(context.Background(), filter)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range page.Items {
			if seen[event.ID] || event.ActorUsername != "audit-admin" || len(event.AfterSummary) == 0 {
				t.Fatalf("bad audit page item = %#v", event)
			}
			seen[event.ID] = true
		}
		filter.Cursor = page.NextCursor
		filter.FromExplicit, filter.ToExplicit = false, false
		if filter.Cursor == "" {
			break
		}
	}
	if len(seen) != 5 {
		t.Fatalf("audit count across pages = %d", len(seen))
	}
	receipt, err := service.Audit(context.Background(), AuditFilter{
		Limit: 10, From: base.Add(-time.Minute), To: base.Add(10 * time.Minute),
		FromExplicit: true, ToExplicit: true, CorrelationID: "audit-3",
	})
	if err != nil || len(receipt.Items) != 1 || receipt.Items[0].CorrelationID != "audit-3" {
		t.Fatalf("audit receipt lookup = %#v, %v", receipt, err)
	}
	if err := service.RecordAuditRead(context.Background(),
		"70000000-0000-4000-8000-000000000011", "127.0.0.1", "audit-read", filter,
	); err != nil {
		t.Fatal(err)
	}
	var readCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events WHERE action = 'audit.read' AND correlation_id = 'audit-read'
	`).Scan(&readCount); err != nil || readCount != 1 {
		t.Fatalf("audit read count = %d, %v", readCount, err)
	}
}

func TestIntegrationCreateRequiresRegisteredAdapterAndAuditsSafely(t *testing.T) {
	pool := monitoringTestPool(t)
	hub := events.NewHub(8)
	registry, err := adapters.NewRegistry(adapters.Descriptor{
		AdapterID: "org.ubnetdef.espial.sample", Executable: "/trusted/sample-adapter",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	service := NewIntegrationConfigService(pool, hub, health.FixedClock{Time: now}, registry)
	_, _, err = service.Create(context.Background(), CreateIntegration{
		AdapterID: "unregistered.adapter", DisplayName: "Bad", Interval: time.Minute,
		CorrelationID: "unknown-adapter",
	})
	if safeErrorCode(err) != "adapter_not_registered" {
		t.Fatalf("unknown adapter error = %v", err)
	}
	id, updatedAt, err := service.Create(context.Background(), CreateIntegration{
		AdapterID: "org.ubnetdef.espial.sample", DisplayName: "API sample", Enabled: true,
		Interval: time.Minute, ConfigNonsecret: map[string]any{"scenario": "healthy"},
		SecretReferences: map[string]string{"api_token": "vault://secret/value"},
		CorrelationID:    "create-integration",
	})
	if err != nil || !integrationUUIDPattern.MatchString(id) || !updatedAt.Equal(now) {
		t.Fatalf("create = %s %s %v", id, updatedAt, err)
	}
	var summary string
	if err := pool.QueryRow(context.Background(), `
		SELECT after_summary::text FROM audit_events WHERE action = 'integration.created'
	`).Scan(&summary); err != nil {
		t.Fatal(err)
	}
	if containsSecretLikeMaterial(summary) || stringContainsAny(summary, "healthy", "vault://secret/value") {
		t.Fatalf("unsafe create audit = %s", summary)
	}
}

func safeErrorCode(err error) string {
	var operational *Error
	if errors.As(err, &operational) {
		return operational.Code
	}
	return ""
}

func stringContainsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
