package storage

import (
	"context"
	"fmt"

	"github.com/PrincepsVIIII/Espial/core/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationLockID int64 = 0x45535049414c // "ESPIAL"

// Migrate applies all pending migrations in one advisory-locked transaction.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	available, err := migrations.All()
	if err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer tx.Rollback(ctx) // A no-op after Commit.

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version integer PRIMARY KEY,
			name text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	applied, err := appliedMigrations(ctx, tx)
	if err != nil {
		return err
	}
	if len(applied) > len(available) {
		return fmt.Errorf("database schema version %d is newer than binary version %d", len(applied), len(available))
	}
	for index, name := range applied {
		expected := available[index]
		if expected.Version != index+1 || expected.Name != name {
			return fmt.Errorf("database migration %d is %q; binary expects %q", index+1, name, expected.Name)
		}
	}

	for _, migration := range available[len(applied):] {
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.Name, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (version, name) VALUES ($1, $2)",
			migration.Version,
			migration.Name,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", migration.Name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

func appliedMigrations(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx, "SELECT version, name FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var version int
		var name string
		if err := rows.Scan(&version, &name); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		if version != len(names)+1 {
			return nil, fmt.Errorf("database migration sequence has version %d; want %d", version, len(names)+1)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	return names, nil
}
