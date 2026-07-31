package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestApplyEnvironmentOverridesDefaults(t *testing.T) {
	cfg := defaults()
	values := map[string]string{
		"ESPIAL_ENV":                           Test,
		"ESPIAL_LISTEN_ADDRESS":                "127.0.0.1:9090",
		"ESPIAL_PUBLIC_URL":                    "https://espial.test",
		"ESPIAL_READ_HEADER_TIMEOUT":           "3s",
		"ESPIAL_SHUTDOWN_TIMEOUT":              "7s",
		"ESPIAL_SSE_HEARTBEAT":                 "12s",
		"ESPIAL_SSE_MAX_CLIENTS":               "75",
		"ESPIAL_DATABASE_DSN_FILE":             "/run/secrets/test_dsn",
		"ESPIAL_DATABASE_MIGRATE_ON_START":     "false",
		"ESPIAL_DATABASE_MAX_OPEN_CONNECTIONS": "8",
		"ESPIAL_DATABASE_CONNECT_TIMEOUT":      "4s",
		"ESPIAL_DATABASE_MIGRATION_TIMEOUT":    "1m",
		"ESPIAL_AUTH_MODE":                     "local",
		"ESPIAL_SAMPLE_ADAPTER_EXECUTABLE":     "/opt/espial/sample-adapter",
		"ESPIAL_ADAPTER_GLOBAL_CONCURRENCY":    "7",
		"ESPIAL_ADAPTER_RECONCILE_INTERVAL":    "3s",
		"ESPIAL_FRESHNESS_INTERVAL":            "2s",
		"ESPIAL_FRESHNESS_BATCH_SIZE":          "25",
		"ESPIAL_EVENT_REPLAY_SIZE":             "512",
	}

	err := applyEnvironment(&cfg, func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("apply environment: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if cfg.Server.ListenAddress != "127.0.0.1:9090" {
		t.Fatalf("listen address = %q", cfg.Server.ListenAddress)
	}
	if cfg.Server.ShutdownTimeout != 7*time.Second {
		t.Fatalf("shutdown timeout = %s", cfg.Server.ShutdownTimeout)
	}
	if cfg.Server.SSEHeartbeat != 12*time.Second || cfg.Server.SSEMaxClients != 75 {
		t.Fatalf("SSE settings = %s %d", cfg.Server.SSEHeartbeat, cfg.Server.SSEMaxClients)
	}
	if cfg.Database.MaxOpenConnections != 8 {
		t.Fatalf("max open connections = %d", cfg.Database.MaxOpenConnections)
	}
	if cfg.Database.MigrateOnStart {
		t.Fatal("database migration-on-start override was not applied")
	}
	if cfg.Database.ConnectTimeout != 4*time.Second {
		t.Fatalf("connect timeout = %s", cfg.Database.ConnectTimeout)
	}
	if cfg.Adapters.SampleExecutable != "/opt/espial/sample-adapter" {
		t.Fatalf("sample executable = %q", cfg.Adapters.SampleExecutable)
	}
	if cfg.Adapters.GlobalConcurrency != 7 || cfg.Adapters.ReconcileInterval != 3*time.Second ||
		cfg.Adapters.FreshnessInterval != 2*time.Second || cfg.Adapters.FreshnessBatchSize != 25 ||
		cfg.Adapters.EventReplaySize != 512 {
		t.Fatalf("adapter settings = %#v", cfg.Adapters)
	}
}

func TestApplyFileRejectsUnknownKeys(t *testing.T) {
	name := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(name, []byte(`{"unknown": true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := defaults()
	err := applyFile(&cfg, name)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadEnvironmentOverridesFile(t *testing.T) {
	name := filepath.Join(t.TempDir(), "config.json")
	contents := `{
		"environment": "test",
		"server": {"listen_address": "127.0.0.1:7000"},
		"database": {"max_open_connections": 6}
	}`
	if err := os.WriteFile(name, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ESPIAL_CONFIG_FILE", name)
	t.Setenv("ESPIAL_LISTEN_ADDRESS", "127.0.0.1:8000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Server.ListenAddress != "127.0.0.1:8000" {
		t.Fatalf("listen address = %q", cfg.Server.ListenAddress)
	}
	if cfg.Database.MaxOpenConnections != 6 {
		t.Fatalf("max open connections = %d", cfg.Database.MaxOpenConnections)
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("ESPIAL_SHUTDOWN_TIMEOUT", "eventually")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "ESPIAL_SHUTDOWN_TIMEOUT") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsInvalidMigrationBoolean(t *testing.T) {
	cfg := defaults()
	err := applyEnvironment(&cfg, func(key string) string {
		if key == "ESPIAL_DATABASE_MIGRATE_ON_START" {
			return "sometimes"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "ESPIAL_DATABASE_MIGRATE_ON_START") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsInsecureProductionURL(t *testing.T) {
	cfg := defaults()
	cfg.Environment = Production

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "must use https") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsUnavailableSSOMode(t *testing.T) {
	cfg := defaults()
	cfg.Auth.Mode = "sso"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsRelativeSampleExecutable(t *testing.T) {
	cfg := defaults()
	cfg.Adapters.SampleExecutable = "./sample-adapter"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateRejectsUnsafeMonitoringBounds(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"concurrency":   func(cfg *Config) { cfg.Adapters.GlobalConcurrency = 0 },
		"reconcile":     func(cfg *Config) { cfg.Adapters.ReconcileInterval = time.Millisecond },
		"freshness":     func(cfg *Config) { cfg.Adapters.FreshnessInterval = 2 * time.Minute },
		"batch":         func(cfg *Config) { cfg.Adapters.FreshnessBatchSize = 1001 },
		"replay":        func(cfg *Config) { cfg.Adapters.EventReplaySize = 10001 },
		"sse heartbeat": func(cfg *Config) { cfg.Server.SSEHeartbeat = time.Millisecond },
		"sse clients":   func(cfg *Config) { cfg.Server.SSEMaxClients = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := defaults()
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("unsafe monitoring setting was accepted")
			}
		})
	}
}

func TestDatabaseDSNReadsFile(t *testing.T) {
	name := filepath.Join(t.TempDir(), "dsn")
	if err := os.WriteFile(name, []byte("postgres://espial:secret@db/espial\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := defaults()
	cfg.Database.DSNFile = name
	dsn, err := cfg.DatabaseDSN()
	if err != nil {
		t.Fatalf("read DSN: %v", err)
	}
	if dsn != "postgres://espial:secret@db/espial" {
		t.Fatalf("DSN was not trimmed")
	}
}

func TestDatabaseDSNRequiresFile(t *testing.T) {
	cfg := defaults()

	_, err := cfg.DatabaseDSN()
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("error = %v", err)
	}
}

func TestSafeSummaryDoesNotContainDSNPath(t *testing.T) {
	cfg := defaults()
	cfg.Database.DSNFile = "/private/secrets/database_dsn"
	cfg.Adapters.SampleExecutable = "/private/bin/sample-adapter"

	summary := cfg.SafeSummary()
	for key, value := range summary {
		text, _ := value.(string)
		if strings.Contains(key, "dsn_file") || strings.Contains(key, "executable") || strings.Contains(text, "/private/") {
			t.Fatalf("unsafe summary field %q = %v", key, value)
		}
	}
}
