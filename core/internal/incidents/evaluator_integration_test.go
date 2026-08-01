package incidents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/observations"
	"github.com/PrincepsVIIII/Espial/core/internal/signals"
	"github.com/PrincepsVIIII/Espial/core/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const incidentIntegrationID = "50000000-0000-4000-8000-000000000021"
const websiteIncidentIntegrationID = "50000000-0000-4000-8000-000000000025"

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

type recordingIntentWriter struct {
	mu    sync.Mutex
	kinds []string
	err   error
}

func (writer *recordingIntentWriter) EnqueueIncidentEvent(_ context.Context, _ pgx.Tx, event NotificationEvent) error {
	writer.mu.Lock()
	writer.kinds = append(writer.kinds, event.Kind)
	writer.mu.Unlock()
	return writer.err
}

func TestIncidentAndNotificationIntentBoundaryRollsBackAtomically(t *testing.T) {
	pool := incidentTestPool(t)
	start := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	clock := &testClock{now: start}
	ingestor := observations.NewService(pool, observations.Options{Clock: clock})
	if _, err := ingestor.Ingest(context.Background(), incidentIntegrationID,
		observationBatch("atomic-node", "Atomic node", "atomic-critical", health.Critical, "failed", start, 60, true)); err != nil {
		t.Fatal(err)
	}
	evaluator := NewEvaluator(pool, nil, Options{Clock: clock,
		Intents: &recordingIntentWriter{err: errors.New("intent store unavailable")}})
	if processed, err := evaluator.ProcessOnce(context.Background()); err != nil || processed != 0 {
		t.Fatalf("rolled-back evaluation = %d, %v", processed, err)
	}
	if count := incidentCount(t, pool, "atomic-node"); count != 0 {
		t.Fatalf("incident committed without its notification intent: %d", count)
	}
	var processedAt *time.Time
	if err := pool.QueryRow(context.Background(), `SELECT processed_at FROM monitoring_signals ORDER BY created_at DESC LIMIT 1`).Scan(&processedAt); err != nil {
		t.Fatal(err)
	}
	if processedAt != nil {
		t.Fatal("signal was marked processed after notification rollback")
	}
	var attempts int
	var safeError string
	if err := pool.QueryRow(context.Background(), `SELECT attempts,last_error_code FROM monitoring_signals ORDER BY created_at DESC LIMIT 1`).Scan(&attempts, &safeError); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || safeError != "incident_evaluation_failed" {
		t.Fatalf("notification rollback retry evidence = %d %q", attempts, safeError)
	}
}

func (writer *recordingIntentWriter) eventKinds() []string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return append([]string(nil), writer.kinds...)
}

func (clock *testClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *testClock) Set(value time.Time) {
	clock.mu.Lock()
	clock.now = value
	clock.mu.Unlock()
}

