# Espial Backend Plan

## 1. Purpose

The backend will be a standalone Go service responsible for ingesting infrastructure health data, normalizing it, evaluating alert rules, creating incidents, sending notifications, enforcing access control, and exposing data to the Svelte frontend.

The backend should not contain vendor-specific logic throughout the codebase. Vendor and service integrations should live in separate adapter processes that implement a versioned contract.

## 2. Architectural goals

- Keep Espial Core written in Go.
- Run the backend and frontend as separate processes.
- Communicate with the frontend through a versioned HTTP API.
- Use server-sent events or WebSockets for live status and incident updates.
- Allow adapters to be written in Go, Python, Rust, C, or another suitable language.
- Allow administrators to install or approve adapters without rewriting Espial Core.
- Isolate adapter crashes, timeouts, excessive resource use, and malformed output.
- Support both event-driven ingestion and scheduled checks.
- Maintain useful last-known state when infrastructure sources are temporarily unreachable.

## 3. High-level backend components

Go is a deliberate fit for the core workload. Espial is dominated by concurrent I/O, cancellation, timeouts, protocol handling, and long-running service supervision rather than low-level numerical processing. The implementation should favor clear goroutine ownership, explicit context cancellation, bounded queues, and simple package boundaries.

```text
Espial Core — Go
├── API server
├── authentication and authorization
├── integration registry
├── adapter supervisor
├── scheduler
├── event ingestion service
├── normalization and validation layer
├── health-state engine
├── incident engine
├── notification router
├── dependency resolver
├── inventory service
├── audit logger
├── retention worker
└── database access layer
```

### Suggested Go project layout

```text
core/
├── cmd/espial/                 # Process entry point and dependency wiring
├── internal/api/               # HTTP handlers, middleware, and API models
├── internal/auth/              # SSO, sessions, roles, and break-glass access
├── internal/adapters/          # Protocol, registry, and process supervision
├── internal/scheduler/         # Timers, jitter, and collection dispatch
├── internal/observations/      # Validation and normalized ingestion
├── internal/health/            # Current-state evaluation
├── internal/incidents/         # Rules, lifecycle, and escalation
├── internal/notifications/     # Destination-independent notification routing
├── internal/inventory/         # Logical and physical asset inventory
├── internal/dependencies/      # Typed relationships and affected services
├── internal/audit/             # Structured audit events and exports
├── internal/storage/           # PostgreSQL repositories and transactions
├── internal/retention/         # Downsampling and expiration workers
├── migrations/                 # Versioned database migrations
└── go.mod
```

Keep domain packages inside `internal` unless there is a proven need for a separately versioned public SDK. Adapter protocol schemas should live in a shared, language-neutral location rather than being represented only as Go types.

### Go engineering requirements

- Pass `context.Context` through network, database, adapter, and long-running operations.
- Set explicit timeouts for every external call.
- Give every goroutine a clear owner and shutdown path.
- Use bounded concurrency and backpressure instead of launching unbounded work.
- Wrap errors with operational context while avoiding secret leakage.
- Use structured logs with correlation, integration, resource, and incident identifiers.
- Shut down gracefully: stop accepting work, cancel collectors, flush audit records, and close database connections.
- Run unit tests, integration tests, static analysis, and the race detector in CI.
- Prefer small interfaces at package boundaries rather than broad application-wide abstractions.

### 3.1 API server

Responsibilities:

- Serve versioned REST endpoints under a path such as `/api/v1`.
- Validate requests and responses against stable schemas.
- Expose current state, history, incidents, inventory, integrations, dependencies, and administration operations.
- Stream live events to authenticated frontend clients.
- Enforce rate limits and request size limits.
- Publish an OpenAPI description once the API stabilizes.

### 3.2 Authentication and authorization

Normal access should use UBNetDef SSO. The backend should also provide a separate emergency authentication path because SSO is itself a monitored dependency.

Recommended model:

- Primary authentication through the SSO protocol selected by the SSO team.
- Local break-glass administrator accounts stored with strong password hashing.
- MFA for break-glass access when feasible.
- Break-glass login exposed through a deliberate route such as `/auth/emergency`.
- Alert and audit entry whenever a local emergency account is used.
- Optional network or IP restrictions for emergency access.

Initial roles:

| Role | Capabilities |
|---|---|
| Viewer | Read dashboards, incidents, history, and inventory |
| Operator | Viewer permissions plus acknowledge, assign, comment, silence, and resolve |
| Administrator | Manage integrations, credentials, users, roles, rules, plugins, and layout metadata |
| Action Approver | Reserved for later controlled-action workflows |

Role checks should be enforced by the backend, never only by the frontend.

### 3.3 Integration registry

The registry stores:

- integration type
- display name
- adapter identifier and version
- enabled or disabled state
- configuration schema
- encrypted credentials or credential references
- polling interval or event settings
- produced resource types
- supported checks
- supported actions
- notification destinations
- ownership and approval metadata

Integration configuration should be validated by both the backend and the adapter before activation.

