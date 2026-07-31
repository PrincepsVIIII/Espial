# Slice 1.6 implementation record: versioned REST and SSE API

**Status:** Implemented  
**Completed:** 2026-07-31  
**Parent:** [Phase 1 implementation plan](PHASE_1_IMPLEMENTATION.md)  
**Contracts:** [API conventions](../architecture/API_CONVENTIONS.md),
[OpenAPI v1](../../api/openapi/v1.json), and [shared schemas](../../api/README.md)

## Outcome

Slice 1.6 turns the persisted monitoring pipeline into a complete authenticated v1
API. A non-browser client can sign in, inspect overview/resource/integration state,
manage a registered integration, read audit history as an administrator, follow
bounded live invalidations, and sign out without relying on Web or an undocumented
database query.

REST remains authoritative. SSE carries only small invalidations and explicitly
tells a client when bounded replay is no longer possible.

## Delivered surface

| Method and path | Permission | Behavior |
|---|---|---|
| `GET /api/v1/overview` | `overview:read` | State and integration-runtime counts, stale/unknown totals, and the ten most recent state changes |
| `GET /api/v1/resources` | `resources:read` | Stable paginated resource/current-health collection |
| `GET /api/v1/resources/{id}` | `resources:read` | Resource, current health, and latest observation detail |
| `GET /api/v1/integrations` | `integrations:read` | Stable paginated integration/runtime/last-collection collection |
| `GET /api/v1/integrations/{id}` | `integrations:read` | Integration detail with configuration and secret-reference key names only |
| `POST /api/v1/integrations` | `integrations:manage` | Creates an integration for an immutable registered adapter and returns `Location` plus `ETag` |
| `PUT /api/v1/integrations/{id}/configuration` | `integrations:manage` | Replaces enabled/interval/configuration under `If-Match` optimistic concurrency |
| `GET /api/v1/audit` | `audit:read` | Bounded redacted audit history and an audit record of the administrative read |
| `GET /api/v1/events/stream` | `overview:read` | Authenticated bounded-replay SSE invalidations |

The health and authentication routes delivered in Slices 1.1–1.2 remain part of the
same documented v1 surface.

## Non-browser client flow

A client uses the configured public origin even when it connects directly to Core:

1. `POST /api/v1/auth/local/login` with JSON credentials and an `Origin` header
   exactly matching `ESPIAL_PUBLIC_URL`; retain both returned cookies.
2. `GET /api/v1/auth/session` with `espial_session` to discover the authoritative
   user and permissions.
3. Read overview, resource, and integration routes with the session cookie. Treat
   `next_cursor` as opaque and send it back with the same filters.
4. For a mutation, send both cookies, the matching `Origin`, and the
   `espial_csrf` cookie value in `X-CSRF-Token`. Fetch integration detail first and
   send its `ETag` as `If-Match` when replacing configuration.
5. Open `/api/v1/events/stream` with the session cookie and retain each numeric
   event ID. Reconnect with `Last-Event-ID`; refetch REST data on every invalidation
   and refetch all visible data on `resync_required`.
6. `POST /api/v1/auth/logout` with the mutation headers. Discard both cookies after
   the `204` response; an existing stream closes by the next heartbeat.

The [OpenAPI document](../../api/openapi/v1.json) defines the exact request fields,
headers, filters, response models, and failure statuses for this sequence.

## Read models and filters

Overview is deliberately small and has no incident count. Incidents do not exist
until Phase 2. Resource reads accept repeated `state`, `kind`, and `integration`
filters and one `stale` boolean. Integration reads accept repeated `adapter_id` and
`runtime_state` filters and one `enabled` boolean. Repetition means OR within a
field; different fields combine with AND. `stale=true` means the current state is
exactly stale; unknown data remains independently selectable with `state=unknown`.

Audit reads accept repeated `action`, `result`, and `target_type`, plus one
`actor_user_id`, `from`, and `to`. The default window is the previous 24 hours; the
maximum is 31 days. The resolved window is returned with every page so a client can
continue the exact same snapshot. Audit summaries are already constrained at write
time and integration configuration reads reveal names, never values.

Collections default to 50 rows and reject limits above 200. Opaque cursors encode:

- the collection kind and a fingerprint of normalized filters;
- the first-page database snapshot time;
- the final row's ordering timestamp and UUID tie-breaker; and
- for audit reads, the resolved bounded time range.

Subsequent pages reject a cursor used with another collection or changed filters.
Queries consistently order by timestamp and UUID descending, and never read rows
newer than the first-page snapshot. This provides no missing or duplicate rows for
a stable snapshot without holding a transaction open across HTTP requests.

