# ADR-0005: Single-host container deployment with separate processes

- Status: Accepted
- Date: 2026-07-30

## Context

High availability is not an MVP requirement. The frontend, backend, database, and
adapters need separate failure and lifecycle boundaries while remaining practical
to operate on a dedicated UBNetDef management host.

## Decision

Ship the initial deployment as a Compose-compatible container stack on one dedicated
management VM or small physical host. Run the reverse proxy, Espial Web, Espial
Core, PostgreSQL, and each adapter as separate processes/containers on a private
network. Publish only the reverse proxy. Pin image versions and use persistent
volumes for PostgreSQL.

## Consequences

This is a topology decision, not a commitment to a particular reverse proxy or
container runtime. Production enablement requires backup/restore validation, an
external heartbeat, secret files or an approved secret store, resource limits, and
a host chosen outside the most obvious monitored failure domain.
