package adapters

import (
	"context"
	"errors"
	"time"
)

type SupervisorOptions struct {
	Clock        Clock
	NewTimer     func(time.Duration) Timer
	Jitter       func() float64
	HealthyReset time.Duration
	Process      ProcessOptions
	Secrets      SecretResolver
	Workload     Workload
	Observer     LifecycleObserver
}

type Timer interface {
	Channel() <-chan time.Time
	Stop() bool
}

type systemTimer struct{ timer *time.Timer }

func (timer systemTimer) Channel() <-chan time.Time { return timer.timer.C }
func (timer systemTimer) Stop() bool                { return timer.timer.Stop() }

type Supervisor struct {
	store    InstanceStore
	registry *Registry
	options  SupervisorOptions
}

func NewSupervisor(store InstanceStore, registry *Registry, options SupervisorOptions) *Supervisor {
	if options.Clock == nil {
		options.Clock = systemClock{}
	}
	if options.NewTimer == nil {
		options.NewTimer = func(duration time.Duration) Timer {
			return systemTimer{timer: time.NewTimer(duration)}
		}
	}
	if options.Jitter == nil {
		options.Jitter = func() float64 { return 1 }
	}
	if options.HealthyReset <= 0 {
		options.HealthyReset = HealthyReset
	}
	if options.Process.Clock == nil {
		options.Process.Clock = options.Clock
	}
	options.Process = normalizeProcessOptions(options.Process)
	return &Supervisor{store: store, registry: registry, options: options}
}

