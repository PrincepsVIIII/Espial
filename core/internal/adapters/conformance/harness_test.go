package conformance

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
)

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

func TestSampleExecutablePassesConformanceHarness(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "espial-sample-adapter")
	command := exec.Command("go", "build", "-o", executable, "../../../cmd/espial-sample-adapter")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build sample: %v: %s", err, output)
	}
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	options := adapters.DefaultProcessOptions()
	options.Clock = fixedClock{now: now}
	options.StartupTimeout = time.Second
	options.RequestTimeout = time.Second
	options.ShutdownTimeout = 250 * time.Millisecond
	options.TerminationTimeout = 250 * time.Millisecond
	result, err := Run(context.Background(), adapters.Descriptor{
		AdapterID: "org.ubnetdef.espial.sample", Executable: executable,
	}, adapters.Integration{
		ID: "70000000-0000-4000-8000-000000000001", AdapterID: "org.ubnetdef.espial.sample",
		ConfigNonsecret: map[string]any{"scenario": "warning", "count": 2, "fault_mode": "none"},
	}, nil, options, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != adapters.ProtocolV1 || len(result.Batch.Resources) != 2 ||
		len(result.Batch.Observations) != 2 || string(result.Batch.Observations[0].State) != "warning" {
		t.Fatalf("result = %#v", result)
	}
}
