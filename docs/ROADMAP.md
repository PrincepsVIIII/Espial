# Espial Project Roadmap

## Roadmap philosophy

The project should establish a reliable monitoring foundation before adding advanced physical visualization or remote actions.

The order of work is intentional:

1. Define stable data contracts.
2. Make monitoring trustworthy.
3. Make incidents and alerts operationally useful.
4. Add inventory and dependency context.
5. Add advanced visualization.
6. Add tightly controlled actions only after auditing is mature.

## Phase 0: Discovery and design

**Status:** Design baseline complete (2026-07-30). See the
[Phase 0 record](phase-0/README.md). Site-specific inventory values and the final
SSO protocol remain externally supplied inputs and are explicitly tracked; they
do not block the local-authenticated Phase 1 vertical slice.

### Goals

- document the current environment
- identify API and event capabilities for each source
- confirm SSO protocol
- inventory initial systems and websites
- define the first roles and permissions
- establish UBNetDef design tokens and branding assets

### Deliverables

- architecture decision records, including the Go core decision
- integration inventory
- normalized resource and event schemas
- adapter protocol specification
- initial database diagram
- API conventions
- wireframes for overview, incident, and infrastructure views
- deployment decision
- Go module, repository layout, formatting, testing, and CI conventions

## Phase 1: Monitoring foundation

**Status:** Complete (2026-07-31). The automated ten-step acceptance scenario,
race suite, dependency/application-image security gates, and operator runbooks pass.
The production example is intentionally not release-approved until the owned site
inputs and upstream base-image findings in the
[Phase 1 threat review](security/THREAT_REVIEW_PHASE_1.md) are closed.

**Implementation plan:** [Phase 1 Monitoring Foundation](plans/PHASE_1_IMPLEMENTATION.md)

### Backend

- Go module and Espial Core service skeleton
- initial `cmd/espial` entry point and internal package layout
- graceful startup and shutdown with context cancellation
- PostgreSQL migrations
- configuration management
- authentication integration
- local break-glass access
- role enforcement
- normalized resource and health state storage
- adapter protocol and supervisor
- scheduler with jitter
- event ingestion path
- stale and unknown state handling
- audit logging foundation

### Frontend

- temporary public informational entry page with sign-in at the top right
- SvelteKit application shell
- UBNetDef dark NOC theme
- dark top navigation: Dashboard, Alerts, Datacenter, Hypervisor, Webpages, and
  permission-gated Audit
- Dashboard as the default authenticated destination; Alerts and Datacenter remain
  one primary navigation action away and show honest unavailable/not-configured
  states until their domain phases are implemented
- authenticated session states
- dashboard summary page
- integration health display
- basic resource and status tables
- live update connection status

### Exit criteria

- frontend and backend run as separate processes
- one sample adapter can report health
- failed adapter data becomes unknown or stale
- roles are enforced by the backend
- operators can see current state through the Svelte UI

## Phase 2: Incidents, notifications, and certificates

Groundwork already established before Phase 2 feature work:

- Alerts is a direct primary route with an honest unavailable state until incident
  read models exist;
- Core already defines the `incidents:operate` authorization boundary;
- administrator Audit and Users routes provide inspectable request-correlated
  evidence for access changes; and
- the shell accepts only implemented dropdown children, so Phase 2 destinations
  must ship with their real route, permission boundary, loading/error states, and
  backend contract rather than appearing as planned navigation.

### Backend

- incident state machine
- configurable rules and thresholds
- acknowledgement and ownership
- incident notes
- maintenance windows and silences
- Mattermost notification adapter
- notification retries and delivery history
- website availability checks
- TLS certificate checks
- recovery notifications

### Frontend

- active incident list
- incident details and timeline
- acknowledgement, assignment, notes, and resolution controls
- notification delivery status
- certificate status page
- filters for severity, status, source, and owner

### Exit criteria

- meaningful failures generate incidents automatically
- incidents are sent to Mattermost
- duplicate alert spam is controlled
- certificate expiration and website outages are visible
- operators can acknowledge and investigate incidents

## Phase 3: Core infrastructure integrations

Prioritize integrations based on environment value and API quality.

Candidate order:

1. UBNetDef SSO
2. Proxmox
3. TrueNAS
4. iDRAC
5. Grafana or its underlying metric source
6. pfSense

### Goals

- collect high-value operational signals
- avoid copying all metrics already retained elsewhere
- provide source-system deep links
- support integration-specific thresholds
- document adapter development for students

### Exit criteria

- Espial represents the primary UBNetDef infrastructure
- adding an adapter does not modify Espial Core
- adapter failures are isolated and auditable
- common integrations render with standard frontend components

## Phase 4: Dependencies and affected services

### Backend

