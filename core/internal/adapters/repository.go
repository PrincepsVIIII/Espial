package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InstanceStore interface {
	LoadIntegration(context.Context, string) (Integration, bool, error)
	LoadInstance(context.Context, string) (Instance, bool, error)
	MarkStarting(context.Context, string, time.Time) error
	MarkHealthy(context.Context, string, string, string, time.Time) error
	MarkFailed(context.Context, string, string, time.Time, float64) (Instance, error)
	ResetFailures(context.Context, string, time.Time) error
	MarkStopped(context.Context, string, time.Time) error
}

type PostgreSQLStore struct{ pool *pgxpool.Pool }

func NewPostgreSQLStore(pool *pgxpool.Pool) *PostgreSQLStore { return &PostgreSQLStore{pool: pool} }

func (store *PostgreSQLStore) LoadIntegration(ctx context.Context, integrationID string) (Integration, bool, error) {
	var result Integration
	var nonsecret, references []byte
	var enabled bool
	err := store.pool.QueryRow(ctx, `
		SELECT id::text, adapter_id, enabled, config_nonsecret, secret_references
		FROM integrations WHERE id = $1
	`, integrationID).Scan(&result.ID, &result.AdapterID, &enabled, &nonsecret, &references)
	if errors.Is(err, pgx.ErrNoRows) {
		return Integration{}, false, nil
	}
	if err != nil {
		return Integration{}, false, fmt.Errorf("load adapter integration: %w", err)
	}
	if !enabled {
		return result, false, nil
	}
	if err := json.Unmarshal(nonsecret, &result.ConfigNonsecret); err != nil {
		return Integration{}, false, fmt.Errorf("decode nonsecret integration config: %w", err)
	}
	if err := json.Unmarshal(references, &result.SecretReferences); err != nil {
		return Integration{}, false, fmt.Errorf("decode integration secret references: %w", err)
	}
	return result, true, nil
}

func (store *PostgreSQLStore) LoadInstance(ctx context.Context, integrationID string) (Instance, bool, error) {
	return loadInstance(ctx, store.pool, integrationID)
}

func (store *PostgreSQLStore) MarkStarting(ctx context.Context, integrationID string, now time.Time) error {
	result, err := store.pool.Exec(ctx, `
		INSERT INTO adapter_instances (
			id, integration_id, adapter_version, state, last_started_at, updated_at
		)
		SELECT gen_random_uuid(), id, 'unknown', 'starting', $2, $2
		FROM integrations WHERE id = $1 AND enabled = true
		ON CONFLICT (integration_id) DO UPDATE SET
			state = 'starting', last_started_at = EXCLUDED.last_started_at,
			last_stopped_at = NULL, updated_at = EXCLUDED.updated_at
	`, integrationID, databaseTime(now))
	if err != nil {
		return fmt.Errorf("mark adapter starting: %w", err)
	}
	if result.RowsAffected() != 1 {
		return runtimeError("integration_not_enabled")
	}
	return nil
}

func (store *PostgreSQLStore) MarkHealthy(
	ctx context.Context,
	integrationID, adapterVersion, protocolVersion string,
	now time.Time,
) error {
	result, err := store.pool.Exec(ctx, `
		UPDATE adapter_instances SET
			adapter_version = $2, protocol_version = $3, state = 'healthy',
			last_healthy_at = $4, last_error_code = NULL, last_error_at = NULL,
			next_restart_at = NULL, updated_at = $4
		WHERE integration_id = $1
	`, integrationID, adapterVersion, protocolVersion, databaseTime(now))
	if err != nil {
		return fmt.Errorf("mark adapter healthy: %w", err)
	}
	if result.RowsAffected() != 1 {
		return runtimeError("integration_not_found")
	}
	return nil
}

