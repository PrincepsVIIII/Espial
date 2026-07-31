package adapters

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRegistryAcceptsOnlyExplicitAbsoluteDescriptors(t *testing.T) {
	descriptor := Descriptor{AdapterID: "org.ubnetdef.espial.sample", Executable: "/opt/espial/sample", Arguments: []string{"serve"}, Environment: map[string]string{"LANG": "C"}}
	registry, err := NewRegistry(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	descriptor.Arguments[0] = "mutated"
	got, err := registry.Lookup("org.ubnetdef.espial.sample")
	if err != nil || got.Arguments[0] != "serve" {
		t.Fatalf("lookup = %#v, %v", got, err)
	}
	_, err = registry.Lookup("unregistered")
	requireRuntimeCode(t, err, "adapter_not_registered")
	for _, invalid := range []Descriptor{
		{AdapterID: "valid.adapter", Executable: "relative"},
		{AdapterID: "Invalid", Executable: "/absolute"},
		{AdapterID: "valid.adapter", Executable: "/absolute", WorkingDirectory: "relative"},
		{AdapterID: "valid.adapter", Executable: "/absolute", Environment: map[string]string{"bad-key": "x"}},
		{AdapterID: "valid.adapter", Executable: "/absolute", Arguments: []string{"bad\x00argument"}},
	} {
		if _, err := NewRegistry(invalid); err == nil {
			t.Fatalf("invalid descriptor accepted: %#v", invalid)
		}
	}
}

func TestResolveConfigMaintainsSecretBoundaryAndRedacts(t *testing.T) {
	manifest := validManifest()
	manifest.SecretFields = []string{"token"}
	resolver := SecretResolverFunc(func(_ context.Context, reference string) (string, error) {
		if reference != "secret://sample" {
			return "", errors.New("wrong reference")
		}
		return "canary-value", nil
	})
	resolved, secrets, err := ResolveConfig(context.Background(), manifest,
		map[string]any{"scenario": "healthy"}, map[string]string{"token": "secret://sample"}, resolver)
	if err != nil || resolved["token"] != "canary-value" || len(secrets) != 1 {
		t.Fatalf("resolved = %#v secrets=%#v err=%v", resolved, secrets, err)
	}
	redacted := Redact("adapter printed canary-value twice canary-value", secrets)
	if strings.Contains(redacted, "canary-value") || strings.Count(redacted, "[REDACTED]") != 2 {
		t.Fatalf("redacted = %q", redacted)
	}

	_, _, err = ResolveConfig(context.Background(), manifest,
		map[string]any{"token": "plain"}, map[string]string{"token": "secret://sample"}, resolver)
	requireRuntimeCode(t, err, "secret_in_nonsecret_config")
	_, _, err = ResolveConfig(context.Background(), manifest, nil, nil, resolver)
	requireRuntimeCode(t, err, "secret_reference_missing")
	_, _, err = ResolveConfig(context.Background(), validManifest(), nil,
		map[string]string{"token": "secret://sample"}, resolver)
	requireRuntimeCode(t, err, "undeclared_secret_reference")
}

func TestRestartDelaySequenceCapAndJitter(t *testing.T) {
	tests := []struct {
		failures int
		jitter   float64
		want     time.Duration
	}{
		{1, 1, time.Second}, {2, 1, 2 * time.Second}, {7, 1, time.Minute},
		{20, 1, time.Minute}, {1, 0, 800 * time.Millisecond}, {7, 2, 72 * time.Second},
	}
	for _, test := range tests {
		if got := RestartDelay(test.failures, test.jitter); got != test.want {
			t.Fatalf("RestartDelay(%d, %v) = %s, want %s", test.failures, test.jitter, got, test.want)
		}
	}
}
