// Package signals owns Espial's durable monitoring-signal journal.
package signals

import (
	"context"
	"fmt"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	KindObservation       = "observation"
	KindFreshness         = "freshness"
	KindMaintenanceExpiry = "maintenance_expiry"
)

type Signal struct {
	ID            string
	Kind          string
	IntegrationID string
	ResourceID    string
	ObservationID string
	CheckType     string
	State         health.State
	Reason        string
	ReasonCode    string
	OccurredAt    time.Time
	CreatedAt     time.Time
	Attempts      int
}

type Input struct {
	SourceKey     string
	Kind          string
	IntegrationID string
	ResourceID    string
	ObservationID string
	CheckType     string
	State         health.State
	Reason        string
	ReasonCode    string
	OccurredAt    time.Time
	AvailableAt   time.Time
}

type Writer struct{}

func NewWriter() *Writer { return &Writer{} }

func (*Writer) Append(ctx context.Context, tx pgx.Tx, input Input) error {
	if input.AvailableAt.IsZero() {
		input.AvailableAt = input.OccurredAt
	}
	var observationID any
	if input.ObservationID != "" {
		observationID = input.ObservationID
	}
	var reasonCode any
	if input.ReasonCode != "" {
		reasonCode = input.ReasonCode
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO monitoring_signals (
			source_key, kind, integration_id, resource_id, observation_id,
			check_type, state, reason, reason_code, occurred_at, available_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (source_key) DO NOTHING
	`, input.SourceKey, input.Kind, input.IntegrationID, input.ResourceID,
		observationID, input.CheckType, input.State, input.Reason, reasonCode,
		input.OccurredAt.UTC().Truncate(time.Microsecond),
		input.AvailableAt.UTC().Truncate(time.Microsecond)); err != nil {
		return fmt.Errorf("append monitoring signal: %w", err)
	}
	return nil
}

func ObservationSourceKey(observationID string) string {
	return "observation:" + observationID
}

func FreshnessSourceKey(resourceID string, state health.State, occurredAt time.Time) string {
	return fmt.Sprintf("freshness:%s:%s:%d", resourceID, state, occurredAt.UTC().UnixMicro())
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Metrics is a bounded, low-cardinality journal snapshot suitable for the
// process metrics exporter. It intentionally contains no integration/resource IDs.
type Metrics struct {
	QueueDepth               int64
	OldestAge                time.Duration
	Claimed                  int64
	Retried                  int64
	DeadLetters              int64
	AverageProcessingLatency time.Duration
}

func (store *Store) Metrics(ctx context.Context, now time.Time) (Metrics, error) {
	var result Metrics
	var oldestSeconds, latencySeconds float64
	err := store.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE processed_at IS NULL AND dead_lettered_at IS NULL),
			COALESCE(EXTRACT(epoch FROM ($1 - min(created_at) FILTER (WHERE processed_at IS NULL AND dead_lettered_at IS NULL))), 0),
			count(*) FILTER (WHERE processed_at IS NULL AND dead_lettered_at IS NULL AND claimed_until > $1),
			count(*) FILTER (WHERE attempts > 1),
			count(*) FILTER (WHERE dead_lettered_at IS NOT NULL),
			COALESCE(avg(EXTRACT(epoch FROM (processed_at - created_at))) FILTER (WHERE processed_at IS NOT NULL), 0)
		FROM monitoring_signals
	`, now.UTC().Truncate(time.Microsecond)).Scan(
		&result.QueueDepth, &oldestSeconds, &result.Claimed, &result.Retried,
		&result.DeadLetters, &latencySeconds,
	)
	if err != nil {
		return Metrics{}, fmt.Errorf("read monitoring signal metrics: %w", err)
	}
	if oldestSeconds > 0 {
		result.OldestAge = time.Duration(oldestSeconds * float64(time.Second))
	}
	if latencySeconds > 0 {
		result.AverageProcessingLatency = time.Duration(latencySeconds * float64(time.Second))
	}
	return result, nil
}

// Claim leases one bounded, chronologically ordered batch. SKIP LOCKED permits a
// future second Core instance without letting a slow claimant block all work.
func (store *Store) Claim(ctx context.Context, now time.Time, limit int, lease time.Duration, maxAttempts int) ([]Signal, error) {
	if limit <= 0 {
		limit = 50
	}
	if lease <= 0 {
		lease = 30 * time.Second
	}
	if maxAttempts <= 0 {
		maxAttempts = 8
	}
	now = now.UTC().Truncate(time.Microsecond)
	rows, err := store.pool.Query(ctx, `
		WITH due AS (
			SELECT id
			FROM monitoring_signals
			WHERE processed_at IS NULL
			  AND dead_lettered_at IS NULL
			  AND available_at <= $1
			  AND (claimed_until IS NULL OR claimed_until <= $1)
			  AND attempts < $4
			ORDER BY occurred_at, created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
		UPDATE monitoring_signals signal
		SET claimed_until = $1 + make_interval(secs => $3),
			attempts = signal.attempts + 1
		FROM due
		WHERE signal.id = due.id
		RETURNING signal.id::text, signal.kind, signal.integration_id::text,
			signal.resource_id::text, COALESCE(signal.observation_id::text, ''),
			signal.check_type, signal.state, signal.reason,
			COALESCE(signal.reason_code, ''), signal.occurred_at,
			signal.created_at, signal.attempts
	`, now, limit, lease.Seconds(), maxAttempts)
	if err != nil {
		return nil, fmt.Errorf("claim monitoring signals: %w", err)
	}
	defer rows.Close()
	result := make([]Signal, 0, limit)
	for rows.Next() {
		var item Signal
		if err := rows.Scan(
			&item.ID, &item.Kind, &item.IntegrationID, &item.ResourceID,
			&item.ObservationID, &item.CheckType, &item.State, &item.Reason,
			&item.ReasonCode, &item.OccurredAt, &item.CreatedAt, &item.Attempts,
		); err != nil {
			return nil, fmt.Errorf("scan monitoring signal: %w", err)
		}
		item.OccurredAt = item.OccurredAt.UTC()
		item.CreatedAt = item.CreatedAt.UTC()
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read monitoring signals: %w", err)
	}
	return result, nil
}

func MarkProcessed(ctx context.Context, tx pgx.Tx, id string, now time.Time) error {
	command, err := tx.Exec(ctx, `
		UPDATE monitoring_signals
		SET processed_at = $2, claimed_until = NULL, last_error_code = NULL
		WHERE id = $1 AND processed_at IS NULL AND dead_lettered_at IS NULL
	`, id, now.UTC().Truncate(time.Microsecond))
	if err != nil {
		return fmt.Errorf("complete monitoring signal: %w", err)
	}
	if command.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return nil
}

func (store *Store) Fail(ctx context.Context, id string, now time.Time, retryAfter time.Duration, maxAttempts int, code string) error {
	if retryAfter < 0 {
		retryAfter = 0
	}
	if maxAttempts <= 0 {
		maxAttempts = 8
	}
	_, err := store.pool.Exec(ctx, `
		UPDATE monitoring_signals
		SET claimed_until = NULL,
			available_at = $2::timestamptz + make_interval(secs => $3),
			last_error_code = $5,
			dead_lettered_at = CASE WHEN attempts >= $4 THEN $2 ELSE NULL END
		WHERE id = $1 AND processed_at IS NULL AND dead_lettered_at IS NULL
	`, id, now.UTC().Truncate(time.Microsecond), retryAfter.Seconds(), maxAttempts, code)
	if err != nil {
		return fmt.Errorf("fail monitoring signal: %w", err)
	}
	return nil
}
