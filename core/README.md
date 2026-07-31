# Espial Core

This directory is the Go module for the standalone Espial backend. Phase 1 currently
provides the process lifecycle, typed configuration, structured logs, PostgreSQL
connection pool and migrations, graceful shutdown, health API, temporary local
authentication with server-side sessions and role enforcement, and normalized
resource/observation persistence with deterministic current-health evaluation. It
also includes the trusted, supervised adapter runtime and standalone deterministic
sample adapter used by the next scheduled-ingestion slice.

Current layout:

```text
cmd/espial/       serve, migrate, bootstrap, and version commands
cmd/espial-sample-adapter/ standalone deterministic adapter executable
internal/adapters/ protocol, registry, process supervision, persistence, conformance
internal/api/     HTTP routing, security boundary, and health/auth handlers
internal/app/     dependency wiring and graceful lifecycle
internal/auth/    credentials, sessions, CSRF, roles, limits, and provider boundary
internal/config/  defaults, JSON file, environment overrides, validation
internal/health/  pure normalized state and freshness evaluation
internal/observations/ domain validation and transactional ingestion
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
| `ESPIAL_SAMPLE_ADAPTER_EXECUTABLE` | none | Absolute trusted path to the bundled sample adapter; never supplied by an integration/API |

The DSN value is read from a file and is never included in configuration summaries
or logs. The adapter executable path is likewise represented only as a configured/
not-configured boolean in safe summaries.

## Health API

- `GET /api/v1/health/live` reports whether the process can serve HTTP.
- `GET /api/v1/health/ready` checks PostgreSQL with a two-second deadline.

Database loss leaves liveness healthy and changes readiness to HTTP 503. Both
responses include a request ID and baseline security headers.

## Normalized observations and health

`internal/observations` accepts transport-independent batches, validates them
before persistence, then commits resource upserts, append-only observations, and
current health in one PostgreSQL transaction. Identical retries are no-ops;
conflicting reuse of a UUID or normalized delivery key rejects the entire batch.

`internal/health` derives state from the newest observation using injected time.
It persists the stale and unknown deadlines needed by the Slice 1.5 freshness
worker. See the [Slice 1.3 record](../docs/plans/SLICE_1_3_NORMALIZED_HEALTH.md) for
freshness semantics and the verification matrix.

## Adapter runtime

The runtime starts only executables supplied through Core's immutable registry. An
integration row selects a registered adapter ID; it cannot provide paths, arguments,
shell text, working directories, or environment values. Runtime configuration and
resolved secrets travel through bounded stdin envelopes and known secrets are
redacted from retained diagnostics.

The bundled `espial-sample-adapter` supports deterministic healthy, warning, and
critical collections plus test-only failure modes. Core wires its descriptor when
`ESPIAL_SAMPLE_ADAPTER_EXECUTABLE` is configured, but Slice 1.5 owns scheduling and
does not start it yet. See the [Slice 1.4 record](../docs/plans/SLICE_1_4_ADAPTER_RUNTIME.md).

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
