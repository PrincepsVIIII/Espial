# Phase 1 implementation plan: monitoring foundation

**Status:** In progress
**Inputs:** [Roadmap](../ROADMAP.md), [backend plan](../BACKEND_PLAN.md),
[frontend/UI plan](../FRONTEND_UI_PLAN.md), and the accepted
[Phase 0 baseline](../phase-0/README.md)

## Implementation progress

Updated 2026-07-31:

| Slice | Status | Evidence |
|---|---|---|
| 1.0 Toolchain and contracts | Implemented | Root npm commands with optional Make aliases, pinned toolchains, 10 schema fixtures, generated TypeScript, documentation link checks, CI, `.env.example` |
| 1.1 Core lifecycle and PostgreSQL | Implemented | `serve`/`migrate`/`version`, typed configuration, JSON logs, health API, graceful shutdown, PostgreSQL pool, three embedded migrations, unit/race/integration tests |
| 1.2 Local authentication | Implemented | One-time administrator bootstrap, Argon2id local credentials, lockout/rate limiting, hashed sessions, CSRF/origin checks, RBAC boundary, audit events, SvelteKit login/protected shell, and race/integration tests |
| Pre-1.3 UI review | Implemented | Corrected the light/green generic shell to a dark UB-blue operational foundation; added visible UBNetDef attribution, contrast checks, anti-template constraints, and a recurring visual review gate |
| Pre-1.3 webpage skeleton | Implemented | Public informational root, Dashboard-first auth return, responsive five-item top navigation, canonical route skeletons, honest domain-unavailable states, Core-unavailable handling, UI contract tests, and viewport review |
| 1.3 Normalized storage and health | Implemented | Domain validation, injectable freshness evaluation, migration 000004, transactional resource/observation ingestion, delivery idempotency, stable row locking, post-commit changes, and PostgreSQL race/integration tests |
| 1.4 Adapter runtime and sample | Implemented | Strict v1 protocol/manifest validation, bounded NDJSON process runtime, trusted registry and secret boundary, durable health/backoff state, graceful escalation/reaping, deterministic fault-capable sample binary, and black-box conformance/race/PostgreSQL tests |
| 1.5 Scheduler, ingestion, audit, events | Implemented | Deterministic bounded scheduling, runtime ownership, atomic collection/audit records, DB-safe freshness, bounded replay/resync hub, and PostgreSQL race/end-to-end tests |
| 1.6–1.8 Monitoring delivery | Planned | Versioned REST/authenticated SSE, operational data UI, and deployment acceptance remain |

The local stack has been exercised against PostgreSQL 17. Liveness remains HTTP
200 during database loss while readiness returns HTTP 503 and recovers after the
database restarts.

## 1. Outcome

Phase 1 will deliver one production-shaped vertical slice:

1. An operator signs in with an audited local account while SSO is unavailable.
2. Core supervises a deterministic sample adapter and schedules collection.
3. Valid normalized resources and observations are persisted in PostgreSQL.
4. Health becomes healthy, warning, critical, stale, or unknown according to explicit
   freshness rules.
5. An authenticated SvelteKit UI shows integration health and current resources.
6. Server-sent events prompt the UI to refresh without making live events a second
   source of truth.
7. Adapter failure, authorization denial, local login, configuration changes, and
   state transitions leave useful structured logs and audit records.

This proves the component boundaries before adding incident automation and vendor
integrations.

## 2. Scope boundaries

### Included

- Go service lifecycle, configuration, logs, health probes, and migrations
- PostgreSQL identity, integration, resource, observation, current-health, session,
  and audit tables
- Local password authentication, sessions, CSRF protection, and Viewer/Operator/
  Administrator role enforcement
- Identity-provider interface and configuration needed to add SSO later
- NDJSON stdio adapter protocol, registry, supervisor, scheduler, and sample adapter
- Freshness evaluation with explicit stale/unknown behavior
- Versioned REST API and authenticated SSE invalidation stream
- Temporary informational public root with no operational data and a top-right
  sign-in action
- SvelteKit shell, tokens, exact five-section top navigation, authentication states,
  Dashboard post-login route, Alerts and Datacenter route states, integration health,
  resource/status table, and live-connection state
- Local deployment stack, test fixtures, CI checks, and contributor commands

### Not included