- logical service inventory
- typed dependency relationships
- affected-service resolution
- manual relationship administration
- relationship API

### Frontend

- service pages
- dependency list or graph
- “show affected services” toggle
- incident-to-service linking
- service status summaries

### Exit criteria

- operators can identify which services rely on a failed component
- Espial can connect physical, virtual, storage, network, and application resources
- no numerical risk score is required

## Phase 5: Physical inventory and rack visualization

### Backend

- sites, rooms, rows, racks, and rack units
- physical device records
- models, serials, asset tags, and roles
- rack placement
- device-to-service and device-to-resource relationships
- chassis template references

### Frontend

- neutral-gray room overview generated from inventory data
- hover/focus highlighting and selection from room to rack
- rack grid component with a straight-on rack focus view
- room selector, breadcrumbs, and direct URLs
- device detail drawer
- filter chips
- hover and focus tooltips
- incident highlighting
- affected-services toggle in physical views

### Exit criteria

- administrators can locate an affected device physically
- rack views are generated from inventory data
- room and rack health are visible without manually maintained drawings
- selection remains usable when animation is disabled

## Phase 6: Chassis and drive-slot visualization

### Backend

- reusable chassis template schema
- drive bays and slot numbering
- drive serial numbers
- drive model, capacity, and health
- enclosure and pool associations
- replacement and rebuild state

### Frontend

- model-aware server graphics for the two main UBNetDef server types
- generic templates for additional hardware
- front and rear chassis views
- per-slot drive occupancy
- separate power, device-health, drive-state, and activity indicators
- chassis-specific drive-bay inspection motion with a no-motion fallback
- failed or degraded drive highlighting
- drive details covering identity, health, pool association, and data freshness

### Exit criteria

- an operator can identify the exact server, slot, and serial number for a failed drive
- templates are reusable rather than tied to hostnames
- new chassis types can be added without redesigning the entire page
- server and drive views are directly addressable and usable with motion disabled

## Phase 7: Spatial editing and visual refinement

### Features

- rack positioning editor
- rack-level incident highlighting
- simple overlays for service, network, power, or cooling zones
- refined room-to-rack-to-chassis transitions
- refined chassis expand/open animations
- smooth front/rear transitions and guided camera motion
- guided focus on affected hardware
- dependency path highlighting

### Guardrail

Animations and advanced visual polish must not delay access to data or become a prerequisite for operating the dashboard.

## Phase 8: SIEM and operational exports

### Features

- structured audit export
- syslog delivery
- generic SIEM adapter
- incident export
- configurable event schemas
- correlation identifiers

### Exit criteria

- administrative and action events can be consumed by external security tooling
- exports include enough context for investigation without exposing unnecessary secrets

## Phase 9: Controlled actions and semester deployments

This is intentionally post-MVP.

### Features

- approved action catalog
- versioned deployment scripts
- parameter schemas
- role restrictions
- optional two-person approval
- execution logs
- timeouts
- signed or checksummed releases
- immutable audit history

### Guardrails

- no arbitrary command text box
- no action capability implicitly granted to monitoring adapters
- actions disabled by default
- secrets scoped to the smallest possible operation
- all output and state changes logged

## Cross-phase workstreams

### Security

- threat modeling
- credential storage
- adapter sandboxing
- dependency updates
- session security
- least privilege
- audit review

### Documentation

- adapter authoring guide
- integration manifest reference
- deployment guide
- administrator guide
- incident workflow guide
- recovery guide
- frontend component guidelines
- branding and design tokens

### Testing

- schema tests
- adapter conformance tests
- incident rule tests
- notification retry tests
- authentication and role tests
- UI accessibility tests
- end-to-end failure simulations
- stale-data and partial-outage tests
- Go race-detector and goroutine-leak checks
- graceful-shutdown and cancellation tests

## Suggested MVP boundary

The MVP should stop after Phases 1–3, with a small part of Phase 4 if time allows.

MVP includes:

- separate Go backend and SvelteKit frontend
- SSO plus break-glass authentication
- role-based access
- adapter framework
- core integrations
- incidents
- Mattermost alerts
- certificate monitoring
- audit logs
- current and historical dashboard views

MVP does not require:

- full room layout
- detailed chassis animations
- every possible vendor integration
- automatic dependency discovery
- high availability
- public status pages
- remote actions

## Immediate next decisions

Before implementation begins, resolve:

- approximate number of hosts, VMs, storage systems, websites, and users
- SSO protocol
- Grafana data sources
- pfSense edition and available integration methods
- certificate discovery method
- exact alert repetition and escalation defaults
- deployment host and backup strategy
- initial server chassis models and bay layouts
- ownership of adapter approval and code review
- initial adapter transport: supervised JSON over standard I/O or local HTTP
