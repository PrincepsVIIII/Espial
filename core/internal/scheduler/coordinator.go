package scheduler

import (
	"context"
	"sync"
	"time"
)

type IntegrationLister interface {
	ListEnabledIntegrationIDs(context.Context) ([]string, error)
}

type IntegrationRunner interface {
	Run(context.Context, string) error
}

type CoordinatorOptions struct {
	ReconcileInterval time.Duration
	NewTimer          func(time.Duration) Timer
	OnError           func(string, error)
}

type Coordinator struct {
	lister   IntegrationLister
	runner   IntegrationRunner
	options  CoordinatorOptions
	restarts chan string
}

type managedRun struct {
	cancel context.CancelFunc
	done   chan error
}

type runExit struct {
	id  string
	run *managedRun
	err error
}

func NewCoordinator(lister IntegrationLister, runner IntegrationRunner, options CoordinatorOptions) *Coordinator {
	if options.ReconcileInterval <= 0 {
		options.ReconcileInterval = 10 * time.Second
	}
	if options.NewTimer == nil {
		options.NewTimer = func(delay time.Duration) Timer { return systemTimer{timer: time.NewTimer(delay)} }
	}
	return &Coordinator{lister: lister, runner: runner, options: options, restarts: make(chan string, 64)}
}

func (coordinator *Coordinator) Restart(integrationID string) {
	select {
	case coordinator.restarts <- integrationID:
	default:
	}
}

func (coordinator *Coordinator) Run(ctx context.Context) error {
	managed := make(map[string]*managedRun)
	exits := make(chan runExit, 128)
	start := func(id string) {
		runContext, cancel := context.WithCancel(ctx)
		run := &managedRun{cancel: cancel, done: make(chan error, 1)}
		managed[id] = run
		go func() {
			err := coordinator.runner.Run(runContext, id)
			run.done <- err
			select {
			case exits <- runExit{id: id, run: run, err: err}:
			case <-ctx.Done():
			}
		}()
	}
	reconcile := func() error {
		ids, err := coordinator.lister.ListEnabledIntegrationIDs(ctx)
		if err != nil {
			return err
		}
		desired := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			desired[id] = struct{}{}
			if _, exists := managed[id]; !exists {
				start(id)
			}
		}
		for id, run := range managed {
			if _, exists := desired[id]; !exists {
				run.cancel()
			}
		}
		return nil
	}
	if err := reconcile(); err != nil {
		return err
	}
	timer := coordinator.options.NewTimer(coordinator.options.ReconcileInterval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			for _, run := range managed {
				run.cancel()
			}
			coordinator.drain(managed)
			return ctx.Err()
		case exit := <-exits:
			if current, exists := managed[exit.id]; exists && current == exit.run {
				delete(managed, exit.id)
			}
			if exit.err != nil && coordinator.options.OnError != nil && ctx.Err() == nil {
				coordinator.options.OnError(exit.id, exit.err)
			}
		case id := <-coordinator.restarts:
			if run := managed[id]; run != nil {
				run.cancel()
			}
		case <-timer.Channel():
			if err := reconcile(); err != nil {
				for _, run := range managed {
					run.cancel()
				}
				coordinator.drain(managed)
				return err
			}
			timer.Reset(coordinator.options.ReconcileInterval)
		}
	}
}

func (coordinator *Coordinator) drain(managed map[string]*managedRun) {
	var wait sync.WaitGroup
	for _, run := range managed {
		wait.Add(1)
		go func(run *managedRun) {
			defer wait.Done()
			<-run.done
		}(run)
	}
	wait.Wait()
}
