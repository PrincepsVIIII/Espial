# Espial

**Espial** is an internal infrastructure operations and observability platform for UBNetDef administrators, instructors, and TAs.

Espial aggregates health information from infrastructure platforms, generates incidents and alerts, retains operational history, and helps technical users identify both the logical and physical location of failures.

## Project goals

- Provide one place to see what is healthy, degraded, unavailable, unknown, or stale.
- Correlate infrastructure failures with affected services.
- Support modular integrations without requiring changes to the Go core.
- Deliver alerts through interchangeable notification adapters such as Mattermost or Discord.
- Track incidents, recovery, operator actions, and audit events.
- Track website availability and TLS certificate health.
- Present physical infrastructure through rack, room, chassis, and drive-slot views.
- Remain usable when the primary SSO service is unavailable.
- Establish a modern UBNetDef visual standard without resembling a generic AI-generated dashboard.

## Technology decisions

| Layer | Choice | Role |
|---|---|---|
| Core backend | Go | Scheduling, ingestion, health evaluation, incidents, authentication, APIs, auditing, and adapter supervision |
| Frontend | SvelteKit with TypeScript | NOC views, incidents, inventory, rack layouts, chassis views, and administration |
| Database | PostgreSQL | Configuration, inventory, incidents, audit records, latest state, and summarized history |
| Integrations | Language-agnostic adapter processes | Vendor APIs, event sources, notification destinations, and future student-built extensions |
| Live updates | Server-sent events initially | Push current status and incident changes to the frontend |

Go is the preferred core language because Espial is primarily a concurrent network service: it coordinates API calls, event streams, timers, database operations, and notification delivery. Go provides a straightforward concurrency model, strong standard networking support, memory safety, a simple deployment artifact, and a lower contributor barrier than a C or Rust service of similar scope.

## Planned architecture

```text
Infrastructure systems and external services
  ├── iDRAC
  ├── Proxmox
  ├── TrueNAS
  ├── Grafana / metrics sources
  ├── UBNetDef SSO
  ├── pfSense
  ├── Websites and TLS certificates
  └── Future student-built integrations
               │
               ▼
Language-agnostic adapter processes
               │
               ▼
Espial Core — Go
  ├── scheduling and event ingestion
  ├── normalized inventory and health state
  ├── incident engine
  ├── notification routing
  ├── authentication and authorization
  ├── audit logging
  ├── persistence
  └── REST and live-event API
               │
               ▼
Espial Web — SvelteKit
  ├── NOC overview
  ├── incidents
  ├── services and dependencies
  ├── infrastructure inventory
  ├── rack and room visualization
  ├── device and drive inspection
  └── administration
```

The frontend and backend run as separate processes. The browser-facing SvelteKit application communicates with the Go API over authenticated HTTP and receives live state changes through a dedicated event stream.

## Documentation

- [Documentation index](docs/README.md)
- [Project roadmap](docs/ROADMAP.md)
- [Phase 0 design baseline](docs/phase-0/README.md)
- [Phase 1 implementation plan](docs/plans/PHASE_1_IMPLEMENTATION.md)
- [Backend plan](docs/BACKEND_PLAN.md)
- [Frontend and UI plan](docs/FRONTEND_UI_PLAN.md)

## Project status

Phase 0's architecture and design baseline is complete, and Phase 1 implementation
is in progress. The repository now includes contract checks, generated TypeScript
models, CI, the initial Go service lifecycle, PostgreSQL migrations, health probes,
and a local Core/PostgreSQL stack. Environment-specific values
that require UBNetDef operator input are recorded in the
[environment inventory](docs/operations/ENVIRONMENT_INVENTORY.md), rather than
being guessed. SSO is an explicit external dependency; Phase 1 will begin with
audited local authentication and retain a provider boundary for SSO when its
protocol is ready.

Implementation is organized in the
[Phase 1 plan](docs/plans/PHASE_1_IMPLEMENTATION.md). Espial is not yet a
production-ready service.

## Development quick start

Requirements: Docker, Node.js 22.17, and npm. A host Go installation is optional;
the repository commands use the pinned Go 1.26 container when Go is unavailable.

```sh
cp .env.example .env
npm install
npm run check
npm run dev
```

Core is then available on `http://127.0.0.1:18080` by default:

```sh
curl http://127.0.0.1:18080/api/v1/health/live
curl http://127.0.0.1:18080/api/v1/health/ready
```

Run `npm run down` to stop the local stack. The Makefile provides optional aliases
for environments that already have `make`. See the
[local deployment guide](deployments/local/README.md) for configuration and data
retention details.

## Guiding principles

1. **The Go core stays stable.** New infrastructure integrations should not require rebuilding or modifying central domain logic.
2. **Adapters are isolated processes.** A failed student-written integration must not crash Espial Core.
3. **Unknown is not healthy.** Stale or missing data must be clearly represented.
4. **Monitoring comes before control.** Read-only visibility is the initial focus; approved actions are a later, isolated subsystem.
5. **Inventory drives visualization.** Rack and chassis views must be generated from structured data, not manually maintained drawings.
6. **Operational clarity over decoration.** Every interface element should help operators detect, understand, or investigate a problem.
7. **Prefer event delivery, tolerate polling.** Consume events when integrations support them and schedule checks when they do not.
8. **Use explicit boundaries.** The core, adapters, frontend, database, and future action runner must have versioned contracts and independent failure handling.

## Initial scope

The first usable release should include:

- SSO authentication with a controlled local break-glass path
- configurable roles
- integration registration and configuration
- scheduled and event-driven health ingestion
- normalized resource, check, and incident models
- Mattermost notifications
- website and TLS certificate checks
- incident history and acknowledgement
- service dependency relationships
- dashboard overview and detail views
- audit logging
- a six-month operational retention policy

Advanced hardware visualization, custom Svelte integration pages, SIEM export, and controlled deployment actions can follow after the monitoring foundation is reliable.

## Working repository shape

```text
espial/
├── core/                  # Go module for Espial Core
├── web/                   # SvelteKit and TypeScript application
├── adapters/              # Official and example language-agnostic adapters
├── api/                   # Shared schemas and protocol documentation
├── deployments/           # Service, container, and reverse-proxy examples
├── docs/                  # Architecture and contributor documentation
└── README.md
```
