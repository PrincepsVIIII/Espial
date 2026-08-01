package monitoring

import (
	"context"
	"errors"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/certificateprojection"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/observations"
	"github.com/jackc/pgx/v5"
)

type Ingestor interface {
	IngestWithCommit(context.Context, string, observations.Batch, observations.CommitHook) (observations.Result, error)
}

type Collector struct {
	ingestor Ingestor
	store    *CollectionStore
	hub      *events.Hub
	clock    health.Clock
}

func NewCollector(ingestor Ingestor, store *CollectionStore, hub *events.Hub, clock health.Clock) *Collector {
	if clock == nil {
		clock = health.SystemClock{}
	}
	return &Collector{ingestor: ingestor, store: store, hub: hub, clock: clock}
}

func (collector *Collector) Collect(ctx context.Context, integration adapters.Integration, session *adapters.Session) error {
	correlationID, err := newCorrelationID()
	if err != nil {
		return &Error{Code: "correlation_id_failed"}
	}
	startedAt := collector.clock.Now().UTC()
	_, batch, err := session.Collect(ctx, startedAt)
	if err != nil {
		attempt := Attempt{
			IntegrationID: integration.ID, StartedAt: startedAt, CompletedAt: collector.nowAfter(startedAt),
			Result: classifyCollectionError(err), ErrorCode: safeCollectionCode(err), CorrelationID: correlationID,
		}
		if ctx.Err() == nil {
			if recordErr := collector.store.RecordOutcome(ctx, attempt); recordErr != nil {
				return &Error{Code: "collection_record_failed"}
			}
			collector.publishAttempt(attempt, integration.AdapterID)
		}
		if failure := session.Process.Failure(); failure != nil {
			return failure
		}
		return nil
	}
	attempt := Attempt{
		IntegrationID: integration.ID, StartedAt: startedAt,
		ResourceCount: len(batch.Resources), ObservationCount: len(batch.Observations),
		CorrelationID: correlationID,
	}
	result, err := collector.ingestor.IngestWithCommit(ctx, integration.ID, batch,
		func(hookContext context.Context, tx pgx.Tx, result observations.Result) error {
			if integration.AdapterID == "org.ubnetdef.espial.webcheck" {
				if err := certificateprojection.ProjectBatch(hookContext, tx, integration.ID, batch); err != nil {
					return err
				}
			}
			attempt.CompletedAt = collector.nowAfter(startedAt)
			return collector.store.RecordCommitted(hookContext, tx, attempt, result)
		})
	_ = result
	if err != nil {
		attempt.CompletedAt = collector.nowAfter(startedAt)
		attempt.Result = classifyIngestionError(err)
		attempt.ErrorCode = safeIngestionCode(err)
		if ctx.Err() == nil {
			if recordErr := collector.store.RecordOutcome(ctx, attempt); recordErr != nil {
				return &Error{Code: "collection_record_failed"}
			}
			collector.publishAttempt(attempt, integration.AdapterID)
		}
		return nil
	}
	attempt.Result = CollectionSucceeded
	if attempt.CompletedAt.IsZero() {
		attempt.CompletedAt = collector.nowAfter(startedAt)
	}
	collector.publishAttempt(attempt, integration.AdapterID)
	return nil
}

func (collector *Collector) publishAttempt(attempt Attempt, adapterID string) {
	if collector.hub == nil {
		return
	}
	collector.hub.Publish(events.Event{
		Kind: events.CollectionChanged, IntegrationID: attempt.IntegrationID,
		Result: attempt.Result, ChangedAt: attempt.CompletedAt,
	})
	// Webpage consumers refetch their authoritative projection after every
	// completed or failed collection attempt.
	if adapterID == "org.ubnetdef.espial.webcheck" {
		collector.hub.Publish(events.Event{Kind: events.WebpageChanged, MonitorID: attempt.IntegrationID,
			IntegrationID: attempt.IntegrationID, Result: attempt.Result, ChangedAt: attempt.CompletedAt})
		collector.hub.Publish(events.Event{Kind: events.CertificateChanged, MonitorID: attempt.IntegrationID,
			IntegrationID: attempt.IntegrationID, Result: attempt.Result, ChangedAt: attempt.CompletedAt})
	}
}

func (collector *Collector) nowAfter(startedAt time.Time) time.Time {
	completed := collector.clock.Now().UTC()
	if completed.Before(startedAt) {
		return startedAt
	}
	return completed
}

func classifyCollectionError(err error) string {
	var runtime *adapters.RuntimeError
	if errors.As(err, &runtime) && runtime.Code == "invalid_collection" {
		return CollectionRejected
	}
	return CollectionFailed
}

func safeCollectionCode(err error) string {
	var safe interface{ SafeCode() string }
	if errors.As(err, &safe) && safeCodePattern.MatchString(safe.SafeCode()) {
		return safe.SafeCode()
	}
	return "collection_failed"
}

func classifyIngestionError(err error) string {
	var validation *observations.ValidationError
	var conflict *observations.ConflictError
	if errors.As(err, &validation) || errors.As(err, &conflict) {
		return CollectionRejected
	}
	return CollectionFailed
}

func safeIngestionCode(err error) string {
	var validation *observations.ValidationError
	if errors.As(err, &validation) {
		return "validation_failed"
	}
	var conflict *observations.ConflictError
	if errors.As(err, &conflict) && safeCodePattern.MatchString(conflict.Code) {
		return conflict.Code
	}
	return "ingestion_failed"
}

type Error struct{ Code string }

func (err *Error) Error() string    { return "monitoring failure: " + err.Code }
func (err *Error) SafeCode() string { return err.Code }
