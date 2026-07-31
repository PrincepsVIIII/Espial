package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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
	if len(args) == 1 && args[0] == "healthcheck" {
		if err := healthcheck(ctx); err != nil {
			fmt.Fprintf(stderr, "healthcheck failed: %v\n", err)
			return 1
		}
		return 0
	}
	admin, adminOK := parseAdminCommand(args)
	if len(args) != 1 && !adminOK {
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
		err = runAdmin(ctx, cfg, admin, os.Stdin, stdout, stderr)
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
	fmt.Fprintln(output, "usage: espial <serve|migrate|version|healthcheck>")
	fmt.Fprintln(output, "       espial admin bootstrap --username NAME")
	fmt.Fprintln(output, "       espial admin user create --username NAME --role ROLE")
	fmt.Fprintln(output, "       espial admin user role --username NAME --role ROLE")
	fmt.Fprintln(output, "       espial admin user password --username NAME")
	fmt.Fprintln(output, "       espial admin user <enable|disable> --username NAME")
}

type adminCommand struct {
	operation string
	username  string
	role      string
}

func parseAdminCommand(args []string) (adminCommand, bool) {
	if len(args) == 4 && args[0] == "admin" && args[1] == "bootstrap" && args[2] == "--username" {
		return adminCommand{operation: "bootstrap", username: args[3]}, true
	}
	if len(args) == 7 && args[0] == "admin" && args[1] == "user" &&
		(args[2] == "create" || args[2] == "role") && args[3] == "--username" && args[5] == "--role" {
		return adminCommand{operation: args[2], username: args[4], role: args[6]}, true
	}
	if len(args) == 5 && args[0] == "admin" && args[1] == "user" &&
		(args[2] == "password" || args[2] == "enable" || args[2] == "disable") && args[3] == "--username" {
		return adminCommand{operation: args[2], username: args[4]}, true
	}
	return adminCommand{}, false
}

func runAdmin(ctx context.Context, cfg config.Config, command adminCommand, input io.Reader, stdout, stderr io.Writer) error {
	switch command.operation {
	case "bootstrap", "create", "password":
		password, err := readPassword(input, stderr)
		if err != nil {
			return err
		}
		switch command.operation {
		case "bootstrap":
			user, err := app.BootstrapAdmin(ctx, cfg, command.username, password)
			if err == nil {
				fmt.Fprintf(stdout, "bootstrapped local administrator %q\n", user.Username)
			}
			return err
		case "create":
			user, err := app.CreateLocalUser(ctx, cfg, command.username, password, command.role)
			if err == nil {
				fmt.Fprintf(stdout, "created local user %q with role %q\n", user.Username, command.role)
			}
			return err
		default:
			err := app.ResetLocalPassword(ctx, cfg, command.username, password)
			if err == nil {
				fmt.Fprintf(stdout, "reset local password for %q and revoked existing sessions\n", auth.NormalizeUsername(command.username))
			}
			return err
		}
	case "role":
		err := app.AssignLocalRole(ctx, cfg, command.username, command.role)
		if err == nil {
			fmt.Fprintf(stdout, "assigned role %q to %q and revoked existing sessions\n", command.role, auth.NormalizeUsername(command.username))
		}
		return err
	case "enable", "disable":
		enabled := command.operation == "enable"
		err := app.SetLocalUserEnabled(ctx, cfg, command.username, enabled)
		if err == nil {
			fmt.Fprintf(stdout, "%sd local user %q and revoked existing sessions\n", command.operation, auth.NormalizeUsername(command.username))
		}
		return err
	default:
		return errors.New("unsupported administrator command")
	}
}

func healthcheck(ctx context.Context) error {
	address := strings.TrimSpace(os.Getenv("ESPIAL_LISTEN_ADDRESS"))
	if address == "" {
		address = "127.0.0.1:8080"
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	requestContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, "http://127.0.0.1:"+port+"/api/v1/health/ready", nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness returned HTTP %d", response.StatusCode)
	}
	return nil
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
