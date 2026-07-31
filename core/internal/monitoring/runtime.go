package monitoring

import (
	"context"
	"errors"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/audit"
	"github.com/PrincepsVIIII/Espial/core/internal/events"
	"github.com/PrincepsVIIII/Espial/core/internal/health"
	"github.com/PrincepsVIIII/Espial/core/internal/observations"
	"github.com/PrincepsVIIII/Espial/core/internal/scheduler"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RuntimeOptions struct {
	Clock              health.Clock
	GlobalConcurrency  int
	ReconcileInterval  time.Duration
	FreshnessInterval  time.Duration
	FreshnessBatchSize int
	EventReplaySize    int
	Process            adapters.ProcessOptions
	Secrets            adapters.SecretResolver
	OnError            func(string, error)
}

// Runtime owns every Slice 1.5 background goroutine and its bounded event hub.
type Runtime struct {
	coordinator *scheduler.Coordinator
	freshness   *FreshnessWorker
	hub         *events.Hub
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
	return &Runtime{coordinator: coordinator, freshness: freshness, hub: hub}
}

func (runtime *Runtime) Hub() *events.Hub { return runtime.hub }

func (runtime *Runtime) Run(ctx context.Context) error {
	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	errorsChannel := make(chan error, 2)
	go func() { errorsChannel <- runtime.coordinator.Run(runContext) }()
	go func() { errorsChannel <- runtime.freshness.Run(runContext) }()

	first := <-errorsChannel
	cancel()
	second := <-errorsChannel
	if ctx.Err() != nil || errors.Is(first, context.Canceled) && errors.Is(second, context.Canceled) {
		return ctx.Err()
	}
	if first != nil && !errors.Is(first, context.Canceled) {
		return first
	}
	return second
}