func TestAutomaticIncidentLifecycleIsDurableAndIdempotent(t *testing.T) {
	pool := incidentTestPool(t)
	start := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)
	clock := &testClock{now: start}
	ingestor := observations.NewService(pool, observations.Options{Clock: clock})
	hub := events.NewHub(32)
	intentWriter := &recordingIntentWriter{}
	subscription := hub.Subscribe(nil, 4)
	defer subscription.Close()
	evaluator := NewEvaluator(pool, hub, Options{Clock: clock, PollInterval: time.Millisecond, Intents: intentWriter})

	critical := observationBatch("node-critical", "Critical node", "critical-1", health.Critical, "host unreachable", start, 60, true)
	if _, err := ingestor.Ingest(context.Background(), incidentIntegrationID, critical); err != nil {
		t.Fatal(err)
	}
	if processed, err := evaluator.ProcessOnce(context.Background()); err != nil || processed != 1 {
		t.Fatalf("critical evaluation = %d, %v", processed, err)
	}
	incident := singleIncident(t, pool, "node-critical")
	if incident.Status != StatusOpen || incident.Severity != SeverityCritical || incident.Version != 1 {
		t.Fatalf("detected incident = %#v", incident)
	}
	assertTimelineKinds(t, pool, incident.ID, "detected")
	select {
	case event := <-subscription.Events:
		if event.Kind != events.IncidentChanged || event.IncidentID != incident.ID || event.Result != string(StatusOpen) {
			t.Fatalf("incident invalidation = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("incident commit did not publish an invalidation")
	}
	if _, err := pool.Exec(context.Background(), "UPDATE incident_timeline SET summary = 'changed' WHERE incident_id = $1", incident.ID); err == nil {
		t.Fatal("incident timeline accepted an update")
	}
	if _, err := pool.Exec(context.Background(), "DELETE FROM incident_timeline WHERE incident_id = $1", incident.ID); err == nil {
		t.Fatal("incident timeline accepted a delete")
	}

	if result, err := ingestor.Ingest(context.Background(), incidentIntegrationID, critical); err != nil || result.DuplicateObservations != 1 {
		t.Fatalf("duplicate ingestion = %#v, %v", result, err)
	}
	if processed, err := evaluator.ProcessOnce(context.Background()); err != nil || processed != 0 {
		t.Fatalf("duplicate evaluation = %d, %v", processed, err)
	}
	if count := incidentCount(t, pool, "node-critical"); count != 1 {
		t.Fatalf("duplicate incident count = %d", count)
	}

	clock.Set(start.Add(time.Minute))
	if _, err := ingestor.Ingest(context.Background(), incidentIntegrationID,
		observationBatch("node-critical", "Critical node", "healthy-1", health.Healthy, "reachable", clock.Now(), 60, false)); err != nil {
		t.Fatal(err)
	}
	if _, err := evaluator.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := singleIncident(t, pool, "node-critical"); got.Status != StatusOpen {
		t.Fatalf("recovered after one healthy observation: %#v", got)
	}

	clock.Set(start.Add(2 * time.Minute))
	if _, err := ingestor.Ingest(context.Background(), incidentIntegrationID,
		observationBatch("node-critical", "Critical node", "healthy-2", health.Healthy, "reachable", clock.Now(), 60, false)); err != nil {
		t.Fatal(err)
	}
	restartedForRecovery := NewEvaluator(pool, hub, Options{Clock: clock, Intents: intentWriter})
	if _, err := restartedForRecovery.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	incident = singleIncident(t, pool, "node-critical")
	if incident.Status != StatusRecovered || incident.RecoveredAt == nil {
		t.Fatalf("incident did not recover = %#v", incident)
	}
	assertTimelineKinds(t, pool, incident.ID, "detected", "recovered")

	clock.Set(start.Add(3 * time.Minute))
	if _, err := ingestor.Ingest(context.Background(), incidentIntegrationID,
		observationBatch("node-critical", "Critical node", "critical-2", health.Critical, "failed again", clock.Now(), 60, false)); err != nil {
		t.Fatal(err)
	}
	if _, err := evaluator.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	incident = singleIncident(t, pool, "node-critical")
	if incident.Status != StatusOpen {
		t.Fatalf("incident did not recur = %#v", incident)
	}
	assertTimelineKinds(t, pool, incident.ID, "detected", "recovered", "recurrence")

	clock.Set(start.Add(4 * time.Minute))
	late := observationBatch("node-critical", "Critical node", "late-critical", health.Critical, "old delayed failure", start.Add(30*time.Second), 60, false)
	if _, err := ingestor.Ingest(context.Background(), incidentIntegrationID, late); err != nil {
		t.Fatal(err)
	}
	if _, err := evaluator.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if kinds := timelineKinds(t, pool, incident.ID); len(kinds) != 3 {
		t.Fatalf("out-of-order signal changed timeline: %v", kinds)
	}
	if kinds := intentWriter.eventKinds(); fmt.Sprint(kinds) != "[detected recovered recurrence]" {
		t.Fatalf("notification event policy = %v", kinds)
	}
}

func TestWebsiteAvailabilityOutageCreatesOneIncidentAndRecovers(t *testing.T) {
	pool := incidentTestPool(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(-5 * time.Minute).Truncate(time.Microsecond)
	clock := &testClock{now: start}
	if _, err := pool.Exec(ctx, `
		INSERT INTO integrations (id, adapter_id, display_name, enabled, interval_seconds)
		VALUES ($1, 'org.ubnetdef.espial.webcheck', 'Website monitor', true, 60)
	`, websiteIncidentIntegrationID); err != nil {
		t.Fatal(err)
	}
	ingestor := observations.NewService(pool, observations.Options{Clock: clock})
	evaluator := NewEvaluator(pool, nil, Options{Clock: clock})

	for index := 0; index < 2; index++ {
		clock.Set(start.Add(time.Duration(index) * time.Minute))
		if _, err := ingestor.Ingest(ctx, websiteIncidentIntegrationID, websiteObservationBatch(
			fmt.Sprintf("website-critical-%d", index), health.Critical, "status_unexpected", clock.Now(), index == 0,
		)); err != nil {
			t.Fatal(err)
		}
		if _, err := evaluator.ProcessOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if count := incidentCount(t, pool, "https://status.example.test/health"); count != 1 {
		t.Fatalf("website outage incident count = %d", count)
	}

	for index := 0; index < 2; index++ {
		clock.Set(start.Add(time.Duration(index+2) * time.Minute))
		if _, err := ingestor.Ingest(ctx, websiteIncidentIntegrationID, websiteObservationBatch(
			fmt.Sprintf("website-healthy-%d", index), health.Healthy, "available", clock.Now(), false,
		)); err != nil {
			t.Fatal(err)
		}
		if _, err := evaluator.ProcessOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	incident := singleIncident(t, pool, "https://status.example.test/health")
	if incident.Status != StatusRecovered || incident.RecoveredAt == nil {
		t.Fatalf("website incident did not recover = %#v", incident)
	}
}

func TestCertificateThresholdCrossingsPreserveOneIncidentAndMeaningfulUpdates(t *testing.T) {
	pool := incidentTestPool(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(-5 * time.Minute).Truncate(time.Microsecond)
	clock := &testClock{now: start}
	if _, err := pool.Exec(ctx, `INSERT INTO integrations (id,adapter_id,display_name,enabled,interval_seconds) VALUES ($1,'org.ubnetdef.espial.webcheck','Certificate monitor',true,60)`, websiteIncidentIntegrationID); err != nil {
		t.Fatal(err)
	}
	ingestor := observations.NewService(pool, observations.Options{Clock: clock})
	intents := &recordingIntentWriter{}
	evaluator := NewEvaluator(pool, nil, Options{Clock: clock, Intents: intents})
	steps := []struct {
		state  health.State
		reason string
	}{{health.Warning, "certificate_approaching_expiry"}, {health.Critical, "certificate_expiring"}, {health.Critical, "certificate_expiry_escalated"}, {health.Critical, "certificate_expiry_escalated"}}
	for index, step := range steps {
		clock.Set(start.Add(time.Duration(index) * time.Minute))
		if _, err := ingestor.Ingest(ctx, websiteIncidentIntegrationID, certificateObservationBatch(fmt.Sprintf("certificate-%d", index), step.state, step.reason, clock.Now(), index == 0)); err != nil {
			t.Fatal(err)
		}
		if _, err := evaluator.ProcessOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	const externalID = "certificate:status.example.test:443"
	if count := incidentCount(t, pool, externalID); count != 1 {
		t.Fatalf("certificate incident count=%d", count)
	}
	incident := singleIncident(t, pool, externalID)
	if incident.Severity != SeverityCritical {
		t.Fatalf("certificate severity=%#v", incident)
	}
	assertTimelineKinds(t, pool, incident.ID, "detected", "severity_changed", "condition_changed")
	if kinds := fmt.Sprint(intents.eventKinds()); kinds != "[detected severity_changed condition_changed]" {
		t.Fatalf("certificate notifications=%s", kinds)
	}
}

func TestWarningDebouncePersistsAcrossEvaluatorRestart(t *testing.T) {
	pool := incidentTestPool(t)
	start := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)
	clock := &testClock{now: start}
	intentWriter := &recordingIntentWriter{}
	ingestor := observations.NewService(pool, observations.Options{Clock: clock})
	firstEvaluator := NewEvaluator(pool, nil, Options{Clock: clock, Intents: intentWriter})
	if _, err := ingestor.Ingest(context.Background(), incidentIntegrationID,
		observationBatch("node-warning", "Warning node", "warning-1", health.Warning, "packet loss", start, 60, true)); err != nil {
		t.Fatal(err)
	}
	if _, err := firstEvaluator.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if count := incidentCount(t, pool, "node-warning"); count != 0 {
		t.Fatalf("warning opened before debounce: %d", count)
	}

	clock.Set(start.Add(time.Minute))
	if _, err := ingestor.Ingest(context.Background(), incidentIntegrationID,
		observationBatch("node-warning", "Warning node", "warning-2", health.Warning, "packet loss persists", clock.Now(), 60, false)); err != nil {
		t.Fatal(err)
	}
	restarted := NewEvaluator(pool, nil, Options{Clock: clock, Intents: intentWriter})
	if _, err := restarted.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	incident := singleIncident(t, pool, "node-warning")
	if incident.Severity != SeverityWarning || incident.Status != StatusOpen {
		t.Fatalf("debounced warning incident = %#v", incident)
	}
	clock.Set(start.Add(2 * time.Minute))
	if _, err := ingestor.Ingest(context.Background(), incidentIntegrationID,
		observationBatch("node-warning", "Warning node", "warning-critical", health.Critical, "condition worsened", clock.Now(), 60, false)); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	incident = singleIncident(t, pool, "node-warning")
	if incident.Severity != SeverityCritical {
		t.Fatalf("incident severity did not change = %#v", incident)
	}
	assertTimelineKinds(t, pool, incident.ID, "detected", "severity_changed")
	if kinds := intentWriter.eventKinds(); fmt.Sprint(kinds) != "[detected severity_changed]" {
		t.Fatalf("severity notification policy = %v", kinds)
	}
}

func TestStaleDoesNotOpenButUnknownDoes(t *testing.T) {
	pool := incidentTestPool(t)
	start := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)
	clock := &testClock{now: start}
	ingestor := observations.NewService(pool, observations.Options{Clock: clock})
	evaluator := NewEvaluator(pool, nil, Options{Clock: clock})
	if _, err := ingestor.Ingest(context.Background(), incidentIntegrationID,
		observationBatch("node-freshness", "Freshness node", "healthy-freshness", health.Healthy, "reachable", start, 10, true)); err != nil {
		t.Fatal(err)
	}
	if _, err := evaluator.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	clock.Set(start.Add(16 * time.Second))
	appendFreshnessSignal(t, pool, "node-freshness", health.Stale, "observation overdue", clock.Now())
	if _, err := evaluator.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if count := incidentCount(t, pool, "node-freshness"); count != 0 {
		t.Fatalf("stale opened incident: %d", count)
	}

	clock.Set(start.Add(31 * time.Second))
	appendFreshnessSignal(t, pool, "node-freshness", health.Unknown, "observation missing", clock.Now())
	if _, err := evaluator.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	incident := singleIncident(t, pool, "node-freshness")
	if incident.Severity != SeverityWarning || incident.Status != StatusOpen {
		t.Fatalf("unknown incident = %#v", incident)
	}
}

func appendFreshnessSignal(t *testing.T, pool *pgxpool.Pool, externalID string, state health.State, reason string, occurredAt time.Time) {
	t.Helper()
	var resourceID string
	if err := pool.QueryRow(context.Background(), "SELECT id::text FROM resources WHERE external_id = $1", externalID).Scan(&resourceID); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := signals.NewWriter().Append(context.Background(), tx, signals.Input{
		SourceKey: signals.FreshnessSourceKey(resourceID, state, occurredAt), Kind: signals.KindFreshness,
		IntegrationID: incidentIntegrationID, ResourceID: resourceID, CheckType: "availability",
		State: state, Reason: reason, ReasonCode: "freshness", OccurredAt: occurredAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentEvaluatorsCreateOneIncidentAndDeadlineSurvives(t *testing.T) {
	pool := incidentTestPool(t)
	start := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)
	clock := &testClock{now: start}
	ingestor := observations.NewService(pool, observations.Options{Clock: clock})
	if _, err := ingestor.Ingest(context.Background(), incidentIntegrationID,
		observationBatch("node-concurrent", "Concurrent node", "critical-concurrent", health.Critical, "down", start, 60, true)); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errorsChannel := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := NewEvaluator(pool, nil, Options{Clock: clock, BatchSize: 1}).ProcessOnce(context.Background())
			errorsChannel <- err
		}()
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	if count := incidentCount(t, pool, "node-concurrent"); count != 1 {
		t.Fatalf("concurrent incident count = %d", count)
	}

	if _, err := pool.Exec(context.Background(), `
		UPDATE incident_rule_conditions SET for_seconds = 10
		WHERE rule_id = '20000000-0000-4000-8000-000000000001' AND state = 'critical'
	`); err != nil {
		t.Fatal(err)
	}
	clock.Set(start.Add(time.Hour))
	if _, err := ingestor.Ingest(context.Background(), incidentIntegrationID,
		observationBatch("node-deadline", "Deadline node", "critical-deadline", health.Critical, "down", clock.Now(), 60, true)); err != nil {
		t.Fatal(err)
	}
	first := NewEvaluator(pool, nil, Options{Clock: clock})
	if _, err := first.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if count := incidentCount(t, pool, "node-deadline"); count != 0 {
		t.Fatalf("duration rule opened early: %d", count)
	}
	clock.Set(clock.Now().Add(11 * time.Second))
	if _, err := NewEvaluator(pool, nil, Options{Clock: clock}).ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if count := incidentCount(t, pool, "node-deadline"); count != 1 {
		t.Fatalf("persisted deadline incident count = %d", count)
	}
}

func TestIncidentReaderFiltersPagesAndReturnsTimeline(t *testing.T) {
	pool := incidentTestPool(t)
	start := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)
	clock := &testClock{now: start}
	ingestor := observations.NewService(pool, observations.Options{Clock: clock})
	evaluator := NewEvaluator(pool, nil, Options{Clock: clock})
	for index := range 3 {
		clock.Set(start.Add(time.Duration(index) * time.Minute))
		name := fmt.Sprintf("reader-%d", index)
		if _, err := ingestor.Ingest(context.Background(), incidentIntegrationID,
			observationBatch(name, "Reader node", fmt.Sprintf("reader-observation-%d", index), health.Critical, "down", clock.Now(), 60, true)); err != nil {
			t.Fatal(err)
		}
		if _, err := evaluator.ProcessOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	reader := NewReader(pool)
	active := true
	first, err := reader.Incidents(context.Background(), Filter{Limit: 2, Active: &active, Severities: []Severity{SeverityCritical}})
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first incident page = %#v, %v", first, err)
	}
	clock.Set(time.Now().UTC().Add(time.Second).Truncate(time.Microsecond))
	if _, err := ingestor.Ingest(context.Background(), incidentIntegrationID,
		observationBatch("reader-new", "New reader node", "reader-new-observation", health.Critical, "new after snapshot", clock.Now(), 60, true)); err != nil {
		t.Fatal(err)
	}
	if _, err := evaluator.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	second, err := reader.Incidents(context.Background(), Filter{Limit: 2, Active: &active, Severities: []Severity{SeverityCritical}, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 1 {
		t.Fatalf("second incident page = %#v, %v", second, err)
	}
	detail, err := reader.Incident(context.Background(), first.Items[0].ID)
	if err != nil || detail.Fingerprint == "" || detail.Version < 1 {
		t.Fatalf("incident detail = %#v, %v", detail, err)
	}
	timeline, err := reader.Timeline(context.Background(), detail.ID, TimelineFilter{Limit: 10})
	if err != nil || len(timeline.Items) != 1 || timeline.Items[0].Kind != "detected" {
		t.Fatalf("incident timeline = %#v, %v", timeline, err)
	}
	if _, err := reader.Incidents(context.Background(), Filter{Limit: 2, Active: &active, Cursor: first.NextCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cursor/filter mismatch error = %v", err)
	}
}

func observationBatch(externalID, displayName, observationID string, state health.State, summary string, observedAt time.Time, refresh int, includeResource bool) observations.Batch {
	batch := observations.Batch{Observations: []observations.ObservationInput{{
		ID: stableObservationID(observationID), ExternalResourceID: externalID, CheckType: "availability",
		State: state, Summary: summary, ObservedAt: observedAt,
		ExpectedRefreshSeconds: refresh, Measurements: map[string]any{}, Metadata: map[string]any{},
	}}}
	if includeResource {
		batch.Resources = []observations.ResourceInput{{
			ExternalID: externalID, Kind: "host", DisplayName: displayName,
			ObservedAt: observedAt, Attributes: map[string]any{},
		}}
	}
	return batch
}

func websiteObservationBatch(observationID string, state health.State, reasonCode string, observedAt time.Time, includeResource bool) observations.Batch {
	const target = "https://status.example.test/health"
	batch := observations.Batch{Observations: []observations.ObservationInput{{
		ID: stableObservationID(observationID), ExternalResourceID: target, CheckType: "website.availability",
		State: state, Summary: "Website availability changed.", ObservedAt: observedAt,
		ExpectedRefreshSeconds: 60, Measurements: map[string]any{}, Metadata: map[string]any{"reason_code": reasonCode},
	}}}
	if includeResource {
		batch.Resources = []observations.ResourceInput{{
			ExternalID: target, Kind: "webpage", DisplayName: "Status endpoint",
			ObservedAt: observedAt, Attributes: map[string]any{}, SourceURL: target,
		}}
	}
	return batch
}

func certificateObservationBatch(observationID string, state health.State, reasonCode string, observedAt time.Time, includeResource bool) observations.Batch {
	const target = "certificate:status.example.test:443"
	batch := observations.Batch{Observations: []observations.ObservationInput{{ID: stableObservationID(observationID), ExternalResourceID: target, CheckType: "certificate.validity", State: state, Summary: "Certificate validity changed.", ObservedAt: observedAt, ExpectedRefreshSeconds: 60, Measurements: map[string]any{}, Metadata: map[string]any{"reason_code": reasonCode}}}}
	if includeResource {
		batch.Resources = []observations.ResourceInput{{ExternalID: target, Kind: "certificate", DisplayName: "status.example.test:443", ObservedAt: observedAt, Attributes: map[string]any{}, SourceURL: "https://status.example.test/"}}
	}
	return batch
}

func stableObservationID(value string) string {
	sum := sha256.Sum256([]byte(value))
	encoded := hex.EncodeToString(sum[:16])
	return encoded[:8] + "-" + encoded[8:12] + "-4" + encoded[13:16] + "-8" + encoded[17:20] + "-" + encoded[20:32]
}

func singleIncident(t *testing.T, pool *pgxpool.Pool, externalID string) Summary {
	t.Helper()
	reader := NewReader(pool)
	result, err := reader.Incidents(context.Background(), Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range result.Items {
		var candidate string
		if err := pool.QueryRow(context.Background(), "SELECT external_id FROM resources WHERE id = $1", item.ResourceID).Scan(&candidate); err != nil {
			t.Fatal(err)
		}
		if candidate == externalID {
			return item
		}
	}
	t.Fatalf("incident for %s was not found", externalID)
	return Summary{}
}

func incidentCount(t *testing.T, pool *pgxpool.Pool, externalID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM incidents incident
		JOIN resources resource ON resource.id = incident.resource_id
		WHERE resource.external_id = $1
	`, externalID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func timelineKinds(t *testing.T, pool *pgxpool.Pool, incidentID string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT kind FROM incident_timeline WHERE incident_id = $1 ORDER BY occurred_at, id
	`, incidentID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			t.Fatal(err)
		}
		result = append(result, kind)
	}
	return result
}

func assertTimelineKinds(t *testing.T, pool *pgxpool.Pool, incidentID string, expected ...string) {
	t.Helper()
	actual := timelineKinds(t, pool, incidentID)
	if fmt.Sprint(actual) != fmt.Sprint(expected) {
		t.Fatalf("timeline kinds = %v, want %v", actual, expected)
	}
}

func incidentTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ESPIAL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ESPIAL_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := base.Ping(ctx); err != nil {
		base.Close()
		t.Fatal(err)
	}
	schema := fmt.Sprintf("espial_incident_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		base.Close()
		t.Fatal(err)
	}
	configuration, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		base.Close()
		t.Fatal(err)
	}
	configuration.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, configuration)
	if err != nil {
		base.Close()
		t.Fatal(err)
	}
	if err := storage.Migrate(ctx, pool); err != nil {
		pool.Close()
		base.Close()
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO integrations (id, adapter_id, display_name, enabled, interval_seconds)
		VALUES ($1, 'org.ubnetdef.espial.sample', 'Incident sample', true, 60)
	`, incidentIntegrationID); err != nil {
		pool.Close()
		base.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := base.Exec(cleanup, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
		base.Close()
	})
	return pool
}
