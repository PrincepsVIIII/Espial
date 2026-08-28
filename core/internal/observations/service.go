package observations

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/signals"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	Clock     health.Clock
	Publisher Publisher
	Signals   *signals.Writer
}

type Service struct {
	pool      *pgxpool.Pool
	clock     health.Clock
	publisher Publisher
	signals   *signals.Writer
}

type CommitHook func(context.Context, pgx.Tx, Result) error

// currentKey identifies one independent health stream for a resource. A
// resource may publish availability, capacity, certificate, and other checks
// without one stream replacing another's current state.
type currentKey struct {
	resourceID string
	checkType  string
}

func NewService(pool *pgxpool.Pool, options Options) *Service {
	if options.Clock == nil {
		options.Clock = health.SystemClock{}
	}
	if options.Signals == nil {
		options.Signals = signals.NewWriter()
	}
	return &Service{pool: pool, clock: options.Clock, publisher: options.Publisher, signals: options.Signals}
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
	affectedResources := make(map[string]struct{}, len(batch.Resources))
	for index, resource := range batch.Resources {
		id, err := repo.upsertResource(ctx, integrationID, resource)
		if err != nil {
			return Result{}, fmt.Errorf("resource %d: %w", index, err)
		}
		resourceIDs[resource.ExternalID] = id
		affectedResources[id] = struct{}{}
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
		affectedResources[id] = struct{}{}
	}

	orderedResourceIDs := make([]string, 0, len(affectedResources))
	for id := range affectedResources {
		orderedResourceIDs = append(orderedResourceIDs, id)
	}
	sort.Strings(orderedResourceIDs)
	for _, id := range orderedResourceIDs {
		if err := repo.lockResource(ctx, integrationID, id); err != nil {
			return Result{}, err
		}
	}

	affectedChecks := make(map[currentKey]struct{}, len(batch.Observations))
	for index, observation := range batch.Observations {
		resourceID := resourceIDs[observation.ExternalResourceID]
		stored, inserted, err := repo.insertObservation(ctx, integrationID, resourceID, observation, receivedAt)
		if err != nil {
			var conflict *ConflictError
			if errors.As(err, &conflict) {
				conflict.Index = index
			}
			return Result{}, err
		}
		if inserted {
			result.ObservationsInserted++
			reasonCode, _ := observation.Metadata["reason_code"].(string)
			if err := service.signals.Append(ctx, tx, signals.Input{
				SourceKey: signals.ObservationSourceKey(stored.ID), Kind: signals.KindObservation,
				IntegrationID: integrationID, ResourceID: resourceID,
				ObservationID: stored.ID, CheckType: stored.CheckType,
				State: stored.State, Reason: stored.Summary, ReasonCode: reasonCode, OccurredAt: stored.ObservedAt,
				AvailableAt: receivedAt,
			}); err != nil {
				return Result{}, err
			}
		} else {
			result.DuplicateObservations++
		}
		affectedChecks[currentKey{resourceID: resourceID, checkType: observation.CheckType}] = struct{}{}
	}

	orderedChecks := make([]currentKey, 0, len(affectedChecks))
	for key := range affectedChecks {
		orderedChecks = append(orderedChecks, key)
	}
	sort.Slice(orderedChecks, func(left, right int) bool {
		if orderedChecks[left].resourceID == orderedChecks[right].resourceID {
			return orderedChecks[left].checkType < orderedChecks[right].checkType
		}
		return orderedChecks[left].resourceID < orderedChecks[right].resourceID
	})
	for _, key := range orderedChecks {
		previous, hadPrevious, err := repo.loadCurrent(ctx, key.resourceID, key.checkType)
		if err != nil {
			return Result{}, err
		}
		latest, hasObservation, err := repo.latestObservation(ctx, key.resourceID, key.checkType)
		if err != nil {
			return Result{}, err
		}
		if !hasObservation {
			continue
		}
		var previousPointer *health.Current
		if hadPrevious {
			previousPointer = &previous
		}
		desired := health.Evaluate(key.resourceID, latest, previousPointer, receivedAt)
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
