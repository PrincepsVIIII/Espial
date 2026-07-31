// Package config loads and validates Espial Core configuration.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	Development = "development"
	Test        = "test"
	Production  = "production"
)

// Config contains the small set of settings needed by the Phase 1 Core.
type Config struct {
	Environment string
	Server      Server
	Database    Database
	Auth        Auth
	Adapters    Adapters
}

type Server struct {
	ListenAddress     string
	PublicURL         *url.URL
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
	SSEHeartbeat      time.Duration
	SSEMaxClients     int
}

type Database struct {
	DSNFile            string
	MigrateOnStart     bool
	MaxOpenConnections int32
	ConnectTimeout     time.Duration
	MigrationTimeout   time.Duration
}

type Auth struct {
	Mode            string
	SessionIdle     time.Duration
	SessionAbsolute time.Duration
	FailureLimit    int
	LockoutDuration time.Duration
	LoginRateLimit  int
	LoginRateWindow time.Duration
}

type Adapters struct {
	SampleExecutable   string
	GlobalConcurrency  int
	ReconcileInterval  time.Duration
	FreshnessInterval  time.Duration
	FreshnessBatchSize int
	EventReplaySize    int
}

// Load applies defaults, an optional JSON file, then environment overrides.
func Load() (Config, error) {
	cfg := defaults()

	if file := strings.TrimSpace(os.Getenv("ESPIAL_CONFIG_FILE")); file != "" {
		if err := applyFile(&cfg, file); err != nil {
			return Config{}, err
		}
	}

	if err := applyEnvironment(&cfg, os.Getenv); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func defaults() Config {
	publicURL, _ := url.Parse("http://localhost:5173")
	return Config{
		Environment: Development,
		Server: Server{
			ListenAddress:     "127.0.0.1:8080",
			PublicURL:         publicURL,
			ReadHeaderTimeout: 5 * time.Second,
			ShutdownTimeout:   10 * time.Second,
			SSEHeartbeat:      15 * time.Second,
			SSEMaxClients:     100,
		},
		Database: Database{
			MigrateOnStart:     true,
			MaxOpenConnections: 20,
			ConnectTimeout:     5 * time.Second,
			MigrationTimeout:   2 * time.Minute,
		},
		Auth: Auth{
			Mode: "local", SessionIdle: 30 * time.Minute, SessionAbsolute: 12 * time.Hour,
			FailureLimit: 5, LockoutDuration: 15 * time.Minute,
			LoginRateLimit: 10, LoginRateWindow: time.Minute,
		},
		Adapters: Adapters{
			GlobalConcurrency: 4, ReconcileInterval: 10 * time.Second,
			FreshnessInterval: time.Second, FreshnessBatchSize: 100,
			EventReplaySize: 1024,
		},
	}
}

type fileConfig struct {
	Environment string `json:"environment"`
	Server      struct {
		ListenAddress     string `json:"listen_address"`
		PublicURL         string `json:"public_url"`
		ReadHeaderTimeout string `json:"read_header_timeout"`
		ShutdownTimeout   string `json:"shutdown_timeout"`
		SSEHeartbeat      string `json:"sse_heartbeat"`
		SSEMaxClients     *int   `json:"sse_max_clients"`
	} `json:"server"`
	Database struct {
		DSNFile            string `json:"dsn_file"`
		MigrateOnStart     *bool  `json:"migrate_on_start"`
		MaxOpenConnections *int32 `json:"max_open_connections"`
		ConnectTimeout     string `json:"connect_timeout"`
		MigrationTimeout   string `json:"migration_timeout"`
	} `json:"database"`
	Auth struct {
		Mode            string `json:"mode"`
		SessionIdle     string `json:"session_idle"`
		SessionAbsolute string `json:"session_absolute"`
		FailureLimit    *int   `json:"failure_limit"`
		LockoutDuration string `json:"lockout_duration"`
		LoginRateLimit  *int   `json:"login_rate_limit"`
		LoginRateWindow string `json:"login_rate_window"`
	} `json:"auth"`
	Adapters struct {
		SampleExecutable   string `json:"sample_executable"`
		GlobalConcurrency  *int   `json:"global_concurrency"`
		ReconcileInterval  string `json:"reconcile_interval"`
		FreshnessInterval  string `json:"freshness_interval"`
		FreshnessBatchSize *int   `json:"freshness_batch_size"`
		EventReplaySize    *int   `json:"event_replay_size"`
	} `json:"adapters"`
}

func applyFile(cfg *Config, name string) error {
	file, err := os.Open(name)
	if err != nil {
		return fmt.Errorf("open config file: %w", err)
	}
	defer file.Close()

	var values fileConfig
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&values); err != nil {
		return fmt.Errorf("decode config file: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("decode config file: expected one JSON object")
	}

	if values.Environment != "" {
		cfg.Environment = values.Environment
	}
	if values.Server.ListenAddress != "" {
		cfg.Server.ListenAddress = values.Server.ListenAddress
	}
	if values.Server.PublicURL != "" {
		parsed, err := url.Parse(values.Server.PublicURL)
		if err != nil {
			return fmt.Errorf("parse server.public_url: %w", err)
		}
		cfg.Server.PublicURL = parsed
	}
	if values.Server.ReadHeaderTimeout != "" {
		duration, err := time.ParseDuration(values.Server.ReadHeaderTimeout)
		if err != nil {
			return fmt.Errorf("parse server.read_header_timeout: %w", err)
		}
		cfg.Server.ReadHeaderTimeout = duration
	}
	if values.Server.ShutdownTimeout != "" {
		duration, err := time.ParseDuration(values.Server.ShutdownTimeout)
		if err != nil {
			return fmt.Errorf("parse server.shutdown_timeout: %w", err)
		}
		cfg.Server.ShutdownTimeout = duration
	}
	if values.Server.SSEHeartbeat != "" {
		duration, err := time.ParseDuration(values.Server.SSEHeartbeat)
		if err != nil {
			return fmt.Errorf("parse server.sse_heartbeat: %w", err)
		}
		cfg.Server.SSEHeartbeat = duration
	}
	if values.Server.SSEMaxClients != nil {
		cfg.Server.SSEMaxClients = *values.Server.SSEMaxClients
	}
	if values.Database.DSNFile != "" {
		cfg.Database.DSNFile = values.Database.DSNFile
	}
	if values.Database.MigrateOnStart != nil {
		cfg.Database.MigrateOnStart = *values.Database.MigrateOnStart
	}
	if values.Database.MaxOpenConnections != nil {
		cfg.Database.MaxOpenConnections = *values.Database.MaxOpenConnections
	}
	if values.Database.ConnectTimeout != "" {
		duration, err := time.ParseDuration(values.Database.ConnectTimeout)
		if err != nil {
			return fmt.Errorf("parse database.connect_timeout: %w", err)
		}
		cfg.Database.ConnectTimeout = duration
	}
	if values.Database.MigrationTimeout != "" {
		duration, err := time.ParseDuration(values.Database.MigrationTimeout)
		if err != nil {
			return fmt.Errorf("parse database.migration_timeout: %w", err)
		}
		cfg.Database.MigrationTimeout = duration
	}
	if values.Auth.Mode != "" {
		cfg.Auth.Mode = values.Auth.Mode
	}
	for name, value := range map[string]string{
		"auth.session_idle": values.Auth.SessionIdle, "auth.session_absolute": values.Auth.SessionAbsolute,
		"auth.lockout_duration": values.Auth.LockoutDuration, "auth.login_rate_window": values.Auth.LoginRateWindow,
	} {
		if value == "" {
			continue
		}
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("parse %s: %w", name, err)
		}
		switch name {
		case "auth.session_idle":
			cfg.Auth.SessionIdle = duration
		case "auth.session_absolute":
			cfg.Auth.SessionAbsolute = duration
		case "auth.lockout_duration":
			cfg.Auth.LockoutDuration = duration
		case "auth.login_rate_window":
			cfg.Auth.LoginRateWindow = duration
		}
	}
	if values.Auth.FailureLimit != nil {
		cfg.Auth.FailureLimit = *values.Auth.FailureLimit
	}
	if values.Auth.LoginRateLimit != nil {
		cfg.Auth.LoginRateLimit = *values.Auth.LoginRateLimit
	}
	if values.Adapters.SampleExecutable != "" {
		cfg.Adapters.SampleExecutable = values.Adapters.SampleExecutable
	}
	if values.Adapters.GlobalConcurrency != nil {
		cfg.Adapters.GlobalConcurrency = *values.Adapters.GlobalConcurrency
	}
	if values.Adapters.FreshnessBatchSize != nil {
		cfg.Adapters.FreshnessBatchSize = *values.Adapters.FreshnessBatchSize
	}
	if values.Adapters.EventReplaySize != nil {
		cfg.Adapters.EventReplaySize = *values.Adapters.EventReplaySize
	}
	for name, value := range map[string]string{
		"adapters.reconcile_interval": values.Adapters.ReconcileInterval,
		"adapters.freshness_interval": values.Adapters.FreshnessInterval,
	} {
		if value == "" {
			continue
		}
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("parse %s: %w", name, err)
		}
		if name == "adapters.reconcile_interval" {
			cfg.Adapters.ReconcileInterval = duration
		} else {
			cfg.Adapters.FreshnessInterval = duration
		}
	}
	return nil
}

