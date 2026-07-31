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

	listener, err := net.Listen("tcp", cfg.Server.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Server.ListenAddress, err)
	}

	handler := api.New(logger, pool.Ping)
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
