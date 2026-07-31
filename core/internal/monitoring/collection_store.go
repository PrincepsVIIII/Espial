// Package monitoring connects scheduled adapter collections to normalized state.
package monitoring

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/audit"
	"github.com/PrincepsVIIII/Espial/core/internal/observations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	CollectionSucceeded = "succeeded"
	CollectionRejected  = "rejected"
	CollectionFailed    = "failed"
	CollectionSkipped   = "skipped"
)

var safeCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,126}$`)

type Attempt struct {
	IntegrationID    string
	StartedAt        time.Time
	CompletedAt      time.Time
	Result           string
	ErrorCode        string
	ResourceCount    int
	ObservationCount int
	CorrelationID    string
}

type CollectionStore struct{ pool *pgxpool.Pool }

func NewCollectionStore(pool *pgxpool.Pool) *CollectionStore { return &CollectionStore{pool: pool} }

func (store *CollectionStore) RecordCommitted(
	ctx context.Context,
	tx pgx.Tx,
	attempt Attempt,
	result observations.Result,
) error {
	attempt.Result = CollectionSucceeded
	attempt.ErrorCode = ""
	if err := insertAttempt(ctx, tx, attempt, result); err != nil {
		return err
	}
	return appendCollectionAudit(ctx, tx, attempt, result)
}

func (store *CollectionStore) RecordOutcome(ctx context.Context, attempt Attempt) error {
	if attempt.Result == CollectionSucceeded {
		return errors.New("successful collection must be recorded with its ingestion transaction")
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin collection outcome: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := insertAttempt(ctx, tx, attempt, observations.Result{}); err != nil {
		return err
	}
	if err := appendCollectionAudit(ctx, tx, attempt, observations.Result{}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit collection outcome: %w", err)
	}
	return nil
}

func insertAttempt(ctx context.Context, database audit.Execer, attempt Attempt, result observations.Result) error {
	if err := validateAttempt(attempt); err != nil {
		return err
	}
	duration := attempt.CompletedAt.Sub(attempt.StartedAt)
	if duration < 0 {
		return errors.New("invalid collection attempt")
	}
	durationMS := duration.Milliseconds()
	if _, err := database.Exec(ctx, `
		INSERT INTO integration_collection_runs (
			id, integration_id, started_at, completed_at, duration_ms, result,
			error_code, resource_count, observation_count, observations_inserted,
			duplicate_observations, correlation_id
		) VALUES (
			gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`, attempt.IntegrationID, attempt.StartedAt.UTC(), attempt.CompletedAt.UTC(), durationMS,
		attempt.Result, nullableCode(attempt.ErrorCode), attempt.ResourceCount,
		attempt.ObservationCount, result.ObservationsInserted, result.DuplicateObservations,
		attempt.CorrelationID); err != nil {
		return fmt.Errorf("insert collection attempt: %w", err)
	}
	return nil
}

func appendCollectionAudit(ctx context.Context, database audit.Execer, attempt Attempt, result observations.Result) error {
	summary := map[string]any{
		"duration_ms":            attempt.CompletedAt.Sub(attempt.StartedAt).Milliseconds(),
		"resource_count":         attempt.ResourceCount,
		"observation_count":      attempt.ObservationCount,
		"observations_inserted":  result.ObservationsInserted,
		"duplicate_observations": result.DuplicateObservations,
	}
	if attempt.ErrorCode != "" {
		summary["error_code"] = attempt.ErrorCode
	}
	auditResult := "succeeded"
	if attempt.Result != CollectionSucceeded {
		auditResult = "failed"
	}
	return audit.Append(ctx, database, audit.Event{
		Action: "integration.collection." + attempt.Result, TargetType: "integration",
		TargetID: attempt.IntegrationID, Result: auditResult,
		CorrelationID: attempt.CorrelationID, AfterSummary: summary,
		OccurredAt: attempt.CompletedAt,
	})
}

func validateAttempt(attempt Attempt) error {
	if attempt.IntegrationID == "" || attempt.StartedAt.IsZero() || attempt.CompletedAt.IsZero() ||
		attempt.CorrelationID == "" || len(attempt.CorrelationID) > 128 ||
		attempt.ResourceCount < 0 || attempt.ObservationCount < 0 {
		return errors.New("invalid collection attempt")
	}
	switch attempt.Result {
	case CollectionSucceeded:
		if attempt.ErrorCode != "" {
			return errors.New("invalid collection attempt")
		}
	case CollectionRejected, CollectionFailed, CollectionSkipped:
		if !safeCodePattern.MatchString(attempt.ErrorCode) {
			return errors.New("invalid collection attempt")
		}
	default:
		return errors.New("invalid collection attempt")
	}
	return nil
}

func nullableCode(value string) any {
	if value == "" {
		return nil
	}
	return value
}
