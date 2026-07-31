# Slice 1.3 execution plan: normalized storage and health

**Status:** Implemented 2026-07-31  
**Parent:** [Phase 1 monitoring foundation](PHASE_1_IMPLEMENTATION.md)  
**Contracts:** [resource schema](../../api/schemas/v1/resource.schema.json),
[observation schema](../../api/schemas/v1/observation.schema.json), and
[initial data model](../architecture/DATA_MODEL.md)

## Outcome

Slice 1.3 establishes the trusted domain and persistence layer used by adapters,
scheduling, APIs, and the Dashboard. It accepts an already-decoded collection
batch, rejects the entire batch when any record is invalid, transactionally stores
resources and observations, derives one current health state per resource, and
reports committed state changes through a small internal boundary.

It does not launch adapters, schedule collection, expose monitoring HTTP routes, or
populate the UI. Those remain Slices 1.4–1.7.

## Foundation used

Already available:

- accepted v1 resource and observation schemas with positive/negative fixtures;
- PostgreSQL pool, forward-only embedded migrations, and isolated-schema integration
  test harness;
- `integrations`, `resources`, `observations`, and `current_health` tables;
- explicit stale/unknown defaults in the parent plan; and
- deterministic sample identifiers that do not require real environment inventory.

Slice 1.3 adds migration `000004_observation_freshness.sql` because the original
tables did not persist the source refresh interval or the scheduled unknown time.
It also adds a normalized observation delivery key for retry idempotency.

No SSO answer, vendor credential, internal hostname, production inventory count, or
operator-supplied secret is required for this slice.

## Accepted semantics

### Identity and ordering

- A resource is identified by `(integration_id, external_id)`; Core owns its UUID.
- An observation-provided UUID is used when present. Otherwise
  `(integration_id, resource_id, check_type, observed_at)` is its retry key.
- Repeating a delivery key with identical normalized content is a no-op. Reusing it
  with different content is a conflict and rolls back the batch.
- The current resource state comes from the newest valid observation by
  `observed_at`; `received_at`, then UUID, provide deterministic tie-breaking.
- A late older observation is retained as history but never replaces newer current
  state.

The one-current-state-per-resource rule follows the accepted Phase 1 data model.
Per-check current state or severity aggregation would change that model and requires
a later explicit contract change rather than being improvised in this slice.

### Freshness

For an observation with expected refresh interval `R`:

```text
grace      = max(R × 0.5, 30 seconds)
stale_at   = observed_at + R + grace
unknown_at = max(observed_at + (R × 3), stale_at)
```

- Before `stale_at`, current state equals the newest observed state.
- At or after `stale_at` and before `unknown_at`, it is `stale`.
- At or after `unknown_at`, it is `unknown`.
- An explicit observed `unknown` is immediately unknown; waiting for freshness adds
  no information.
- `disabled` remains disabled until configuration or a newer observation changes
  it; intentionally disabled collection must not age into an alarm.
- `maintenance` requires fresh evidence and ages to stale/unknown like other
  observed states until Phase 2 maintenance windows provide a separate authority.
- `last_success_at` means the latest observation that made a positive determination
  (`healthy`, `warning`, `critical`, `maintenance`, or `disabled`), not merely a
  healthy result.

All comparisons use UTC instants. Boundary tests cover equality, not only time just
before and after a transition.

## Initial validation limits

Schema validation remains the transport boundary. Domain validation adds:

| Rule | Initial bound |
|---|---:|
| Future source timestamp skew | 5 minutes |
| Expected refresh | 1–86,400 seconds, matching schema |
| Resources plus observations per batch | 1,000 records |
| Encoded resource attributes | 64 KiB per resource |
| Encoded observation measurements | 64 KiB per observation |
| Encoded observation metadata | 64 KiB per observation |
| Source URL schemes | `http` or `https` |

Older observations are allowed so delayed delivery can be retained. The protocol's
message-size limit remains an independent outer bound. These defaults are package
constants in Slice 1.3; making them configuration is deferred until measured sample
or production scale justifies the added surface.

## Package boundaries

```text
core/internal/observations/
├── types.go          transport-independent inputs and committed result types
├── validate.go       batch and domain validation
├── repository.go     PostgreSQL transaction and row-lock operations
├── service.go        all-or-nothing ingestion orchestration
└── *_test.go         unit and PostgreSQL integration tests

core/internal/health/
├── types.go          state, current health, and transition reason types
├── evaluator.go      pure newest-observation and freshness evaluation
├── clock.go          injectable time boundary
└── *_test.go         table-driven boundary tests
```

Rules:

- transport JSON structs do not become database models by aliasing;
- `health` performs no SQL and reads no wall clock directly;
- the observations service owns transaction scope but delegates SQL to its
  repository;
- repositories do not publish events or decide validation/health policy; and
- callers receive typed validation/conflict errors without raw SQL or payloads.

## Ingestion algorithm

1. Capture one injected `received_at` instant for the batch.
2. Validate every resource and observation in memory. Return indexed safe errors
   before opening a transaction. References to resources included in the same batch
   are resolved in memory; references to previously stored resources are resolved
   inside the transaction.
3. Begin a transaction and upsert resources by integration/external identity.
   Preserve `first_seen_at`; update mutable display/attributes/source fields and
   advance `last_seen_at` only when source time is newer.
4. Resolve every observation to a resource belonging to the same integration.
5. Insert observations by UUID/delivery key. Treat identical retries as accepted
   duplicates and reject conflicting retries.
6. Lock affected `current_health` rows in stable resource-ID order to prevent
   deadlocks and serialize concurrent updates.
7. Evaluate the newest persisted observation for each affected resource and upsert
   current health only when the deterministic winner is newer.
8. Commit the complete batch. Roll back resources, observations, and current health
   together on any failure.
9. After commit, return/publish immutable state-change records. A commit failure or
   no-op duplicate emits nothing.

Slice 1.3 exposes the post-commit records through an interface; the bounded event
hub and freshness worker that consume them are implemented in Slice 1.5.

## Test matrix

### Pure validation and evaluator tests

- every valid state and invalid/unknown state;
- empty/whitespace identity, names, summaries, and check types;
- future skew at, below, and above five minutes;
- refresh, record-count, property-count, and encoded-size boundaries;
- invalid URL scheme and oversized/nested metadata;
- stale/unknown boundaries for ordinary and sub-minute refresh intervals;
- explicit unknown, durable disabled, and expiring maintenance behavior;
- newest/tie-break selection and `last_success_at` preservation; and
- context cancellation before persistence.

### PostgreSQL integration tests

- resource insert and later upsert preserve first seen and monotonic last seen;
- resource plus observations plus current health commit atomically;
- one invalid record leaves every table unchanged;
- identical retry is a no-op and conflicting retry rolls back;
- older delivery stays in history without replacing current health;
- recovery from stale/unknown retains prior observations;
- two concurrent batches for one resource choose the correct observation;
- batches touching resources in inverse input order do not deadlock;
- integration/resource ownership cannot be crossed; and
- post-commit changes are absent on rollback and emitted once on success.

Run the concurrency-bearing packages with `go test -race`. Database tests use the
existing `ESPIAL_TEST_DATABASE_URL` isolated-schema helper and never drop the
configured database.

## Implementation order

1. Land and verify the freshness migration.
2. Implement pure health types/evaluator and exhaustive time-boundary tests.
3. Implement transport-independent inputs and domain validation.
4. Implement repository queries and transactional service.
5. Add idempotency, late-delivery, rollback, and concurrency integration tests.
6. Add the post-commit state-change interface and tests.
7. Run formatting, vet, unit, race, PostgreSQL integration, and full repository
   checks; update the parent progress table only after all gates pass.

## Questions and escalation points

No user decision blocks implementation. Ask the project owner before changing any
of these accepted behaviors:

- replacing newest-observation state with worst-severity or per-check aggregation;
- aging `disabled` into stale/unknown;
- allowing non-HTTP source links;
- increasing validation limits based on assumed production scale; or
- changing the default freshness schedule.

Real scale, vendor behavior, and credentials are intentionally deferred to the
first production adapter and remain tracked in the environment inventory.

## Implementation evidence

Completed in this slice:

- migration `000004_observation_freshness.sql` persists expected refresh and
  unknown transition times and enforces the natural delivery key;
- `core/internal/health` implements the pure evaluator, injected clocks,
  deterministic ordering, no-observation state, and exact freshness boundaries;
- `core/internal/observations` implements safe indexed validation, transactional
  repository/service boundaries, monotonic resource upserts, retry conflict
  handling, stable row locking, and post-commit state changes; and
- unit plus PostgreSQL integration tests cover the validation limits, all observed
  states, rollback, natural-key and UUID idempotency, late delivery, recovery,
  ownership isolation, cancellation, post-commit publication, and concurrent
  inverse-order batches under the Go race detector.

The repository-wide formatting, static analysis, unit, contract, documentation,
frontend, migration, PostgreSQL integration, and race checks are the final gate for
this status. Freshness scheduling is intentionally not hidden in ingestion: the
worker that reevaluates persisted `stale_at` and `unknown_at` deadlines remains
Slice 1.5.