- A speculative SSO implementation before the SSO team confirms its contract
- Incident state machine, acknowledgement, notifications, certificates, dependency
  resolution, physical inventory, rack/chassis views, or controlled actions
- Production vendor adapters beyond the deterministic sample adapter
- High availability, Kubernetes, a public operational status page, or arbitrary
  plugin install UI

The UI may show disabled navigation labels for later areas only if they are clearly
marked; it must not ship fake counts or imply unimplemented monitoring.

The room-to-rack-to-server-to-drive experience is specified now in the
[physical drill-down contract](../design/PHYSICAL_DRILLDOWN.md), but its inventory,
visualization, and refinement work remains assigned to Phases 5–7.

## 3. Decisions Phase 1 will implement

- Go 1.26 Core module with explicit `internal` package boundaries.
- PostgreSQL with forward-only versioned SQL migrations.
- Supervised NDJSON over stdio as adapter transport v1.
- Server-sent events as small invalidation messages.
- Separate Web and Core processes behind one reverse proxy.
- Local authentication now through the provider-neutral session/role model. SSO is
  added later without replacing authorization or session storage.
- SvelteKit renders initial authenticated state; Core remains authoritative for
  health rules and authorization.
- Web uses the accepted dark UBNetDef design language: Harriman/deep navy anchors
  the top bar and spatial chrome, UB Blue is reserved for selection and action, and
  `UBNetDef Operations` remains visible in product chrome. Green is reserved for
  healthy/success state.
- The unauthenticated root is a temporary factual explanation of Espial with a
  top-right sign-in action. It exposes no monitoring state, resource identity, or
  environment metadata.
- The authenticated shell uses the persistent top-level order `Dashboard`, `Alerts`,
  `Datacenter`, `Hypervisor`, and `Webpages`; the current permanent side rail is
  transitional, not the Slice 1.7 target. Dashboard is the post-login destination.
- A frontend slice is not complete merely because it functions. It must pass the
  anti-template and viewport review in the [design system](../design/DESIGN_SYSTEM.md)
  and record material findings in the [UI review notes](../design/UI_REVIEW.md).

Details live in the [ADR index](../adr/README.md).

## 4. Target repository layout

```text
core/
├── cmd/espial/                 process wiring and subcommands
├── internal/api/               routes, middleware, JSON models, SSE
├── internal/app/               lifecycle and dependency wiring
├── internal/auth/              credentials, providers, sessions, permissions
├── internal/config/            load, defaults, validation, redacted display
├── internal/adapters/          manifests, protocol, process supervisor, registry
├── internal/scheduler/         jittered schedules and bounded dispatch
├── internal/observations/      validation and transactional ingestion
├── internal/health/            current-state and freshness transitions
├── internal/audit/             append-only events
├── internal/storage/           PostgreSQL repositories and transactions
└── migrations/                 embedded numbered SQL migrations

web/
├── src/lib/api/                typed fetch client and API models
├── src/lib/components/         Phase 1 operational components
├── src/lib/server/             Core-facing server helpers only
├── src/lib/stores/             live connection and safe UI state
├── src/routes/(public)/        temporary informational root
├── src/routes/(public)/login/
├── src/routes/(app)/           protected application shell
├── src/routes/(app)/dashboard/
├── src/routes/(app)/alerts/
├── src/routes/(app)/datacenter/
├── src/routes/(app)/hypervisor/
├── src/routes/(app)/webpages/
├── src/routes/(app)/resources/
└── src/routes/(app)/integrations/

adapters/sample/                deterministic protocol reference adapter
api/fixtures/v1/                valid and invalid contract examples
deployments/local/              developer stack
deployments/production-example/ pinned, documented baseline
```

Package names may be refined while implementing, but ownership must remain clear:
API handlers do not execute SQL, repositories do not decide permissions, adapters
do not decide Core health, and Web does not copy domain rules.

## 5. Delivery slices

Each slice ends in a testable state and merges without relying on an unreviewable
big-bang branch.

For any slice that changes Web, “testable” includes visual evidence at large,
laptop, and narrow viewports. Reviewers reject generic AI/SaaS composition—gradients,
glass, oversized rounded card grids, excessive pills, vague copy, and invented
metrics—even when automated functional checks pass. A restrained dark-blue linear
gradient is permitted only in structural navigation chrome and must have a flat
fallback; ambient, multicolor, pastel, and content-surface gradients remain out of
scope. New UI extends the shared dark-blue tokens and UBNetDef lockup; it does not
establish a separate aesthetic.

