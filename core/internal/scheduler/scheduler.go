package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
)

type Timer interface {
	Channel() <-chan time.Time
	Reset(time.Duration) bool
	Stop() bool
}

type systemTimer struct{ timer *time.Timer }

func (timer systemTimer) Channel() <-chan time.Time      { return timer.timer.C }
func (timer systemTimer) Reset(delay time.Duration) bool { return timer.timer.Reset(delay) }
func (timer systemTimer) Stop() bool                     { return timer.timer.Stop() }

type Executor interface {
	Collect(context.Context, adapters.Integration, *adapters.Session) error
}

type Options struct {
	GlobalConcurrency int
	Jitter            Jitter
	NewTimer          func(time.Duration) Timer
}

type Stats struct {
	Active        int64
	MaximumActive int64
	Completed     uint64
	Coalesced     uint64
}

type Scheduler struct {
	executor  Executor
	options   Options
	tokens    chan struct{}
	active    atomic.Int64
	maximum   atomic.Int64
	completed atomic.Uint64
	coalesced atomic.Uint64
	triggerMu sync.RWMutex
	triggers  map[string]chan struct{}
}

func New(executor Executor, options Options) *Scheduler {
	if options.GlobalConcurrency <= 0 {
		options.GlobalConcurrency = 4
	}
	if options.Jitter == nil {
		options.Jitter = StableJitter{}
	}
	if options.NewTimer == nil {
		options.NewTimer = func(delay time.Duration) Timer { return systemTimer{timer: time.NewTimer(delay)} }
	}
	return &Scheduler{executor: executor, options: options, tokens: make(chan struct{}, options.GlobalConcurrency), triggers: map[string]chan struct{}{}}
}

// Request schedules one bounded manual collection through the same per-integration
// and global concurrency controls as periodic work. Repeated requests coalesce.
func (scheduler *Scheduler) Request(integrationID string) bool {
	scheduler.triggerMu.RLock()
	trigger := scheduler.triggers[integrationID]
	scheduler.triggerMu.RUnlock()
	if trigger == nil {
		return false
	}
	select {
	case trigger <- struct{}{}:
	default:
		scheduler.coalesced.Add(1)
	}
	return true
}

// Run implements adapters.Workload for one healthy process generation.
func (scheduler *Scheduler) Run(ctx context.Context, integration adapters.Integration, session *adapters.Session) error {
	if integration.Interval <= 0 {
		return &Error{Code: "invalid_collection_interval"}
	}
	sequence := uint64(0)
	trigger := make(chan struct{}, 1)
	scheduler.triggerMu.Lock()
	scheduler.triggers[integration.ID] = trigger
	scheduler.triggerMu.Unlock()
	defer func() {
		scheduler.triggerMu.Lock()
		if scheduler.triggers[integration.ID] == trigger {
			delete(scheduler.triggers, integration.ID)
		}
		scheduler.triggerMu.Unlock()
	}()
	timer := scheduler.options.NewTimer(scheduler.delay(integration, sequence))
	defer timer.Stop()
	var active, pending bool
	done := make(chan error, 1)
	start := func() {
		active = true
		go func() {
			select {
			case scheduler.tokens <- struct{}{}:
			case <-ctx.Done():
				done <- ctx.Err()
				return
			}
			current := scheduler.active.Add(1)
			scheduler.updateMaximum(current)
			err := scheduler.executor.Collect(ctx, integration, session)
			scheduler.active.Add(-1)
			<-scheduler.tokens
			scheduler.completed.Add(1)
			done <- err
		}()
	}
	for {
		select {
		case <-ctx.Done():
			timer.Stop()
			if active {
				<-done
			}
			return ctx.Err()
		case <-timer.Channel():
			sequence++
			timer.Reset(scheduler.delay(integration, sequence))
			if active {
				if !pending {
					pending = true
					scheduler.coalesced.Add(1)
				}
				continue
			}
			start()
		case <-trigger:
			if active {
				if !pending {
					pending = true
					scheduler.coalesced.Add(1)
				}
				continue
			}
			start()
		case err := <-done:
			active = false
			if err != nil {
				return err
			}
			if pending {
				pending = false
				start()
			}
		}
	}
}

func (scheduler *Scheduler) Snapshot() Stats {
	return Stats{
		Active: scheduler.active.Load(), MaximumActive: scheduler.maximum.Load(),
		Completed: scheduler.completed.Load(), Coalesced: scheduler.coalesced.Load(),
	}
}

func (scheduler *Scheduler) delay(integration adapters.Integration, sequence uint64) time.Duration {
	offset := scheduler.options.Jitter.Offset(integration.ID, sequence, integration.Interval)
	if sequence == 0 {
		if offset < 0 {
			return 0
		}
		return offset
	}
	delay := integration.Interval + offset
	if delay <= 0 {
		return time.Nanosecond
	}
	return delay
}

func (scheduler *Scheduler) updateMaximum(current int64) {
	for {
		maximum := scheduler.maximum.Load()
		if current <= maximum || scheduler.maximum.CompareAndSwap(maximum, current) {
			return
		}
	}
}

type Error struct{ Code string }

func (err *Error) Error() string    { return "scheduler failure: " + err.Code }
func (err *Error) SafeCode() string { return err.Code }