### 3.4 Adapter supervisor

Adapters should run outside the Espial Core process.

The supervisor should:

- start and stop local adapter processes
- perform health checks
- enforce collection timeouts
- capture logs
- validate every response
- limit restart loops
- apply CPU and memory restrictions where supported
- restrict filesystem and network access where practical
- record adapter failures as operational events

A failed adapter must cause affected data to become `unknown` or `stale`; it must not preserve a false healthy status indefinitely.

## 4. Adapter contract

The adapter protocol should be versioned from the beginning.

Two supported transport models are reasonable:

1. JSON messages over standard input and output for locally supervised adapters.
2. Local HTTP for independently managed adapters.

A first adapter contract could include equivalents of:

```text
manifest
validate_config
collect
subscribe_or_receive_event
health
shutdown
```

### 4.1 Adapter manifest

The manifest should declare:

- adapter name and version
- protocol version
- integration category
- supported resource types
- supported check types
- required configuration fields
- secret fields
- optional dashboard widget hints
- supported notification destinations
- supported controlled actions
- read-only or action-capable status

### 4.2 Normalized adapter output

Adapters should emit normalized objects instead of vendor-specific blobs wherever possible.

Core object categories:

- `resource`
- `metric_sample`
- `check_result`
- `inventory_item`
- `relationship`
- `event`
- `incident_candidate`
- `certificate_observation`
- `notification_delivery_result`

Vendor-specific raw data may be stored as optional metadata for troubleshooting, but central logic should rely on normalized fields.

## 5. Health-state model

Recommended states:

- `healthy`
- `warning`
- `critical`
- `unknown`
- `stale`
- `maintenance`
- `disabled`

Every state should include:

- observed timestamp
- source adapter
- reason
- supporting check or metric
- expected refresh interval
- last successful observation

### 5.1 Scheduled and event-driven collection

Use a hybrid model.

Prefer event-driven ingestion when systems support:

- webhooks
- syslog
- SNMP traps
- alert-manager events
- vendor event subscriptions
- internal service events

Use scheduled checks for:

- TLS certificate expiration
- website availability
- API availability
- SSO login-flow checks
- systems without reliable event support

Five minutes is the default acceptable interval for ordinary checks. More urgent checks may use shorter intervals or event subscriptions. Add scheduling jitter so integrations do not all run simultaneously.

## 6. Incident engine

Incidents should be generated automatically from configured rules.

Suggested lifecycle:

```text
Detected → Alerted → Acknowledged → Investigating → Recovered → Resolved
```

An incident should store:

- incident identifier
- title and summary
- severity
- source integration
- affected resource
- affected service relationships
- detection time
- first and latest observation
- state transitions
- notification attempts and results
- acknowledgement and owner
- operator notes
- relevant metric snapshots
- recovery time
- resolution time and notes

The system may mark an incident as technically recovered automatically, while an operator performs final resolution.

### 6.1 Alert behavior

Rules should support:

- per-integration thresholds
- global defaults for simple availability checks
- warning and critical levels
- debounce periods
- recovery conditions
- acknowledgement-aware escalation
- repeat suppression
- maintenance windows
- temporary silences

Recommended notification behavior:

- send an initial alert
- avoid duplicate spam while the state remains unchanged
- escalate after a configurable delay if unacknowledged
- send updates on meaningful severity changes
- send a recovery notification

## 7. Notification adapters

Notification delivery should be modular and independent from monitoring adapters.

Initial target:

- Mattermost

Future targets:

- Discord
- email
- generic webhook
- syslog
- SIEM-specific exporter

Espial Core should create a normalized notification event. Each destination adapter handles formatting and delivery.

A notification record should store:

- incident or event identifier
- destination
- delivery attempt time
- delivery result
- retry count
- response metadata

## 8. Website and certificate monitoring

Certificate and website checks should be a built-in integration category.

Track:

- DNS resolution
- TCP reachability
- TLS negotiation
- certificate expiration
- hostname match
- chain validity
- issuer changes
- unexpected replacement
- HTTP response status
- response time
- optional content match

Expiration thresholds should be configurable, with sensible defaults such as 30, 14, and 7 days.

Endpoints may eventually be entered manually, imported, or discovered from configured services.

## 9. Inventory and dependency model

The backend should support both logical and physical inventory.

### 9.1 Logical resources

Examples:

- service
- application
- VM
- container
- storage pool
- dataset
- network gateway
- certificate endpoint

### 9.2 Physical resources

Examples:

- site
- room
- row
- rack
- physical server
- switch
- storage shelf
- chassis
- drive bay
- drive
- power supply

### 9.3 Relationships

Relationships should be typed, for example:

- `runs_on`
- `depends_on`
- `stored_on`
- `connected_through`
- `located_in`
- `contains`
- `monitored_by`

This supports statements such as:

> The TrueNAS pool is degraded and supports the Proxmox VM that hosts the SSO service.

The system does not need a numeric risk score. It needs transparent dependency relationships and clear affected-service indicators.

## 10. Drive and chassis inventory