### Slice 1.0 — Toolchain, contracts, and developer commands

**Build**

- Add root task entry points: `make format`, `make lint`, `make test`, `make check`,
  `make dev`, and `make generate` (or an equally small documented task runner).
- Pin Go and Node versions; commit the frontend lockfile and dependency update policy.
- Add JSON Schema validation plus positive/negative fixtures for every v1 schema.
- Generate or check TypeScript API types from the accepted schemas; generation must
  be deterministic and CI must fail on an uncommitted diff.
- Add pull-request CI jobs for contracts/docs, Core, Web, and the later vertical
  integration suite. Cache only dependency downloads, not generated results.
- Provide `.env.example` with non-secret local values and explicit secret-file
  placeholders.

**Verify**

- A clean checkout has one documented bootstrap path.
- Formatting/check commands are idempotent.
- Invalid schema fixtures fail for the intended reason.
- No CI job needs production credentials or network access to monitored systems.

**Done when:** contributors can run contract checks even before Core or Web is
fully scaffolded.

### Slice 1.1 — Core lifecycle, configuration, and PostgreSQL

**Build**

- Create `cmd/espial` with `serve`, `migrate`, `admin bootstrap`, and `version`
  subcommands. Keep command parsing thin; commands call application services.
- Implement a typed configuration loader with defaults, file/environment precedence,
  unknown-key rejection in production, secret references, and a redacted diagnostic
  view.
- Establish structured JSON logs with timestamp, level, component, request ID,
  integration/resource IDs when relevant, and safe errors.
- Add `/api/v1/health/live` for process liveness and `/api/v1/health/ready` for
  database/migration/readiness state. Neither exposes secrets or requires login.
- Connect PostgreSQL with bounded pools, connection/check timeouts, and context-aware
  repositories.
- Embed and apply migrations under an advisory lock. Refuse startup on a newer
  database schema.
- Implement signal-driven graceful shutdown with one root context and a bounded
  drain deadline.

**Initial migrations**

1. `000001_identity_and_sessions`: users, local credentials, roles, user roles,
   sessions, built-in role definitions.
2. `000002_integrations_and_health`: integrations, adapter instances, resources,
   observations, current health, constraints and indexes.
3. `000003_audit`: append-only audit events and lookup indexes.

Migrations include `created_at`/`updated_at`, foreign-key behavior, unique external
resource identity, timestamp indexes for retention, and comments for non-obvious
constraints.

**Verify**

- Empty database migrates to current; second run is a no-op.
- Two migrators cannot race.
- Newer schema prevents serving; database loss makes readiness fail, not liveness.
- SIGTERM during a blocked operation cancels it and exits within the deadline.
- Configuration tests cover precedence, invalid duration/URL/mode, unknown keys,
  missing secret files, and redaction.

**Done when:** Core can start, report readiness, and stop cleanly against an empty
PostgreSQL instance without yet exposing protected domain routes.

### Slice 1.2 — Temporary local authentication and authorization

This slice is not a development bypass. It is the controlled fallback that remains
useful after SSO arrives.

**Build**

- Implement `local`, `sso_with_local_fallback`, and `sso` configuration modes; only
  `local` is usable until a real SSO provider is implemented. Reject unsupported
  configured modes at startup rather than silently falling back.
- Add `espial admin bootstrap --username NAME`. Read the password interactively or
  from stdin, enforce a minimum policy, hash with a documented Argon2id baseline,
  create the first administrator transactionally, and refuse a second bootstrap.
- Implement credential verification with generic failure responses, per-address
  rate limiting, timed per-identity lockout, and safe audit data.
- Implement provider-neutral sessions: random token, hashed storage, secure cookie,
  idle/absolute expiry, rotation, revocation, and cleanup.
- Add session-bound CSRF tokens and trusted-origin validation for mutations.
- Seed stable Viewer, Operator, and Administrator roles and named permissions.
- Implement middleware for authentication, permission checks, request IDs, secure
  headers, body limits, and audit context.
- Add `POST /auth/local/login`, `POST /auth/logout`, and `GET /auth/session` under
  `/api/v1`.
- Define the SSO provider interface and disabled placeholder that returns a clear
  configuration error—never a mock accepted identity.

**Frontend**

