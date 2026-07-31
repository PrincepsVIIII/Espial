package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
)

type zeroJitter struct{}

func (zeroJitter) Offset(string, uint64, time.Duration) time.Duration { return 0 }

type manualTimer struct{ ticks chan time.Time }

func (timer *manualTimer) Channel() <-chan time.Time { return timer.ticks }
func (timer *manualTimer) Reset(time.Duration) bool  { return true }
func (timer *manualTimer) Stop() bool                { return true }
func (timer *manualTimer) fire()                     { timer.ticks <- time.Now() }

type blockingExecutor struct {
	active  atomic.Int64
	maximum atomic.Int64
	started chan string
	release chan struct{}
}

func (executor *blockingExecutor) Collect(ctx context.Context, integration adapters.Integration, _ *adapters.Session) error {
	current := executor.active.Add(1)
	for {
		maximum := executor.maximum.Load()
		if current <= maximum || executor.maximum.CompareAndSwap(maximum, current) {
			break
		}
	}
	executor.started <- integration.ID
	select {
	case <-executor.release:
	case <-ctx.Done():
	}
	executor.active.Add(-1)
	return nil
}

func TestStableJitterIsDeterministicAndBounded(t *testing.T) {
	jitter := StableJitter{Seed: 42}
	interval := 10 * time.Minute
	startup := jitter.Offset("integration-a", 0, interval)
	if startup != jitter.Offset("integration-a", 0, interval) || startup < 0 || startup > 30*time.Second {
		t.Fatalf("startup offset = %s", startup)
	}
	for sequence := uint64(1); sequence < 100; sequence++ {
		offset := jitter.Offset("integration-a", sequence, interval)
		if offset < -30*time.Second || offset > 30*time.Second {
			t.Fatalf("sequence %d offset = %s", sequence, offset)
		}
	}
}

func TestSchedulerBoundsGlobalConcurrencyAndDrainsCancellation(t *testing.T) {
	executor := &blockingExecutor{started: make(chan string, 8), release: make(chan struct{}, 8)}
	var mu sync.Mutex
	var timers []*manualTimer
	scheduler := New(executor, Options{
		GlobalConcurrency: 2, Jitter: zeroJitter{},
		NewTimer: func(time.Duration) Timer {
			timer := &manualTimer{ticks: make(chan time.Time, 8)}
			mu.Lock()
			timers = append(timers, timer)
			mu.Unlock()
			return timer
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan error, 4)
	for index := 0; index < 4; index++ {
		go func(index int) {
			results <- scheduler.Run(ctx, adapters.Integration{ID: string(rune('a' + index)), Interval: time.Minute}, nil)
		}(index)
	}
	requireEventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(timers) == 4
	})
	mu.Lock()
	for _, timer := range timers {
		timer.fire()
	}
	mu.Unlock()
	requireEventually(t, func() bool { return executor.active.Load() == 2 })
	if executor.maximum.Load() > 2 || scheduler.Snapshot().MaximumActive > 2 {
		t.Fatalf("maximum executor=%d scheduler=%d", executor.maximum.Load(), scheduler.Snapshot().MaximumActive)
	}
	cancel()
	for range 4 {
		select {
		case <-results:
		case <-time.After(2 * time.Second):
			t.Fatal("scheduler did not drain cancellation")
		}
	}
	if executor.active.Load() != 0 || scheduler.Snapshot().Active != 0 {
		t.Fatal("active work remained after cancellation")
	}
}

func TestSchedulerCoalescesTicksWithoutPerIntegrationOverlap(t *testing.T) {
	executor := &blockingExecutor{started: make(chan string, 4), release: make(chan struct{}, 4)}
	timer := &manualTimer{ticks: make(chan time.Time, 8)}
	scheduler := New(executor, Options{GlobalConcurrency: 2, Jitter: zeroJitter{}, NewTimer: func(time.Duration) Timer { return timer }})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- scheduler.Run(ctx, adapters.Integration{ID: "one", Interval: time.Minute}, nil) }()
	timer.fire()
	<-executor.started
	timer.fire()
	timer.fire()
	if executor.active.Load() != 1 {
		t.Fatalf("active = %d", executor.active.Load())
	}
	executor.release <- struct{}{}
	select {
	case <-executor.started:
	case <-time.After(2 * time.Second):
		t.Fatal("coalesced collection did not start")
	}
	if executor.maximum.Load() != 1 || scheduler.Snapshot().Coalesced < 1 {
		t.Fatalf("max=%d stats=%#v", executor.maximum.Load(), scheduler.Snapshot())
	}
	cancel()
	<-result
}

func requireEventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition was not met")
		}
		time.Sleep(time.Millisecond)
	}
}