## Write safety

Integration mutations use the existing provider-neutral session boundary, require
`integrations:manage`, and enforce trusted `Origin` plus a session-bound CSRF token.
Bodies are capped at 128 KiB, decoded as exactly one JSON object, and reject unknown
or missing required values. Collection interval is limited to 1–86,400 seconds and
stored configuration documents remain subject to Core's 64 KiB normalized limit.

Creation checks the immutable in-process adapter registry before inserting. An
unknown adapter returns `409 adapter_not_registered`; callers cannot supply a path,
arguments, environment, or executable. Configuration replacement uses the detail
response `ETag`; absent `If-Match` returns `428`, malformed tags return `400`, and a
concurrent change returns `412`. Creation and replacement commit their redacted
audit event in the same PostgreSQL transaction and publish an invalidation only
after commit.

## Live invalidations

`GET /api/v1/events/stream` authenticates the session and verifies `overview:read`
before reserving one of the bounded stream slots. Each event contains an increasing
numeric `id`, an event name, and JSON matching `live-invalidation.schema.json`.
Heartbeats are SSE comments and do not masquerade as domain events.

The default heartbeat is 15 seconds. Every heartbeat re-authenticates the cookie
with a bounded database deadline and rechecks permission, so logout, expiry,
disablement, or role loss closes a stream within the configured interval. Request
cancellation and Core shutdown close subscriptions immediately. The default process
limit is 100 streams; excess clients receive `503 stream_limit_reached` and
`Retry-After: 15`.

Clients reconnect with `Last-Event-ID`. Events still inside the configured replay
window are replayed in order. A cursor older than the window or ahead of the server
receives one `resync_required` event and the stream closes. The client then refetches
all visible REST state and reconnects without a cursor. Slow subscribers are also
moved to resync rather than blocking collection or growing an unbounded queue.

Operators can tune the behavior with:

| Variable | Default | Safe range |
|---|---:|---:|
| `ESPIAL_EVENT_REPLAY_SIZE` | `1024` events | 1–10,000 |
| `ESPIAL_SSE_HEARTBEAT` | `15s` | 1s–1m |
| `ESPIAL_SSE_MAX_CLIENTS` | `100` streams | 1–10,000 |

## Error and compatibility contract

All API failures use the shared safe error envelope with a stable machine code,
operator-safe message, request ID, and optional field errors. Request IDs are
accepted or generated once, returned in `X-Request-ID`, and propagated to audit
correlation IDs. Handler tests cover authentication and permission boundaries,
validation, hidden/missing resources, registered-adapter conflicts, optimistic
concurrency, body limits, and redacted internal errors.

The checked OpenAPI 3.1 document describes all 16 current operations and references
the shared JSON Schemas instead of cloning response models. Contract checks validate
positive and negative fixtures, operation IDs, reference resolution, and meaningful
reuse. Type generation produces the Web models from those same schemas.

## Verification record

- Unit and race tests cover handlers, strict query parsing, authentication and
  permission boundaries, SSE replay/resync/revocation, and configuration bounds.
- PostgreSQL integration tests cover overview calculation, details, filters,
  cursor fingerprints, stable multi-page reads, bounded audit windows, redaction,
  registered-adapter creation, and transactional audit behavior.
- An HTTP/PostgreSQL vertical test bootstraps an administrator, logs in through v1,
  creates a registered integration, ingests normalized state, and reads overview,
  filtered resources, filtered integrations, and audit solely through documented
  authenticated endpoints.
- Fuzz targets exercise query decoders and `Last-Event-ID`; explicit size/count
  limits prevent unbounded query, cursor, body, and repeated-filter allocation.
- Root contract, generation, formatting, lint, Core, Web, docs-link, and Compose
  validation checks remain the final acceptance gate.

## Slice 1.7 handoff

Slice 1.7 should consume the generated types and REST/SSE semantics as delivered:

1. Initial Dashboard data comes from REST and shows no invented incident metric.
2. Filters use documented URL parameters and preserve opaque cursors unchanged.
3. Live events trigger coalesced REST refreshes; they never directly replace an
   authoritative resource or integration model.
4. `resync_required` clears local pagination/live assumptions and refetches all
   visible monitoring data.
5. The client displays live/reconnecting/disconnected state and the last successful
   REST refresh, with bounded jittered reconnect.
6. UI work must first re-read the authoritative [UI guidance](../design/UI_GUIDANCE.md)
   and pass the recorded viewport and anti-template review gate.
