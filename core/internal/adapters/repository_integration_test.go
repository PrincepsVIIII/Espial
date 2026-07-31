package adapters

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	adapterIntegrationA = "60000000-0000-4000-8000-000000000001"
	adapterIntegrationB = "60000000-0000-4000-8000-000000000002"
)

func TestPostgreSQLStorePersistsRuntimeLifecycleAndBackoff(t *testing.T) {
	pool := adapterTestPool(t)
	insertAdapterIntegration(t, pool, adapterIntegrationA, true)
	store := NewPostgreSQLStore(pool)
	base := time.Date(2026, 7, 31, 12, 0, 0, 123456789, time.UTC)

	integration, enabled, err := store.LoadIntegration(context.Background(), adapterIntegrationA)
	if err != nil || !enabled || integration.AdapterID != "org.ubnetdef.espial.sample" ||
		integration.ConfigNonsecret["scenario"] != "healthy" || integration.SecretReferences["token"] != "secret://sample" {
		t.Fatalf("integration = %#v enabled=%v err=%v", integration, enabled, err)
	}
	if err := store.MarkStarting(context.Background(), adapterIntegrationA, base); err != nil {
		t.Fatal(err)
	}
	instance, found, err := store.LoadInstance(context.Background(), adapterIntegrationA)
	if err != nil || !found || instance.State != "starting" || instance.LastStartedAt == nil {
		t.Fatalf("starting instance = %#v found=%v err=%v", instance, found, err)
	}
	if err := store.MarkHealthy(context.Background(), adapterIntegrationA, "0.1.0", ProtocolV1, base.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	instance, err = store.MarkFailed(context.Background(), adapterIntegrationA, "process_exit", base.Add(2*time.Second), 1)
	if err != nil || instance.State != "unhealthy" || instance.ConsecutiveFailures != 1 ||
		instance.NextRestartAt == nil || !instance.NextRestartAt.Equal(databaseTime(base.Add(3*time.Second))) {
		t.Fatalf("first failure = %#v, %v", instance, err)
	}
	if err := store.MarkStarting(context.Background(), adapterIntegrationA, base.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	instance, err = store.MarkFailed(context.Background(), adapterIntegrationA, "request_timeout", base.Add(4*time.Second), 1)
	if err != nil || instance.ConsecutiveFailures != 2 || instance.NextRestartAt == nil ||
		!instance.NextRestartAt.Equal(databaseTime(base.Add(6*time.Second))) {
		t.Fatalf("second failure = %#v, %v", instance, err)
	}
	if err := store.MarkStarting(context.Background(), adapterIntegrationA, base.Add(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkHealthy(context.Background(), adapterIntegrationA, "0.1.0", ProtocolV1, base.Add(7*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetFailures(context.Background(), adapterIntegrationA, base.Add(HealthyReset)); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkStopped(context.Background(), adapterIntegrationA, base.Add(HealthyReset+time.Second)); err != nil {
		t.Fatal(err)
	}
	instance, found, err = store.LoadInstance(context.Background(), adapterIntegrationA)
	if err != nil || !found || instance.State != "stopped" || instance.ConsecutiveFailures != 0 ||
		instance.NextRestartAt != nil || instance.LastStoppedAt == nil || instance.ProtocolVersion != ProtocolV1 {
		t.Fatalf("stopped instance = %#v found=%v err=%v", instance, found, err)
	}
}

func TestPostgreSQLStoreScopesInstancesAndDisabledIntegrations(t *testing.T) {
	pool := adapterTestPool(t)
	insertAdapterIntegration(t, pool, adapterIntegrationA, true)
	insertAdapterIntegration(t, pool, adapterIntegrationB, false)
	store := NewPostgreSQLStore(pool)
	_, enabled, err := store.LoadIntegration(context.Background(), adapterIntegrationB)
	if err != nil || enabled {
		t.Fatalf("disabled integration enabled=%v err=%v", enabled, err)
	}
	if err := store.MarkStarting(context.Background(), adapterIntegrationB, time.Now()); err == nil {
		t.Fatal("started a disabled integration")
	}
	if err := store.MarkStarting(context.Background(), adapterIntegrationA, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkHealthy(context.Background(), adapterIntegrationB, "0.1.0", ProtocolV1, time.Now()); err == nil {
		t.Fatal("updated integration without an owned instance")
	}
	instance, found, err := store.LoadInstance(context.Background(), adapterIntegrationA)
	if err != nil || !found || instance.State != "starting" {
		t.Fatalf("integration A changed: %#v found=%v err=%v", instance, found, err)
	}
}

func insertAdapterIntegration(t *testing.T, pool *pgxpool.Pool, id string, enabled bool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO integrations (
			id, adapter_id, display_name, enabled, config_nonsecret, secret_references
		) VALUES ($1, 'org.ubnetdef.espial.sample', $2, $3,
			'{"scenario":"healthy"}'::jsonb, '{"token":"secret://sample"}'::jsonb)
	`, id, "Integration "+id, enabled); err != nil {
		t.Fatal(err)
	}
}

func adapterTestPool(t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("espial_adapters_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		base.Close()
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		base.Close()
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
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
