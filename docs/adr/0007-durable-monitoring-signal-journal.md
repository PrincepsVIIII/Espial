# ADR 0007: Durable monitoring-signal journal

**Status:** Accepted

## Context

Phase 1 publishes bounded server-sent-event invalidations after monitoring commits.
That in-memory replay hub is appropriate for telling browsers to refetch, but a
Core restart or disconnected subscriber can lose an event. Incident correctness
therefore cannot depend on SSE delivery.

Observation ingestion and freshness evaluation already commit authoritative health
evidence in PostgreSQL. Phase 2 needs a durable, replayable handoff from those
commits to rule evaluation without turning every Core table into an event store.

## Decision

Append one normalized `monitoring_signals` row in the same PostgreSQL transaction
as each newly accepted observation or persisted freshness transition.

- A stable `source_key` makes producer retries idempotent.
- Rows carry only bounded normalized match inputs: integration, resource, check,
  state, safe reason/reason code, and occurrence time.
- Evaluators claim bounded ordered batches with a lease and `FOR UPDATE SKIP
  LOCKED`; completion, retry scheduling, attempts, and terminal dead-letter state
  are durable.
- Rule debounce and recovery state is separately persisted by rule/resource/check.
- SSE remains a post-commit browser invalidation channel. It is never a worker
  queue or correctness dependency.
- Low-cardinality journal metrics expose queue depth/age, active claims, retries,
  dead letters, and average processing latency without resource identifiers.

## Consequences

Core can restart during debounce, recovery, or claim processing without losing the
incident decision. Multiple evaluators can safely share PostgreSQL, while incident
fingerprint uniqueness provides a second idempotency boundary.

The journal adds retained operational rows and requires backlog/dead-letter
monitoring. It is deliberately narrow: PostgreSQL read models remain authoritative,
and later notification delivery uses its own purpose-built outbox rather than
expanding this journal into a generic event bus.
