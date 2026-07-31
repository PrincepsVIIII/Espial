package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/PrincepsVIIII/Espial/core/internal/app"
	"github.com/PrincepsVIIII/Espial/core/internal/config"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		printUsage(stderr)
		return 2
	}
	if args[0] == "version" {
		fmt.Fprintln(stdout, version)
		return 0
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "load configuration: %v\n", err)
		return 1
	}
	logger := slog.New(slog.NewJSONHandler(stderr, nil))

	switch args[0] {
	case "serve":
		err = app.Serve(ctx, cfg, logger)
	case "migrate":
		err = app.Migrate(ctx, cfg)
	default:
		printUsage(stderr)
		return 2
	}
	if err != nil {
		logger.Error("command failed", "command", args[0], "error", err)
		return 1
	}
	return 0
}

func printUsage(output io.Writer) {
	fmt.Fprintln(output, "usage: espial <serve|migrate|version>")
}
