# Phase 2 acceptance runbook

## Automated gate

Run `npm run acceptance:phase2` from a clean checkout with Docker available. It
executes contract/document compatibility, the PostgreSQL race suite, representative
and 10× backlog profiles, a disposable clean-stack lifecycle/SSE/shutdown/restore
drill, frontend diagnostics/unit/browser tests, and the production Web build.

The clean-stack portion uses generated test-only credentials, isolated Compose
projects, non-production sample data, and disposable volumes. The database tests
cover the deterministic website, Mattermost, incident, suppression, and certificate
matrices without contacting public DNS, websites, or Mattermost. Cleanup validates
its generated project/container prefixes before removing disposable state.

`npm run acceptance:phase2` is an aggregate release gate. The detailed scenario is
intentionally split between a real process/HTTP/restore stack and deterministic
race-safe PostgreSQL/TLS/Mattermost fixtures so CI never needs public egress or
wall-clock certificate manipulation.

## Manual cross-route and runbook sign-off

An Operator other than the runbook author records:

- release commit and immutable image digests;
- automated gate run URL and security workflow run URL;
- 1440×900, 1280×800, 500×900, 200% zoom, touch, keyboard, reduced-motion, and Core-
  unavailable results for Dashboard, Alerts, Webpages, Audit receipts, dropdowns,
  and mutation conflicts;
- the incident/rule/maintenance/silence dry run required by
  [INCIDENT_AND_RULE_OPERATIONS.md](INCIDENT_AND_RULE_OPERATIONS.md);
- one Mattermost token rotation/failure drill and one website/certificate diagnosis;
- an isolated restore containing real Phase 2 state; and
- every production blocker, accountable person, decision, and due date.

Repository automation cannot mark this external sign-off complete. The project
explicitly deferred it on 2026-08-01 so Phase 2 could close for development
sequencing and Phase 3 pre-planning could begin. The deferral is not evidence of a
dry run and does not waive this production-release gate; complete this record before
promoting Espial to production.