// SafeSummary returns useful startup settings without secret values or paths.
func (cfg Config) SafeSummary() map[string]any {
	return map[string]any{
		"environment":                   cfg.Environment,
		"listen_address":                cfg.Server.ListenAddress,
		"public_url":                    cfg.Server.PublicURL.String(),
		"auth_mode":                     cfg.Auth.Mode,
		"auth_session_idle":             cfg.Auth.SessionIdle,
		"auth_session_absolute":         cfg.Auth.SessionAbsolute,
		"auth_lockout_duration":         cfg.Auth.LockoutDuration,
		"database_dsn_configured":       cfg.Database.DSNFile != "",
		"database_migrate_on_start":     cfg.Database.MigrateOnStart,
		"database_max_open_connections": cfg.Database.MaxOpenConnections,
		"database_connect_timeout":      cfg.Database.ConnectTimeout,
		"database_migration_timeout":    cfg.Database.MigrationTimeout,
		"sample_adapter_configured":     cfg.Adapters.SampleExecutable != "",
		"sse_heartbeat":                 cfg.Server.SSEHeartbeat,
		"sse_max_clients":               cfg.Server.SSEMaxClients,
		"adapter_global_concurrency":    cfg.Adapters.GlobalConcurrency,
		"adapter_reconcile_interval":    cfg.Adapters.ReconcileInterval,
		"freshness_interval":            cfg.Adapters.FreshnessInterval,
		"freshness_batch_size":          cfg.Adapters.FreshnessBatchSize,
		"event_replay_size":             cfg.Adapters.EventReplaySize,
	}
}