Drive inventory is an explicit roadmap requirement.

Store, where available:

- drive serial number
- manufacturer and model
- capacity
- media type
- protocol
- firmware
- chassis identifier
- slot or bay number
- enclosure
- associated server
- associated pool or array
- current health state
- replacement or rebuild state
- first and last observed timestamps

Physical devices should reference reusable chassis templates, such as:

- generic 1U
- generic 2U
- eight-bay server
- twelve-bay server
- twenty-four-bay storage chassis
- model-specific templates for the two primary UBNetDef server types

The backend stores template IDs and slot occupancy. The frontend performs rendering and animation.

## 11. Database recommendation

Use PostgreSQL as the default operational database.

Suggested data groups:

- users, roles, and sessions
- integrations and adapter versions
- encrypted integration configuration
- resources and inventory
- physical locations
- relationships and dependencies
- latest health state
- metric summaries
- checks and rules
- incidents and incident events
- comments and assignments
- notification deliveries
- certificate observations
- audit logs
- chassis templates and drive slots

### 11.1 Retention

Target operational history: six months.

Recommended policy:

- retain incidents and audit history longer than detailed metrics when storage allows
- keep detailed metric samples for 30–90 days
- downsample older metrics into hourly or daily summaries
- expire detailed operational samples by six months
- avoid duplicating all metrics already retained by Grafana or its data source

## 12. Audit and SIEM integration

Audit logging is required for administrative configuration and any non-read-only feature.

Audit events should include:

- actor
- role
- action
- target
- timestamp
- source address or session
- before and after summaries when appropriate
- success or failure
- correlation identifier

Logs should be exportable through:

- structured JSON files
- syslog
- generic webhook
- future SIEM-specific adapters

## 13. Controlled actions: post-MVP

Monitoring adapters should not receive unrestricted command execution.

A future action runner may support approved operations such as:

- semester deployments
- service restart
- backup trigger
- certificate renewal workflow
- maintenance-mode entry
- documented recovery procedures

Every action must have:

- predefined executable or script
- version control
- parameter schema and validation
- role restrictions
- optional two-person approval
- execution timeout
- captured output
- immutable audit record
- no arbitrary shell command field

This subsystem should be designed separately and added only after monitoring and auditing are reliable.

## 14. Deployment

Recommended initial deployment:

```text
Dedicated management VM or small physical server
├── reverse proxy
├── Espial Core Go service
├── SvelteKit frontend service
├── PostgreSQL
└── adapter processes
```

The host should be separated from obvious shared failure domains where practical. Espial Core should persist last-known observations in PostgreSQL so the frontend can remain useful when upstream systems are unavailable. The Go service should be distributed as a single versioned binary or container image and run under a service manager with automatic restart and resource limits.

High availability is not required for the first release. Backups, recovery documentation, and a simple external heartbeat are still recommended.

## 15. Suggested API surface

Illustrative endpoint groups:

```text
/api/v1/auth/*
/api/v1/overview
/api/v1/resources
/api/v1/services
/api/v1/dependencies
/api/v1/incidents
/api/v1/integrations
/api/v1/checks
/api/v1/certificates
/api/v1/notifications
/api/v1/inventory
/api/v1/locations
/api/v1/racks
/api/v1/chassis
/api/v1/drives
/api/v1/audit
/api/v1/events/stream
```

## 16. Go implementation choices

The initial implementation should bias toward the Go standard library and a small dependency set. Exact libraries can be selected through architecture decision records, but the following boundaries should remain stable:

- `net/http`-compatible API and middleware model
- standard `context` cancellation and deadlines
- structured logging through a consistent application logger
- PostgreSQL access through a maintained driver with explicit transactions
- schema-first JSON messages for adapter communication
- operating-system process supervision for local adapters
- server-sent events as the initial one-way live-update mechanism
- configuration loaded from files and environment variables, with secrets supplied separately

Avoid designing the core around a large application framework. Espial benefits more from explicit packages and testable services than from framework-specific conventions.

## 17. Backend implementation order

1. Define normalized schemas and API conventions.
2. Establish the PostgreSQL schema and migrations.
3. Implement authentication, roles, and emergency access.
4. Build resource, inventory, and latest-state storage.
5. Build the adapter protocol and supervisor.
6. Implement scheduled checks and event ingestion.
7. Build the incident engine.
8. Add Mattermost delivery.
9. Add website and certificate monitoring.
10. Add dependencies and affected-service resolution.
11. Add audit export.
12. Add physical inventory, chassis templates, and drive-slot data.
13. Consider controlled actions only after the preceding systems are stable.

## 18. Backend success criteria

The backend is successful when:

- a new adapter can be added without changing Espial Core
- adapter failures cannot crash the backend
- stale data is distinguishable from healthy data
- incidents are generated and recovered predictably
- notifications are traceable and retryable
- SSO failure does not lock administrators out
- all administrative actions are auditable
- physical and logical dependencies can be queried through the same API
- the frontend can render most integrations through normalized data alone