- Build local login with accessible labels, password-manager-friendly fields,
  generic errors, pending state, and safe return-to routing restricted to local
  application paths.
- Protect application layouts based on authoritative session lookup.
- Handle expired/revoked sessions consistently and preserve no sensitive state in
  local storage.
- Hide unauthorized controls for clarity while still relying on Core denial.
- Reserve the SSO button and emergency disclosure layout defined in the wireframe;
  render only controls enabled by server-provided auth capabilities.

**Verify**

- Correct login, wrong password, unknown/disabled/locked user, logout, idle expiry,
  absolute expiry, rotation, role change, and administrator revocation.
- Bootstrap concurrency cannot create two first administrators.
- Cookies have correct flags in production; CSRF and untrusted origins fail.
- Viewer receives `403` on administrator endpoints even when calling Core directly.
- Logs/audit/errors never contain password, raw cookie, CSRF token, or Argon2 input.
- Successful local use emits `auth.local.used` so future production alerting has a
  stable event.

**Done when:** a newly bootstrapped administrator can sign in and reach a protected
placeholder page, and a Viewer cannot cross an administrator boundary.

**Implemented evidence (2026-07-30):**

- `espial admin bootstrap --username NAME` accepts a no-echo terminal password or
  two matching stdin lines and serializes first-administrator creation with a
  PostgreSQL transaction/advisory lock.
- Passwords use Argon2id at the accepted OWASP baseline (19 MiB, two iterations,
  parallelism one), a 15–128 Unicode-character policy, unique salts, bounded PHC
  parsing, and a common-value deny list. Re-benchmark before production rollout.
- Login uses a 10-attempt/minute source-address limiter and a 15-minute account
  lock after five failures. Unknown, wrong, disabled, and locked identities share
  one public error.
- Session and CSRF secrets contain 256 random bits and only SHA-256 digests are
  stored. Idle/absolute defaults are 30 minutes/12 hours; rotation, logout,
  administrative revocation, disabled-user rejection, and periodic cleanup exist.
- Core exposes capabilities, local login, current session, logout, and a minimal
  administrator permission probe under `/api/v1`; mutations enforce exact trusted
  origin and session-bound CSRF validation.
- Web provides a SvelteKit local-login page, safe local return targets, authoritative
  session lookup, protected placeholder overview, and logout without local storage.
- Unit, PostgreSQL integration, and race tests cover credential policy, bootstrap
  concurrency, failure/lockout/success, idle and absolute expiry, rotation, role
  refresh, revocation, disabled accounts, cookie flags, CSRF/origin rejection, and
  direct Viewer denial.

Operational use is documented in the [local authentication runbook](../operations/LOCAL_AUTH.md).

The pre-Slice 1.3 [UI review](../design/UI_REVIEW.md) found that the initial visual
implementation had diverged from the accepted dark UBNetDef direction. The login,
shell, and placeholder overview were corrected before monitoring-domain work began.

### Slice 1.3 — Normalized storage and health evaluation

**Detailed execution plan:** [Slice 1.3 normalized storage and health](SLICE_1_3_NORMALIZED_HEALTH.md)

**Status:** Implemented 2026-07-31. The detailed plan records the accepted
semantics, package boundaries, implementation evidence, and verification gates.

**Build**

- Implement Go representations of v1 resources and observations, keeping transport
  models separate from storage/domain types.
- Validate structural schema at the boundary, then domain invariants: timestamp
  skew, allowed state, expected refresh bounds, external identity, attribute size,
  and batch limits.
- Upsert a resource and append its observations in one transaction; update current
  health only after the full batch validates.
- Preserve source-observed time and Core-received time separately.
- Evaluate current health from the newest valid observation. `stale` is a Core state
  derived after `expected_refresh + grace`; `unknown` applies when no valid state
  exists or the configured unknown threshold is crossed.
- Make time injectable for deterministic boundary tests.
- Publish an internal state-change event only after transaction commit.

**Freshness defaults for the sample slice**

- Ordinary check interval: 5 minutes.
- Stale transition: expected refresh plus 50% grace, bounded to at least 30 seconds.
- Unknown transition: three missed expected refreshes after the last success.
- A new integration with no successful collection is `unknown`, never healthy.

Defaults become validated configuration; per-integration overrides remain bounded.

**Verify**