func (store *PostgreSQLStore) MarkFailed(
	ctx context.Context,
	integrationID, code string,
	now time.Time,
	jitter float64,
) (Instance, error) {
	if !errorCodePattern.MatchString(code) {
		return Instance{}, runtimeError("invalid_error_code")
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return Instance{}, fmt.Errorf("begin adapter failure update: %w", err)
	}
	defer tx.Rollback(ctx)
	var failures int
	if err := tx.QueryRow(ctx, `
		SELECT consecutive_failures FROM adapter_instances
		WHERE integration_id = $1 FOR UPDATE
	`, integrationID).Scan(&failures); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Instance{}, runtimeError("integration_not_found")
		}
		return Instance{}, fmt.Errorf("lock adapter failure state: %w", err)
	}
	failures++
	now = databaseTime(now)
	nextRestart := databaseTime(now.Add(RestartDelay(failures, jitter)))
	if _, err := tx.Exec(ctx, `
		UPDATE adapter_instances SET
			state = 'unhealthy', last_error_code = $2, last_error_at = $3,
			consecutive_failures = $4, next_restart_at = $5, updated_at = $3
		WHERE integration_id = $1
	`, integrationID, code, now, failures, nextRestart); err != nil {
		return Instance{}, fmt.Errorf("mark adapter failed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Instance{}, fmt.Errorf("commit adapter failure: %w", err)
	}
	result, found, err := store.LoadInstance(ctx, integrationID)
	if err != nil {
		return Instance{}, err
	}
	if !found {
		return Instance{}, runtimeError("integration_not_found")
	}
	return result, nil
}

func (store *PostgreSQLStore) ResetFailures(ctx context.Context, integrationID string, now time.Time) error {
	result, err := store.pool.Exec(ctx, `
		UPDATE adapter_instances SET consecutive_failures = 0, next_restart_at = NULL, updated_at = $2
		WHERE integration_id = $1 AND state = 'healthy'
	`, integrationID, databaseTime(now))
	if err != nil {
		return fmt.Errorf("reset adapter failures: %w", err)
	}
	if result.RowsAffected() != 1 {
		return runtimeError("invalid_instance_state")
	}
	return nil
}

func (store *PostgreSQLStore) MarkStopped(ctx context.Context, integrationID string, now time.Time) error {
	result, err := store.pool.Exec(ctx, `
		UPDATE adapter_instances SET
			state = 'stopped', last_stopped_at = $2, next_restart_at = NULL, updated_at = $2
		WHERE integration_id = $1
	`, integrationID, databaseTime(now))
	if err != nil {
		return fmt.Errorf("mark adapter stopped: %w", err)
	}
	if result.RowsAffected() != 1 {
		return runtimeError("integration_not_found")
	}
	return nil
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadInstance(ctx context.Context, source queryRower, integrationID string) (Instance, bool, error) {
	var result Instance
	var protocolVersion, lastErrorCode pgtype.Text
	var lastStarted, lastHealthy, lastStopped, lastError, nextRestart pgtype.Timestamptz
	err := source.QueryRow(ctx, `
		SELECT integration_id::text, adapter_version, protocol_version, state,
			last_started_at, last_healthy_at, last_stopped_at, last_error_at,
			last_error_code, consecutive_failures, next_restart_at, updated_at
		FROM adapter_instances WHERE integration_id = $1
	`, integrationID).Scan(
		&result.IntegrationID, &result.AdapterVersion, &protocolVersion, &result.State,
		&lastStarted, &lastHealthy, &lastStopped, &lastError, &lastErrorCode,
		&result.ConsecutiveFailures, &nextRestart, &result.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Instance{}, false, nil
	}
	if err != nil {
		return Instance{}, false, fmt.Errorf("load adapter instance: %w", err)
	}
	if protocolVersion.Valid {
		result.ProtocolVersion = protocolVersion.String
	}
	if lastErrorCode.Valid {
		result.LastErrorCode = lastErrorCode.String
	}
	result.LastStartedAt = nullableTime(lastStarted)
	result.LastHealthyAt = nullableTime(lastHealthy)
	result.LastStoppedAt = nullableTime(lastStopped)
	result.LastErrorAt = nullableTime(lastError)
	result.NextRestartAt = nullableTime(nextRestart)
	result.UpdatedAt = result.UpdatedAt.UTC()
	return result, true, nil
}

func nullableTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func databaseTime(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }
