# Espial Core

This directory is the Go module for the standalone Espial backend. Phase 0 records
the module boundary; service packages arrive in Phase 1 according to the
[implementation plan](../docs/plans/PHASE_1_IMPLEMENTATION.md).

Planned layout:

```text
cmd/espial
internal/api
internal/auth
internal/adapters
internal/scheduler
internal/observations
internal/health
internal/audit
internal/storage
migrations
```

Core owns domain decisions, authorization, auditing, state persistence, adapter
supervision, and live invalidation events. Vendor-specific collection stays outside
this process.