- Duplicate delivery is idempotent where identifiers match.
- Older late-arriving observations remain history but do not replace newer state.
- Mixed-invalid batch performs no partial writes.
- Healthy becomes stale and then unknown exactly at tested clock boundaries.
- Adapter recovery changes state from unknown/stale using a new observation and
  retains the old history.
- Concurrent updates cannot make current state reference the wrong observation.

**Done when:** repository/domain tests demonstrate trustworthy state without an
adapter process or HTTP handler.

### Slice 1.4 — Adapter registry, supervisor, and sample adapter

**Detailed execution plan:** [Slice 1.4 adapter runtime and sample adapter](SLICE_1_4_ADAPTER_RUNTIME.md)

**Status:** Implemented 2026-07-31. Core constructs the trusted sample registry at
startup when its absolute executable path is configured; the completed Slice 1.5
now owns enabled-integration scheduling and collection ingestion.

**Build**

- Implement manifest parsing and protocol version negotiation from the v1 schemas.
- Store integration registration, non-secret config, secret references, enabled
  state, interval, adapter identity/version, and last process health.
- Implement a process runner with explicit executable allow/configuration boundary,
  minimal environment, context deadlines, bounded stdin/stdout/stderr, and no shell.
- Correlate one response per request ID; reject protocol noise, duplicates,
  unsupported operations, oversized lines, and payloads after a terminal response.
- Add exponential restart backoff with jitter, cap, healthy reset, and shutdown
  escalation. Avoid infinite fast restart loops.
- Capture redacted adapter diagnostics and expose last safe error code/time.
- Build a deterministic sample adapter with configurable healthy/warning/critical
  fixture output, delay, malformed-output mode, crash mode, and recovery behavior.
- Add a conformance harness usable by future student adapters.

**Verify**

- Happy manifest/config/collect/health/shutdown exchange.
- Unknown major and compatible minor negotiation.
- Malformed/oversized stdout, slow response, early exit, stderr flood, duplicate
  response, and shutdown refusal.
- Child processes are reaped and no goroutine/pipe leaks remain after cancellation.
- Secret values are absent from process listing, logs, audit, and API responses.
- Adapter crash changes integration process health, enters bounded restart, and
  never crashes Core. Slice 1.5 verifies the end-to-end freshness worker advances
  dependent resource data to stale/unknown instead of freezing it as healthy.

**Done when:** the sample adapter can be started, collected, stopped, deliberately
broken, and recovered under automated tests.

### Slice 1.5 — Scheduler, ingestion, audit, and live event hub

**Implementation record:** [Slice 1.5 scheduled monitoring pipeline](SLICE_1_5_SCHEDULING_PIPELINE.md)

**Status:** Implemented 2026-07-31. Core now owns enabled-integration scheduling,
atomic normalized collection/audit persistence, freshness transitions, and bounded
post-commit invalidations.

**Build**

- Schedule enabled integrations with deterministic injectable jitter and no
  synchronized startup burst.
- Bound global and per-integration concurrency; skip/coalesce rather than overlap a
  still-running collection unless explicitly supported.
- Feed validated collection batches into the observation service and record success,
  rejection, duration, record counts, and safe failure categories.
- Run a freshness worker from one owned goroutine using database-safe claim/update
  semantics so a future multi-instance deployment will not double-apply transitions.
- Implement append-only audit writes for auth, integration lifecycle/configuration,
  adapter failure/recovery, and administrative reads if policy requires.
- Implement an in-process bounded event hub after transaction commits. Slow SSE
  consumers lose replay and receive `resync_required`; they never block ingestion.

**Verify**

- Jitter stays inside bounds and seeded tests are deterministic.
- Maximum concurrency is never exceeded; cancellation drains workers.
- Slow/crashed adapters do not delay unrelated integrations.
- A failed transaction emits neither audit success nor live resource change.
- Slow live subscribers cannot exhaust memory or block producers.
- Audit records contain actor/system identity, action, target, result, time, source,
  and correlation ID without secrets.

**Done when:** scheduled sample data flows process → validation → PostgreSQL → current
health → internal event, with failures isolated and auditable.

### Slice 1.6 — Versioned REST and SSE API

**Build**

- Implement the Phase 1 endpoints in [API conventions](../architecture/API_CONVENTIONS.md)
  with consistent errors, cursors, limits, request IDs, and permission checks.
