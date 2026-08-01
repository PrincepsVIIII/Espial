package monitoring

import (
	"context"
	"errors"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/audit"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/incidents"
	"github.com/PrincepsVIIII/Espial/core/internal/observations"
	"github.com/PrincepsVIIII/Espial/core/internal/scheduler"
	"github.com/PrincepsVIIII/Espial/core/internal/suppressions"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RuntimeOptions struct {
	Clock               health.Clock
	GlobalConcurrency   int
	ReconcileInterval   time.Duration
	FreshnessInterval   time.Duration
	FreshnessBatchSize  int
	EventReplaySize     int
	IncidentBatchSize   int
	IncidentPoll        time.Duration
	IncidentClaimLease  time.Duration
	IncidentMaxAttempts int
	IncidentIntents     incidents.IntentWriter
	Process             adapters.ProcessOptions
	Secrets             adapters.SecretResolver
	OnError             func(string, error)
}

// Runtime owns the bounded collection, freshness, and incident evaluator
// goroutines plus their shared invalidation hub.
type Runtime struct {
	coordinator        *scheduler.Coordinator
	dispatcher         *scheduler.Scheduler
	freshness          *FreshnessWorker
	incidents          *incidents.Evaluator
	suppressions       *suppressions.Worker
	suppressionService *suppressions.Service
	hub                *events.Hub
}

func NewRuntime(
	pool *pgxpool.Pool,
	registry *adapters.Registry,
	options RuntimeOptions,
) *Runtime {
	hub := events.NewHub(options.EventReplaySize)
	observationService := observations.NewService(pool, observations.Options{
		Clock: options.Clock, Publisher: hub,
	})
	collector := NewCollector(observationService, NewCollectionStore(pool), hub, options.Clock)
	dispatcher := scheduler.New(collector, scheduler.Options{
		GlobalConcurrency: options.GlobalConcurrency,
	})
	store := adapters.NewPostgreSQLStore(pool)
	supervisor := adapters.NewSupervisor(store, registry, adapters.SupervisorOptions{
		Clock: options.Clock, Process: options.Process, Secrets: options.Secrets,
		Workload: dispatcher, Observer: NewLifecycleAudit(audit.NewWriter(pool), hub),
	})
	coordinator := scheduler.NewCoordinator(store, supervisor, scheduler.CoordinatorOptions{
		ReconcileInterval: options.ReconcileInterval, OnError: options.OnError,
	})
	freshness := NewFreshnessWorker(pool, FreshnessOptions{
		Clock: options.Clock, Publisher: hub, BatchSize: options.FreshnessBatchSize,
		PollInterval: options.FreshnessInterval,
	})
	incidentEvaluator := incidents.NewEvaluator(pool, hub, incidents.Options{
		Clock: options.Clock, BatchSize: options.IncidentBatchSize,
		PollInterval: options.IncidentPoll, ClaimLease: options.IncidentClaimLease,
		MaxAttempts: options.IncidentMaxAttempts, Intents: options.IncidentIntents,
		OnError: func(err error) {
			if options.OnError != nil {
				options.OnError("incidents", err)
			}
		},
	})
	suppressionService := suppressions.NewService(pool, hub, nil)
	return &Runtime{coordinator: coordinator, dispatcher: dispatcher, freshness: freshness, incidents: incidentEvaluator,
		suppressions:       suppressions.NewWorker(suppressionService, time.Second),
		suppressionService: suppressionService, hub: hub}
}

func (runtime *Runtime) Hub() *events.Hub                    { return runtime.hub }
func (runtime *Runtime) Suppressions() *suppressions.Service { return runtime.suppressionService }
func (runtime *Runtime) RequestCollection(integrationID string) bool {
	return runtime.dispatcher.Request(integrationID)
}
func (runtime *Runtime) RestartIntegration(integrationID string) {
	runtime.coordinator.Restart(integrationID)
}

func (runtime *Runtime) Run(ctx context.Context) error {
	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	errorsChannel := make(chan error, 4)
	go func() { errorsChannel <- runtime.coordinator.Run(runContext) }()
	go func() { errorsChannel <- runtime.freshness.Run(runContext) }()
	go func() { errorsChannel <- runtime.incidents.Run(runContext) }()
	go func() { errorsChannel <- runtime.suppressions.Run(runContext) }()

	first := <-errorsChannel
	cancel()
	second := <-errorsChannel
	third := <-errorsChannel
	fourth := <-errorsChannel
	if ctx.Err() != nil || errors.Is(first, context.Canceled) &&
		errors.Is(second, context.Canceled) && errors.Is(third, context.Canceled) && errors.Is(fourth, context.Canceled) {
		return ctx.Err()
	}
	if first != nil && !errors.Is(first, context.Canceled) {
		return first
	}
	if second != nil && !errors.Is(second, context.Canceled) {
		return second
	}
	if third != nil && !errors.Is(third, context.Canceled) {
		return third
	}
	return fourth
}
