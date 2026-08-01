package incidents

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/observations"
	"github.com/PrincepsVIIII/Espial/core/internal/suppressions"
	"github.com/jackc/pgx/v5/pgxpool"
)

const slice23ActorID = "60000000-0000-4000-8000-000000000031"

func TestRuleAdministrationPrecedenceConcurrencyAndPersistedDebounce(t *testing.T) {
	pool := incidentTestPool(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	clock := &testClock{now: start}
	ingestor := observations.NewService(pool, observations.Options{Clock: clock})
	if _, err := ingestor.Ingest(ctx, incidentIntegrationID,
		observationBatch("rule-node", "Rule node", "rule-healthy", health.Healthy, "ready", start, 60, true)); err != nil {
		t.Fatal(err)
	}
	seedSlice23Actor(t, pool)
	var resourceID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM resources WHERE external_id='rule-node'`).Scan(&resourceID); err != nil {
		t.Fatal(err)
	}
	service := NewRuleService(pool, clock.Now)
	definition := RuleDefinition{
		Name: "Exact warning rule", Priority: 500, ResourceID: resourceID,
		RecoveryState: health.Healthy, RecoveryMinOccurrences: 1,
		Conditions: []RuleCondition{{State: health.Warning, Severity: SeverityWarning, MinOccurrences: 2}},
	}
	receipt, err := service.Create(ctx, RuleWrite{Definition: definition, Enabled: true,
		IdempotencyKey: "create-exact-rule", ActorUserID: slice23ActorID, ActorName: "Slice Admin", CorrelationID: "rule-create"})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.Create(ctx, RuleWrite{Definition: definition, Enabled: true,
		IdempotencyKey: "create-exact-rule", ActorUserID: slice23ActorID, ActorName: "Slice Admin", CorrelationID: "rule-create"})
	if err != nil || !replay.Replayed || replay.ID != receipt.ID {
		t.Fatalf("idempotent rule replay = %#v, %v", replay, err)
	}
	preview, err := service.Preview(ctx, RulePreviewInput{IntegrationID: incidentIntegrationID, ResourceID: resourceID, CheckType: "availability", State: health.Warning})
	if err != nil || preview.Winner == nil || preview.Winner.ID != receipt.ID || len(preview.Candidates) < 2 {
		t.Fatalf("rule preview = %#v, %v", preview, err)
	}

	clock.Set(start.Add(time.Minute))
	if _, err := ingestor.Ingest(ctx, incidentIntegrationID,
		observationBatch("rule-node", "Rule node", "rule-warning-1", health.Warning, "loss", clock.Now(), 60, false)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEvaluator(pool, nil, Options{Clock: clock}).ProcessOnce(ctx); err != nil {
		t.Fatal(err)
	}
	definition.Name = "Exact warning rule updated"
	replaced, err := service.Replace(ctx, receipt.ID, RuleWrite{Definition: definition, Enabled: true, ExpectedVersion: receipt.Version,
		IdempotencyKey: "replace-exact-rule", ActorUserID: slice23ActorID, ActorName: "Slice Admin", CorrelationID: "rule-replace"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Replace(ctx, receipt.ID, RuleWrite{Definition: definition, Enabled: true, ExpectedVersion: receipt.Version,
		IdempotencyKey: "stale-rule-replace", ActorUserID: slice23ActorID, ActorName: "Slice Admin", CorrelationID: "rule-stale"})
	if !errors.Is(err, ErrRuleConflict) {
		t.Fatalf("stale rule replacement error = %v", err)
	}
	clock.Set(start.Add(2 * time.Minute))
	if _, err := ingestor.Ingest(ctx, incidentIntegrationID,
		observationBatch("rule-node", "Rule node", "rule-warning-2", health.Warning, "loss remains", clock.Now(), 60, false)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEvaluator(pool, nil, Options{Clock: clock}).ProcessOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if count := incidentCount(t, pool, "rule-node"); count != 1 {
		t.Fatalf("persisted debounce incident count = %d", count)
	}
	detail, err := service.Detail(ctx, receipt.ID)
	if err != nil || detail.Version != replaced.Version || detail.Name != definition.Name {
		t.Fatalf("replaced rule = %#v, %v", detail, err)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE correlation_id IN ('rule-create','rule-replace')`).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("rule audit count = %d, %v", auditCount, err)
	}
	disabled, err := service.Replace(ctx, receipt.ID, RuleWrite{Definition: definition, Enabled: false, ExpectedVersion: replaced.Version,
		IdempotencyKey: "disable-exact-rule", ActorUserID: slice23ActorID, ActorName: "Slice Admin", CorrelationID: "rule-disable"})
	if err != nil || disabled.Version != replaced.Version+1 {
		t.Fatalf("disable rule = %#v, %v", disabled, err)
	}
	preview, err = service.Preview(ctx, RulePreviewInput{IntegrationID: incidentIntegrationID, ResourceID: resourceID, CheckType: "availability", State: health.Warning})
	if err != nil || preview.Winner == nil || preview.Winner.ID == receipt.ID {
		t.Fatalf("disabled rule preview = %#v, %v", preview, err)
	}
}

