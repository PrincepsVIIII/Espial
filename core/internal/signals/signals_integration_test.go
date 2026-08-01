package signals

import (
	"context"
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
	signalIntegrationID = "50000000-0000-4000-8000-000000000031"
	signalResourceID    = "51000000-0000-4000-8000-000000000031"
)

func TestSignalJournalCommitRollbackAndConcurrentClaim(t *testing.T) {
	pool := signalTestPool(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	writer := NewWriter()

	rolledBack, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Append(context.Background(), rolledBack, signalInput("rollback", now)); err != nil {
		t.Fatal(err)
	}
	if err := rolledBack.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}

	committed, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Append(context.Background(), committed, signalInput("committed", now)); err != nil {
		t.Fatal(err)
	}
	if err := committed.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM monitoring_signals").Scan(&count); err != nil || count != 1 {
		t.Fatalf("durable signal count = %d, %v", count, err)
	}

	store := NewStore(pool)
	start := make(chan struct{})
	results := make(chan []Signal, 2)
	errorsChannel := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			claimed, claimErr := store.Claim(context.Background(), now, 1, time.Minute, 3)
			results <- claimed
			errorsChannel <- claimErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsChannel)
	claimed := 0
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	for result := range results {
		claimed += len(result)
	}
	if claimed != 1 {
		t.Fatalf("concurrent claim count = %d", claimed)
	}
	replayed, err := store.Claim(context.Background(), now.Add(2*time.Minute), 1, time.Minute, 3)
	if err != nil || len(replayed) != 1 || replayed[0].Attempts != 2 {
		t.Fatalf("lease-expired replay = %#v, %v", replayed, err)
	}
	completion, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := MarkProcessed(context.Background(), completion, replayed[0].ID, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := completion.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	remaining, err := store.Claim(context.Background(), now.Add(4*time.Minute), 1, time.Minute, 3)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("completed signal was reclaimed = %#v, %v", remaining, err)
	}
}

func TestSignalFailureRetriesThenDeadLetters(t *testing.T) {
	pool := signalTestPool(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := NewWriter().Append(context.Background(), tx, signalInput("retry", now)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool)
	first, err := store.Claim(context.Background(), now, 1, time.Second, 2)
	if err != nil || len(first) != 1 || first[0].Attempts != 1 {
		t.Fatalf("first claim = %#v, %v", first, err)
	}
	if err := store.Fail(context.Background(), first[0].ID, now, time.Second, 2, "evaluation_failed"); err != nil {
		t.Fatal(err)
	}
	second, err := store.Claim(context.Background(), now.Add(2*time.Second), 1, time.Second, 2)
	if err != nil || len(second) != 1 || second[0].Attempts != 2 {
		t.Fatalf("second claim = %#v, %v", second, err)
	}
	if err := store.Fail(context.Background(), second[0].ID, now.Add(2*time.Second), time.Second, 2, "evaluation_failed"); err != nil {
		t.Fatal(err)
	}
	var deadLettered bool
	if err := pool.QueryRow(context.Background(), "SELECT dead_lettered_at IS NOT NULL FROM monitoring_signals WHERE id = $1", second[0].ID).Scan(&deadLettered); err != nil || !deadLettered {
		t.Fatalf("dead letter state = %v, %v", deadLettered, err)
	}
	metrics, err := store.Metrics(context.Background(), now.Add(3*time.Second))
	if err != nil || metrics.QueueDepth != 0 || metrics.Retried != 1 || metrics.DeadLetters != 1 {
		t.Fatalf("signal metrics = %#v, %v", metrics, err)
	}
}

func signalInput(key string, now time.Time) Input {
	return Input{
		SourceKey: key, Kind: KindObservation, IntegrationID: signalIntegrationID,
		ResourceID: signalResourceID, CheckType: "availability", State: health.Healthy,
		Reason: "reachable", OccurredAt: now, AvailableAt: now,
	}
}

func signalTestPool(t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("espial_signal_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		base.Close()
		t.Fatal(err)
	}
	configuration, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	configuration.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO integrations (id, adapter_id, display_name, enabled, interval_seconds)
		VALUES ($1, 'org.ubnetdef.espial.sample', 'Signal sample', true, 60)
	`, signalIntegrationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO resources (id, integration_id, external_id, kind, display_name, attributes, first_seen_at, last_seen_at)
		VALUES ($2, $1, 'signal-resource', 'host', 'Signal resource', '{}', now(), now())
	`, signalIntegrationID, signalResourceID); err != nil {
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
