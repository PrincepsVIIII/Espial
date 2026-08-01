// Package app wires Espial Core's process-level dependencies.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/adapters"
	"github.com/PrincepsVIIII/Espial/core/internal/api"
	"github.com/PrincepsVIIII/Espial/core/internal/auth"
	"github.com/PrincepsVIIII/Espial/core/internal/config"
	"github.com/PrincepsVIIII/Espial/core/internal/incidents"
	"github.com/PrincepsVIIII/Espial/core/internal/monitoring"
	"github.com/PrincepsVIIII/Espial/core/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Serve migrates the database and runs the HTTP API until ctx is canceled.
func Serve(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	processContext, cancelProcess := context.WithCancel(ctx)
	defer cancelProcess()
	logger.Info("configuration loaded", "config", cfg.SafeSummary())
	pool, err := openDatabase(processContext, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.Database.MigrateOnStart {
		migrationContext, cancelMigration := context.WithTimeout(processContext, cfg.Database.MigrationTimeout)
		err = storage.Migrate(migrationContext, pool)
		cancelMigration()
		if err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
	}
	authService, err := auth.NewService(pool, authOptions(cfg))
	if err != nil {
		return fmt.Errorf("initialize authentication: %w", err)
	}
	monitoringRuntime, registry, err := adapterRuntime(pool, cfg, logger)
	if err != nil {
		return fmt.Errorf("initialize adapter runtime: %w", err)
	}
	listener, err := net.Listen("tcp", cfg.Server.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Server.ListenAddress, err)
	}

	incidentWorkflow := incidents.NewWorkflow(pool, monitoringRuntime.Hub(), nil)
	handler := api.New(api.Dependencies{
		Logger: logger, Ready: pool.Ping, Auth: authService, PublicURL: cfg.Server.PublicURL,
		SecureCookies: secureCookies(cfg), Monitoring: monitoring.NewReadService(pool),
		Incidents:        incidents.NewReader(pool),
		IncidentWorkflow: incidentWorkflow,
		IncidentRules:    incidents.NewRuleService(pool, nil),
		Suppressions:     monitoringRuntime.Suppressions(),
		Integrations:     monitoring.NewIntegrationConfigService(pool, monitoringRuntime.Hub(), nil, registry),
		Users:            authService,
		Events:           monitoringRuntime.Hub(), SSEHeartbeat: cfg.Server.SSEHeartbeat,
		SSEMaxClients: cfg.Server.SSEMaxClients,
	})
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		IdleTimeout:       60 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return processContext
		},
	}

	logger.Info("Espial Core ready", "address", listener.Addr().String())
	runtimeErrors := make(chan error, 1)
	go func() { runtimeErrors <- monitoringRuntime.Run(processContext) }()
	go cleanSessions(processContext, logger, authService)
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- runHTTPServer(processContext, server, listener, cfg.Server.ShutdownTimeout) }()
	select {
	case err := <-serverErrors:
		cancelProcess()
		<-runtimeErrors
		return err
	case err := <-runtimeErrors:
		cancelProcess()
		serverErr := <-serverErrors
		if ctx.Err() != nil {
			return serverErr
		}
		return fmt.Errorf("monitoring runtime: %w", err)
	}
}

func adapterRuntime(pool *pgxpool.Pool, cfg config.Config, logger *slog.Logger) (*monitoring.Runtime, *adapters.Registry, error) {
	descriptors := make([]adapters.Descriptor, 0, 1)
	if cfg.Adapters.SampleExecutable != "" {
		descriptors = append(descriptors, adapters.Descriptor{
			AdapterID:  "org.ubnetdef.espial.sample",
			Executable: cfg.Adapters.SampleExecutable,
		})
	}
	registry, err := adapters.NewRegistry(descriptors...)
	if err != nil {
		return nil, nil, err
	}
	return monitoring.NewRuntime(pool, registry, monitoring.RuntimeOptions{
		GlobalConcurrency:  cfg.Adapters.GlobalConcurrency,
		ReconcileInterval:  cfg.Adapters.ReconcileInterval,
		FreshnessInterval:  cfg.Adapters.FreshnessInterval,
		FreshnessBatchSize: cfg.Adapters.FreshnessBatchSize,
		EventReplaySize:    cfg.Adapters.EventReplaySize,
		OnError: func(integrationID string, err error) {
			logger.Error("integration supervisor exited", "integration_id", integrationID, "error", err)
		},
	}), registry, nil
}

