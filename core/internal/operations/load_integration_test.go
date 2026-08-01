package operations

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/incidents"
	"github.com/PrincepsVIIII/Espial/core/internal/notifications"
	"github.com/PrincepsVIIII/Espial/core/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	loadIntegrationID = "50000000-0000-4000-8000-000000000027"
	loadResourceID    = "40000000-0000-4000-8000-000000000027"
)

type deliveredDriver struct{}

func (deliveredDriver) Deliver(context.Context, notifications.DeliveryRequest) notifications.DeliveryResult {
	return notifications.DeliveryResult{Delivered: true}
}

func TestPhase2BacklogProfiles(t *testing.T) {
	if os.Getenv("ESPIAL_PHASE2_LOAD_TEST") != "1" {
		t.Skip("set ESPIAL_PHASE2_LOAD_TEST=1 to run Phase 2 backlog profiles")
	}
	profiles := []struct {
		name          string
		signals       int
		notifications int
		budget        time.Duration
	}{
		{name: "representative", signals: 1_000, notifications: 250, budget: 30 * time.Second},
		{name: "oversized", signals: 10_000, notifications: 2_500, budget: 2 * time.Minute},
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			pool := loadTestPool(t)
			seedLoadDomain(t, pool, profile.signals, profile.notifications)
			started := time.Now()
			processBacklogs(t, pool, profile.signals, profile.notifications, profile.budget)
			elapsed := time.Since(started)
			t.Logf("profile=%s signals=%d notifications=%d elapsed=%s", profile.name, profile.signals, profile.notifications, elapsed)
			if elapsed > profile.budget {
				t.Fatalf("profile exceeded %s budget: %s", profile.budget, elapsed)
			}
		})
	}
}

func seedLoadDomain(t *testing.T, pool *pgxpool.Pool, signalCount, notificationCount int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `INSERT INTO integrations(id,adapter_id,display_name,enabled,interval_seconds)
		VALUES($1,'org.ubnetdef.espial.sample','Phase 2 load',true,60)`, loadIntegrationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO resources(id,integration_id,external_id,kind,display_name,attributes,first_seen_at,last_seen_at)
		VALUES($1,$2,'load-resource','host','Load resource','{}',now(),now())`, loadResourceID, loadIntegrationID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO monitoring_signals(source_key,kind,integration_id,resource_id,check_type,state,reason,occurred_at,available_at)
		SELECT 'phase2-load-signal-'||value,'observation',$1,$2,'availability','healthy','load profile',now(),now()
		FROM generate_series(1,$3) value`, loadIntegrationID, loadResourceID, signalCount); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `WITH destination AS (
		INSERT INTO notification_destinations(display_name,destination_type,enabled,endpoint_host,endpoint_port,endpoint_path_prefix,secret_reference)
		VALUES('Load Mattermost','mattermost',true,'mattermost.example.invalid',443,'/hooks','load-token') RETURNING id
	) INSERT INTO notification_intents(destination_id,event_kind,is_test,title,summary,event_occurred_at,state,available_at)
		SELECT destination.id,'test',true,'Load test','Bounded notification backlog',now(),'queued',now()
		FROM destination CROSS JOIN generate_series(1,$1)`, notificationCount); err != nil {
		t.Fatal(err)
	}
}

func processBacklogs(t *testing.T, pool *pgxpool.Pool, signalCount, notificationCount int, budget time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	evaluator := incidents.NewEvaluator(pool, nil, incidents.Options{Concurrency: 2, BatchSize: 50, PollInterval: 10 * time.Millisecond, ClaimLease: 30 * time.Second, MaxAttempts: 8})
	publicURL, _ := url.Parse("https://espial.example.invalid")
	worker := notifications.NewWorker(pool, nil, notifications.WorkerOptions{
		Concurrency: 2, PollInterval: 10 * time.Millisecond, ClaimLease: 30 * time.Second, MaxAttempts: 6,
		PublicURL: publicURL,
		Secrets:   notifications.SecretResolverFunc(func(context.Context, string) (string, error) { return "test-token", nil }),
		Drivers:   map[string]notifications.Driver{notifications.DestinationMattermost: deliveredDriver{}},
	})
	errorsChannel := make(chan error, 2)
	go func() { errorsChannel <- evaluator.Run(ctx) }()
	go func() { errorsChannel <- worker.Run(ctx) }()
	deadline := time.NewTicker(10 * time.Millisecond)
	defer deadline.Stop()
	for {
		var processedSignals, delivered int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_signals WHERE processed_at IS NOT NULL`).Scan(&processedSignals); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_intents WHERE state='delivered'`).Scan(&delivered); err != nil {
			t.Fatal(err)
		}
		if processedSignals == signalCount && delivered == notificationCount {
			cancel()
			for range 2 {
				<-errorsChannel
			}
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("backlog timeout: processed_signals=%d/%d delivered=%d/%d", processedSignals, signalCount, delivered, notificationCount)
		case err := <-errorsChannel:
			if err != nil && ctx.Err() == nil {
				t.Fatalf("worker exited: %v", err)
			}
		case <-deadline.C:
		}
	}
}

func loadTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ESPIAL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ESPIAL_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("espial_phase2_load_%d", time.Now().UnixNano())
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
	configuration.MaxConns = 20
	pool, err := pgxpool.NewWithConfig(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		if _, err := base.Exec(cleanup, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
		base.Close()
	})
	return pool
}