// Run owns one integration until cancellation or disablement. Unexpected failures
// are persisted before the next restart is attempted.
func (supervisor *Supervisor) Run(ctx context.Context, integrationID string) error {
	for {
		integration, enabled, err := supervisor.store.LoadIntegration(ctx, integrationID)
		if err != nil {
			return err
		}
		if !enabled {
			instance, exists, loadErr := supervisor.store.LoadInstance(ctx, integrationID)
			if loadErr != nil {
				return loadErr
			}
			if exists && instance.State != "stopped" {
				stoppedAt := supervisor.options.Clock.Now()
				if err := supervisor.store.MarkStopped(ctx, integrationID, stoppedAt); err != nil {
					return err
				}
				if supervisor.options.Observer != nil {
					return supervisor.options.Observer.Stopped(ctx, integration, stoppedAt)
				}
			}
			return nil
		}
		descriptor, err := supervisor.registry.Lookup(integration.AdapterID)
		if err != nil {
			return err
		}
		instance, exists, err := supervisor.store.LoadInstance(ctx, integrationID)
		if err != nil {
			return err
		}
		if exists && instance.NextRestartAt != nil {
			if err := supervisor.waitUntil(ctx, *instance.NextRestartAt); err != nil {
				return err
			}
		}
		now := supervisor.options.Clock.Now()
		if err := supervisor.store.MarkStarting(ctx, integrationID, now); err != nil {
			return err
		}
		if supervisor.options.Observer != nil {
			if err := supervisor.options.Observer.Starting(ctx, integration, now); err != nil {
				return err
			}
		}
		session, err := StartSession(ctx, descriptor, integration, supervisor.options.Secrets, supervisor.options.Process)
		if err != nil {
			if ctx.Err() != nil {
				stopContext, cancel := supervisor.persistenceContext()
				stoppedAt := supervisor.options.Clock.Now()
				stopErr := supervisor.store.MarkStopped(stopContext, integrationID, stoppedAt)
				if stopErr == nil && supervisor.options.Observer != nil {
					_ = supervisor.options.Observer.Stopped(stopContext, integration, stoppedAt)
				}
				cancel()
				return ctx.Err()
			}
			failedAt := supervisor.options.Clock.Now()
			failed, markErr := supervisor.store.MarkFailed(ctx, integrationID, runtimeCode(err), failedAt, supervisor.options.Jitter())
			if markErr != nil {
				return markErr
			}
			if supervisor.options.Observer != nil {
				if observeErr := supervisor.options.Observer.Failed(ctx, integration, failed, failedAt); observeErr != nil {
					return observeErr
				}
			}
			continue
		}
		healthyAt := supervisor.options.Clock.Now()
		if err := supervisor.store.MarkHealthy(ctx, integrationID, session.Manifest.AdapterVersion, session.Version, healthyAt); err != nil {
			_ = session.Close(context.Background())
			return err
		}
		healthyInstance, _, err := supervisor.store.LoadInstance(ctx, integrationID)
		if err != nil {
			_ = session.Close(context.Background())
			return err
		}
		recovered := exists && (instance.State == "unhealthy" || instance.ConsecutiveFailures > 0)
		if supervisor.options.Observer != nil {
			if err := supervisor.options.Observer.Healthy(ctx, integration, healthyInstance, recovered, healthyAt); err != nil {
				_ = session.Close(context.Background())
				return err
			}
		}
		workContext, cancelWork := context.WithCancel(ctx)
		var workDone chan error
		if supervisor.options.Workload != nil {
			workDone = make(chan error, 1)
			go func() { workDone <- supervisor.options.Workload.Run(workContext, integration, session) }()
		}
		resetTimer := supervisor.options.NewTimer(supervisor.options.HealthyReset)
		reset := resetTimer.Channel()
		for {
			select {
			case <-ctx.Done():
				resetTimer.Stop()
				cancelWork()
				stopContext, cancel := context.WithTimeout(context.Background(), supervisor.options.Process.ShutdownTimeout+supervisor.options.Process.TerminationTimeout)
				_ = session.Close(stopContext)
				cancel()
				supervisor.drainWorkload(workDone)
				persistenceContext, persistenceCancel := supervisor.persistenceContext()
				err := supervisor.store.MarkStopped(persistenceContext, integrationID, supervisor.options.Clock.Now())
				if err == nil && supervisor.options.Observer != nil {
					err = supervisor.options.Observer.Stopped(persistenceContext, integration, supervisor.options.Clock.Now())
				}
				persistenceCancel()
				if err != nil {
					return err
				}
				return ctx.Err()
			case <-session.Process.done:
				resetTimer.Stop()
				cancelWork()
				supervisor.drainWorkload(workDone)
				failedAt := supervisor.options.Clock.Now()
				failed, err := supervisor.store.MarkFailed(ctx, integrationID, runtimeCode(session.Process.Failure()), failedAt, supervisor.options.Jitter())
				if err != nil {
					return err
				}
				if supervisor.options.Observer != nil {
					if err := supervisor.options.Observer.Failed(ctx, integration, failed, failedAt); err != nil {
						return err
					}
				}
				goto restart
			case err := <-workDone:
				resetTimer.Stop()
				cancelWork()
				if ctx.Err() != nil {
					_ = session.Close(context.Background())
					persistenceContext, persistenceCancel := supervisor.persistenceContext()
					stopErr := supervisor.store.MarkStopped(persistenceContext, integrationID, supervisor.options.Clock.Now())
					if stopErr == nil && supervisor.options.Observer != nil {
						stopErr = supervisor.options.Observer.Stopped(persistenceContext, integration, supervisor.options.Clock.Now())
					}
					persistenceCancel()
					if stopErr != nil {
						return stopErr
					}
					return ctx.Err()
				}
				_ = session.Close(context.Background())
				failedAt := supervisor.options.Clock.Now()
				failed, markErr := supervisor.store.MarkFailed(ctx, integrationID, runtimeCode(err), failedAt, supervisor.options.Jitter())
				if markErr != nil {
					return markErr
				}
				if supervisor.options.Observer != nil {
					if observeErr := supervisor.options.Observer.Failed(ctx, integration, failed, failedAt); observeErr != nil {
						return observeErr
					}
				}
				goto restart
			case <-reset:
				if err := supervisor.store.ResetFailures(ctx, integrationID, supervisor.options.Clock.Now()); err != nil {
					cancelWork()
					_ = session.Close(context.Background())
					supervisor.drainWorkload(workDone)
					return err
				}
				resetTimer.Stop()
				reset = nil
			}
		}
	restart:
	}
}

func (supervisor *Supervisor) drainWorkload(done <-chan error) {
	if done == nil {
		return
	}
	timer := supervisor.options.NewTimer(supervisor.options.Process.RequestTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.Channel():
	}
}

func (supervisor *Supervisor) persistenceContext() (context.Context, context.CancelFunc) {
	timeout := supervisor.options.Process.ShutdownTimeout + supervisor.options.Process.TerminationTimeout
	return context.WithTimeout(context.Background(), timeout)
}

func (supervisor *Supervisor) waitUntil(ctx context.Context, target time.Time) error {
	delay := target.Sub(supervisor.options.Clock.Now())
	if delay <= 0 {
		return nil
	}
	timer := supervisor.options.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.Channel():
		return nil
	}
}

func runtimeCode(err error) string {
	var safe interface{ SafeCode() string }
	if errors.As(err, &safe) && errorCodePattern.MatchString(safe.SafeCode()) {
		return safe.SafeCode()
	}
	var runtime *RuntimeError
	if errors.As(err, &runtime) && errorCodePattern.MatchString(runtime.Code) {
		return runtime.Code
	}
	return "internal_failure"
}
