# Slice 1.5 implementation record: scheduled monitoring pipeline

**Status:** Implemented  
**Scope:** Scheduler, collection ingestion, freshness ownership, operational audit,
and bounded in-process events  
**Depends on:** [Slice 1.3 normalized health](SLICE_1_3_NORMALIZED_HEALTH.md) and
[Slice 1.4 adapter runtime](SLICE_1_4_ADAPTER_RUNTIME.md)

## Outcome

Core now owns the complete internal monitoring path. At startup it reconciles
enabled integrations, supervises one trusted adapter process per integration,
schedules bounded collection calls, validates the returned normalized batch, and
commits resources, observations, current health, the successful collection record,
and its audit record. Only after commit does it publish bounded invalidation events.

The deterministic sample adapter proves this path:

```text
enabled integration
  -> supervised sample process
  -> deterministic jitter and bounded scheduler
  -> strict collection decoding and validation
  -> one PostgreSQL ingestion transaction
  -> current_health
  -> post-commit resource and collection events
```

This slice does not expose the event hub over HTTP. Versioned reads and authenticated
SSE are Slice 1.6. Adapter-originated push events also remain disabled; the accepted
v1 runtime continues to allow read-only `collect` capability only.

## Ownership and scheduling

- One coordinator periodically reads enabled integration IDs. It starts one
  supervisor owner for a newly enabled row, cancels a removed/disabled owner, and
  drains every owner during shutdown.
- Each healthy process generation gives its session to one scheduler workload.
  There is never more than one active collection for that integration.
- A process-wide semaphore bounds collections across integrations. The default is
  four and configuration rejects values outside 1–64.
- A tick arriving during an active collection becomes one pending collection.
  Further ticks are coalesced; they cannot grow an unbounded queue.
- Startup jitter is a stable hash of integration ID, seed, and sequence. Initial
  delay is within 0–10% of the interval, capped at 30 seconds. Periodic jitter is
  within plus or minus the same spread. The clock/timer and jitter boundaries are
  injectable in tests.
- Cancellation stops new dispatch, cancels work waiting for or holding a global
  token, waits for the active call, then lets the supervisor perform its bounded
  adapter shutdown.

## Transaction and failure rules

Successful collection metadata is not a best-effort side write. The ingestion
service has a pre-commit hook that appends `integration_collection_runs` and the
redacted collection audit event in the same transaction as normalized state. A
hook or commit failure therefore produces no resource change event and no audit
success. Rejected/failed calls are recorded in their own bounded transaction because
there is no normalized state to commit.

Collection records contain timestamps, duration, result, safe error category,
resource/observation counts, inserted/duplicate counts, and a correlation ID. They
never contain adapter diagnostics, raw payloads, configuration values, secret
references, executable paths, or remote error messages.

Adapter start, healthy, failure, recovery, and stop transitions append system audit
events and publish integration invalidations. Configuration updates use an
optimistic `updated_at` check and commit the update plus its audit event together.
Their audit summaries contain only enabled state, interval, non-secret key names,
and secret-reference key names—not either set of values. A null `actor_user_id`
means the Core system actor; authenticated mutation callers supply the user ID.

## Freshness

One runtime-owned freshness worker polls persisted deadlines. It claims a bounded
batch with `FOR UPDATE ... SKIP LOCKED`, reevaluates it with the same pure health
function used by ingestion, commits changes, and publishes only after commit. This
makes a second future Core instance cooperate instead of double-applying a due row.

Exact deadlines are inclusive: a healthy/warning/critical state becomes `stale` at
`stale_at`, and `stale` becomes `unknown` at `unknown_at`. Disabled and already
unknown resources are not scheduled. A worker object rejects a second concurrent
`Run`, making goroutine ownership explicit.

## Bounded event hub

The event hub is an in-memory invalidation mechanism, never a source of truth. It
assigns ordered IDs and schema version 1, retains a fixed replay window, and gives
each subscriber a capped channel (maximum 256). Publish is non-blocking. If replay
is too old or a subscriber falls behind, queued events are discarded and exactly
one `resync_required` marker is delivered; the consumer must reread PostgreSQL.

Slice 1.6 will adapt this boundary to authenticated SSE and apply authorization to
the database reread. Restarting Core intentionally loses replay history and causes
clients with old cursors to resynchronize.

## Configuration

| Setting | Default | Bounds/purpose |
|---|---:|---|
| `ESPIAL_ADAPTER_GLOBAL_CONCURRENCY` | `4` | 1–64 simultaneous collection calls |
| `ESPIAL_ADAPTER_RECONCILE_INTERVAL` | `10s` | 100 ms–5 min enabled-row reconciliation |
| `ESPIAL_FRESHNESS_INTERVAL` | `1s` | 100 ms–1 min due-deadline polling |
| `ESPIAL_FRESHNESS_BATCH_SIZE` | `100` | 1–1000 rows claimed per transaction |
| `ESPIAL_EVENT_REPLAY_SIZE` | `1024` | 1–10000 retained invalidations |

These values are non-secret and appear in the safe startup configuration summary.
The sample executable remains an immutable trusted path configured separately.

## Verification evidence

Automated tests cover:

- deterministic jitter bounds, global maximum concurrency, per-integration
  coalescing, enabled-set reconciliation, and cancellation drain;
- slow-work isolation through the shared semaphore and independent integration
  owners;
- PostgreSQL end-to-end sample process → protocol validation → normalized rows →
  current health → audit/run record → ordered events;
- exact stale/unknown boundaries and concurrent freshness claims applying one
  transition;
- a deliberately failed transaction rolling back normalized state and the inserted
  audit-success row while publishing nothing;
- redacted, atomic optimistic configuration updates and lifecycle failure/recovery
  audit events;
- bounded replay, old-cursor resync, and a slow subscriber that cannot block a
  producer; and
- the full Go race suite against PostgreSQL 17.

## Slice 1.6 handoff

Slice 1.6 should consume, not recreate, these boundaries:

1. Query PostgreSQL for versioned resource/integration/overview read models.
2. Authorize every read and audit only administrative reads required by policy.
3. Translate a hub subscription to authenticated SSE with heartbeat and connection
   limits.
4. Treat `resync_required`, an old cursor, and process restart identically: refetch
   authoritative reads and open a new subscription from the latest event ID.
5. Keep HTTP response delivery out of ingestion transactions and never expose audit
   summaries as a substitute for resource details.
