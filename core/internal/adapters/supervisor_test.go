package adapters

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memoryInstanceStore struct {
	mu          sync.Mutex
	integration Integration
	enabled     bool
	instance    Instance
	exists      bool
	starts      int
	healthy     int
	failures    int
	resets      int
	stops       int
	healthySeen chan struct{}
	failedSeen  chan struct{}
	resetSeen   chan struct{}
}

type channelTimer struct{ channel <-chan time.Time }

func (timer channelTimer) Channel() <-chan time.Time { return timer.channel }
func (channelTimer) Stop() bool                      { return true }

func (store *memoryInstanceStore) LoadIntegration(context.Context, string) (Integration, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.integration, store.enabled, nil
}

func (store *memoryInstanceStore) LoadInstance(context.Context, string) (Instance, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.instance, store.exists, nil
}

func (store *memoryInstanceStore) MarkStarting(_ context.Context, integrationID string, now time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.starts++
	store.exists = true
	store.instance.IntegrationID = integrationID
	store.instance.State = "starting"
	store.instance.LastStartedAt = timeValue(now)
	return nil
}

func (store *memoryInstanceStore) MarkHealthy(_ context.Context, integrationID, adapterVersion, protocolVersion string, now time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.healthy++
	store.instance.IntegrationID = integrationID
	store.instance.AdapterVersion = adapterVersion
	store.instance.ProtocolVersion = protocolVersion
	store.instance.State = "healthy"
	store.instance.LastHealthyAt = timeValue(now)
	if store.healthy == 1 && store.healthySeen != nil {
		close(store.healthySeen)
	}
	return nil
}

func (store *memoryInstanceStore) MarkFailed(_ context.Context, integrationID, code string, now time.Time, jitter float64) (Instance, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.failures++
	store.instance.IntegrationID = integrationID
	store.instance.State = "unhealthy"
	store.instance.LastErrorCode = code
	store.instance.LastErrorAt = timeValue(now)
	store.instance.ConsecutiveFailures++
	store.instance.NextRestartAt = timeValue(now.Add(RestartDelay(store.instance.ConsecutiveFailures, jitter)))
	if store.failures == 2 && store.failedSeen != nil {
		close(store.failedSeen)
	}
	return store.instance, nil
}

func (store *memoryInstanceStore) ResetFailures(_ context.Context, _ string, _ time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resets++
	store.instance.ConsecutiveFailures = 0
	store.instance.NextRestartAt = nil
	if store.resets == 1 && store.resetSeen != nil {
		close(store.resetSeen)
	}
	return nil
}

func TestSupervisorResetsBackoffOnlyAfterHealthyWindow(t *testing.T) {
	path := buildSampleAdapter(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	store := &memoryInstanceStore{
		integration: Integration{ID: adapterIntegrationA, AdapterID: "org.ubnetdef.espial.sample", ConfigNonsecret: map[string]any{"scenario": "healthy", "count": 1, "fault_mode": "none"}},
		enabled:     true, instance: Instance{ConsecutiveFailures: 3},
		healthySeen: make(chan struct{}), resetSeen: make(chan struct{}),
	}
	registry, err := NewRegistry(sampleDescriptor(path))
	if err != nil {
		t.Fatal(err)
	}
	releaseReset := make(chan time.Time, 1)
	supervisor := NewSupervisor(store, registry, SupervisorOptions{
		Clock: fixedAdapterClock{now: now}, NewTimer: func(time.Duration) Timer { return channelTimer{channel: releaseReset} },
		Process: fastProcessOptions(now), HealthyReset: time.Minute,
	})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- supervisor.Run(ctx, adapterIntegrationA) }()
	select {
	case <-store.healthySeen:
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor did not become healthy")
	}
	store.mu.Lock()
	if store.resets != 0 {
		store.mu.Unlock()
		t.Fatal("failures reset before healthy window")
	}
	store.mu.Unlock()
	releaseReset <- now.Add(time.Minute)
	select {
	case <-store.resetSeen:
	case <-time.After(3 * time.Second):
		t.Fatal("healthy reset did not run")
	}
	cancel()
	<-result
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.instance.ConsecutiveFailures != 0 || store.resets != 1 {
		t.Fatalf("reset state = %#v resets=%d", store.instance, store.resets)
	}
}

func (store *memoryInstanceStore) MarkStopped(_ context.Context, _ string, now time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.stops++
	store.instance.State = "stopped"
	store.instance.LastStoppedAt = timeValue(now)
	return nil
}

func TestSupervisorStartsHealthyProcessAndStopsOnCancellation(t *testing.T) {
	path := buildSampleAdapter(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	store := &memoryInstanceStore{
		integration: Integration{ID: adapterIntegrationA, AdapterID: "org.ubnetdef.espial.sample", ConfigNonsecret: map[string]any{"scenario": "healthy", "count": 1, "fault_mode": "none"}},
		enabled:     true, healthySeen: make(chan struct{}),
	}
	registry, err := NewRegistry(sampleDescriptor(path))
	if err != nil {
		t.Fatal(err)
	}
	never := make(chan time.Time)
	supervisor := NewSupervisor(store, registry, SupervisorOptions{
		Clock: fixedAdapterClock{now: now}, NewTimer: func(time.Duration) Timer { return channelTimer{channel: never} },
		Process: fastProcessOptions(now),
	})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- supervisor.Run(ctx, adapterIntegrationA) }()
	select {
	case <-store.healthySeen:
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor did not become healthy")
	}
	cancel()
	select {
	case err := <-result:
		if err != context.Canceled {
			t.Fatalf("run error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor did not stop")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.starts != 1 || store.healthy != 1 || store.stops != 1 || store.instance.State != "stopped" {
		t.Fatalf("store = %#v", store)
	}
}

func TestSupervisorPersistsFailureBeforeRestart(t *testing.T) {
	path := buildSampleAdapter(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	store := &memoryInstanceStore{
		integration: Integration{ID: adapterIntegrationA, AdapterID: "org.ubnetdef.espial.sample"},
		enabled:     true, failedSeen: make(chan struct{}),
	}
	descriptor := sampleDescriptor(path)
	descriptor.Arguments = []string{"--startup-fault=malformed_ready"}
	registry, err := NewRegistry(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	immediate := func(time.Duration) Timer {
		value := make(chan time.Time, 1)
		value <- now
		return channelTimer{channel: value}
	}
	supervisor := NewSupervisor(store, registry, SupervisorOptions{
		Clock: fixedAdapterClock{now: now}, NewTimer: immediate, Jitter: func() float64 { return 1 },
		Process: fastProcessOptions(now),
	})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- supervisor.Run(ctx, adapterIntegrationA) }()
	select {
	case <-store.failedSeen:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("supervisor did not persist two failures")
	}
	cancel()
	select {
	case <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("failed supervisor did not stop")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.starts < 2 || store.failures < 2 || store.instance.ConsecutiveFailures < 2 ||
		store.instance.LastErrorCode == "" {
		t.Fatalf("restart state = %#v starts=%d failures=%d", store.instance, store.starts, store.failures)
	}
}

func timeValue(value time.Time) *time.Time { return &value }
