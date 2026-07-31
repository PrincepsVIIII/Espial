package storage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrateAgainstPostgreSQL(t *testing.T) {
	pool := isolatedTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("second migration: %v", err)
	}

	var migrationCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != 3 {
		t.Fatalf("migration count = %d", migrationCount)
	}

	var roleCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM roles").Scan(&roleCount); err != nil {
		t.Fatalf("count roles: %v", err)
	}
	if roleCount != 3 {
		t.Fatalf("role count = %d", roleCount)
	}
}

func TestConcurrentMigrationsSerialize(t *testing.T) {
	pool := isolatedTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var wait sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errors <- Migrate(ctx, pool)
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent migration: %v", err)
		}
	}
}

func TestMigrateRejectsNewerDatabase(t *testing.T) {
	pool := isolatedTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx,
		"INSERT INTO schema_migrations (version, name) VALUES (4, '000004_future.sql')",
	); err != nil {
		t.Fatalf("insert future migration: %v", err)
	}

	err := Migrate(ctx, pool)
	if err == nil || !strings.Contains(err.Error(), "newer than binary") {
		t.Fatalf("error = %v", err)
	}
}

func isolatedTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ESPIAL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ESPIAL_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open base database: %v", err)
	}
	if err := base.Ping(ctx); err != nil {
		base.Close()
		t.Fatalf("ping base database: %v", err)
	}

	schema := fmt.Sprintf("espial_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		base.Close()
		t.Fatalf("create test schema: %v", err)
	}

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		base.Close()
		t.Fatalf("parse test database URL: %v", err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		base.Close()
		t.Fatalf("open isolated database: %v", err)
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