- Build `/overview` as a small read model: counts by state, integration health, stale
  count, unknown count, and most recent state changes. Do not invent incident counts.
- Expose resource and integration collections/details with filters for state, kind,
  integration, and stale data; enforce stable cursor ordering.
- Expose administrator audit reads with bounded time filters and redacted summaries.
- Add authenticated SSE with heartbeat, event IDs, bounded replay, `Last-Event-ID`,
  resync behavior, and prompt cleanup on logout/shutdown.
- Generate/check OpenAPI once handlers stabilize, using shared schema references
  rather than copying incompatible models.

**Verify**

- Handler contract tests cover success, validation, `401`, `403`, `404`, `409/412`,
  body/limit violations, and safe `500` behavior.
- Pagination has no missing/duplicate rows during a stable snapshot.
- SSE reconnect replays within the window; an old cursor requests full resync.
- A session revoked while streaming loses the stream within the defined revalidation
  or heartbeat interval.
- API fuzz tests cannot panic decoders or allocate unbounded memory.

**Done when:** a non-browser client can complete the authenticated monitoring flow
using the documented v1 API alone.

### Slice 1.7 — SvelteKit operational shell and primary pages

**Build**

- Scaffold SvelteKit with TypeScript strict mode, linting, formatting, unit tests,
  component tests, and accessibility checks.
- Implement the Phase 0 tokens as CSS custom properties and test status semantics
  with icon plus label, never color alone.
- Build the temporary public root: a concise factual explanation of Espial, no live
  or identifying operational data, and an obvious sign-in action at the top right.
- Preserve the reviewed dark UBNetDef foundation: visible organization attribution,
  deep navy/Harriman structural chrome, restrained UB Blue actions and selection,
  compact divided layouts, and no ambient gradient, glass, glow, oversized card
  grid, or fake operational content.
- Build the application shell, skip link, dark top navigation with `Dashboard`,
  `Alerts`, `Datacenter`, `Hypervisor`, and `Webpages` in that order, user/role
  dropdown, sign-out, responsive behavior, error boundary, and Core-unavailable
  state.
- Route successful login to Dashboard. Until Phases 2 and 5 provide their domain
  data, render honest unavailable/not-configured states at Alerts and Datacenter;
  do not fabricate alerts, a room, or devices.
- Use a typed API client with request IDs, same-origin credentials, safe error
  mapping, and no browser calls to infrastructure sources.
- Load authoritative initial data through SvelteKit. Avoid persisting sessions,
  credentials, operational payloads, or resource data in browser local storage.
- Build Dashboard counts, integration-health list, resource status table, timestamp,
  loading/empty/error states, filters reflected in the URL, and accessible narrow
  layout.
- Add the SSE client after initial load. Show `Live`, `Reconnecting`, `Disconnected`,
  and last successful refresh; use bounded exponential reconnect with jitter.
- Coalesce invalidations and refetch authoritative affected queries. On
  `resync_required`, refresh all visible data.

**Verify**

- The public root explains the product without disclosing live status, hostnames,
  locations, integrations, counts, or other operational metadata.
- Keyboard-only login, navigation, filtering, table inspection, and logout.
- The post-login route is Dashboard; all five primary items remain present and in
  order across routes and narrow screens.
- Real navigation/user dropdowns work by click, keyboard, and touch; focus returns
  predictably and Escape closes an open menu.
- Automated contrast/accessibility checks plus manual screen-reader landmarks and
  status-label review.
- Unknown and stale are distinguishable from healthy in text, icon, and styling.
- 401 routes to login safely; 403 explains lack of permission; Core outage retains
  shell context and never shows old data as fresh.
- SSE disconnect/reconnect and reduced-motion behavior are covered by browser tests.
- Large display, laptop, and narrow viewport snapshots contain the same critical
  information.
- The visual review checklist passes and finds no anti-template pattern; screenshots
  visibly belong to the same UBNetDef product as the login and protected shell.

**Done when:** a visitor can understand Espial and reach sign-in from the public
root, and an operator can identify unhealthy/stale resources and monitoring coverage
within seconds using only the Web UI.

### Slice 1.8 — Deployment, hardening, and phase acceptance

**Build**

- Add a developer Compose stack with PostgreSQL, Core, Web, and sample adapter;
  include health checks and one-command initialization without default passwords.