func TestMaintenancePreservesRawHealthAndReevaluatesAtExpiry(t *testing.T) {
	pool := incidentTestPool(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)
	clock := &testClock{now: start}
	ingestor := observations.NewService(pool, observations.Options{Clock: clock})
	seedSlice23Actor(t, pool)
	if _, err := ingestor.Ingest(ctx, incidentIntegrationID,
		observationBatch("maint-node", "Maintenance node", "maint-healthy", health.Healthy, "ready", start, 60, true)); err != nil {
		t.Fatal(err)
	}
	var resourceID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM resources WHERE external_id='maint-node'`).Scan(&resourceID); err != nil {
		t.Fatal(err)
	}
	control := suppressions.NewService(pool, nil, clock.Now)
	overlapReceipt, err := control.CreateMaintenance(ctx, suppressions.MaintenanceDefinition{
		Reason: "Integration work", IntegrationID: incidentIntegrationID,
		StartsAt: start, EndsAt: start.Add(10 * time.Minute), Enabled: true,
	}, suppressions.MutationMetadata{IdempotencyKey: "maintenance-overlap", ActorUserID: slice23ActorID, ActorName: "Slice Admin", CorrelationID: "maintenance-overlap"})
	if err != nil {
		t.Fatal(err)
	}
	windowReceipt, err := control.CreateMaintenance(ctx, suppressions.MaintenanceDefinition{
		Reason: "Planned patch", ResourceID: resourceID, CheckType: "availability",
		StartsAt: start, EndsAt: start.Add(10 * time.Minute), Enabled: true,
	}, suppressions.MutationMetadata{IdempotencyKey: "maintenance-create", ActorUserID: slice23ActorID, ActorName: "Slice Admin", CorrelationID: "maintenance-create"})
	if err != nil {
		t.Fatal(err)
	}
	clock.Set(start.Add(time.Minute))
	if _, err := ingestor.Ingest(ctx, incidentIntegrationID,
		observationBatch("maint-node", "Maintenance node", "maint-critical", health.Critical, "host down", clock.Now(), 60, false)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEvaluator(pool, nil, Options{Clock: clock}).ProcessOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if count := incidentCount(t, pool, "maint-node"); count != 0 {
		t.Fatalf("maintenance created incident count = %d", count)
	}
	var rawState health.State
	if err := pool.QueryRow(ctx, `SELECT state FROM current_health WHERE resource_id=$1`, resourceID).Scan(&rawState); err != nil || rawState != health.Critical {
		t.Fatalf("raw maintenance health = %s, %v", rawState, err)
	}
	var evidenceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM incident_evaluation_evidence evidence JOIN monitoring_signals signal ON signal.id=evidence.signal_id WHERE evidence.maintenance_window_id=$1 AND evidence.outcome='maintenance' AND signal.state='critical'`, windowReceipt.ID).Scan(&evidenceCount); err != nil || evidenceCount != 1 {
		t.Fatalf("maintenance evidence = %d, %v", evidenceCount, err)
	}
	if _, err := control.RevokeMaintenance(ctx, overlapReceipt.ID, suppressions.MutationMetadata{ExpectedVersion: overlapReceipt.Version, IdempotencyKey: "maintenance-overlap-revoke", ActorUserID: slice23ActorID, ActorName: "Slice Admin", CorrelationID: "maintenance-overlap-revoke"}); err != nil {
		t.Fatal(err)
	}

	endAt := start.Add(90 * time.Second)
	if _, err := pool.Exec(ctx, `UPDATE maintenance_windows SET ends_at=$2 WHERE id=$1`, windowReceipt.ID, endAt); err != nil {
		t.Fatal(err)
	}
	clock.Set(time.Now().UTC().Truncate(time.Microsecond))
	worker := suppressions.NewWorker(control, time.Millisecond)
	if processed, err := worker.ProcessOnce(ctx); err != nil || processed < 1 {
		t.Fatalf("expiry worker = %d, %v", processed, err)
	}
	if _, err := NewEvaluator(pool, nil, Options{Clock: clock}).ProcessOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if count := incidentCount(t, pool, "maint-node"); count != 1 {
		t.Fatalf("post-maintenance incident count = %d", count)
	}
	incident := singleIncident(t, pool, "maint-node")
	version := incident.Version
	silenceReceipt, err := control.CreateSilence(ctx, suppressions.SilenceDefinition{Reason: "Investigation", IncidentID: incident.ID, StartsAt: clock.Now(), ExpiresAt: clock.Now().Add(time.Hour), Enabled: true}, suppressions.MutationMetadata{IdempotencyKey: "silence-create", ActorUserID: slice23ActorID, ActorName: "Slice Admin", CorrelationID: "silence-create"})
	if err != nil {
		t.Fatal(err)
	}
	match, err := control.MatchSilence(ctx, suppressions.IncidentContext{IncidentID: incident.ID, RuleID: incident.RuleID, ResourceID: incident.ResourceID}, clock.Now())
	if err != nil || match == nil || match.ID != silenceReceipt.ID {
		t.Fatalf("silence match = %#v, %v", match, err)
	}
	if noMatch, err := control.MatchSilence(ctx, suppressions.IncidentContext{IncidentID: "50000000-0000-4000-8000-000000000099"}, clock.Now()); err != nil || noMatch != nil {
		t.Fatalf("silence non-match = %#v, %v", noMatch, err)
	}
	if _, err := control.RevokeSilence(ctx, silenceReceipt.ID, suppressions.MutationMetadata{ExpectedVersion: silenceReceipt.Version, IdempotencyKey: "silence-revoke", ActorUserID: slice23ActorID, ActorName: "Slice Admin", CorrelationID: "silence-revoke"}); err != nil {
		t.Fatal(err)
	}
	if match, err := control.MatchSilence(ctx, suppressions.IncidentContext{IncidentID: incident.ID}, clock.Now()); err != nil || match != nil {
		t.Fatalf("revoked silence match = %#v, %v", match, err)
	}
	expiring, err := control.CreateSilence(ctx, suppressions.SilenceDefinition{Reason: "Short resource silence", ResourceID: incident.ResourceID, StartsAt: clock.Now(), ExpiresAt: clock.Now().Add(time.Minute), Enabled: true}, suppressions.MutationMetadata{IdempotencyKey: "silence-expiring", ActorUserID: slice23ActorID, ActorName: "Slice Admin", CorrelationID: "silence-expiring"})
	if err != nil {
		t.Fatal(err)
	}
	if match, err := control.MatchSilence(ctx, suppressions.IncidentContext{ResourceID: incident.ResourceID}, clock.Now().Add(time.Minute)); err != nil || match != nil {
		t.Fatalf("expiry-boundary silence match = %#v, %v", match, err)
	}
	if expiring.ID == "" {
		t.Fatal("expiring silence did not return a receipt")
	}
	if current := singleIncident(t, pool, "maint-node"); current.Status != StatusOpen || current.Version != version {
		t.Fatalf("silence changed incident = %#v", current)
	}
}

func seedSlice23Actor(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO users (id,username,display_name,identity_provider,enabled)
		VALUES ($1,'slice-admin','Slice Admin','local',true)
		ON CONFLICT (id) DO NOTHING
	`, slice23ActorID); err != nil {
		t.Fatal(err)
	}
}
