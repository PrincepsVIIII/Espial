# ADR 0008: Durable notification outbox and in-process Mattermost driver

- Status: Accepted
- Date: 2026-08-01

## Context

Incident notification correctness cannot depend on the live-event stream or an
in-memory subscriber. Delivery also crosses a high-risk network and secret boundary:
an operator supplies a destination, DNS may change between validation and dialing,
and webhook tokens must never enter normal configuration or evidence.

## Decision

Core creates one destination-independent intent per enabled destination in the same
PostgreSQL transaction as each notifiable incident timeline event. Detected,
severity-change, recurrence, and recovery events are notifiable; acknowledgement,
assignment, notes, investigation, and resolution are not. A matching silence creates
terminal `suppressed` evidence instead of delayed work.

A bounded in-process worker claims due rows with leases and `SKIP LOCKED`, records an
append-only attempt, and moves each intent through queued, attempting, retry-wait,
delivered, failed, dead-letter, or suppressed state. It makes at most six attempts.
Worker restart can reclaim an expired lease without creating another intent.

The Mattermost driver is separate from incident policy. It accepts an exact host,
path prefix, port, and opaque file-secret reference. Configuration and delivery both
require the host and every DNS answer to match explicit allowlists. Delivery pins the
validated addresses, uses HTTPS with certificate verification, disables proxy
environment and redirects, applies strict time/body/header caps, neutralizes
mentions, escapes operator/source text, and builds incident links only from Core's
configured public URL.

## Consequences

- Incident commit fails if durable intent evidence cannot be written; remote delivery
  never participates in that transaction.
- A remote provider can receive a request whose local completion is lost. The stable
  Espial delivery ID lets operators/provider logs correlate that ambiguity; Espial
  does not automatically replay terminal rows.
- Destination reads and audit summaries are redacted. Replacements require all
  protected endpoint and secret-reference fields again.
- Operators must maintain both host and CIDR allowlists and rotate mounted token files
  deliberately. Empty host/CIDR lists fail closed.
