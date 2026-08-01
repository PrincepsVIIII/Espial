# Slice 2.7 implementation record: Phase integration and acceptance

**Implemented:** 2026-08-01  
**Acceptance status:** accepted for development sequencing on 2026-08-01 after the
remaining independent operational dry run and named production-owner sign-off were
explicitly deferred; both remain external production-release gates.

## Runtime hardening

- Incident evaluator concurrency, batch, poll, lease, and maximum attempts are
  typed file/environment settings with bounded validation, safe startup summaries,
  Compose examples, and tests. Two evaluator workers use PostgreSQL `SKIP LOCKED`
  claims without weakening fingerprint or signal idempotency.
- Readiness now requires both PostgreSQL and required worker initialization.
- Private Core `/metrics` exposes fixed-enum, low-cardinality signal, incident,
  suppression, notification, webpage, and certificate counts/ages. The production
  proxy does not route it, and tests prohibit resource/hostname labels.
- Phase 2 operational evidence is retained indefinitely; no untested automatic
  deletion or arbitrary notification replay endpoint was introduced.

## Acceptance and evidence

- `npm run test:phase2-db` provisions disposable PostgreSQL and runs the full Core
  race suite with database-backed tests enabled.
- `npm run load:phase2` provisions disposable PostgreSQL and drains 1,000/250 and
  10,000/2,500 signal/notification profiles through production worker code.
- `npm run acceptance:phase2` aggregates contracts, race/PostgreSQL, load, clean-
  stack lifecycle/SSE/shutdown/restore, frontend diagnostics/unit/browser, and
  production-build gates. A dedicated CI workflow runs it on `main` and manually.
- The initial load run measured 4.52 seconds for the representative profile and
  47.41 seconds for the 10× profile, below the 30/120-second budgets.
- The disposable clean-stack lifecycle/SSE/shutdown/restore drill passed; bounded
  Core shutdown completed in 687 milliseconds and the restored stack recovered the
  authoritative state.
- Production dependency audits, `govulncheck`, repository secret/misconfiguration
  scanning, and release-shaped Core/Web image scans reported no blocking finding.
- Per-slice UI evidence is consolidated in the final cross-route review. Existing
  browser coverage retains all implemented routes, permission boundaries, receipts,
  three viewports, keyboard/touch menu behavior, reduced motion, reconnect, and
  Core-unavailable states.

## Operational and security records

Added incident/rule/suppression, notification recovery, website/certificate,
acceptance, load, deployment, and restore guidance. The Phase 2 threat review owns
every critical/high residual by role and explicitly carries Phase 1 production
blockers forward.

## External production acceptance deferred

The repository cannot provide an independent operator dry run, actual UBNetDef
backup/secret/egress values, named accountable people, or risk acceptance for
upstream infrastructure-image findings. An explicit project decision on 2026-08-01
deferred this evidence and accepted Slice 2.7 for development sequencing. This is
not evidence that the exercise occurred and must not be represented as production
approval until the linked external record exists.
