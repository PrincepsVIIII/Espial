# Slice 2.6 implementation record: certificate monitoring and Webpages experience

**Status:** Implemented (2026-08-01)  
**Decision:** [ADR 0010](../adr/0010-certificate-resource-projection.md)  
**Execution contract:** [Phase 2 implementation plan](PHASE_2_IMPLEMENTATION.md)

## Delivered

- Added bounded certificate summary/detail schemas, positive/negative fixtures,
  OpenAPI list/detail operations, generated TypeScript, `certificate_id` live
  invalidations, and additive authoritative Dashboard warning counts.
- Added migration `000014` with normalized certificate observation history,
  expiry/latest/attention indexes, fingerprint and issuer change evidence, the
  specific certificate rule, and persisted incident reason-code state.
- Extended the trusted `webcheck` collection to emit a distinct certificate
  resource and `certificate.validity` observation after HTTPS handshakes. Go's TLS
  verifier uses the configured hostname, approved roots, and injected time; bounded
  peer identity evidence is recovered from certificate-verification errors, and
  HTTP is never sent when verification fails.
- Distinguished no reported certificate, untrusted chain, hostname mismatch,
  expired, not-yet-valid, approaching expiry, critical expiry, and the 7-day
  escalation threshold without retaining private keys, full chains, raw handshake
  errors, response content, or protected headers.
- Added per-monitor warning/critical/escalation thresholds with validated 30/14/7
  defaults. Replacement and issuer changes remain informational evidence unless a
  later administrator rule explicitly matches them.
- Added a transaction-owned projection package and Viewer-readable certificate
  list/detail services with stable cursors, status/hostname/expiry filters,
  identity, validity, freshness, source, and active incident linkage.
- Added `/webpages/certificates`, direct certificate detail routes, a real
  Certificates navigation child for every `webpages:read` session, monitor
  threshold controls, and the Dashboard certificate-attention summary.

## Verification evidence

- Contracts: 60 positive/negative fixtures pass; OpenAPI v1 validates 25
  operations and 559 references; generated frontend types are current.
- Adapter: injected-clock certificates cover valid, exact 30/14/7-day boundaries,
  expired, not-yet-valid, wrong-host, and untrusted-chain results. Existing DNS,
  TCP, redirect, proxy, body, and timeout coverage remains green.
- PostgreSQL/race: migration `000014` applies twice safely; projection history
  records fingerprint and issuer replacement evidence; secrets and full chains are
  absent. The targeted Go race suite passes for projection, incidents, monitoring,
  and storage.
- Incidents: warning→critical→7-day escalation preserves one active fingerprint
  and produces exactly `detected`, `severity_changed`, and `condition_changed`
  timeline/notification events; a repeated 7-day observation creates no new
  notification event.
- API: direct tests prove `webpages:read` allow/deny behavior, bounded query filter
  parsing, and opaque direct certificate URLs.
- Web: Svelte diagnostics report zero errors and warnings and all 48 unit tests
  pass. The 35-scenario Chromium suite covers certificate list/detail at 1440×900,
  1280×800, and 500×900 with reduced-motion-safe static UI and zero serious or
  critical axe findings.

## Deferred by contract

Slice 2.7 owns full Phase 2 clean-stack acceptance, operational certificate
diagnosis/upgrade runbooks, threat-review closure, load evidence, backup/restore,
and the cross-route final UI review.
