# ADR-0002: PostgreSQL as the operational store

- Status: Accepted
- Date: 2026-07-30

## Context

Espial needs transactional configuration, authentication, normalized inventory,
latest health, observations, audit records, and later incident histories. These
records have relationships and retention needs that are a poor fit for in-memory
or file-only storage.

## Decision

Use PostgreSQL as the authoritative operational database. Apply forward-only,
versioned SQL migrations before the API starts serving traffic. Keep vendor metric
systems authoritative for high-cardinality telemetry; Espial stores operationally
useful observations and summaries.

## Consequences

Local development and production require PostgreSQL. Repository operations accept
contexts and explicit transactions. Phase 1 must include backup/restore smoke
documentation even though high availability is out of scope.
