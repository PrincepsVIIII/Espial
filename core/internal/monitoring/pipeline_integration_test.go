package monitoring

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/audit"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/observations"
	"github.com/PrincepsVIIII/Espial/core/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pipelineIntegrationID = "60000000-0000-4000-8000-000000000001"

type mutableClock struct {
	mu  sync.Mutex
	now time.Time
}

func (clock *mutableClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *mutableClock) set(now time.Time) {
	clock.mu.Lock()
	clock.now = now
	clock.mu.Unlock()
}

func TestSampleProcessFlowsThroughStorageAuditHealthAndEvents(t *testing.T) {
	pool := monitoringTestPool(t)
	insertPipelineIntegration(t, pool)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: now}
	hub := events.NewHub(16)
	subscription := hub.Subscribe(nil, 8)
	defer subscription.Close()
	service := observations.NewService(pool, observations.Options{Clock: clock, Publisher: hub})
	collector := NewCollector(service, NewCollectionStore(pool), hub, clock)
	integration := adapters.Integration{
		ID: pipelineIntegrationID, AdapterID: "org.ubnetdef.espial.sample", Interval: time.Minute,
		ConfigNonsecret: map[string]any{"scenario": "healthy", "count": 2, "delay_ms": 0, "fault_mode": "none"},
	}
	processOptions := adapters.DefaultProcessOptions()
	processOptions.Clock = clock
	processOptions.StartupTimeout = 2 * time.Second
	processOptions.RequestTimeout = 2 * time.Second
	processOptions.ShutdownTimeout = time.Second
	processOptions.TerminationTimeout = time.Second
	session, err := adapters.StartSession(context.Background(), adapters.Descriptor{
		AdapterID: integration.AdapterID, Executable: buildMonitoringSampleAdapter(t),
	}, integration, nil, processOptions)
	if err != nil {
		t.Fatalf("start sample session: %v", err)
	}
	defer session.Close(context.Background())
	if err := collector.Collect(context.Background(), integration, session); err != nil {
		t.Fatalf("collect: %v", err)
	}

	var resources, observationsCount, current, runs, audits int
	if err := pool.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM resources), (SELECT count(*) FROM observations),
			(SELECT count(*) FROM current_health),
			(SELECT count(*) FROM integration_collection_runs WHERE result = 'succeeded'),
			(SELECT count(*) FROM audit_events WHERE action = 'integration.collection.succeeded')
	`).Scan(&resources, &observationsCount, &current, &runs, &audits); err != nil {
		t.Fatal(err)
	}
	if resources != 2 || observationsCount != 2 || current != 2 || runs != 1 || audits != 1 {
		t.Fatalf("counts resources=%d observations=%d current=%d runs=%d audits=%d", resources, observationsCount, current, runs, audits)
	}
	for _, kind := range []string{events.StateChanged, events.StateChanged, events.CollectionChanged} {
		select {
		case event := <-subscription.Events:
			if event.Kind != kind {
				t.Fatalf("event kind = %q, want %q", event.Kind, kind)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing %s event", kind)
		}
	}
	var safeSummary string
	if err := pool.QueryRow(context.Background(), `
		SELECT after_summary::text FROM audit_events WHERE action = 'integration.collection.succeeded'
	`).Scan(&safeSummary); err != nil {
		t.Fatal(err)
	}
	if safeSummary == "" || containsSecretLikeMaterial(safeSummary) {
		t.Fatalf("unsafe audit summary = %s", safeSummary)
	}
}

func TestRuntimeSchedulesEnabledSampleAndDrains(t *testing.T) {
	pool := monitoringTestPool(t)
	insertPipelineIntegration(t, pool)
	if _, err := pool.Exec(context.Background(), `
		UPDATE integrations SET interval_seconds = 1,
			config_nonsecret = '{"scenario":"warning","count":1,"delay_ms":0,"fault_mode":"none"}'::jsonb
		WHERE id = $1
	`, pipelineIntegrationID); err != nil {
		t.Fatal(err)
	}
	registry, err := adapters.NewRegistry(adapters.Descriptor{
		AdapterID: "org.ubnetdef.espial.sample", Executable: buildMonitoringSampleAdapter(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	processOptions := adapters.DefaultProcessOptions()
	processOptions.StartupTimeout = 2 * time.Second
	processOptions.RequestTimeout = 2 * time.Second
	processOptions.ShutdownTimeout = time.Second
	processOptions.TerminationTimeout = time.Second
	runtimeOwner := NewRuntime(pool, registry, RuntimeOptions{
		GlobalConcurrency: 2, ReconcileInterval: 100 * time.Millisecond,
		FreshnessInterval: time.Second, FreshnessBatchSize: 10,
		EventReplaySize: 16, Process: processOptions,
	})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- runtimeOwner.Run(ctx) }()
	deadline := time.Now().Add(6 * time.Second)
	for {
		var count, incidentCount int
		err := pool.QueryRow(context.Background(), `
			SELECT count(*) FROM integration_collection_runs WHERE result = 'succeeded'
		`).Scan(&count)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		if err := pool.QueryRow(context.Background(), `
			SELECT count(*) FROM incidents WHERE status = 'open' AND severity = 'warning'
		`).Scan(&incidentCount); err != nil {
			cancel()
			t.Fatal(err)
		}
		if count >= 2 && incidentCount == 1 {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("sustained scheduled warning did not create one incident: collections=%d incidents=%d", count, incidentCount)
		}
		time.Sleep(20 * time.Millisecond)
	}
	requireHealthState(t, pool, "warning")
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runtime shutdown error = %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("runtime did not drain")
	}
	var state string
	if err := pool.QueryRow(context.Background(), `
		SELECT state FROM adapter_instances WHERE integration_id = $1
	`, pipelineIntegrationID).Scan(&state); err != nil || state != "stopped" {
		t.Fatalf("adapter state = %q, %v", state, err)
	}
}

func TestFreshnessBoundaryAndConcurrentClaimAreDeterministic(t *testing.T) {
	pool := monitoringTestPool(t)
	insertPipelineIntegration(t, pool)
	base := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: base}
	hub := events.NewHub(16)
	service := observations.NewService(pool, observations.Options{Clock: clock, Publisher: hub})
	batch := observations.Batch{
		Resources: []observations.ResourceInput{{
			ExternalID: "freshness-node", Kind: "host", DisplayName: "Freshness node",
			ObservedAt: base, Attributes: map[string]any{},
		}},
		Observations: []observations.ObservationInput{{
			ExternalResourceID: "freshness-node", CheckType: "availability", State: health.Healthy,
			Summary: "healthy", ObservedAt: base, ExpectedRefreshSeconds: 60,
			Measurements: map[string]any{}, Metadata: map[string]any{},
		}},
	}
	if _, err := service.Ingest(context.Background(), pipelineIntegrationID, batch); err != nil {
		t.Fatal(err)
	}
	clock.set(base.Add(90 * time.Second))
	workers := []*FreshnessWorker{
		NewFreshnessWorker(pool, FreshnessOptions{Clock: clock, Publisher: hub, BatchSize: 10}),
		NewFreshnessWorker(pool, FreshnessOptions{Clock: clock, Publisher: hub, BatchSize: 10}),
	}
	results := make(chan []health.Change, 2)
	errorsSeen := make(chan error, 2)
	for _, worker := range workers {
		go func(worker *FreshnessWorker) {
			changes, err := worker.RefreshDue(context.Background())
			results <- changes
			errorsSeen <- err
		}(worker)
	}
	total := 0
	for range workers {
		if err := <-errorsSeen; err != nil {
			t.Fatal(err)
		}
		total += len(<-results)
	}
	if total != 1 {
		t.Fatalf("concurrent transition count = %d, want 1", total)
	}
	requireHealthState(t, pool, "stale")

	clock.set(base.Add(3 * time.Minute))
	changes, err := workers[0].RefreshDue(context.Background())
	if err != nil || len(changes) != 1 || changes[0].After.State != health.Unknown {
		t.Fatalf("unknown transition = %#v, %v", changes, err)
	}
	requireHealthState(t, pool, "unknown")
}

func TestFreshnessWorkerRejectsMultipleOwners(t *testing.T) {
	pool := monitoringTestPool(t)
	never := make(chan time.Time)
	worker := NewFreshnessWorker(pool, FreshnessOptions{
		NewTimer: func(time.Duration) FreshnessTimer { return testFreshnessTimer{channel: never} },
	})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- worker.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for !worker.running.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	err := worker.Run(context.Background())
	var monitoringError *Error
	if !errors.As(err, &monitoringError) || monitoringError.Code != "freshness_already_running" {
		t.Fatalf("second owner error = %v", err)
	}
	cancel()
	<-result
}

func TestIntegrationConfigurationUpdateIsAtomicAndRedacted(t *testing.T) {
	pool := monitoringTestPool(t)
	insertPipelineIntegration(t, pool)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, username, display_name, identity_provider)
		VALUES ('70000000-0000-4000-8000-000000000001', 'config-admin', 'Config admin', 'local')
	`); err != nil {
		t.Fatal(err)
	}
	var expected time.Time
	if err := pool.QueryRow(context.Background(), `
		SELECT updated_at FROM integrations WHERE id = $1
	`, pipelineIntegrationID).Scan(&expected); err != nil {
		t.Fatal(err)
	}
	now := expected.Add(time.Second)
	hub := events.NewHub(8)
	subscription := hub.Subscribe(nil, 2)
	defer subscription.Close()
	service := NewIntegrationConfigService(pool, hub, health.FixedClock{Time: now})
	updatedAt, err := service.Update(context.Background(), IntegrationConfigUpdate{
		IntegrationID: pipelineIntegrationID, Enabled: true, Interval: 2 * time.Minute,
		ConfigNonsecret:   map[string]any{"scenario": "critical"},
		SecretReferences:  map[string]string{"api_token": "vault://production/secret-token"},
		ExpectedUpdatedAt: expected, ActorUserID: "70000000-0000-4000-8000-000000000001",
		SourceAddress: "127.0.0.1", CorrelationID: "config-test",
	})
	if err != nil || !updatedAt.Equal(now) {
		t.Fatalf("update = %s, %v", updatedAt, err)
	}
	var before, after, actor, source, correlation, target, result string
	var occurredAt time.Time
	if err := pool.QueryRow(context.Background(), `
		SELECT before_summary::text, after_summary::text, actor_user_id::text,
			source_address::text, correlation_id, target_id, result, occurred_at
		FROM audit_events WHERE action = 'integration.configuration.updated'
	`).Scan(&before, &after, &actor, &source, &correlation, &target, &result, &occurredAt); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(before+after, "critical") || strings.Contains(before+after, "vault://") ||
		!strings.Contains(after, "scenario") || !strings.Contains(after, "api_token") {
		t.Fatalf("audit summaries were not safely redacted: before=%s after=%s", before, after)
	}
	if actor != "70000000-0000-4000-8000-000000000001" || source != "127.0.0.1/32" ||
		correlation != "config-test" || target != pipelineIntegrationID || result != "succeeded" ||
		!occurredAt.Equal(now) {
		t.Fatalf("incomplete audit identity/source metadata: actor=%s source=%s correlation=%s target=%s result=%s at=%s", actor, source, correlation, target, result, occurredAt)
	}
	select {
	case event := <-subscription.Events:
		if event.Kind != events.IntegrationChanged {
			t.Fatalf("event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("configuration event was not published")
	}

	_, err = service.Update(context.Background(), IntegrationConfigUpdate{
		IntegrationID: pipelineIntegrationID, Enabled: false, Interval: time.Minute,
		ConfigNonsecret: map[string]any{}, SecretReferences: map[string]string{},
		ExpectedUpdatedAt: expected, CorrelationID: "stale-config-test",
	})
	var monitoringError *Error
	if !errors.As(err, &monitoringError) || monitoringError.Code != "integration_config_conflict" {
		t.Fatalf("stale update error = %v", err)
	}
	var auditCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events WHERE action = 'integration.configuration.updated'
	`).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("audit count = %d, %v", auditCount, err)
	}
}

func TestAdapterLifecycleFailureAndRecoveryAreAudited(t *testing.T) {
	pool := monitoringTestPool(t)
	insertPipelineIntegration(t, pool)
	hub := events.NewHub(8)
	observer := NewLifecycleAudit(audit.NewWriter(pool), hub)
	integration := adapters.Integration{ID: pipelineIntegrationID}
	now := time.Date(2026, 7, 31, 14, 0, 0, 0, time.UTC)
	if err := observer.Starting(context.Background(), integration, now); err != nil {
		t.Fatal(err)
	}
	if err := observer.Failed(context.Background(), integration, adapters.Instance{
		LastErrorCode: "request_timeout", ConsecutiveFailures: 1,
	}, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := observer.Healthy(context.Background(), integration, adapters.Instance{
		AdapterVersion: "1.2.3", ProtocolVersion: "1.0",
	}, true, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := observer.Stopped(context.Background(), integration, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(context.Background(), `
		SELECT action, after_summary::text FROM audit_events ORDER BY occurred_at
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := []string{
		"integration.adapter.starting", "integration.adapter.failed",
		"integration.adapter.recovered", "integration.adapter.stopped",
	}
	for index := 0; rows.Next(); index++ {
		var action, summary string
		if err := rows.Scan(&action, &summary); err != nil {
			t.Fatal(err)
		}
		if index >= len(want) || action != want[index] || strings.Contains(summary, "secret") {
			t.Fatalf("lifecycle row %d = %s %s", index, action, summary)
		}
		want[index] = ""
	}
	for _, missing := range want {
		if missing != "" {
			t.Fatalf("missing lifecycle audit %s", missing)
		}
	}
	_, _, replayed := hub.Stats()
	if replayed != 4 {
		t.Fatalf("lifecycle event count = %d", replayed)
	}
	var systemEvents int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events
		WHERE actor_user_id IS NULL AND correlation_id <> '' AND target_id = $1
	`, pipelineIntegrationID).Scan(&systemEvents); err != nil || systemEvents != 4 {
		t.Fatalf("system lifecycle audit count = %d, %v", systemEvents, err)
	}
}

type testFreshnessTimer struct{ channel <-chan time.Time }

func (timer testFreshnessTimer) Channel() <-chan time.Time { return timer.channel }
func (testFreshnessTimer) Stop() bool                      { return true }

func requireHealthState(t *testing.T, pool *pgxpool.Pool, want string) {
	t.Helper()
	var state string
	if err := pool.QueryRow(context.Background(), "SELECT state FROM current_health").Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != want {
		t.Fatalf("state = %q, want %q", state, want)
	}
}

func containsSecretLikeMaterial(summary string) bool {
	return summary == "null" || len(summary) > 4096
}

func insertPipelineIntegration(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO integrations (id, adapter_id, display_name, enabled, interval_seconds)
		VALUES ($1, 'org.ubnetdef.espial.sample', 'Pipeline sample', true, 60)
	`, pipelineIntegrationID); err != nil {
		t.Fatal(err)
	}
}

var (
	monitoringBuildOnce  sync.Once
	monitoringExecutable string
	monitoringBuildError error
)

func buildMonitoringSampleAdapter(t *testing.T) string {
	t.Helper()
	monitoringBuildOnce.Do(func() {
		directory, err := os.MkdirTemp("", "espial-monitoring-sample-")
		if err != nil {
			monitoringBuildError = err
			return
		}
		name := "espial-sample-adapter"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		monitoringExecutable = filepath.Join(directory, name)
		command := exec.Command("go", "build", "-o", monitoringExecutable, "../../cmd/espial-sample-adapter")
		if output, err := command.CombinedOutput(); err != nil {
			monitoringBuildError = fmt.Errorf("%s: %w", output, err)
		}
	})
	if monitoringBuildError != nil {
		t.Fatal(monitoringBuildError)
	}
	return monitoringExecutable
}

func monitoringTestPool(t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("espial_monitoring_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		base.Close()
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_, _ = base.Exec(cleanup, "DROP SCHEMA "+identifier+" CASCADE")
		base.Close()
	})
	return pool
}
