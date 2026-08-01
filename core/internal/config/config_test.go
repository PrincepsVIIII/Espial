package config

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestApplyEnvironmentOverridesDefaults(t *testing.T) {
	cfg := defaults()
	values := map[string]string{
		"ESPIAL_ENV":                                    Test,
		"ESPIAL_LISTEN_ADDRESS":                         "127.0.0.1:9090",
		"ESPIAL_PUBLIC_URL":                             "https://espial.test",
		"ESPIAL_READ_HEADER_TIMEOUT":                    "3s",
		"ESPIAL_SHUTDOWN_TIMEOUT":                       "7s",
		"ESPIAL_SSE_HEARTBEAT":                          "12s",
		"ESPIAL_SSE_MAX_CLIENTS":                        "75",
		"ESPIAL_DATABASE_DSN_FILE":                      "/run/secrets/test_dsn",
		"ESPIAL_DATABASE_MIGRATE_ON_START":              "false",
		"ESPIAL_DATABASE_MAX_OPEN_CONNECTIONS":          "8",
		"ESPIAL_DATABASE_CONNECT_TIMEOUT":               "4s",
		"ESPIAL_DATABASE_MIGRATION_TIMEOUT":             "1m",
		"ESPIAL_AUTH_MODE":                              "local",
		"ESPIAL_SAMPLE_ADAPTER_EXECUTABLE":              "/opt/espial/sample-adapter",
		"ESPIAL_ADAPTER_GLOBAL_CONCURRENCY":             "7",
		"ESPIAL_ADAPTER_RECONCILE_INTERVAL":             "3s",
		"ESPIAL_FRESHNESS_INTERVAL":                     "2s",
		"ESPIAL_FRESHNESS_BATCH_SIZE":                   "25",
		"ESPIAL_EVENT_REPLAY_SIZE":                      "512",
		"ESPIAL_INCIDENT_WORKER_CONCURRENCY":            "4",
		"ESPIAL_INCIDENT_CLAIM_BATCH_SIZE":              "75",
		"ESPIAL_INCIDENT_POLL_INTERVAL":                 "750ms",
		"ESPIAL_INCIDENT_CLAIM_LEASE":                   "45s",
		"ESPIAL_INCIDENT_MAX_SIGNAL_ATTEMPTS":           "9",
		"ESPIAL_NOTIFICATION_WORKER_CONCURRENCY":        "3",
		"ESPIAL_NOTIFICATION_MAX_ATTEMPTS":              "5",
		"ESPIAL_NOTIFICATION_REQUEST_TIMEOUT":           "8s",
		"ESPIAL_NOTIFICATION_RESOLVE_TIMEOUT":           "1500ms",
		"ESPIAL_NOTIFICATION_RESPONSE_BODY_LIMIT_BYTES": "2048",
		"ESPIAL_NOTIFICATION_APPROVED_HOSTS":            "chat.example.test,backup.example.test",
		"ESPIAL_NOTIFICATION_APPROVED_CIDRS":            "192.0.2.0/24,2001:db8::/32",
		"ESPIAL_NOTIFICATION_ALLOWED_PORTS":             "443,8443",
		"ESPIAL_NOTIFICATION_SECRET_DIRECTORY":          "/run/notification-secrets",
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
	if cfg.Incidents.WorkerConcurrency != 4 || cfg.Incidents.ClaimBatchSize != 75 ||
		cfg.Incidents.PollInterval != 750*time.Millisecond || cfg.Incidents.ClaimLease != 45*time.Second ||
		cfg.Incidents.MaxSignalAttempts != 9 {
		t.Fatalf("incident settings = %#v", cfg.Incidents)
	}
	if cfg.Notifications.WorkerConcurrency != 3 || cfg.Notifications.MaxAttempts != 5 ||
		cfg.Notifications.RequestTimeout != 8*time.Second || cfg.Notifications.ResolveTimeout != 1500*time.Millisecond ||
		cfg.Notifications.ResponseBodyLimit != 2048 || len(cfg.Notifications.ApprovedHosts) != 2 ||
		len(cfg.Notifications.ApprovedCIDRs) != 2 || len(cfg.Notifications.AllowedPorts) != 2 ||
		cfg.Notifications.SecretDirectory != "/run/notification-secrets" {
		t.Fatalf("notification settings = %#v", cfg.Notifications)
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
		"database": {"max_open_connections": 6},
		"incidents": {"worker_concurrency": 3, "claim_batch_size": 80, "poll_interval": "2s", "claim_lease": "40s", "max_signal_attempts": 10}
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
	if cfg.Incidents.WorkerConcurrency != 3 || cfg.Incidents.ClaimBatchSize != 80 ||
		cfg.Incidents.PollInterval != 2*time.Second || cfg.Incidents.ClaimLease != 40*time.Second ||
		cfg.Incidents.MaxSignalAttempts != 10 {
		t.Fatalf("incident file settings = %#v", cfg.Incidents)
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

func TestValidateRejectsPublicURLCredentials(t *testing.T) {
	cfg := defaults()
	parsed, err := url.Parse("http://operator:secret@espial.test/")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Server.PublicURL = parsed
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "user information") {
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
		"concurrency":              func(cfg *Config) { cfg.Adapters.GlobalConcurrency = 0 },
		"reconcile":                func(cfg *Config) { cfg.Adapters.ReconcileInterval = time.Millisecond },
		"freshness":                func(cfg *Config) { cfg.Adapters.FreshnessInterval = 2 * time.Minute },
		"batch":                    func(cfg *Config) { cfg.Adapters.FreshnessBatchSize = 1001 },
		"replay":                   func(cfg *Config) { cfg.Adapters.EventReplaySize = 10001 },
		"sse heartbeat":            func(cfg *Config) { cfg.Server.SSEHeartbeat = time.Millisecond },
		"sse clients":              func(cfg *Config) { cfg.Server.SSEMaxClients = 0 },
		"notification concurrency": func(cfg *Config) { cfg.Notifications.WorkerConcurrency = 0 },
		"notification attempts":    func(cfg *Config) { cfg.Notifications.MaxAttempts = 7 },
		"notification timeout":     func(cfg *Config) { cfg.Notifications.RequestTimeout = 2 * time.Minute },
		"notification ports":       func(cfg *Config) { cfg.Notifications.AllowedPorts = []int{0} },
		"notification secret path": func(cfg *Config) { cfg.Notifications.SecretDirectory = "relative" },
		"incident concurrency":     func(cfg *Config) { cfg.Incidents.WorkerConcurrency = 0 },
		"incident batch":           func(cfg *Config) { cfg.Incidents.ClaimBatchSize = 1001 },
		"incident poll":            func(cfg *Config) { cfg.Incidents.PollInterval = time.Millisecond },
		"incident lease":           func(cfg *Config) { cfg.Incidents.ClaimLease = 20 * time.Minute },
		"incident attempts":        func(cfg *Config) { cfg.Incidents.MaxSignalAttempts = 33 },
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
	cfg.Notifications.SecretDirectory = "/private/secrets/notifications"
	cfg.Notifications.ApprovedHosts = []string{"sensitive.internal.example"}
	cfg.Notifications.ApprovedCIDRs = []string{"10.0.0.0/8"}

	summary := cfg.SafeSummary()
	for key, value := range summary {
		text, _ := value.(string)
		if strings.Contains(key, "dsn_file") || strings.Contains(key, "executable") || strings.Contains(text, "/private/") ||
			strings.Contains(text, "sensitive.internal.example") || strings.Contains(text, "10.0.0.0/8") {
			t.Fatalf("unsafe summary field %q = %v", key, value)
		}
	}
}