- Add production-example images/configuration with non-root users, read-only
  filesystems where practical, resource limits, pinned versions, private network,
  persistent database, secret references, and SSE proxy settings.
- Document initial administrator bootstrap, role creation/assignment, local-account
  recovery, database backup/restore, upgrades/migrations, logs, adapter diagnosis,
  and complete shutdown.
- Add dependency, container, and secret scanning appropriate to repository hosting.
- Run a compact threat review covering credential theft, session fixation/CSRF,
  adapter escape/output abuse, SSRF/configuration misuse, secret leakage, privilege
  escalation, audit tampering, and denial of service.
- Measure idle and sample-load resource use and set evidence-based defaults for pools,
  queues, timeouts, log caps, and retention batches.

**Vertical acceptance scenario**

1. Start from an empty database and run migrations.
2. Bootstrap one local administrator without exposing the password.
3. Sign in through Web and confirm an unauthenticated API call is denied.
4. Enable the sample adapter and observe healthy/warning resources in PostgreSQL and
   Web.
5. Stop/crash the adapter; integration health changes and resource data becomes
   stale, then unknown, without Core failure.
6. Recover the adapter; Web updates through SSE-triggered refetch.
7. Assign a Viewer and prove direct administrator API calls return `403`.
8. Review audit records for bootstrap, local login, integration lifecycle, failure,
   recovery, and role change.
9. Terminate the stack during collection and prove bounded graceful shutdown.
10. Restore a database backup into a clean stack and verify current state/auth data.

**Done when:** the acceptance scenario and CI pass from a clean checkout, production
blockers are listed with owners, and operation/recovery docs have been dry-run by
someone other than their author.

## 6. Configuration surface

Keep the initial surface narrow and validated. Representative keys:

```yaml
server:
  listen_address: "127.0.0.1:8080"
  public_url: "https://espial.example.invalid"
  trusted_proxies: []
database:
  dsn_file: "/run/secrets/database_dsn"
  max_open_connections: 20
auth:
  mode: "local"
  session_idle: "30m"
  session_absolute: "12h"
  secure_cookies: true
adapters:
  global_concurrency: 4
  collection_timeout: "30s"
  max_message_bytes: 1048576
health:
  stale_grace_ratio: 0.5
  unknown_after_missed_refreshes: 3
logging:
  level: "info"
  format: "json"
```

The committed example uses invalid/example addresses and file references. Defaults
must not enable a remote listener, insecure production cookie, arbitrary executable,
or known password.

## 7. Test strategy

| Layer | Primary tests | Required failure cases |
|---|---|---|
| Schemas/contracts | Meta-schema validation, golden fixtures, compatibility diff | Unknown required field/type, oversize/bounds, incompatible version |
| Core unit | Domain tables, permissions, freshness clock, jitter/backoff | Boundary times, cancellation, duplicate/older observations |
| Core integration | Real PostgreSQL migrations/repositories/transactions | Rollback, connection loss, concurrent migrator/update |
| Adapter conformance | Child process and golden NDJSON | Crash, hang, flood, malformed, duplicate, unsupported version |
| API | Handler/OpenAPI/property/fuzz tests | 401/403, invalid cursor/body, safe 500, SSE resync |
| Web | Unit/component/accessibility | Loading, empty, stale, unknown, 401/403, Core down |
| End to end | Browser + Core + PostgreSQL + sample adapter | Login, role denial, crash→stale→unknown→recovery |
| Operational | Compose smoke, shutdown, backup/restore | Partial startup, migration mismatch, proxy SSE disconnect |

CI should keep fast deterministic checks on every pull request and schedule slower
race, leak, fuzz-smoke, backup/restore, and dependency scans. Flaky timing sleeps are
not accepted; tests use injectable clocks, readiness probes, and bounded eventual
assertions.

## 8. Observability baseline

Structured logs and health endpoints arrive before complex workers. Phase 1 records
at least:

- HTTP request count/duration by route template and status class
- PostgreSQL pool utilization and query error category
- adapter process state, restart count, collection duration/result, record count
- scheduler queue/concurrency and skipped/coalesced runs
- observations accepted/rejected and resources by current state
- stale/unknown transition count
- SSE connected clients, dropped/replayed/resync events
- auth success/failure/lockout counts without username labels
- audit write failures (which fail closed for protected administrative mutations)

