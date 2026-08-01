# ADR 0010: Certificate health uses a distinct resource and bounded projection

- Status: Accepted
- Date: 2026-08-01

## Context

Each normalized resource has one authoritative `current_health` row. Website
availability and certificate validity can differ: an endpoint may respond while its
certificate approaches expiry, or a successful TLS protocol handshake may expose a
certificate that fails hostname or trust verification. Storing both checks on the
webpage resource would let the newest observation overwrite the other health
dimension.

Certificate identity also needs indexed expiry and replacement reads, but generic
observation metadata must not become an unbounded certificate-chain store or a
second configuration authority.

## Decision

For every HTTPS monitor attempt that reaches TLS, the trusted `webcheck` adapter
emits a distinct `certificate` resource and one `certificate.validity` observation
in the same collection as website availability. The resource remains owned by the
same typed integration/monitor row; there is no certificate configuration table or
independent scheduler.

The adapter completes a TLS protocol handshake, then verifies the leaf with the
configured hostname, approved roots, and injected current time before any HTTP
request is sent. This preserves bounded certificate evidence for untrusted,
wrong-host, expired, and not-yet-valid results without bypassing verification.

Inside the observation transaction, a narrow projector copies only endpoint,
subject/SAN and issuer summaries, serial/fingerprint, validity bounds, hostname/
chain results, remaining days, state/reason, and fingerprint/issuer-change flags to
`certificate_observations`. The projection is indexed for latest, expiry, and
attention reads. Private keys, full chains, trust roots, protected headers,
response bodies, and raw handshake errors are prohibited.

One certificate-specific incident rule owns the stable `(rule, certificate
resource, certificate.validity)` fingerprint. Warning and critical threshold
crossings update that incident. A reason-code change at the configured 7-day
escalation creates one meaningful condition update without opening a new incident.

## Consequences

- Availability and certificate current health cannot overwrite one another.
- The integration registry remains the single monitor configuration and scheduling
  authority, including validated per-monitor 30/14/7-day defaults.
- Certificate list/detail and Dashboard counts are authoritative indexed reads,
  while generic observations remain the source evidence.
- Replacement and issuer changes are visible factual evidence but do not change
  health or open an incident unless an administrator later enables a matching rule.
- Retention of a source observation cascades to its projection; no orphaned
  certificate identity record remains.
