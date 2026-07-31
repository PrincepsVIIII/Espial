package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	sampleExecutable string
	sampleBuildError error
	buildSampleOnce  sync.Once
)

type fixedAdapterClock struct{ now time.Time }

func (clock fixedAdapterClock) Now() time.Time { return clock.now }

func TestSampleAdapterHappyPathForEveryScenario(t *testing.T) {
	path := buildSampleAdapter(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	for _, scenario := range []string{"healthy", "warning", "critical"} {
		t.Run(scenario, func(t *testing.T) {
			session, err := StartSession(context.Background(), sampleDescriptor(path), Integration{
				ID: "50000000-0000-4000-8000-000000000001", AdapterID: "org.ubnetdef.espial.sample",
				ConfigNonsecret: map[string]any{"scenario": scenario, "count": 3, "fault_mode": "none"},
			}, nil, fastProcessOptions(now))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = session.Close(context.Background()) })
			collection, batch, err := session.Collect(context.Background(), now)
			if err != nil {
				t.Fatal(err)
			}
			if session.Version != ProtocolV1 || session.Manifest.AdapterID != "org.ubnetdef.espial.sample" ||
				len(collection.Resources) != 3 || len(batch.Observations) != 3 {
				t.Fatalf("session=%s/%s collection=%#v", session.Version, session.Manifest.AdapterID, collection)
			}
			for _, observation := range batch.Observations {
				if string(observation.State) != scenario || !observation.ObservedAt.Equal(now) {
					t.Fatalf("observation = %#v", observation)
				}
			}
			if err := session.Health(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := session.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSampleAdapterProtocolFaultsAreSafeAndTerminal(t *testing.T) {
	path := buildSampleAdapter(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	for _, fault := range []string{
		"malformed", "oversized", "partial", "crash_before_response",
		"terminal_error", "unsolicited", "wrong_operation",
	} {
		t.Run(fault, func(t *testing.T) {
			session, err := StartSession(context.Background(), sampleDescriptor(path), Integration{
				ID: "50000000-0000-4000-8000-000000000001", AdapterID: "org.ubnetdef.espial.sample",
				ConfigNonsecret: map[string]any{"scenario": "healthy", "count": 1, "fault_mode": fault},
			}, nil, fastProcessOptions(now))
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = session.Collect(context.Background(), now)
			if err == nil {
				t.Fatal("faulty collection succeeded")
			}
			var runtimeErrorValue *RuntimeError
			if !errors.As(err, &runtimeErrorValue) || strings.Contains(err.Error(), "sample adapter request failed") {
				t.Fatalf("unsafe/untyped error = %v", err)
			}
			_ = session.Close(context.Background())
		})
	}
}

func TestDuplicateResponseInvalidatesGeneration(t *testing.T) {
	path := buildSampleAdapter(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	session, err := StartSession(context.Background(), sampleDescriptor(path), Integration{
		ID: "50000000-0000-4000-8000-000000000001", AdapterID: "org.ubnetdef.espial.sample",
		ConfigNonsecret: map[string]any{"scenario": "healthy", "count": 1, "fault_mode": "duplicate_response"},
	}, nil, fastProcessOptions(now))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _ = session.Collect(context.Background(), now)
	if err := session.Health(context.Background()); err == nil {
		t.Fatal("generation remained usable after duplicate response")
	}
	_ = session.Close(context.Background())
}

func TestOnlyOneRequestCanBeInFlight(t *testing.T) {
	path := buildSampleAdapter(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	session, err := StartSession(context.Background(), sampleDescriptor(path), Integration{
		ID: "50000000-0000-4000-8000-000000000001", AdapterID: "org.ubnetdef.espial.sample",
		ConfigNonsecret: map[string]any{"scenario": "healthy", "count": 1, "delay_ms": 250, "fault_mode": "none"},
	}, nil, fastProcessOptions(now))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close(context.Background())
	finished := make(chan error, 1)
	go func() {
		_, _, err := session.Collect(context.Background(), now)
		finished <- err
	}()
	waitForPending(t, session.Process)
	err = session.Health(context.Background())
	requireRuntimeCode(t, err, "request_in_flight")
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestDiagnosticsAreBoundedAndLongLinesDropped(t *testing.T) {
	buffer := NewDiagnosticBuffer(64, 16)
	buffer.SetSecrets([]string{"canary"})
	_, _ = buffer.Write([]byte("first canary\n" + strings.Repeat("x", 32) + "\nsecond\nthird\n"))
	value, dropped := buffer.Snapshot()
	if len(value) > 64 || strings.Contains(value, "canary") || dropped == 0 {
		t.Fatalf("diagnostics len=%d dropped=%d value=%q", len(value), dropped, value)
	}
}

func TestStderrFloodIsDrainedWithoutUnboundedRetention(t *testing.T) {
	path := buildSampleAdapter(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	options := fastProcessOptions(now)
	options.DiagnosticBytes = 1024
	session, err := StartSession(context.Background(), sampleDescriptor(path), Integration{
		ID: "50000000-0000-4000-8000-000000000001", AdapterID: "org.ubnetdef.espial.sample",
		ConfigNonsecret: map[string]any{"scenario": "healthy", "count": 1, "fault_mode": "stderr_flood"},
	}, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := session.Collect(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	value, dropped := session.Process.Diagnostics()
	if len(value) > options.DiagnosticBytes || dropped == 0 {
		t.Fatalf("diagnostics len=%d dropped=%d", len(value), dropped)
	}
	_ = session.Close(context.Background())
}

func TestShutdownRefusalEscalatesAndReaps(t *testing.T) {
	path := buildSampleAdapter(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	options := fastProcessOptions(now)
	options.RequestTimeout = 300 * time.Millisecond
	options.ShutdownTimeout = 75 * time.Millisecond
	options.TerminationTimeout = 150 * time.Millisecond
	session, err := StartSession(context.Background(), sampleDescriptor(path), Integration{
		ID: "50000000-0000-4000-8000-000000000001", AdapterID: "org.ubnetdef.espial.sample",
		ConfigNonsecret: map[string]any{"scenario": "healthy", "count": 1, "fault_mode": "refuse_shutdown"},
	}, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.Process.done:
	default:
		t.Fatal("child was not reaped")
	}
}

func TestStartupFailures(t *testing.T) {
	path := buildSampleAdapter(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		fault string
		code  string
	}{
		{"malformed_ready", "invalid_envelope"},
		{"wrong_major", "unsupported_protocol_major"},
		{"no_ready", "startup_timeout"},
	} {
		t.Run(test.fault, func(t *testing.T) {
			descriptor := sampleDescriptor(path)
			descriptor.Arguments = []string{"--startup-fault=" + test.fault}
			options := fastProcessOptions(now)
			options.StartupTimeout = 75 * time.Millisecond
			_, err := StartProcess(context.Background(), descriptor, options)
			requireRuntimeCode(t, err, test.code)
		})
	}
}

func TestCanceledStartupDoesNotLaunchExecutable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := StartProcess(ctx, Descriptor{AdapterID: "valid.adapter", Executable: "/definitely/not/present"}, DefaultProcessOptions())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

func TestDuplicateReadyInvalidatesStartupGeneration(t *testing.T) {
	path := buildSampleAdapter(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	descriptor := sampleDescriptor(path)
	descriptor.Arguments = []string{"--startup-fault=duplicate_ready"}
	process, err := StartProcess(context.Background(), descriptor, fastProcessOptions(now))
	if err == nil {
		_, err = process.Call(context.Background(), OperationManifest, map[string]any{})
		_ = process.Stop(context.Background())
	}
	if err == nil {
		t.Fatal("duplicate ready generation remained usable")
	}
}

func TestSlowCollectionTimesOutAndNewGenerationRecovers(t *testing.T) {
	path := buildSampleAdapter(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	options := fastProcessOptions(now)
	options.RequestTimeout = 75 * time.Millisecond
	session, err := StartSession(context.Background(), sampleDescriptor(path), Integration{
		ID: adapterIntegrationA, AdapterID: "org.ubnetdef.espial.sample",
		ConfigNonsecret: map[string]any{"scenario": "critical", "count": 1, "delay_ms": 500, "fault_mode": "none"},
	}, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = session.Collect(context.Background(), now)
	requireRuntimeCode(t, err, "request_timeout")
	_ = session.Close(context.Background())

	recovered, err := StartSession(context.Background(), sampleDescriptor(path), Integration{
		ID: adapterIntegrationA, AdapterID: "org.ubnetdef.espial.sample",
		ConfigNonsecret: map[string]any{"scenario": "healthy", "count": 1, "fault_mode": "none"},
	}, nil, fastProcessOptions(now))
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close(context.Background())
	_, batch, err := recovered.Collect(context.Background(), now)
	if err != nil || len(batch.Observations) != 1 || batch.Observations[0].State != "healthy" {
		t.Fatalf("recovery batch = %#v err=%v", batch, err)
	}
}

func TestRepeatedStartStopReapsEveryGeneration(t *testing.T) {
	path := buildSampleAdapter(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	for generation := 0; generation < 8; generation++ {
		session, err := StartSession(context.Background(), sampleDescriptor(path), Integration{
			ID: adapterIntegrationA, AdapterID: "org.ubnetdef.espial.sample",
			ConfigNonsecret: map[string]any{"scenario": "healthy", "count": 1, "fault_mode": "none"},
		}, nil, fastProcessOptions(now))
		if err != nil {
			t.Fatalf("generation %d: %v", generation, err)
		}
		if err := session.Close(context.Background()); err != nil {
			t.Fatalf("close generation %d: %v", generation, err)
		}
		select {
		case <-session.Process.done:
		default:
			t.Fatalf("generation %d was not reaped", generation)
		}
	}
}

func TestResolvedSecretTravelsOnlyInRedactedStdinBoundary(t *testing.T) {
	if os.Getenv("ESPIAL_SECRET_FIXTURE") == "1" {
		return
	}
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	descriptor := Descriptor{
		AdapterID: "org.ubnetdef.espial.secret-fixture", Executable: os.Args[0],
		Arguments:   []string{"-test.run=^TestSecretFixtureProcess$"},
		Environment: map[string]string{"ESPIAL_SECRET_FIXTURE": "1"},
	}
	canary := "super-secret-canary-value"
	resolver := SecretResolverFunc(func(context.Context, string) (string, error) { return canary, nil })
	session, err := StartSession(context.Background(), descriptor, Integration{
		ID: adapterIntegrationA, AdapterID: descriptor.AdapterID,
		ConfigNonsecret:  map[string]any{"scenario": "healthy"},
		SecretReferences: map[string]string{"token": "secret://test-token"},
	}, resolver, fastProcessOptions(now))
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, _ := session.Process.Diagnostics()
	if strings.Contains(diagnostics, canary) || !strings.Contains(diagnostics, "[REDACTED]") {
		t.Fatalf("diagnostics were not redacted: %q", diagnostics)
	}
	for _, argument := range descriptor.Arguments {
		if strings.Contains(argument, canary) {
			t.Fatal("secret appeared in process arguments")
		}
	}
	for _, value := range descriptor.Environment {
		if strings.Contains(value, canary) {
			t.Fatal("secret appeared in process environment")
		}
	}
	_ = session.Close(context.Background())
}

func TestSecretFixtureProcess(t *testing.T) {
	if os.Getenv("ESPIAL_SECRET_FIXTURE") != "1" {
		return
	}
	codec := NewCodec(os.Stdin, os.Stdout, MaxLineBytes)
	now := time.Now().UTC()
	if err := codec.Write(Envelope{ProtocolVersion: ProtocolV1, Kind: KindNotification, Operation: OperationReady, SentAt: now, Payload: json.RawMessage(`{}`)}); err != nil {
		os.Exit(2)
	}
	for {
		request, err := codec.Read()
		if err != nil {
			os.Exit(3)
		}
		var payload any = map[string]any{}
		switch request.Operation {
		case OperationManifest:
			manifest := validManifest()
			manifest.AdapterID = "org.ubnetdef.espial.secret-fixture"
			manifest.SecretFields = []string{"token"}
			payload = manifest
		case OperationValidateConfig:
			var wrapper struct {
				Config map[string]any `json:"config"`
			}
			if json.Unmarshal(request.Payload, &wrapper) != nil {
				os.Exit(4)
			}
			token, _ := wrapper.Config["token"].(string)
			_, _ = fmt.Fprintln(os.Stderr, token)
			logPayload, _ := json.Marshal(map[string]any{"message": token})
			if err := codec.Write(Envelope{ProtocolVersion: ProtocolV1, Kind: KindNotification, Operation: OperationLog, SentAt: now, Payload: logPayload}); err != nil {
				os.Exit(5)
			}
			payload = map[string]any{"valid": true}
		case OperationHealth:
			payload = map[string]any{"status": "healthy"}
		case OperationShutdown:
			payload = map[string]any{"stopping": true}
		}
		encoded, _ := json.Marshal(payload)
		if err := codec.Write(Envelope{
			ProtocolVersion: request.ProtocolVersion, Kind: KindResponse, Operation: request.Operation,
			RequestID: request.RequestID, SentAt: now, Payload: encoded,
		}); err != nil {
			os.Exit(6)
		}
		if request.Operation == OperationShutdown {
			return
		}
	}
}

func buildSampleAdapter(t *testing.T) string {
	t.Helper()
	buildSampleOnce.Do(func() {
		directory, err := os.MkdirTemp("", "espial-sample-adapter-")
		if err != nil {
			sampleBuildError = err
			return
		}
		name := "espial-sample-adapter"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		sampleExecutable = filepath.Join(directory, name)
		command := exec.Command("go", "build", "-o", sampleExecutable, "../../cmd/espial-sample-adapter")
		if output, err := command.CombinedOutput(); err != nil {
			sampleBuildError = errors.New(string(output))
		}
	})
	if sampleBuildError != nil {
		t.Fatalf("build sample adapter: %v", sampleBuildError)
	}
	return sampleExecutable
}

func sampleDescriptor(path string) Descriptor {
	return Descriptor{AdapterID: "org.ubnetdef.espial.sample", Executable: path}
}

func fastProcessOptions(now time.Time) ProcessOptions {
	options := DefaultProcessOptions()
	options.Clock = fixedAdapterClock{now: now}
	options.StartupTimeout = time.Second
	options.RequestTimeout = time.Second
	options.ShutdownTimeout = 250 * time.Millisecond
	options.TerminationTimeout = 250 * time.Millisecond
	return options
}

func waitForPending(t *testing.T, process *Process) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		process.mu.Lock()
		pending := process.pending != nil
		process.mu.Unlock()
		if pending {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("request did not become pending")
}
