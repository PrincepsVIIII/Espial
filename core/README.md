# Espial Core

This directory is the Go module for the standalone Espial backend. Phase 1 currently
provides the process lifecycle, typed configuration, structured logs, PostgreSQL
connection pool and migrations, graceful shutdown, health API, and temporary local
authentication with server-side sessions and role enforcement.

Current layout:

```text
cmd/espial/       serve, migrate, bootstrap, and version commands
internal/api/     HTTP routing, security boundary, and health/auth handlers
internal/app/     dependency wiring and graceful lifecycle
internal/auth/    credentials, sessions, CSRF, roles, limits, and provider boundary
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
espial admin bootstrap --username NAME
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
| `ESPIAL_AUTH_SESSION_IDLE` | `30m` | Sliding authenticated-session idle timeout |
| `ESPIAL_AUTH_SESSION_ABSOLUTE` | `12h` | Maximum authenticated-session lifetime |
| `ESPIAL_AUTH_FAILURE_LIMIT` | `5` | Failed passwords before timed account lock |
| `ESPIAL_AUTH_LOCKOUT_DURATION` | `15m` | Timed account lock duration |
| `ESPIAL_AUTH_LOGIN_RATE_LIMIT` | `10` | Login attempts allowed per source/window |
| `ESPIAL_AUTH_LOGIN_RATE_WINDOW` | `1m` | Source-address login rate window |

The DSN value is read from a file and is never included in configuration summaries
or logs.

## Health API

- `GET /api/v1/health/live` reports whether the process can serve HTTP.
- `GET /api/v1/health/ready` checks PostgreSQL with a two-second deadline.

Database loss leaves liveness healthy and changes readiness to HTTP 503. Both
responses include a request ID and baseline security headers.

## Authentication API

- `GET /api/v1/auth/capabilities` reports local availability and keeps unfinished
  SSO controls disabled.
- `POST /api/v1/auth/local/login` creates an audited server-side session.
- `GET /api/v1/auth/session` returns the authoritative user, roles, and permissions.
- `POST /api/v1/auth/logout` validates trusted origin and CSRF, then revokes the
  session.

See the [local authentication runbook](../docs/operations/LOCAL_AUTH.md) for
bootstrap and recovery procedures.

## Checks

Run `npm run check` from the repository root. When Go is unavailable on the host,
the command uses the pinned Go container. Set `ESPIAL_TEST_DATABASE_URL` to include
the isolated PostgreSQL migration tests; CI always runs them with the race detector.
