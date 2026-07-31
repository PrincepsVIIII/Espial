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

	"github.com/PrincepsVIIII/Espial/core/internal/api"
	"github.com/PrincepsVIIII/Espial/core/internal/auth"
	"github.com/PrincepsVIIII/Espial/core/internal/config"
	"github.com/PrincepsVIIII/Espial/core/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Serve migrates the database and runs the HTTP API until ctx is canceled.
func Serve(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	logger.Info("configuration loaded", "config", cfg.SafeSummary())
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
	authService, err := auth.NewService(pool, authOptions(cfg))
	if err != nil {
		return fmt.Errorf("initialize authentication: %w", err)
	}
	go cleanSessions(ctx, logger, authService)

	listener, err := net.Listen("tcp", cfg.Server.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Server.ListenAddress, err)
	}

	handler := api.New(api.Dependencies{
		Logger: logger, Ready: pool.Ping, Auth: authService, PublicURL: cfg.Server.PublicURL,
		SecureCookies: secureCookies(cfg),
	})
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		IdleTimeout:       60 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	logger.Info("Espial Core ready", "address", listener.Addr().String())
	return runHTTPServer(ctx, server, listener, cfg.Server.ShutdownTimeout)
}

// BootstrapAdmin creates the one-time local administrator account.
func BootstrapAdmin(ctx context.Context, cfg config.Config, username, password string) (auth.User, error) {
	pool, err := openDatabase(ctx, cfg)
	if err != nil {
		return auth.User{}, err
	}
	defer pool.Close()

	migrationContext, cancelMigration := context.WithTimeout(ctx, cfg.Database.MigrationTimeout)
	err = storage.Migrate(migrationContext, pool)
	cancelMigration()
	if err != nil {
		return auth.User{}, fmt.Errorf("migrate database: %w", err)
	}

	service, err := auth.NewService(pool, authOptions(cfg))
	if err != nil {
		return auth.User{}, fmt.Errorf("initialize authentication: %w", err)
	}
	return service.BootstrapAdmin(ctx, username, password, "bootstrap-cli", "")
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
