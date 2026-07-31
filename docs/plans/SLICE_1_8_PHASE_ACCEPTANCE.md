# Slice 1.8 record: deployment, hardening, and Phase 1 acceptance

**Status:** Implemented; production release remains blocked by the explicitly owned
site inputs and upstream image findings below.  
**Parent:** [Phase 1 implementation plan](PHASE_1_IMPLEMENTATION.md)  
**Acceptance date:** 2026-07-31

## Delivered

- `npm run init` performs empty-clone local initialization with generated secrets,
  migrations, hidden administrator bootstrap, health checks, and no default password.
- The local stack runs PostgreSQL, Core, Web, and the deterministic sample adapter
  with Core/Web health dependencies and configurable trusted public origin.
- The production reference publishes only an SSE-aware TLS proxy and uses pinned
  images, private networking, read-only/non-root application runtimes, dropped
  capabilities, resource/log limits, persistent data, secret files, a one-shot
  migration owner, and a least-privilege runtime database role.
- Audited CLI flows create users, assign roles, reset passwords, and enable/disable
  users while revoking affected sessions.
- Database migration 000007 prevents application update/delete of audit history.
- Weekly/on-main security and end-to-end acceptance workflows use commit-pinned
  actions and a digest-pinned scanner.
- Operator runbooks cover bootstrap/recovery, deployment/upgrades/rollback,
  backup/restore, logs/adapter diagnosis, and complete shutdown.

## Automated vertical acceptance

`npm run acceptance:phase1` builds isolated images and uses unique ports, projects,
secrets, volumes, and users. It exercises all ten planned steps:

1. empty PostgreSQL plus all migrations;
2. non-echoed administrator bootstrap;
3. Web login and direct anonymous Core `401`;
4. two sample integrations and healthy/warning state in PostgreSQL and Web SSR;
5. adapter failure, durable degraded integration, real stale then unknown resource
   transitions while Core stays ready;
6. recovery observed through SSE and healthy resource state;
7. audited user creation/role assignment and direct Viewer administrator `403`;
8. bootstrap/login/lifecycle/failure/recovery/role audit evidence with no password;
9. termination during collection and bounded graceful Core shutdown;
10. logical backup restore into a second clean stack with auth and state preserved.

The final 2026-07-31 run passed. Core stopped in 647 ms during collection. The harness
cleans only its uniquely named disposable projects and volumes.

## Resource/default evidence

The acceptance sample observed approximately 21.7 MiB Web, 60.8 MiB Core, and
30.8 MiB PostgreSQL resident memory. Sample CPU was transiently 0%, 1.00%, and
1.35%, respectively. The production limits (256 MiB Web/proxy, 1 GiB Core, 2 GiB
PostgreSQL) deliberately retain large initial headroom until site scale is known.

Phase 1 bounds are: 20 database connections, four concurrent adapter collections,
30-second collection deadline, 1 MiB adapter message, 100-record freshness batch,
1,024 replay events, 100 SSE clients, 15-second heartbeat, and five 10 MiB JSON log
files per container. Scheduler work is coalesced rather than queued without bound;
adapter output/restart loops and HTTP bodies/cursors are also bounded. Scale inputs
and a six-month audit capacity estimate remain production blockers, not guessed
defaults.

## Security evidence and release blockers

Root/Web production dependency audits are clean at the supported threshold;
`govulncheck` reports no reachable Espial-code vulnerability; repository secret/
misconfiguration scanning passes; release-shaped Espial Core/Web images have zero
high/critical findings. See the [threat review](../security/THREAT_REVIEW_PHASE_1.md)
for controls and current official Caddy/PostgreSQL findings.

Production is intentionally blocked until Operations/Security close the named host,
DNS/TLS, backup target/retention, secret manager, external heartbeat, adapter egress,
site-scale, immutable Espial release images, and upstream-image scan decisions in
the [environment inventory](../operations/ENVIRONMENT_INVENTORY.md). Continued local
development and Phase 2 domain work do not depend on those site facts.

## Reproduce the gates

```sh
npm ci
npm --prefix web ci
npm run check
npm run acceptance:phase1
npm audit --omit=dev --audit-level=high
npm --prefix web audit --omit=dev --audit-level=high
```

The Security workflow additionally runs `govulncheck` and Trivy repository/image
scans. The acceptance workflow repeats the clean-stack scenario weekly and on main.

## External exercise record

The author automated every destructive/recovery command so another operator can
dry-run it without production access. A non-author must record their date, commit,
platform, `npm run acceptance:phase1` result, and any runbook correction here before
the first production release. This is a production-release sign-off item; it does
not weaken the automated Phase 1 implementation gate.
