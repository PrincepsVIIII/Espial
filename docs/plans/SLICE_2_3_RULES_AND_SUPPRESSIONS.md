# Slice 2.3 implementation record: rules and suppressions

**Status:** Implemented (2026-07-31)  
**Plan:** [Phase 2 implementation plan](PHASE_2_IMPLEMENTATION.md), Slice 2.3  
**UI contract:** [UI guidance](../design/UI_GUIDANCE.md)

## Outcome

Administrators can list, inspect, create, replace, enable or disable, and preview
incident rules through Core-authorized APIs and `/alerts/rules`. The precedence
preview returns every matching rule in evaluation order and explains the winner;
the evaluator uses the same exact-resource, specificity, priority, and opaque-ID
order.

Administrators can also create, replace, and revoke one-time maintenance windows
and expiring notification silences through `/alerts/suppressions`. Maintenance
changes the effective resource state to `maintenance` while retaining the raw
state and reason. It never reports a failure as healthy. Silences are a separate
notification-decision boundary and never change incident health, lifecycle, or
visibility.

Every administrative mutation is origin- and CSRF-checked, permission-gated,
bounded, idempotent, audited in the mutation transaction, and returned with a
request receipt that links authorized users to the exact Audit filter.

## Evaluation and boundary contract

- Rules are normalized over integration, resource, resource kind, check type,
  health state, and reason code. Exact resource wins first; the number of other
  exact fields wins next; priority and opaque rule ID provide stable final ties.
- Replacing a rule updates the same rule and condition rows without deleting its
  persisted `(rule, resource, check_type)` debounce/recovery state. Due processing
  reloads that exact rule and safely clears a deadline if it is disabled or no
  longer matches.
- A maintenance interval is start-inclusive and end-exclusive. Overlaps resolve
  deterministically by resource scope, combined specificity, newest start, and
  opaque ID. Each maintenance-suppressed signal writes durable evaluation evidence
  naming both the winning rule and window.
- A maintenance signal clears in-progress failure/recovery counts and records an
  effective maintenance state without touching raw `current_health` or an existing
  incident. At expiry or active revocation, the control worker atomically appends
  idempotent synthetic signals from current raw health. Evaluation starts normal
  debounce at the window end, so a failure still present after planned work cannot
  disappear between restarts.
- A silence has exactly one incident, rule, or resource target. Matching is
  start-inclusive and expiry-exclusive, preferring incident, then rule, then
  resource. `suppressions.Service.MatchSilence` is the Slice 2.4 notification-intent
  decision seam; it performs no incident mutation.

## Persistence and concurrency

Migration `000011_alert_rules_and_suppressions.sql` adds rule versions,
`maintenance_windows`, `silences`, administrative idempotency receipts, evaluation
evidence, active/due indexes, and the `maintenance_expiry` signal kind. Window and
silence intervals are one-time UTC timestamps, strictly ordered, and capped at 366
days.

Rule, window, and silence replacements and revocations require version ETags.
Every retryable mutation requires an `Idempotency-Key`; an actor/target/operation/key
advisory transaction lock serializes concurrent retries. An exact retry returns the
original target/version/correlation receipt, while key reuse with a different hash
returns `409`. A stale ETag returns `412` without an audit-success event.

The expiry worker claims bounded rows with `FOR UPDATE SKIP LOCKED`. Window expiry,
synthetic re-evaluation signals, and the processed marker commit together, making
restart replay safe. Silence expiry only changes control evidence; matching also
checks the timestamp directly, so a delayed worker cannot prolong a silence.

## Delivered surface

- Core rule and suppression services, evaluator integration, effective-health read
  models, bounded expiry worker, post-commit suppression invalidations, and
  notification-silence matcher.
- Rule list/detail/create/replace/preview, maintenance list/detail/create/replace/
  revoke, and silence list/detail/create/replace/revoke API operations.
- Checked JSON Schemas, valid fixtures, OpenAPI operations, and generated
  TypeScript for rule writes/previews, controls, receipts, raw/effective health,
  and expiry signals.
- Permission-gated Alerts dropdown children and narrow-screen links. The rule page
  includes a condition editor and overlap preview. The suppression page exposes
  authoritative lists, create/replace/revoke forms, local-time inputs converted to
  explicit UTC timestamps, and language distinguishing maintenance from
  notification silencing.
- Dashboard resource rows show raw state and reason whenever effective health is
  maintenance.

## Verification evidence

- [Slice 2.3 incident integration tests](../../core/internal/incidents/slice23_integration_test.go)
  cover exact-rule precedence, overlap explanation, durable idempotent creation,
  stale replacement, audit evidence, debounce retained across replacement,
  suppression during maintenance, durable evaluation evidence, restart-safe expiry
  re-evaluation, silence match/non-match/revoke, and unchanged incident status.
- [Monitoring read integration tests](../../core/internal/monitoring/read_service_integration_test.go)
  verify effective maintenance in detail, list filters, overview counts, recent
  changes, and retained raw failure evidence.
- [Suppression unit tests](../../core/internal/suppressions/service_test.go) cover
  invalid/empty/multiple scopes, identifier bounds, and interval boundaries.
- [Administrative HTTP tests](../../core/internal/api/administration_handlers_test.go)
  cover direct Operator denial, required optimistic-concurrency and idempotency
  headers, metadata propagation, ETags, and audit-linked receipts.
- [UI contract and Chromium tests](../../web/src/lib/ui-contract.test.ts) verify the
  two real routes and factual semantics; the browser suite verifies Administrator
  visibility, Viewer omission, click, `Escape`, desktop dropdown, and collapsed
  navigation behavior alongside existing focus/hover coverage.

The acceptance gate is the complete database-backed Core suite, contracts,
generated-type freshness, repository formatting/docs checks, Svelte diagnostics,
unit tests, Chromium suite, race suite, and production build.

## Deferred work

Slice 2.3 does not create notification intents or claim that a silence has
suppressed a delivery. Slice 2.4 will call the delivered silence matcher while
creating destination-independent intents and will persist terminal `suppressed`
delivery evidence. Recurring calendars, timezone/DST schedules, arbitrary rule
expressions, and manual incident forcing remain outside Phase 2.
