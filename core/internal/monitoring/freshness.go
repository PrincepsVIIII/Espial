package monitoring

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/observations"
	"github.com/PrincepsVIIII/Espial/core/internal/signals"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FreshnessTimer interface {
	Channel() <-chan time.Time
	Stop() bool
}

type freshnessSystemTimer struct{ timer *time.Timer }

func (timer freshnessSystemTimer) Channel() <-chan time.Time { return timer.timer.C }
func (timer freshnessSystemTimer) Stop() bool                { return timer.timer.Stop() }

type FreshnessOptions struct {
	Clock        health.Clock
	Publisher    observations.Publisher
	BatchSize    int
	PollInterval time.Duration
	NewTimer     func(time.Duration) FreshnessTimer
	Signals      *signals.Writer
}

type FreshnessWorker struct {
	pool        *pgxpool.Pool
	options     FreshnessOptions
	running     atomic.Bool
	transitions atomic.Uint64
}

func NewFreshnessWorker(pool *pgxpool.Pool, options FreshnessOptions) *FreshnessWorker {
	if options.Clock == nil {
		options.Clock = health.SystemClock{}
	}
	if options.BatchSize <= 0 {
		options.BatchSize = 100
	}
	if options.PollInterval <= 0 {
		options.PollInterval = time.Second
	}
	if options.NewTimer == nil {
		options.NewTimer = func(delay time.Duration) FreshnessTimer {
			return freshnessSystemTimer{timer: time.NewTimer(delay)}
		}
	}
	if options.Signals == nil {
		options.Signals = signals.NewWriter()
	}
	return &FreshnessWorker{pool: pool, options: options}
}

// Run owns the single polling goroutine for this worker instance.
func (worker *FreshnessWorker) Run(ctx context.Context) error {
	if !worker.running.CompareAndSwap(false, true) {
		return &Error{Code: "freshness_already_running"}
	}
	defer worker.running.Store(false)
	for {
		for {
			changes, err := worker.RefreshDue(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return err
			}
			if len(changes) < worker.options.BatchSize {
				break
			}
		}
		timer := worker.options.NewTimer(worker.options.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.Channel():
			timer.Stop()
		}
	}
}

// RefreshDue claims and updates one bounded batch. Row locks plus SKIP LOCKED make
// concurrent future Core instances cooperate without duplicate transitions.
func (worker *FreshnessWorker) RefreshDue(ctx context.Context) ([]health.Change, error) {
	now := worker.options.Clock.Now().UTC().Truncate(time.Microsecond)
	tx, err := worker.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin freshness transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		SELECT
			r.integration_id::text, ch.resource_id::text, ch.state, ch.reason, ch.observation_id::text,
			ch.observed_at, ch.last_success_at, ch.stale_at, ch.unknown_at, ch.updated_at,
			o.check_type, o.observed_state, o.summary, o.observed_at, o.received_at,
			o.expected_refresh_seconds
		FROM current_health ch
		JOIN observations o ON o.id = ch.observation_id
		JOIN resources r ON r.id = ch.resource_id
		WHERE ch.state <> 'disabled' AND (
			(ch.state NOT IN ('stale', 'unknown') AND ch.stale_at <= $1) OR
			(ch.state = 'stale' AND ch.unknown_at <= $1)
		)
		ORDER BY
			CASE WHEN ch.state = 'stale' THEN ch.unknown_at ELSE ch.stale_at END,
			ch.resource_id
		FOR UPDATE OF ch SKIP LOCKED
		LIMIT $2
	`, now, worker.options.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("claim due freshness rows: %w", err)
	}
	type dueRow struct {
		integrationID string
		previous      health.Current
		observation   health.Observation
	}
	claimed := make([]dueRow, 0, worker.options.BatchSize)
	for rows.Next() {
		var row dueRow
		var currentState, observedState string
		var currentObserved, lastSuccess, staleAt, unknownAt pgtype.Timestamptz
		var refreshSeconds int
		if err := rows.Scan(
			&row.integrationID, &row.previous.ResourceID, &currentState, &row.previous.Reason, &row.observation.ID,
			&currentObserved, &lastSuccess, &staleAt, &unknownAt, &row.previous.UpdatedAt,
			&row.observation.CheckType, &observedState, &row.observation.Summary, &row.observation.ObservedAt,
			&row.observation.ReceivedAt, &refreshSeconds,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan due freshness row: %w", err)
		}
		row.previous.State = health.State(currentState)
		row.previous.ObservationID = stringPointer(row.observation.ID)
		row.previous.ObservedAt = nullableTimestamp(currentObserved)
		row.previous.LastSuccessAt = nullableTimestamp(lastSuccess)
		row.previous.StaleAt = nullableTimestamp(staleAt)
		row.previous.UnknownAt = nullableTimestamp(unknownAt)
		row.observation.ResourceID = row.previous.ResourceID
		row.observation.State = health.State(observedState)
		row.observation.ExpectedRefresh = time.Duration(refreshSeconds) * time.Second
		claimed = append(claimed, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("read due freshness rows: %w", err)
	}
	rows.Close()

	changes := make([]health.Change, 0, len(claimed))
	for _, row := range claimed {
		desired := health.Evaluate(row.previous.ResourceID, row.observation, &row.previous, now)
		if desired.State == row.previous.State {
			continue
		}
		observationID := ""
		if desired.ObservationID != nil {
			observationID = *desired.ObservationID
		}
		if _, err := tx.Exec(ctx, `
			UPDATE current_health SET
				state = $2, reason = $3, observation_id = NULLIF($4, '')::uuid,
				observed_at = $5, last_success_at = $6, stale_at = $7,
				unknown_at = $8, updated_at = $9
			WHERE resource_id = $1
		`, desired.ResourceID, desired.State, desired.Reason, observationID,
			desired.ObservedAt, desired.LastSuccessAt, desired.StaleAt, desired.UnknownAt,
			desired.UpdatedAt); err != nil {
			return nil, fmt.Errorf("update due freshness row: %w", err)
		}
		if err := worker.options.Signals.Append(ctx, tx, signals.Input{
			SourceKey: signals.FreshnessSourceKey(desired.ResourceID, desired.State, desired.UpdatedAt),
			Kind:      signals.KindFreshness, IntegrationID: row.integrationID,
			ResourceID: desired.ResourceID, ObservationID: row.observation.ID,
			CheckType: row.observation.CheckType, State: desired.State,
			Reason: desired.Reason, OccurredAt: desired.UpdatedAt, AvailableAt: now,
		}); err != nil {
			return nil, err
		}
		before := row.previous
		changes = append(changes, health.Change{Before: &before, After: desired})
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit freshness transitions: %w", err)
	}
	if worker.options.Publisher != nil && len(changes) > 0 {
		worker.options.Publisher.PublishCommitted(append([]health.Change(nil), changes...))
	}
	worker.transitions.Add(uint64(len(changes)))
	return changes, nil
}

func (worker *FreshnessWorker) TransitionCount() uint64 { return worker.transitions.Load() }

func nullableTimestamp(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func stringPointer(value string) *string { return &value }