Expose metrics only on the private management network or through authenticated
access. Never use raw resource IDs, usernames, URLs, or error strings as unbounded
metric labels.

## 9. Security gates

Before declaring Phase 1 complete:

- Local login has rate limits, strong password hashing, secure sessions, CSRF/origin
  checks, and generic failures.
- Every protected handler declares a permission and has a direct denial test.
- Adapter launch never invokes a shell; inputs, output, time, concurrency, restarts,
  environment, and logs are bounded.
- Secrets use references, are redacted everywhere, and do not enter frontend/API
  response models.
- Database roles follow least privilege; application mutation cannot edit existing
  audit records.
- Production publishes only the reverse proxy and uses TLS.
- Dependency and image vulnerabilities have a documented triage owner.
- The threat review has no unowned critical/high finding.

## 10. SSO follow-on insertion plan

SSO discovery may finish during Phase 1, but it does not interrupt the vertical
slice. Once the readiness contract is answered:

1. Record protocol/claim choices in a new ADR.
2. Implement the provider behind the existing identity-provider boundary.
3. Link by `(provider, external_subject)`, never mutable display name or email.
4. Configure explicit group-to-role mapping with default Viewer or default deny as
   approved by administrators.
5. Add start/callback/logout tests, state/nonce/PKCE validation as applicable, key
   rotation, clock skew, replay defense, and provider outage tests.
6. Enable `sso_with_local_fallback` in a test environment, verify local emergency
   access and alerts, then make it production default.
7. Keep at least one reviewed local administrator unless operators formally accept
   and document `sso`-only lockout risk.

No database rewrite, permission rewrite, or frontend session rewrite should be
needed if the Phase 1 boundary is respected.

## 11. Risks and controls

| Risk | Control | Trigger to revisit |
|---|---|---|
| SSO contract arrives late or differs from OIDC | Provider boundary; local mode; readiness checklist | Confirmed SSO documentation/test tenant |
| Local fallback becomes a forgotten permanent password | Audited use, strong hashing, account review, alert hook, recovery procedure | Production enablement and each semester |
| Adapter can exhaust Core | Process isolation, message/time/concurrency/restart caps, bounded queues | Conformance or load test breaks a bound |
| Stale data appears healthy | Core-derived freshness, persisted transitions, status semantics tests | Any state path lacks last-success/freshness data |
| UI and API models drift | Schema-derived types and CI diff | Manual duplicate model introduced |
| Single host is a monitoring blind spot | External heartbeat, backups, restore drill | HA becomes a requirement or outages justify it |
| Unknown site scale invalidates defaults | Environment inventory plus load test | Operators supply counts or measured usage exceeds headroom |

## 12. Phase 1 exit criteria

Phase 1 is complete only when all are true:

- [ ] Core and Web run as separate processes and can be upgraded independently.
- [ ] Local authentication is safe, audited, and role enforcement is proven directly
  against Core; the SSO provider boundary and insertion plan remain intact.
- [ ] A sample adapter passes conformance and reports normalized health without Core
  changes.
- [ ] Adapter crash/hang/malformed output is isolated and drives stale/unknown state.
- [ ] PostgreSQL migrations, transactional ingestion, and current health are tested.
- [ ] Operators can see current counts, integration health, resources, timestamps,
  and live connection state in an accessible Svelte UI.
- [ ] The public root explains Espial without operational disclosure and keeps
  sign-in available at the top right.
- [ ] Authenticated primary navigation uses an accessible top bar and presents
  Dashboard, Alerts, Datacenter, Hypervisor, and Webpages in that order on narrow
  screens as well as desktop.
- [ ] Login, shell, Dashboard, Alerts, Datacenter, loading, empty, error, and
  monitoring states share the reviewed dark UBNetDef visual language and pass the
  design-system review gate.
- [ ] SSE improves freshness but loss/reconnect never corrupts authoritative state.
- [ ] Administrative and authentication activity has redacted audit evidence.
- [ ] Graceful shutdown, race-sensitive code, bounded queues, and leak-prone adapter
  lifecycle paths pass automated checks.
- [ ] Local deployment, production example, bootstrap, backup/restore, upgrade, and
  recovery documentation have been exercised.
- [ ] Production-specific unknowns are either resolved or explicitly block production
  without blocking continued development.

After these criteria pass, Phase 2 can build incidents and notifications on a
monitoring foundation whose failure semantics are already observable and tested.
