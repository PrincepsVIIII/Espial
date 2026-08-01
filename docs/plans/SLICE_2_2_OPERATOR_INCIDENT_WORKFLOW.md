# Slice 2.2 implementation record: operator incident workflow

**Status:** Implemented (2026-07-31)  
**Plan:** [Phase 2 implementation plan](PHASE_2_IMPLEMENTATION.md), Slice 2.2  
**UI contract:** [UI guidance](../design/UI_GUIDANCE.md)

## Outcome

Operators can acknowledge, investigate, assign, append a plain-text note to, and
resolve a recovered incident through Core-authorized APIs and the incident detail
page. Every committed action advances the incident version, appends one immutable
timeline event, appends one redacted audit event, stores one durable idempotency
receipt, and then publishes an invalidation.

Viewers retain the factual incident and timeline read path without mutation
controls. Administrators receive the same visible request receipt plus a filtered
Audit link; Operators without `audit:read` receive the request ID without a link
they cannot open.

## Lifecycle and evidence contract

Status-changing actions use this complete transition table:

| Current status | Acknowledge | Investigate | Resolve |
|---|---|---|---|
| `open` | `acknowledged` | Rejected | Rejected |
| `acknowledged` | Rejected | `investigating` | Rejected |
| `investigating` | Rejected | Rejected | Rejected |
| `recovered` | Rejected | Rejected | `resolved` with required note |
| `resolved` | Rejected | Rejected | Rejected |

Assignment is valid for `open`, `acknowledged`, `investigating`, and `recovered`,
and rejected for `resolved`. Append-only notes are valid in every status so later
operational evidence can be recorded without rewriting existing evidence. Neither
assignment nor notes change incident status; both still advance its version.

Operator-controlled notes are trimmed, required where applicable, capped at 2,000
Unicode code points, stored only as timeline text, and rendered through Svelte text
interpolation. Audit summaries contain note length but never note content.

## Concurrency, retry, and identity decisions

- Mutation responses and detail reads use the incident version ETag. All actions
  require `If-Match`; a stale version returns `412` and the current ETag. Web
  invalidates its read model and asks the Operator to review it.
- All actions require an `Idempotency-Key`. The durable key scope is actor,
  incident, action, and key. An advisory transaction lock serializes identical
  in-flight retries before the receipt lookup.
- The request hash includes action, original expected version, owner, and note. An
  exact retry returns the original result version, timeline event ID, and
  correlation receipt without repeating any database effect. Reusing the key for a
  different request returns `409`.
- Assignee reads expose only opaque user ID and display name for enabled users whose
  current roles contain `incidents:operate`. Assignment validates eligibility again
  inside the mutation transaction.
- Operator and assigned-owner display names are copied into the immutable timeline
  event. Later account rename, disablement, or removal cannot rewrite historical
  identity text.

## Delivered surface

- Migration `000010_incident_operator_workflow.sql` adds historical identity fields
  and durable action receipts.
- `incidents.Workflow` owns transition validation, row locks, idempotency, timeline,
  audit, assignee eligibility, commit, and post-commit invalidation.
- Core exposes `acknowledge`, `investigate`, owner replacement, notes, resolution,
  and restricted assignee endpoints from the Phase 2 plan.
- Checked schemas, fixtures, OpenAPI operations, and generated TypeScript cover
  action requests, mutation receipts, assignees, and identity snapshots.
- Incident detail exposes role-aware lifecycle, assignment, note, and resolution
  forms with pending, validation, permission, Core-unavailable, conflict, and
  success receipt states.

## Verification evidence

- [Workflow integration tests](../../core/internal/incidents/workflow_integration_test.go)
  cover every valid and invalid transition, same-version concurrency, durable
  replay, idempotency-key misuse, one timeline/note/audit effect, redaction,
  historical identity, eligible/disabled/unauthorized owners, long notes, and empty
  resolution notes.
- [HTTP handler tests](../../core/internal/api/incident_handlers_test.go) cover
  Viewer `403`, origin, CSRF, required headers, request caps, safe `412` current
  version context, restricted assignees, and role-aware receipts.
- [Chromium workflow tests](../../web/tests/browser/dashboard.spec.ts) prove an
  Operator receives a visible receipt and exactly one audit record, a Viewer sees
  no mutation controls, and a stale action refetches with review guidance.
- The complete Core race/database suite, repository contracts, generated-type
  freshness, Svelte diagnostics, unit suite, Chromium suite, production build, and
  1440px/1280px/narrow UI review are the acceptance gate for this record.

## Deferred work

Rules, maintenance windows, silences, delivery evidence, and notification routing
remain in Slices 2.3 and 2.4. Slice 2.2 does not manually force an active condition
to resolved and does not imply those later controls exist.
