# Phase 0: design baseline record

**Baseline date:** 2026-07-30  
**Status:** Complete with two declared external inputs: site inventory values and
the unfinished UBNetDef SSO protocol.

Phase 0 converts the goals in the [roadmap](../ROADMAP.md),
[backend plan](../BACKEND_PLAN.md), and [frontend plan](../FRONTEND_UI_PLAN.md)
into decisions and contracts that can be implemented without inventing architecture
during Phase 1.

## Deliverable map

| Roadmap deliverable | Result |
|---|---|
| Architecture decision records | [ADR index](../adr/README.md), including Go, PostgreSQL, adapter transport, SSE, deployment, and authentication |
| Integration inventory | [Environment and integration inventory](../operations/ENVIRONMENT_INVENTORY.md) |
| Normalized resource and event schemas | [Versioned JSON schemas](../../api/README.md) |
| Adapter protocol specification | [Adapter protocol](../architecture/ADAPTER_PROTOCOL.md) |
| Initial database diagram | [Initial data model](../architecture/DATA_MODEL.md) |
| API conventions | [API conventions](../architecture/API_CONVENTIONS.md) |
| Overview, incident, and infrastructure wireframes | [Phase 0 wireframes](../design/WIREFRAMES.md) |
| Deployment decision | [ADR-0005](../adr/0005-initial-deployment.md) and [deployment baseline](../architecture/DEPLOYMENT.md) |
| Go module and repository conventions | [`core/go.mod`](../../core/go.mod), [repository conventions](../../CONTRIBUTING.md), and component READMEs |
| Roles and permissions | [Authentication and authorization](../architecture/AUTHENTICATION.md) |
| Design tokens and branding baseline | [Design system](../design/DESIGN_SYSTEM.md) |

## Explicit external inputs

Phase 0 cannot manufacture facts owned by infrastructure or SSO teams. These are
tracked, with owners and when they become blocking, in the
[environment inventory](../operations/ENVIRONMENT_INVENTORY.md):

1. Host counts, endpoints, supported event mechanisms, credentials strategy, and
   the first monitored website list. These are required before a real adapter is
   configured, but sample data can drive the first vertical slice.
2. The SSO protocol, claims, group mapping, logout behavior, and test environment.
   This is not required for the Phase 1 local-auth vertical slice. The auth provider
   seam, session model, roles, and SSO acceptance contract are defined now.

No unknown in this list is treated as a security default. Production rollout must
close every item labeled `Production blocker` in the inventory.

## Phase 1 handoff

Implementation should follow the [Phase 1 plan](../plans/PHASE_1_IMPLEMENTATION.md).
The first demonstrable slice is intentionally narrow: local login, a supervised
sample adapter, persisted current health, a protected overview, and live refresh.
