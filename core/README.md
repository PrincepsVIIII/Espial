# Espial Core

This directory is the Go module for the standalone Espial backend. Phase 1 currently
provides the process lifecycle, typed configuration, structured logs, PostgreSQL
connection pool and migrations, graceful shutdown, health API, temporary local
authentication with server-side sessions and role enforcement, and normalized
resource/observation persistence with deterministic current-health evaluation. It
also includes the trusted, supervised adapter runtime, scheduled normalized
collection pipeline, database-safe freshness worker, bounded event hub, and the
standalone deterministic sample adapter used to prove the pipeline.
The authenticated v1 monitoring API now exposes stable read models and bounded
SSE invalidations for non-browser clients and the upcoming operational UI.

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
internal/monitoring/ collection/audit transaction, freshness worker, runtime wiring
internal/observations/ domain validation and transactional ingestion
internal/scheduler/ deterministic jitter, bounded dispatch, enabled reconciliation
internal/events/   bounded post-commit invalidation replay and resync
internal/audit/    append-only redacted operational audit writer
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
| `ESPIAL_ADAPTER_GLOBAL_CONCURRENCY` | `4` | Process-wide collection limit, from 1 through 64 |
| `ESPIAL_ADAPTER_RECONCILE_INTERVAL` | `10s` | Enabled-integration reconciliation interval |
| `ESPIAL_FRESHNESS_INTERVAL` | `1s` | Persisted freshness deadline polling interval |
| `ESPIAL_FRESHNESS_BATCH_SIZE` | `100` | Maximum claimed freshness rows per transaction |
| `ESPIAL_EVENT_REPLAY_SIZE` | `1024` | Bounded in-process invalidation replay capacity |
| `ESPIAL_SSE_HEARTBEAT` | `15s` | Stream heartbeat and session revalidation interval, from 1s through 1m |
| `ESPIAL_SSE_MAX_CLIENTS` | `100` | Maximum concurrent authenticated event streams, from 1 through 10,000 |

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
critical collections plus test-only failure modes. When its immutable executable
path is configured, Core reconciles enabled integrations, starts the supervised
process, and schedules collection. Successful normalized state, collection metadata,
and audit success commit atomically; current-health and collection invalidations are
published only afterward. See the [Slice 1.5 record](../docs/plans/SLICE_1_5_SCHEDULING_PIPELINE.md).

## Authentication API

- `GET /api/v1/auth/capabilities` reports local availability and keeps unfinished
  SSO controls disabled.
- `POST /api/v1/auth/local/login` creates an audited server-side session.
- `GET /api/v1/auth/session` returns the authoritative user, roles, and permissions.
- `POST /api/v1/auth/logout` validates trusted origin and CSRF, then revokes the
  session.

See the [local authentication runbook](../docs/operations/LOCAL_AUTH.md) for
bootstrap and recovery procedures.

## Monitoring API

Authenticated clients can read `/api/v1/overview`, paginated resources and
integrations, and their detail routes. Administrators can read bounded audit
windows at `/api/v1/audit`. Operators and administrators can create a registered
integration and replace its configuration through an `If-Match` guarded update.
`/api/v1/events/stream` carries invalidations only; clients always refetch REST
state after an event or `resync_required`.

The default collection page is 50 items and the maximum is 200. Mutation bodies
are capped at 128 KiB. See the [Slice 1.6 record](../docs/plans/SLICE_1_6_REST_SSE_API.md)
and checked [OpenAPI document](../api/openapi/v1.json) for filters, permissions,
schemas, and example client flow.

## Checks

Run `npm run check` from the repository root. When Go is unavailable on the host,
the command uses the pinned Go container. Set `ESPIAL_TEST_DATABASE_URL` to include
the isolated PostgreSQL migration tests; CI always runs them with the race detector.
