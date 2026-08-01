# Slice 2.1 implementation record: automatic incidents

**Status:** Implemented (2026-07-31)
**Plan:** [Phase 2 implementation plan](PHASE_2_IMPLEMENTATION.md)
**Decision:** [ADR 0007](../adr/0007-durable-monitoring-signal-journal.md)

## Delivered path

Slice 2.1 turns committed monitoring evidence into an authoritative, read-only
incident experience:

```text
observation/freshness transaction
  → durable monitoring_signals row
  → bounded leased evaluator
  → persisted rule debounce/recovery state
  → incident + append-only timeline transaction
  → incident_changed SSE invalidation
  → Dashboard/Alerts REST refetch
```

Migrations `000008` and `000009` add the journal, Phase 2 built-in permissions,
incident rules/conditions, persisted evaluator state, incidents, active-fingerprint
uniqueness, timeline evidence, due-work indexes, and the conservative default rule.
The journal exposes low-cardinality queue/claim/retry/dead-letter/latency metrics
and does not use SSE as a work queue.

The default policy opens critical immediately, opens warning after two matching
evaluations, opens unknown as warning, ignores stale by default, and requires two
healthy evaluations to recover. Matching is deterministic by resource specificity,
other scoped match fields, priority, and rule ID. Debounce, duration deadlines,
recovery counts, last signal, and active incident identity survive restart.

Core now serves:

- `GET /api/v1/incidents` with stable snapshot cursors and severity, status,
  integration, resource, owner, active, and time filters;
- `GET /api/v1/incidents/{id}` with a monotonic version `ETag`;
- `GET /api/v1/incidents/{id}/timeline` with stable cursor pagination; and
- additive active incident counts/newest-five summaries in `GET /api/v1/overview`.

All reads require `incidents:read`. Viewer, Operator, and Administrator built-in
roles receive that permission through an additive JSON union. No incident mutation
route or control is exposed before Slice 2.2.

Web replaces the Alerts placeholder with active/history lists, URL-backed filters,
direct incident URLs, factual current state, and the immutable timeline. Dashboard
uses only the additive overview read model for its active counts and compact list.
Both routes share the existing SSE reconnect/resync invalidation dependency and
retain explicit empty, denied, unavailable, and disconnected states.

## Contract evidence

- Five checked schemas have valid and invalid fixtures: monitoring signal,
  incident rule, summary, detail, and timeline event.
- Overview and live invalidation schemas are additive; generated TypeScript is
  current.
- OpenAPI declares and validates all three incident reads, including filters,
  pagination, errors, and detail `ETag`.

## Verification evidence

Database-backed race tests cover journal commit/rollback, concurrent claims,
retries/dead-letter/metrics, lifecycle boundaries, duplicate/out-of-order input,
concurrent evaluators, restart persistence, immutable timeline enforcement,
post-commit invalidation, pagination, and overview summaries.

API tests cover authentication, Viewer read, permission denial, invalid filters and
cursors, safe `404`/`500`, detail `ETag`, timeline reads, and incident SSE receipts.
Contract, documentation-link, generated-type, formatting, vet, and build checks
pass.

Web diagnostics and 46 unit/contract tests pass. Chromium exercises active list
and direct detail/timeline at 1440×900, 1280×800, and 500×900 with serious/critical
axe findings at zero. The complete browser suite also verifies keyboard menus,
reduced motion, REST refetch after SSE reconnect/resync, permission/Core failures,
and the shared authenticated shell.

## Deferred by design

Acknowledge, investigate, assign, note, resolve, rule administration, maintenance,
silence, and notification controls remain absent. They are Slice 2.2 and later work
and must preserve the version, audit-receipt, and capability-evidence contracts in
the parent plan.