func applyEnvironment(cfg *Config, getenv func(string) string) error {
	if value := strings.TrimSpace(getenv("ESPIAL_ENV")); value != "" {
		cfg.Environment = value
	}
	if value := strings.TrimSpace(getenv("ESPIAL_LISTEN_ADDRESS")); value != "" {
		cfg.Server.ListenAddress = value
	}
	if value := strings.TrimSpace(getenv("ESPIAL_PUBLIC_URL")); value != "" {
		parsed, err := url.Parse(value)
		if err != nil {
			return fmt.Errorf("parse ESPIAL_PUBLIC_URL: %w", err)
		}
		cfg.Server.PublicURL = parsed
	}
	if value := strings.TrimSpace(getenv("ESPIAL_READ_HEADER_TIMEOUT")); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("parse ESPIAL_READ_HEADER_TIMEOUT: %w", err)
		}
		cfg.Server.ReadHeaderTimeout = duration
	}
	if value := strings.TrimSpace(getenv("ESPIAL_SHUTDOWN_TIMEOUT")); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("parse ESPIAL_SHUTDOWN_TIMEOUT: %w", err)
		}
		cfg.Server.ShutdownTimeout = duration
	}
	if value := strings.TrimSpace(getenv("ESPIAL_SSE_HEARTBEAT")); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("parse ESPIAL_SSE_HEARTBEAT: %w", err)
		}
		cfg.Server.SSEHeartbeat = duration
	}
	if value := strings.TrimSpace(getenv("ESPIAL_SSE_MAX_CLIENTS")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse ESPIAL_SSE_MAX_CLIENTS: %w", err)
		}
		cfg.Server.SSEMaxClients = parsed
	}
	if value := strings.TrimSpace(getenv("ESPIAL_DATABASE_DSN_FILE")); value != "" {
		cfg.Database.DSNFile = value
	}
	if value := strings.TrimSpace(getenv("ESPIAL_DATABASE_MIGRATE_ON_START")); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse ESPIAL_DATABASE_MIGRATE_ON_START: %w", err)
		}
		cfg.Database.MigrateOnStart = parsed
	}
	if value := strings.TrimSpace(getenv("ESPIAL_DATABASE_MAX_OPEN_CONNECTIONS")); value != "" {
		count, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return fmt.Errorf("parse ESPIAL_DATABASE_MAX_OPEN_CONNECTIONS: %w", err)
		}
		cfg.Database.MaxOpenConnections = int32(count)
	}
	if value := strings.TrimSpace(getenv("ESPIAL_DATABASE_CONNECT_TIMEOUT")); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("parse ESPIAL_DATABASE_CONNECT_TIMEOUT: %w", err)
		}
		cfg.Database.ConnectTimeout = duration
	}
	if value := strings.TrimSpace(getenv("ESPIAL_DATABASE_MIGRATION_TIMEOUT")); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("parse ESPIAL_DATABASE_MIGRATION_TIMEOUT: %w", err)
		}
		cfg.Database.MigrationTimeout = duration
	}
	if value := strings.TrimSpace(getenv("ESPIAL_AUTH_MODE")); value != "" {
		cfg.Auth.Mode = value
	}
	if value := strings.TrimSpace(getenv("ESPIAL_SAMPLE_ADAPTER_EXECUTABLE")); value != "" {
		cfg.Adapters.SampleExecutable = value
	}
	for name, destination := range map[string]*time.Duration{
		"ESPIAL_ADAPTER_RECONCILE_INTERVAL": &cfg.Adapters.ReconcileInterval,
		"ESPIAL_FRESHNESS_INTERVAL":         &cfg.Adapters.FreshnessInterval,
	} {
		if value := strings.TrimSpace(getenv(name)); value != "" {
			duration, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("parse %s: %w", name, err)
			}
			*destination = duration
		}
	}
	for name, destination := range map[string]*int{
		"ESPIAL_ADAPTER_GLOBAL_CONCURRENCY": &cfg.Adapters.GlobalConcurrency,
		"ESPIAL_FRESHNESS_BATCH_SIZE":       &cfg.Adapters.FreshnessBatchSize,
		"ESPIAL_EVENT_REPLAY_SIZE":          &cfg.Adapters.EventReplaySize,
	} {
		if value := strings.TrimSpace(getenv(name)); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("parse %s: %w", name, err)
			}
			*destination = parsed
		}
	}
	for name, destination := range map[string]*time.Duration{
		"ESPIAL_AUTH_SESSION_IDLE":      &cfg.Auth.SessionIdle,
		"ESPIAL_AUTH_SESSION_ABSOLUTE":  &cfg.Auth.SessionAbsolute,
		"ESPIAL_AUTH_LOCKOUT_DURATION":  &cfg.Auth.LockoutDuration,
		"ESPIAL_AUTH_LOGIN_RATE_WINDOW": &cfg.Auth.LoginRateWindow,
	} {
		if value := strings.TrimSpace(getenv(name)); value != "" {
			duration, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("parse %s: %w", name, err)
			}
			*destination = duration
		}
	}
	for name, destination := range map[string]*int{
		"ESPIAL_AUTH_FAILURE_LIMIT":    &cfg.Auth.FailureLimit,
		"ESPIAL_AUTH_LOGIN_RATE_LIMIT": &cfg.Auth.LoginRateLimit,
	} {
		if value := strings.TrimSpace(getenv(name)); value != "" {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("parse %s: %w", name, err)
			}
			*destination = parsed
		}
	}
	return nil
}

