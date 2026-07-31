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
				return supervisor.store.MarkStopped(ctx, integrationID, supervisor.options.Clock.Now())
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
		session, err := StartSession(ctx, descriptor, integration, supervisor.options.Secrets, supervisor.options.Process)
		if err != nil {
			if ctx.Err() != nil {
				stopContext, cancel := supervisor.persistenceContext()
				_ = supervisor.store.MarkStopped(stopContext, integrationID, supervisor.options.Clock.Now())
				cancel()
				return ctx.Err()
			}
			if _, markErr := supervisor.store.MarkFailed(ctx, integrationID, runtimeCode(err), supervisor.options.Clock.Now(), supervisor.options.Jitter()); markErr != nil {
				return markErr
			}
			continue
		}
		if err := supervisor.store.MarkHealthy(ctx, integrationID, session.Manifest.AdapterVersion, session.Version, supervisor.options.Clock.Now()); err != nil {
			_ = session.Close(context.Background())
			return err
		}
		resetTimer := supervisor.options.NewTimer(supervisor.options.HealthyReset)
		reset := resetTimer.Channel()
		for {
			select {
			case <-ctx.Done():
				resetTimer.Stop()
				stopContext, cancel := context.WithTimeout(context.Background(), supervisor.options.Process.ShutdownTimeout+supervisor.options.Process.TerminationTimeout)
				_ = session.Close(stopContext)
				cancel()
				persistenceContext, persistenceCancel := supervisor.persistenceContext()
				err := supervisor.store.MarkStopped(persistenceContext, integrationID, supervisor.options.Clock.Now())
				persistenceCancel()
				if err != nil {
					return err
				}
				return ctx.Err()
			case <-session.Process.done:
				resetTimer.Stop()
				if _, err := supervisor.store.MarkFailed(ctx, integrationID, runtimeCode(session.Process.Failure()), supervisor.options.Clock.Now(), supervisor.options.Jitter()); err != nil {
					return err
				}
				goto restart
			case <-reset:
				if err := supervisor.store.ResetFailures(ctx, integrationID, supervisor.options.Clock.Now()); err != nil {
					return err
				}
				resetTimer.Stop()
				reset = nil
			}
		}
	restart:
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
	var runtime *RuntimeError
	if errors.As(err, &runtime) && errorCodePattern.MatchString(runtime.Code) {
		return runtime.Code
	}
	return "internal_failure"
}
