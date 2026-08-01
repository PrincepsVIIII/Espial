# Slice 2.5 implementation record: website availability monitoring

**Status:** Implemented (2026-08-01)  
**Decision:** [ADR 0009](../adr/0009-website-monitor-integration-projection.md)  
**Execution contract:** [Phase 2 implementation plan](PHASE_2_IMPLEMENTATION.md)

## Delivered

- Added redacted website-monitor write/read contracts, webpage summary/detail
  contracts, fixtures, generated TypeScript, OpenAPI operations, and
  `monitor_id` live invalidations.
- Added migration `000013`: webcheck projection/history indexes, a specific
  aggregate availability incident rule, and website-monitor administrative
  idempotency support.
- Added the supervised `espial-webcheck-adapter` binary and manifest. Exact URL,
  status, latency, content, redirect, refresh, and up to four protected-header
  slots are strictly validated.
- Enforced host and port allowlists, all-answer CIDR approval, pinned dialing,
  DNS/TCP/TLS/HTTP/body/whole-check bounds, verified TLS, per-hop redirect
  revalidation, cross-origin credential rejection, and no ambient proxy.
- Added typed monitor create/replace/read/manual-check services with optimistic
  concurrency, idempotency, redacted atomic audit receipts, scheduler coalescing,
  stable opaque list cursors, and runtime generation restart. Generic integration mutations reject the
  `webcheck` adapter so the registry row has one typed mutation path.
- Added Viewer-readable webpage list/detail services with effective and raw state,
  safe reason, completed stages, elapsed measurements, freshness, and active
  incident link. Administrators receive the real Monitors child route and complete
  create/replace/manual-check receipt path.
- Added fail-closed local and production configuration surfaces and built the
  adapter into the Core image.

## Verification evidence

- Contract validation: 56 positive/negative fixtures; OpenAPI v1 validates 25
  operations and 545 references; generated frontend types are current.
- Core: all packages pass under pinned Go 1.26. Deterministic local adapter tests
  cover healthy response, unexpected status, content mismatch, oversized headers
  and body, slow response, connection refusal and recovery, redirect policy,
  protected-header redirect rejection, mixed IPv4/IPv6 answer rejection, ambient
  proxy isolation, DNS failure, TLS failure, URL ambiguity, and secret/header
  injection validation.
- PostgreSQL/race: a critical aggregate `website.availability` observation creates
  exactly one incident; a repeated failure retains that incident and the configured
  two healthy observations recover it.
- API: direct tests prove Viewer webpage reads, Viewer monitor denial, Administrator
  redacted monitor reads, typed mutation input, and audit-linked receipts.
- Web: Svelte diagnostics report zero errors and warnings; 48 frontend unit tests
  pass; the production build is checked in the repository-wide gate.
- Browser: 32 Chromium scenarios pass. Webpages list/detail is captured and axe-
  checked at 1440×900, 1280×800, and 500×900; manual check exposes its request ID
  and matching Audit link.

## Deferred by contract

Certificate identity, expiry projection, threshold rules, certificate routes, and
the Certificates navigation child remain Slice 2.6 work and are not exposed here.