// BootstrapAdmin creates the one-time local administrator account.
func BootstrapAdmin(ctx context.Context, cfg config.Config, username, password string) (auth.User, error) {
	service, closeService, err := localAuthService(ctx, cfg)
	if err != nil {
		return auth.User{}, err
	}
	defer closeService()
	return service.BootstrapAdmin(ctx, username, password, "bootstrap-cli", "")
}

// CreateLocalUser creates a supported host-administered local account.
func CreateLocalUser(ctx context.Context, cfg config.Config, username, password, role string) (auth.User, error) {
	service, closeService, err := localAuthService(ctx, cfg)
	if err != nil {
		return auth.User{}, err
	}
	defer closeService()
	return service.CreateLocalUser(ctx, username, password, role, "admin-cli")
}

// AssignLocalRole replaces a user's role and immediately revokes their sessions.
func AssignLocalRole(ctx context.Context, cfg config.Config, username, role string) error {
	service, closeService, err := localAuthService(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeService()
	return service.AssignRole(ctx, username, role, "admin-cli")
}

// ResetLocalPassword replaces a credential and immediately revokes sessions.
func ResetLocalPassword(ctx context.Context, cfg config.Config, username, password string) error {
	service, closeService, err := localAuthService(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeService()
	return service.ResetLocalPassword(ctx, username, password, "admin-cli")
}

// SetLocalUserEnabled changes account availability and immediately revokes sessions.
func SetLocalUserEnabled(ctx context.Context, cfg config.Config, username string, enabled bool) error {
	service, closeService, err := localAuthService(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeService()
	return service.SetLocalUserEnabled(ctx, username, enabled, "admin-cli")
}

func localAuthService(ctx context.Context, cfg config.Config) (*auth.Service, func(), error) {
	pool, err := openDatabase(ctx, cfg)
	if err != nil {
		return nil, func() {}, err
	}

	migrationContext, cancelMigration := context.WithTimeout(ctx, cfg.Database.MigrationTimeout)
	err = storage.Migrate(migrationContext, pool)
	cancelMigration()
	if err != nil {
		pool.Close()
		return nil, func() {}, fmt.Errorf("migrate database: %w", err)
	}

	service, err := auth.NewService(pool, authOptions(cfg))
	if err != nil {
		pool.Close()
		return nil, func() {}, fmt.Errorf("initialize authentication: %w", err)
	}
	return service, pool.Close, nil
}

// Migrate applies pending database migrations and exits.
func Migrate(ctx context.Context, cfg config.Config) error {
	pool, err := openDatabase(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	migrationContext, cancelMigration := context.WithTimeout(ctx, cfg.Database.MigrationTimeout)
	err = storage.Migrate(migrationContext, pool)
	cancelMigration()
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return nil
}

func openDatabase(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	dsn, err := cfg.DatabaseDSN()
	if err != nil {
		return nil, err
	}
	pool, err := storage.Open(ctx, dsn, cfg.Database.MaxOpenConnections, cfg.Database.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func authOptions(cfg config.Config) auth.Options {
	options := auth.DefaultOptions()
	options.SessionIdle = cfg.Auth.SessionIdle
	options.SessionAbsolute = cfg.Auth.SessionAbsolute
	options.FailureLimit = cfg.Auth.FailureLimit
	options.LockoutDuration = cfg.Auth.LockoutDuration
	options.LoginLimiter = auth.NewLoginLimiter(cfg.Auth.LoginRateLimit, cfg.Auth.LoginRateWindow)
	return options
}

func secureCookies(cfg config.Config) bool {
	if cfg.Environment != config.Development {
		return true
	}
	host := cfg.Server.PublicURL.Hostname()
	return host != "localhost" && !net.ParseIP(host).IsLoopback()
}

func cleanSessions(ctx context.Context, logger *slog.Logger, service *auth.Service) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := service.DeleteExpiredSessions(ctx, now.UTC()); err != nil {
				logger.ErrorContext(ctx, "session cleanup failed", "error", err)
			}
		}
	}
}

func runHTTPServer(ctx context.Context, server *http.Server, listener net.Listener, shutdownTimeout time.Duration) error {
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Serve(listener)
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			_ = server.Close()
			return fmt.Errorf("shut down HTTP server: %w", err)
		}

		err := <-serverErrors
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
