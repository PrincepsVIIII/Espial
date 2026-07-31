package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/PrincepsVIIII/Espial/core/internal/app"
	"github.com/PrincepsVIIII/Espial/core/internal/auth"
	"github.com/PrincepsVIIII/Espial/core/internal/config"
	"golang.org/x/term"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if len(args) != 1 && !(len(args) == 4 && args[0] == "admin" && args[1] == "bootstrap" && args[2] == "--username") {
		printUsage(stderr)
		return 2
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
	case "admin":
		var password string
		password, err = readPassword(os.Stdin, stderr)
		if err == nil {
			var user auth.User
			user, err = app.BootstrapAdmin(ctx, cfg, args[3], password)
			if err == nil {
				fmt.Fprintf(stdout, "bootstrapped local administrator %q\n", user.Username)
			}
		}
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
	fmt.Fprintln(output, "       espial admin bootstrap --username NAME")
}

func readPassword(input io.Reader, output io.Writer) (string, error) {
	if file, ok := input.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		fmt.Fprint(output, "Password: ")
		first, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(output)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		fmt.Fprint(output, "Confirm password: ")
		second, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(output)
		if err != nil {
			return "", fmt.Errorf("confirm password: %w", err)
		}
		if string(first) != string(second) {
			return "", errors.New("passwords do not match")
		}
		return string(first), nil
	}
	scanner := bufio.NewScanner(input)
	values := make([]string, 0, 2)
	for scanner.Scan() && len(values) < 2 {
		values = append(values, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	if len(values) != 2 {
		return "", errors.New("password and confirmation are required on standard input")
	}
	if values[0] != values[1] {
		return "", errors.New("passwords do not match")
	}
	return values[0], nil
}
