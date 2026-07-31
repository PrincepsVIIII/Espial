package observations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	integrationA = "20000000-0000-4000-8000-000000000001"
	integrationB = "20000000-0000-4000-8000-000000000002"
)

func TestIngestCommitsNormalizedStateAndPublishesAfterCommit(t *testing.T) {
	pool := observationTestPool(t)
	createIntegration(t, pool, integrationA)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	var published [][]health.Change
	service := NewService(pool, Options{
		Clock: health.FixedClock{Time: now},
		Publisher: PublisherFunc(func(changes []health.Change) {
			published = append(published, changes)
		}),
	})

	result, err := service.Ingest(context.Background(), integrationA, validBatch(now))
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.ResourcesUpserted != 1 || result.ObservationsInserted != 1 ||
		result.DuplicateObservations != 0 || len(result.Changes) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(published) != 1 || len(published[0]) != 1 || published[0][0].After.State != health.Healthy {
		t.Fatalf("published = %#v", published)
	}

	var resourceCount, observationCount, currentCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM resources),
			(SELECT count(*) FROM observations),
			(SELECT count(*) FROM current_health)
	`).Scan(&resourceCount, &observationCount, &currentCount); err != nil {
		t.Fatal(err)
	}
	if resourceCount != 1 || observationCount != 1 || currentCount != 1 {
		t.Fatalf("counts = resource %d observation %d current %d", resourceCount, observationCount, currentCount)
	}

	var state string
	var expectedRefresh int
	var staleAt, unknownAt time.Time
	if err := pool.QueryRow(context.Background(), `
		SELECT ch.state, o.expected_refresh_seconds, ch.stale_at, ch.unknown_at
		FROM current_health ch
		JOIN observations o ON o.id = ch.observation_id
	`).Scan(&state, &expectedRefresh, &staleAt, &unknownAt); err != nil {
		t.Fatal(err)
	}
	if state != "healthy" || expectedRefresh != 300 ||
		!staleAt.Equal(now.Add(7*time.Minute+30*time.Second)) ||
		!unknownAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("current state=%s refresh=%d stale=%s unknown=%s", state, expectedRefresh, staleAt, unknownAt)
	}
}

func TestResourceUpsertPreservesFirstSeenAndNewestFields(t *testing.T) {
	pool := observationTestPool(t)
	createIntegration(t, pool, integrationA)
	base := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	service := NewService(pool, Options{Clock: health.FixedClock{Time: base.Add(time.Hour)}})

	batch := validBatch(base)
	batch.Observations = nil
	if _, err := service.Ingest(context.Background(), integrationA, batch); err != nil {
		t.Fatal(err)
	}
	newer := batch
	newer.Resources = append([]ResourceInput(nil), batch.Resources...)
	newer.Resources[0].ObservedAt = base.Add(10 * time.Minute)
	newer.Resources[0].DisplayName = "Newest name"
	newer.Resources[0].Attributes = map[string]any{"version": "new"}
	if _, err := service.Ingest(context.Background(), integrationA, newer); err != nil {
		t.Fatal(err)
	}
	older := batch
	older.Resources = append([]ResourceInput(nil), batch.Resources...)
	older.Resources[0].ObservedAt = base.Add(-10 * time.Minute)
	older.Resources[0].DisplayName = "Late old name"
	older.Resources[0].Attributes = map[string]any{"version": "old"}
	if _, err := service.Ingest(context.Background(), integrationA, older); err != nil {
		t.Fatal(err)
	}

	var displayName, version string
	var firstSeen, lastSeen time.Time
	if err := pool.QueryRow(context.Background(), `
		SELECT display_name, attributes->>'version', first_seen_at, last_seen_at FROM resources
	`).Scan(&displayName, &version, &firstSeen, &lastSeen); err != nil {
		t.Fatal(err)
	}
	if displayName != "Newest name" || version != "new" ||
		!firstSeen.Equal(base.Add(-10*time.Minute)) || !lastSeen.Equal(base.Add(10*time.Minute)) {
		t.Fatalf("resource = %q %q first=%s last=%s", displayName, version, firstSeen, lastSeen)
	}
	var state string
	if err := pool.QueryRow(context.Background(), "SELECT state FROM current_health").Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "unknown" {
		t.Fatalf("resource without observation state = %q", state)
	}
}

func TestInvalidBatchAndConflictRollbackEverything(t *testing.T) {
	pool := observationTestPool(t)
	createIntegration(t, pool, integrationA)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	published := 0
	service := NewService(pool, Options{
		Clock:     health.FixedClock{Time: now},
		Publisher: PublisherFunc(func([]health.Change) { published++ }),
	})

	invalid := validBatch(now)
	invalid.Observations[0].Summary = ""
	if _, err := service.Ingest(context.Background(), integrationA, invalid); err == nil {
		t.Fatal("invalid batch succeeded")
	}
	requireTableCount(t, pool, "resources", 0)

	original := validBatch(now)
	if _, err := service.Ingest(context.Background(), integrationA, original); err != nil {
		t.Fatal(err)
	}
	conflict := validBatch(now)
	conflict.Resources[0].DisplayName = "Must roll back"
	conflict.Observations[0].Summary = "Different content at same delivery key"
	_, err := service.Ingest(context.Background(), integrationA, conflict)
	var conflictError *ConflictError
	if !errors.As(err, &conflictError) || conflictError.Code != "idempotency_conflict" {
		t.Fatalf("conflict error = %v", err)
	}
	var displayName string
	if err := pool.QueryRow(context.Background(), "SELECT display_name FROM resources").Scan(&displayName); err != nil {
		t.Fatal(err)
	}
	if displayName != original.Resources[0].DisplayName {
		t.Fatalf("rolled-back display name = %q", displayName)
	}
	requireTableCount(t, pool, "observations", 1)
	if published != 1 {
		t.Fatalf("publish count = %d, want 1", published)
	}
}

func TestIdenticalRetryIsNoOp(t *testing.T) {
	pool := observationTestPool(t)
	createIntegration(t, pool, integrationA)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	service := NewService(pool, Options{Clock: health.FixedClock{Time: now.Add(time.Minute)}})
	batch := validBatch(now)
	if _, err := service.Ingest(context.Background(), integrationA, batch); err != nil {
		t.Fatal(err)
	}
	result, err := service.Ingest(context.Background(), integrationA, batch)
	if err != nil {
		t.Fatal(err)
	}
	if result.ObservationsInserted != 0 || result.DuplicateObservations != 1 || len(result.Changes) != 0 {
		t.Fatalf("retry result = %#v", result)
	}
	requireTableCount(t, pool, "observations", 1)
}

func TestProvidedObservationIDIsIdempotentAndCannotBeReused(t *testing.T) {
	pool := observationTestPool(t)
	createIntegration(t, pool, integrationA)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	service := NewService(pool, Options{Clock: health.FixedClock{Time: now}})
	batch := validBatch(now)
	batch.Observations[0].ID = "30000000-0000-4000-8000-000000000001"
	if _, err := service.Ingest(context.Background(), integrationA, batch); err != nil {
		t.Fatal(err)
	}
	result, err := service.Ingest(context.Background(), integrationA, batch)
	if err != nil || result.DuplicateObservations != 1 {
		t.Fatalf("identical UUID retry = %#v, %v", result, err)
	}

	conflict := batch
	conflict.Observations = append([]ObservationInput(nil), batch.Observations...)
	conflict.Observations[0].ObservedAt = now.Add(time.Second)
	conflict.Observations[0].Summary = "different event reusing the UUID"
	_, err = service.Ingest(context.Background(), integrationA, conflict)
	var conflictError *ConflictError
	if !errors.As(err, &conflictError) || conflictError.Code != "idempotency_conflict" {
		t.Fatalf("reused UUID error = %v", err)
	}
	requireTableCount(t, pool, "observations", 1)
}

func TestLateObservationDoesNotReplaceNewerCurrent(t *testing.T) {
	pool := observationTestPool(t)
	createIntegration(t, pool, integrationA)
	base := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	service := NewService(pool, Options{Clock: health.FixedClock{Time: base.Add(10 * time.Minute)}})
	first := validBatch(base)
	if _, err := service.Ingest(context.Background(), integrationA, first); err != nil {
		t.Fatal(err)
	}
	newer := Batch{Observations: []ObservationInput{observationAt(base.Add(5*time.Minute), health.Critical, "new critical")}}
	if _, err := service.Ingest(context.Background(), integrationA, newer); err != nil {
		t.Fatal(err)
	}
	late := Batch{Observations: []ObservationInput{observationAt(base.Add(-5*time.Minute), health.Warning, "late warning")}}
	if _, err := service.Ingest(context.Background(), integrationA, late); err != nil {
		t.Fatal(err)
	}
	var state, summary string
	if err := pool.QueryRow(context.Background(), `
		SELECT ch.state, o.summary FROM current_health ch JOIN observations o ON o.id = ch.observation_id
	`).Scan(&state, &summary); err != nil {
		t.Fatal(err)
	}
	if state != "critical" || summary != "new critical" {
		t.Fatalf("current = %s %q", state, summary)
	}
	requireTableCount(t, pool, "observations", 3)
}

func TestNewObservationRecoversUnknownAndRetainsHistory(t *testing.T) {
	pool := observationTestPool(t)
	createIntegration(t, pool, integrationA)
	base := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	service := NewService(pool, Options{Clock: health.FixedClock{Time: base}})
	old := validBatch(base.Add(-time.Hour))
	result, err := service.Ingest(context.Background(), integrationA, old)
	if err != nil || len(result.Changes) != 1 || result.Changes[0].After.State != health.Unknown {
		t.Fatalf("old ingest = %#v, %v", result, err)
	}
	recovery := Batch{Observations: []ObservationInput{observationAt(base, health.Healthy, "recovered")}}
	result, err = service.Ingest(context.Background(), integrationA, recovery)
	if err != nil || len(result.Changes) != 1 || result.Changes[0].Before == nil ||
		result.Changes[0].Before.State != health.Unknown || result.Changes[0].After.State != health.Healthy {
		t.Fatalf("recovery = %#v, %v", result, err)
	}
	requireTableCount(t, pool, "observations", 2)
}

func TestConcurrentIngestionSelectsNewestAndAvoidsInverseOrderDeadlock(t *testing.T) {
	pool := observationTestPool(t)
	createIntegration(t, pool, integrationA)
	base := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	service := NewService(pool, Options{Clock: health.FixedClock{Time: base.Add(10 * time.Minute)}})
	setup := validBatch(base)
	setup.Resources = append(setup.Resources, ResourceInput{
		ExternalID: "sample-node-02", Kind: "host", DisplayName: "Sample node 02", ObservedAt: base,
	})
	setup.Observations = nil
	if _, err := service.Ingest(context.Background(), integrationA, setup); err != nil {
		t.Fatal(err)
	}

	batchA := Batch{Observations: []ObservationInput{
		observationFor("sample-node-01", base.Add(time.Minute), health.Warning, "node one warning"),
		observationFor("sample-node-02", base.Add(2*time.Minute), health.Healthy, "node two healthy"),
	}}
	batchB := Batch{Observations: []ObservationInput{
		observationFor("sample-node-02", base.Add(3*time.Minute), health.Critical, "node two critical"),
		observationFor("sample-node-01", base.Add(4*time.Minute), health.Healthy, "node one healthy"),
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	errorsSeen := make(chan error, 2)
	var wait sync.WaitGroup
	for _, batch := range []Batch{batchA, batchB} {
		wait.Add(1)
		go func(input Batch) {
			defer wait.Done()
			_, err := service.Ingest(ctx, integrationA, input)
			errorsSeen <- err
		}(batch)
	}
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent ingest: %v", err)
		}
	}

	rows, err := pool.Query(context.Background(), `
		SELECT r.external_id, ch.state
		FROM current_health ch JOIN resources r ON r.id = ch.resource_id
		ORDER BY r.external_id
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := []struct{ externalID, state string }{{"sample-node-01", "healthy"}, {"sample-node-02", "critical"}}
	count := 0
	for ; rows.Next(); count++ {
		var externalID, state string
		if err := rows.Scan(&externalID, &state); err != nil {
			t.Fatal(err)
		}
		if count >= len(want) || externalID != want[count].externalID || state != want[count].state {
			t.Fatalf("row %d = %s %s", count, externalID, state)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != len(want) {
		t.Fatalf("current row count = %d, want %d", count, len(want))
	}
}

func TestIntegrationOwnershipCannotBeCrossed(t *testing.T) {
	pool := observationTestPool(t)
	createIntegration(t, pool, integrationA)
	createIntegration(t, pool, integrationB)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	service := NewService(pool, Options{Clock: health.FixedClock{Time: now}})
	batch := validBatch(now)
	batch.Observations = nil
	if _, err := service.Ingest(context.Background(), integrationA, batch); err != nil {
		t.Fatal(err)
	}
	_, err := service.Ingest(context.Background(), integrationB, Batch{
		Observations: []ObservationInput{observationAt(now, health.Healthy, "cross integration")},
	})
	var conflict *ConflictError
	if !errors.As(err, &conflict) || conflict.Code != "resource_not_found" {
		t.Fatalf("ownership error = %v", err)
	}
	requireTableCount(t, pool, "observations", 0)
}

func TestCanceledContextDoesNotTouchDatabase(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := NewService(nil, Options{Clock: health.FixedClock{Time: time.Now()}})
	if _, err := service.Ingest(ctx, integrationA, Batch{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func observationAt(observedAt time.Time, state health.State, summary string) ObservationInput {
	return observationFor("sample-node-01", observedAt, state, summary)
}

func observationFor(externalID string, observedAt time.Time, state health.State, summary string) ObservationInput {
	return ObservationInput{
		ExternalResourceID: externalID, CheckType: "sample.availability", State: state,
		Summary: summary, ObservedAt: observedAt, ExpectedRefreshSeconds: 300,
		Measurements: map[string]any{}, Metadata: map[string]any{},
	}
}

func createIntegration(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO integrations (id, adapter_id, display_name, enabled)
		VALUES ($1, 'sample', $2, true)
	`, id, "Integration "+id); err != nil {
		t.Fatal(err)
	}
}

func requireTableCount(t *testing.T, pool *pgxpool.Pool, table string, want int) {
	t.Helper()
	var count int
	identifier := pgx.Identifier{table}.Sanitize()
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+identifier).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("%s count = %d, want %d", table, count, want)
	}
}

func observationTestPool(t *testing.T) *pgxpool.Pool {
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

	schema := fmt.Sprintf("espial_observations_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		base.Close()
		t.Fatal(err)
	}
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		base.Close()
		t.Fatal(err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		base.Close()
		t.Fatal(err)
	}
	if err := storage.Migrate(ctx, pool); err != nil {
		pool.Close()
		base.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := base.Exec(cleanupContext, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		base.Close()
	})
	return pool
}
