package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memoryLister struct {
	mu  sync.Mutex
	ids []string
}

func (lister *memoryLister) ListEnabledIntegrationIDs(context.Context) ([]string, error) {
	lister.mu.Lock()
	defer lister.mu.Unlock()
	return append([]string(nil), lister.ids...), nil
}

func (lister *memoryLister) set(ids ...string) {
	lister.mu.Lock()
	lister.ids = append([]string(nil), ids...)
	lister.mu.Unlock()
}

type trackingRunner struct {
	started chan string
	stopped chan string
}

func (runner *trackingRunner) Run(ctx context.Context, id string) error {
	runner.started <- id
	<-ctx.Done()
	runner.stopped <- id
	return ctx.Err()
}

func TestCoordinatorReconcilesEnabledSetAndDrains(t *testing.T) {
	lister := &memoryLister{ids: []string{"a"}}
	runner := &trackingRunner{started: make(chan string, 4), stopped: make(chan string, 4)}
	timer := &manualTimer{ticks: make(chan time.Time, 4)}
	coordinator := NewCoordinator(lister, runner, CoordinatorOptions{
		ReconcileInterval: time.Second, NewTimer: func(time.Duration) Timer { return timer },
	})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- coordinator.Run(ctx) }()
	if id := <-runner.started; id != "a" {
		t.Fatalf("first start = %q", id)
	}
	lister.set("b")
	timer.fire()
	if id := <-runner.started; id != "b" {
		t.Fatalf("second start = %q", id)
	}
	if id := <-runner.stopped; id != "a" {
		t.Fatalf("disabled stop = %q", id)
	}
	cancel()
	if err := <-result; err != context.Canceled {
		t.Fatalf("coordinator error = %v", err)
	}
	if id := <-runner.stopped; id != "b" {
		t.Fatalf("shutdown stop = %q", id)
	}
}
