package observations

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	Clock     health.Clock
	Publisher Publisher
}

type Service struct {
	pool      *pgxpool.Pool
	clock     health.Clock
	publisher Publisher
}

type CommitHook func(context.Context, pgx.Tx, Result) error

func NewService(pool *pgxpool.Pool, options Options) *Service {
	if options.Clock == nil {
		options.Clock = health.SystemClock{}
	}
	return &Service{pool: pool, clock: options.Clock, publisher: options.Publisher}
}

func (service *Service) Ingest(ctx context.Context, integrationID string, batch Batch) (Result, error) {
	return service.IngestWithCommit(ctx, integrationID, batch, nil)
}

// IngestWithCommit runs hook inside the ingestion transaction after all normalized
// writes succeed and before commit. A hook failure rolls back data and emits no
// post-commit state changes.
func (service *Service) IngestWithCommit(
	ctx context.Context,
	integrationID string,
	batch Batch,
	hook CommitHook,
) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	receivedAt := service.clock.Now().UTC()
	if !uuidPattern.MatchString(integrationID) {
		return Result{}, &ValidationError{Fields: []FieldError{{
			Record: "integration", Index: -1, Field: "id", Code: "invalid_uuid",
		}}}
	}
	if err := ValidateBatch(batch, receivedAt); err != nil {
		return Result{}, err
	}

	tx, err := service.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Result{}, fmt.Errorf("begin observation ingestion: %w", err)
	}
	defer tx.Rollback(ctx)
	repo := repository{tx: tx}
	exists, err := repo.integrationExists(ctx, integrationID)
	if err != nil {
		return Result{}, err
	}
	if !exists {
		return Result{}, &ConflictError{Record: "integration", Index: -1, Code: "not_found"}
	}

	result := Result{}
	resourceIDs := make(map[string]string, len(batch.Resources))
	affected := make(map[string]struct{}, len(batch.Resources))
	for index, resource := range batch.Resources {
		id, err := repo.upsertResource(ctx, integrationID, resource)
		if err != nil {
			return Result{}, fmt.Errorf("resource %d: %w", index, err)
		}
		resourceIDs[resource.ExternalID] = id
		affected[id] = struct{}{}
		result.ResourcesUpserted++
	}
	for index, observation := range batch.Observations {
		if _, ok := resourceIDs[observation.ExternalResourceID]; ok {
			continue
		}
		id, err := repo.resolveResource(ctx, integrationID, observation.ExternalResourceID)
		if err != nil {
			var conflict *ConflictError
			if errors.As(err, &conflict) {
				conflict.Index = index
			}
			return Result{}, err
		}
		resourceIDs[observation.ExternalResourceID] = id
		affected[id] = struct{}{}
	}

	orderedResourceIDs := make([]string, 0, len(affected))
	for id := range affected {
		orderedResourceIDs = append(orderedResourceIDs, id)
	}
	sort.Strings(orderedResourceIDs)
	for _, id := range orderedResourceIDs {
		if err := repo.lockResource(ctx, integrationID, id); err != nil {
			return Result{}, err
		}
	}

	for index, observation := range batch.Observations {
		resourceID := resourceIDs[observation.ExternalResourceID]
		_, inserted, err := repo.insertObservation(ctx, integrationID, resourceID, observation, receivedAt)
		if err != nil {
			var conflict *ConflictError
			if errors.As(err, &conflict) {
				conflict.Index = index
			}
			return Result{}, err
		}
		if inserted {
			result.ObservationsInserted++
		} else {
			result.DuplicateObservations++
		}
	}

	for _, resourceID := range orderedResourceIDs {
		previous, hadPrevious, err := repo.loadCurrent(ctx, resourceID)
		if err != nil {
			return Result{}, err
		}
		latest, hasObservation, err := repo.latestObservation(ctx, resourceID)
		if err != nil {
			return Result{}, err
		}
		var desired health.Current
		if hasObservation {
			var previousPointer *health.Current
			if hadPrevious {
				previousPointer = &previous
			}
			desired = health.Evaluate(resourceID, latest, previousPointer, receivedAt)
		} else {
			desired = health.NoObservation(resourceID, receivedAt)
		}
		if hadPrevious && currentEquivalent(previous, desired) {
			continue
		}
		if err := repo.saveCurrent(ctx, desired); err != nil {
			return Result{}, err
		}
		if !hadPrevious || previous.State != desired.State {
			change := health.Change{After: desired}
			if hadPrevious {
				before := previous
				change.Before = &before
			}
			result.Changes = append(result.Changes, change)
		}
	}
	if hook != nil {
		hookResult := result
		hookResult.Changes = append([]health.Change(nil), result.Changes...)
		if err := hook(ctx, tx, hookResult); err != nil {
			return Result{}, fmt.Errorf("record observation commit: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("commit observation ingestion: %w", err)
	}
	if service.publisher != nil && len(result.Changes) > 0 {
		changes := append([]health.Change(nil), result.Changes...)
		service.publisher.PublishCommitted(changes)
	}
	return result, nil
}
