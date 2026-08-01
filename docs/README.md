# Espial documentation

This directory is the entry point for project decisions, implementation plans,
design guidance, and operator inputs. The original backend, frontend, and roadmap
documents remain the product-level source material; the folders below turn those
intentions into implementable contracts.

## Start here

| Document | Use it for |
|---|---|
| [Roadmap](ROADMAP.md) | Phase boundaries, ordering, and product scope |
| [Phase 0 record](phase-0/README.md) | Completed design deliverables, decisions, and known external inputs |
| [Phase 1 plan](plans/PHASE_1_IMPLEMENTATION.md) | Work sequencing, acceptance tests, ownership boundaries, and exit criteria |
| [Phase 2 plan](plans/PHASE_2_IMPLEMENTATION.md) | Incident, notification, website, and certificate delivery slices and acceptance gates |
| [Slice 2.1 record](plans/SLICE_2_1_AUTOMATIC_INCIDENTS.md) | Durable signals, automatic lifecycle, incident read APIs, Dashboard/Alerts UI, and verification evidence |
| [Slice 1.3 plan](plans/SLICE_1_3_NORMALIZED_HEALTH.md) | Transactional ingestion, freshness semantics, package boundaries, and test matrix |
| [Slice 1.4 plan](plans/SLICE_1_4_ADAPTER_RUNTIME.md) | Trusted process runtime, protocol negotiation, sample adapter, and conformance gates |
| [Slice 1.5 record](plans/SLICE_1_5_SCHEDULING_PIPELINE.md) | Bounded scheduling, atomic collection/audit, freshness ownership, and internal events |
| [Slice 1.6 record](plans/SLICE_1_6_REST_SSE_API.md) | Authenticated REST read models, stable cursors, administrator audit reads, SSE replay, and OpenAPI |
| [Slice 1.7 record](plans/SLICE_1_7_OPERATIONAL_UI.md) | Authoritative Dashboard, typed SvelteKit loaders, live invalidation refresh, accessibility, and viewport verification |
| [Slice 1.8 record](plans/SLICE_1_8_PHASE_ACCEPTANCE.md) | Hardened deployment, threat review, clean-stack acceptance, resource evidence, and Phase 1 release gates |
| [Backend plan](BACKEND_PLAN.md) | Long-term backend responsibilities and domain direction |
| [Frontend/UI plan](FRONTEND_UI_PLAN.md) | Long-term information architecture and interaction direction |

## Architecture and contracts

- [Architecture index](architecture/README.md)
- [API conventions](architecture/API_CONVENTIONS.md)
- [Adapter protocol](architecture/ADAPTER_PROTOCOL.md)
- [Authentication and authorization](architecture/AUTHENTICATION.md)
- [Operational data model](architecture/DATA_MODEL.md)
- [Deployment baseline](architecture/DEPLOYMENT.md)
- [Versioned JSON schemas](../api/README.md)

## Design

- [Authoritative general UI guidance](design/UI_GUIDANCE.md)
- [Design tokens and status semantics](design/DESIGN_SYSTEM.md)
- [Product wireframes](design/WIREFRAMES.md)
- [Physical infrastructure drill-down](design/PHYSICAL_DRILLDOWN.md)
- [UI review notes](design/UI_REVIEW.md)

## Operations and discovery

- [Initial environment and integration inventory](operations/ENVIRONMENT_INVENTORY.md)
- [Temporary local authentication runbook](operations/LOCAL_AUTH.md)
- [Deployment and upgrade runbook](operations/DEPLOYMENT_RUNBOOK.md)
- [Backup and restore runbook](operations/BACKUP_RESTORE.md)
- [Troubleshooting runbook](operations/TROUBLESHOOTING.md)

## Security

- [Phase 1 threat review and vulnerability triage](security/THREAT_REVIEW_PHASE_1.md)

## Decisions

Architecture decision records are indexed in [adr/README.md](adr/README.md).
Accepted ADRs are the authority when a planning document presents several options.

## Documentation conventions

- Use repository-relative links so documents render in GitHub and local editors.
- Record durable choices in an ADR; record execution order in a plan.
- Put operator-specific facts in `operations/`, not in schemas or application code.
- Mark unknown facts as `TBD (owner)` instead of silently assuming them.
- Update this index when adding a new documentation category.
