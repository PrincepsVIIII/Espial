# ADR-0004: Server-sent events for initial live updates

- Status: Accepted
- Date: 2026-07-30

## Context

The browser needs one-way notifications that current resource or integration state
changed. Commands remain normal authenticated HTTP requests. WebSockets would add
bidirectional lifecycle complexity without a Phase 1 requirement.

## Decision

Expose authenticated server-sent events at `/api/v1/events/stream`. Events carry
an ID and a small invalidation payload. The frontend treats an event as a prompt to
refetch authoritative REST data and reconnects with bounded backoff.

## Consequences

The reverse proxy must disable response buffering and allow long-lived connections.
Clients show disconnected/reconnecting state. A future transport can replace SSE
without changing REST resources or domain rules.
