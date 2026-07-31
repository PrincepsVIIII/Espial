# Espial Core

This directory is the Go module for the standalone Espial backend. Phase 1 currently
provides the process lifecycle, typed configuration, structured logs, PostgreSQL
connection pool and migrations, graceful shutdown, and liveness/readiness API.

Current layout:

```text
cmd/espial/       serve, migrate, and version commands
internal/api/     HTTP routing and health handlers
internal/app/     dependency wiring and graceful lifecycle
internal/config/  defaults, JSON file, environment overrides, validation
internal/storage/ PostgreSQL pool and migration runner
migrations/       embedded, forward-only SQL migrations
```

Core owns domain decisions, authorization, auditing, state persistence, adapter
supervision, and live invalidation events. Vendor-specific collection stays outside
this process.

## Commands

```text
espial serve
espial migrate
espial version
```

`serve` verifies PostgreSQL, applies pending migrations under a transaction-scoped
advisory lock, then listens for HTTP traffic. `migrate` applies the same migrations
without starting the server. The process rejects a database schema newer than the
binary.

## Configuration

Defaults may be overridden by an optional JSON file named by `ESPIAL_CONFIG_FILE`,
then by environment variables. Unknown JSON keys are rejected.

| Variable | Default | Purpose |
|---|---|---|
| `ESPIAL_ENV` | `development` | `development`, `test`, or `production` |
| `ESPIAL_LISTEN_ADDRESS` | `127.0.0.1:8080` | Core HTTP listen address |
| `ESPIAL_PUBLIC_URL` | `http://localhost:5173` | Browser-facing URL; HTTPS required in production |
| `ESPIAL_READ_HEADER_TIMEOUT` | `5s` | HTTP header timeout |
| `ESPIAL_SHUTDOWN_TIMEOUT` | `10s` | Graceful drain deadline |
| `ESPIAL_DATABASE_DSN_FILE` | none | Required file containing the PostgreSQL DSN |
| `ESPIAL_DATABASE_MAX_OPEN_CONNECTIONS` | `20` | Pool limit, from 1 through 200 |
| `ESPIAL_DATABASE_CONNECT_TIMEOUT` | `5s` | PostgreSQL connection timeout |
| `ESPIAL_DATABASE_MIGRATION_TIMEOUT` | `2m` | Startup and CLI migration deadline |
| `ESPIAL_AUTH_MODE` | `local` | Auth transition mode; only local behavior is implemented yet |

The DSN value is read from a file and is never included in configuration summaries
or logs.

## Health API

- `GET /api/v1/health/live` reports whether the process can serve HTTP.
- `GET /api/v1/health/ready` checks PostgreSQL with a two-second deadline.

Database loss leaves liveness healthy and changes readiness to HTTP 503. Both
responses include a request ID and baseline security headers.

## Checks

Run `npm run check` from the repository root. When Go is unavailable on the host,
the command uses the pinned Go container. Set `ESPIAL_TEST_DATABASE_URL` to include
the isolated PostgreSQL migration tests; CI always runs them with the race detector.