// Validate rejects ambiguous or unsafe values before the service starts.
func (cfg Config) Validate() error {
	switch cfg.Environment {
	case Development, Test, Production:
	default:
		return fmt.Errorf("environment must be %q, %q, or %q", Development, Test, Production)
	}

	if _, _, err := net.SplitHostPort(cfg.Server.ListenAddress); err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	if cfg.Server.PublicURL == nil || cfg.Server.PublicURL.Host == "" {
		return errors.New("public URL must be absolute")
	}
	if cfg.Server.PublicURL.Scheme != "http" && cfg.Server.PublicURL.Scheme != "https" {
		return errors.New("public URL scheme must be http or https")
	}
	if cfg.Environment == Production && cfg.Server.PublicURL.Scheme != "https" {
		return errors.New("production public URL must use https")
	}
	if cfg.Server.ReadHeaderTimeout <= 0 || cfg.Server.ShutdownTimeout <= 0 {
		return errors.New("server timeouts must be positive")
	}
	if cfg.Server.SSEHeartbeat < time.Second || cfg.Server.SSEHeartbeat > time.Minute ||
		cfg.Server.SSEMaxClients < 1 || cfg.Server.SSEMaxClients > 10000 {
		return errors.New("SSE settings are outside safe bounds")
	}
	if cfg.Database.MaxOpenConnections < 1 || cfg.Database.MaxOpenConnections > 200 {
		return errors.New("database max open connections must be between 1 and 200")
	}
	if cfg.Database.ConnectTimeout <= 0 || cfg.Database.MigrationTimeout <= 0 {
		return errors.New("database timeouts must be positive")
	}
	if cfg.Auth.Mode != "local" {
		return errors.New("only local auth mode is available; SSO is planned but not implemented")
	}
	if cfg.Auth.SessionIdle <= 0 || cfg.Auth.SessionAbsolute <= cfg.Auth.SessionIdle {
		return errors.New("auth session durations must be positive and absolute must exceed idle")
	}
	if cfg.Auth.FailureLimit < 1 || cfg.Auth.LoginRateLimit < 1 || cfg.Auth.LockoutDuration <= 0 || cfg.Auth.LoginRateWindow <= 0 {
		return errors.New("auth limits and durations must be positive")
	}
	if cfg.Adapters.SampleExecutable != "" && !filepath.IsAbs(cfg.Adapters.SampleExecutable) {
		return errors.New("sample adapter executable must be an absolute path")
	}
	if cfg.Adapters.GlobalConcurrency < 1 || cfg.Adapters.GlobalConcurrency > 64 {
		return errors.New("adapter global concurrency must be between 1 and 64")
	}
	if cfg.Adapters.ReconcileInterval < 100*time.Millisecond || cfg.Adapters.ReconcileInterval > 5*time.Minute ||
		cfg.Adapters.FreshnessInterval < 100*time.Millisecond || cfg.Adapters.FreshnessInterval > time.Minute {
		return errors.New("adapter monitoring intervals are outside safe bounds")
	}
	if cfg.Adapters.FreshnessBatchSize < 1 || cfg.Adapters.FreshnessBatchSize > 1000 ||
		cfg.Adapters.EventReplaySize < 1 || cfg.Adapters.EventReplaySize > 10000 {
		return errors.New("adapter monitoring capacities are outside safe bounds")
	}
	return nil
}

// DatabaseDSN reads the database connection string without logging it.
func (cfg Config) DatabaseDSN() (string, error) {
	if strings.TrimSpace(cfg.Database.DSNFile) == "" {
		return "", errors.New("database DSN file is required")
	}
	value, err := os.ReadFile(cfg.Database.DSNFile)
	if err != nil {
		return "", fmt.Errorf("read database DSN file: %w", err)
	}
	dsn := strings.TrimSpace(string(value))
	if dsn == "" {
		return "", errors.New("database DSN file is empty")
	}
	return dsn, nil
}
