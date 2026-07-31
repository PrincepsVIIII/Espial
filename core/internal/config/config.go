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
}

type Server struct {
	ListenAddress     string
	PublicURL         *url.URL
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
}

type Database struct {
	DSNFile            string
	MaxOpenConnections int32
	ConnectTimeout     time.Duration
	MigrationTimeout   time.Duration
}

type Auth struct {
	Mode string
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
		},
		Database: Database{
			MaxOpenConnections: 20,
			ConnectTimeout:     5 * time.Second,
			MigrationTimeout:   2 * time.Minute,
		},
		Auth: Auth{Mode: "local"},
	}
}

type fileConfig struct {
	Environment string `json:"environment"`
	Server      struct {
		ListenAddress     string `json:"listen_address"`
		PublicURL         string `json:"public_url"`
		ReadHeaderTimeout string `json:"read_header_timeout"`
		ShutdownTimeout   string `json:"shutdown_timeout"`
	} `json:"server"`
	Database struct {
		DSNFile            string `json:"dsn_file"`
		MaxOpenConnections *int32 `json:"max_open_connections"`
		ConnectTimeout     string `json:"connect_timeout"`
		MigrationTimeout   string `json:"migration_timeout"`
	} `json:"database"`
	Auth struct {
		Mode string `json:"mode"`
	} `json:"auth"`
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
	if values.Database.DSNFile != "" {
		cfg.Database.DSNFile = values.Database.DSNFile
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
	return nil
}

// SafeSummary returns useful startup settings without secret values or paths.
func (cfg Config) SafeSummary() map[string]any {
	return map[string]any{
		"environment":                   cfg.Environment,
		"listen_address":                cfg.Server.ListenAddress,
		"public_url":                    cfg.Server.PublicURL.String(),
		"auth_mode":                     cfg.Auth.Mode,
		"database_dsn_configured":       cfg.Database.DSNFile != "",
		"database_max_open_connections": cfg.Database.MaxOpenConnections,
		"database_connect_timeout":      cfg.Database.ConnectTimeout,
		"database_migration_timeout":    cfg.Database.MigrationTimeout,
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
	if value := strings.TrimSpace(getenv("ESPIAL_DATABASE_DSN_FILE")); value != "" {
		cfg.Database.DSNFile = value
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
	if cfg.Database.MaxOpenConnections < 1 || cfg.Database.MaxOpenConnections > 200 {
		return errors.New("database max open connections must be between 1 and 200")
	}
	if cfg.Database.ConnectTimeout <= 0 || cfg.Database.MigrationTimeout <= 0 {
		return errors.New("database timeouts must be positive")
	}
	switch cfg.Auth.Mode {
	case "local", "sso_with_local_fallback", "sso":
	default:
		return errors.New("auth mode must be local, sso_with_local_fallback, or sso")
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
