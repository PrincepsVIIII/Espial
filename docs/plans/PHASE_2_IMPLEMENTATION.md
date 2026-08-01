# Phase 2 implementation plan: incidents, notifications, and certificates

**Status:** In progress (2026-08-01)
**Inputs:** [Roadmap](../ROADMAP.md),
[Phase 1 implementation record](PHASE_1_IMPLEMENTATION.md),
[backend plan](../BACKEND_PLAN.md), [frontend/UI plan](../FRONTEND_UI_PLAN.md),
[API conventions](../architecture/API_CONVENTIONS.md),
[authentication contract](../architecture/AUTHENTICATION.md), and the authoritative
[UI guidance](../design/UI_GUIDANCE.md)

## Implementation progress

Update this table when a slice starts and again when it is accepted. Evidence must
link to tests, operational records, or a detailed slice record; code presence alone
does not make a slice complete.

| Slice | Status | Evidence |
|---|---|---|
| 2.0 Contracts, permissions, and durable work foundations | Implemented | [Slice 2.1 record](SLICE_2_1_AUTOMATIC_INCIDENTS.md) and [ADR 0007](../adr/0007-durable-monitoring-signal-journal.md) |
| 2.1 Automatic incident creation and read path | Implemented | [Slice 2.1 implementation record](SLICE_2_1_AUTOMATIC_INCIDENTS.md) |
| 2.2 Operator incident workflow | Implemented | [Slice 2.2 record](SLICE_2_2_OPERATOR_INCIDENT_WORKFLOW.md) |
| 2.3 Rules, maintenance windows, and silences | Implemented | [Slice 2.3 record](SLICE_2_3_RULES_AND_SUPPRESSIONS.md) |
| 2.4 Notification routing and Mattermost delivery | Implemented | [Slice 2.4 record](SLICE_2_4_NOTIFICATION_ROUTING.md) and [ADR 0008](../adr/0008-durable-notification-outbox.md) |
| 2.5 Website availability monitoring | Implemented (2026-08-01) | [Slice 2.5 record](SLICE_2_5_WEBSITE_AVAILABILITY.md) and [ADR 0009](../adr/0009-website-monitor-integration-projection.md) |
| 2.6 Certificate monitoring and Webpages experience | Implemented (2026-08-01) | [Slice 2.6 record](SLICE_2_6_CERTIFICATE_MONITORING.md) and [ADR 0010](../adr/0010-certificate-resource-projection.md) |
| 2.7 Phase integration, hardening, and acceptance | Planned | — |

## 1. Outcome

Phase 2 delivers a second production-shaped vertical slice:

1. An authoritative health signal is durably evaluated against a bounded rule.
2. A sustained meaningful failure creates one incident rather than repeated alerts.
3. Viewers can inspect the incident and its factual timeline; Operators can
   acknowledge, assign, investigate, note, and resolve it through Core-authorized
   workflows.
4. Maintenance windows prevent expected work from opening incidents, while silences
   suppress delivery without hiding the underlying incident.
5. A durable notification outbox delivers initial, severity-change, and recovery
   messages to Mattermost with bounded retries and inspectable history.
6. A supervised website-check adapter reports DNS, TCP, TLS, and HTTP availability.
7. Certificate identity and expiry are stored as normalized observations and shown
   under Webpages with warning and critical thresholds.
8. Dashboard, Alerts, Webpages, Audit, and live invalidations agree after retries,
   process restarts, concurrent operator actions, and Core/Web reconnects.

The phase turns trustworthy monitoring state into an operational incident workflow.
It does not add dependency impact, physical inventory, SSO, vendor infrastructure
adapters, or remote actions.

## 2. Scope boundaries

### Included

- Durable monitoring-signal journal produced by observation and freshness commits
- Bounded, declarative incident rules with deterministic precedence, debounce, and
  recovery conditions
- Incident lifecycle, current owner, immutable timeline, operator notes, and
  optimistic concurrency
- Active/history queries and filters for severity, status, source, and owner
- One-time UTC maintenance windows and expiring notification silences
- Durable notification intents, attempts, retry scheduling, and terminal delivery
  history
- One administrator-configured Mattermost destination type using secret references
- Supervised website checks for DNS, TCP, TLS, HTTP status/latency, and bounded
  optional exact content matching
- Certificate validity, hostname match, issuer/fingerprint change, and expiry
  thresholds
- Alerts and Webpages routes backed by authoritative Core read models
- Dashboard incident and certificate summaries only after those read models exist
- Audit events and correlation receipts for incident, rule, suppression,
  destination, and monitor mutations
- Phase-specific migrations, schemas, OpenAPI, generated frontend types, runbooks,
  threat review, and clean-stack acceptance

### Not included

- Recurring calendar maintenance, iCalendar ingestion, or timezone/DST scheduling;
  Phase 2 windows use explicit UTC start and end timestamps
- Arbitrary rule-expression languages, user-supplied code, SQL, or regular
  expressions
- Alert escalation/on-call schedules, paging rotations, email, Discord, generic
  webhooks, SIEM export, or inbound Mattermost commands
- Certificate renewal or any other remote action
- Synthetic browser journeys, JavaScript rendering, website screenshots, website
  performance budgets, or public status pages
- Service dependency/affected-service resolution, incident-to-service linking, and
  physical incident overlays; those begin in Phase 4 and later
- Production vendor infrastructure adapters or UBNetDef SSO
- High availability or exactly-once effects at the remote Mattermost server;
  Espial provides idempotent intent creation and at-least-once delivery attempts

Content matching is an exact, bounded byte-string check against a capped response
body. Regex and script execution are outside Phase 2. Website URLs cannot contain
userinfo or secret query values; authenticated checks use configured secret header
references.

## 3. Phase decisions and invariants

### 3.1 Durable work, not live-stream coupling

The Phase 1 SSE hub remains a browser invalidation mechanism only. Incident and
notification correctness must not depend on an in-memory event subscriber.

- Every accepted authoritative observation and every freshness transition appends a
  `monitoring_signal` in the same database transaction as the health write.
- The incident evaluator claims pending signals with bounded batches and
  database-safe ownership. A crash releases work for replay.
- Rule evaluation is idempotent by signal ID. Incident creation is additionally
  protected by a stable fingerprint and a partial unique constraint.
- An incident timeline change that requires notification creates its notification
  intent in the same transaction.
- Only committed incident, suppression, and delivery changes publish SSE
  invalidations. Consumers refetch Core read models.

The journal is a narrow Phase 2 mechanism, not a generic event-sourcing rewrite.
The current PostgreSQL rows remain authoritative.

### 3.2 Incident rule model

Rules match normalized, bounded fields only:

- integration ID, resource ID or resource kind;
- check type;
- observed/effective health state; and
- optional reason code from a controlled enum when a check defines one.

Each condition maps a state to an incident severity and may require both a minimum
duration and a minimum number of authoritative evaluations. Recovery similarly
requires a configured healthy duration/evaluation count. No rule parses arbitrary
metadata.

When rules overlap, Core selects one deterministically by exact resource match,
then integration/check/resource-kind specificity, then explicit priority, then
opaque rule ID. The read API exposes the winning rule and an administrator preview
endpoint explains the match before a rule is enabled.

Seed one conservative global health rule:

- `critical` opens a critical incident immediately;
- persistent `warning` opens a warning incident after two authoritative
  evaluations;
- `unknown` after the existing freshness threshold opens a warning incident;
- `stale` remains visible but does not open a second incident by default; and
- two authoritative healthy evaluations are required for recovery.

Administrators may disable or replace the default. Website/certificate slices add
specific rules without silently changing the global rule.

### 3.3 Incident identity and lifecycle

The active fingerprint is `(rule_id, resource_id, check_type)`. At most one
non-resolved incident exists for it.

```text
Open → Acknowledged → Investigating → Recovered → Resolved
  └──────────── severity/owner/note changes ────────────┘
                         Recovered → Open (condition recurs)
```

- Detection creates `open`; delivery state is not an incident status.
- Acknowledge records actor and time. Investigating is an explicit Operator action.
- Assignment changes the current owner and appends immutable history.
- Notes are append-only plain text; editing/deleting notes is outside Phase 2.
- Core marks `recovered` only after the rule's recovery condition holds.
- Operators resolve a recovered incident with a required resolution note. An active
  condition cannot be manually presented as resolved; use a silence or maintenance
  window instead.
- A recurrence before resolution reopens the same incident and records recurrence.
  A later failure after resolution creates a new incident.
- Severity may rise or fall with evidence while preserving the same incident and
  recording the change.

Every mutable incident has a monotonic version exposed as an `ETag`. Operator
actions require `If-Match`; stale actions return `412` with safe current-version
context. Retried actions use `Idempotency-Key`.

### 3.4 Maintenance and silence semantics

- A maintenance window has a scope, reason, creator, UTC start, UTC end, and
  enabled/revoked state. Phase 2 scopes are integration, resource, or check type.
- During active maintenance, raw current health is preserved. Read models expose an
  effective `maintenance` state and the window that caused it.
- Matching failures during maintenance do not open or worsen an incident. Each
  decision remains traceable in rule evaluation evidence. At window end, Core
  evaluates the current authoritative health immediately; a still-failing check
  starts normal debounce from the end of maintenance.
- A silence has incident/rule/resource scope, reason, creator, and expiry. It never
  changes health or incident status and never prevents incident creation.
- Notification intents covered by a silence become `suppressed` with the matching
  silence ID; they are not queued for later surprise delivery. Recovery remains in
  the incident timeline even when its notification is suppressed.
- Revoking either control is audited. Existing incidents are not deleted or
  rewritten.

### 3.5 Notification guarantees

The vendor-neutral router consumes normalized incident events. Mattermost is the
first destination driver behind a small interface; incident packages do not format
Mattermost payloads or perform network calls.

- Intent uniqueness is `(incident_event_id, destination_id)`.
- Delivery states are `queued`, `attempting`, `delivered`, `retry_wait`, `failed`,
  `dead_letter`, and `suppressed`.
- Initial detection, meaningful severity change, recurrence, and recovery create
  intents. Acknowledgement, assignment, and notes remain timeline-only by default.
- Retryable transport errors and `429`/`5xx` responses use bounded exponential
  backoff with jitter and a capped `Retry-After`; default maximum is six attempts.
- Other `4xx` responses fail permanently. Timeouts, body sizes, redirects, and
  concurrent sends are bounded.
- A process crash can repeat a remote request. Messages therefore include a stable
  Espial event ID and incident link so operators can recognize a duplicate; the
  database never creates duplicate intents.
- Stored response evidence is limited to status, safe error category, attempt time,
  duration, and a bounded provider request identifier. Response bodies and webhook
  secrets are never persisted.

Mattermost destinations store an HTTPS endpoint as an administrator-approved host
and path policy plus a secret reference, not a webhook URL containing a token.
Redirects and ambient proxy environment variables are disabled. Private/internal
addresses require an explicit deployment allowlist because Mattermost may be
internal; validation and connection use the same resolved-address policy to resist
SSRF and DNS rebinding.

### 3.6 Website and certificate checks

Website monitoring is a trusted, supervised adapter rather than browser-side fetch
logic or vendor-specific Core code. It emits normalized resources and observations
through the existing adapter protocol. If certificate-specific fields cannot be
represented compatibly, add an additive v1 protocol payload and fixtures before
the adapter relies on it.

Each monitor has an exact URL, interval, timeout, allowed status set, optional exact
content value, redirect policy, approved DNS/address policy, and optional secret
header references. The adapter:

1. resolves DNS under a deadline and policy;
2. connects to an approved resolved address without a second uncontrolled lookup;
3. performs TLS with the configured hostname and system/admin-approved trust roots;
4. records HTTP status, elapsed time, and bounded check outcome;
5. reads only the capped bytes needed for an optional content match; and
6. emits no response body, credential, header value, or URL secret.

Redirects are off by default and, when enabled, every hop is revalidated with a
small hop limit. Configuration rejects URL userinfo and secret query parameters.

Certificate observations store endpoint, subject/SAN summary, issuer, serial or
fingerprint, validity interval, hostname validity, chain validity, and observed
time. Private keys and full unbounded chains are never collected. Default expiry
behavior is warning at 30 days, critical at 14 days, and a meaningful critical
update at 7 days; thresholds are validated per monitor. Unexpected replacement or
issuer change is informational evidence unless an administrator enables a rule for
it.

### 3.7 Authorization and receipts

Add explicit read permissions rather than treating the reserved operate permission
as read access:

| Permission | Viewer | Operator | Administrator |
|---|:---:|:---:|:---:|
| `incidents:read`, `webpages:read` | Yes | Yes | Yes |
| `incidents:operate` | No | Yes | Yes |
| `incident_rules:manage`, `suppressions:manage` | No | No | Yes |
| `notification_destinations:manage`, `website_monitors:manage` | No | No | Yes |

Migration code adds permissions to the built-in roles without replacing any local
custom role JSON. Every handler declares and directly tests its permission.

Incident operator mutations are audited. Rule, maintenance, silence, destination,
and website-monitor mutations are administrative: the API and UI expose the
request/correlation receipt and administrators can follow it to
`/audit?correlation_id=...`. A user without `audit:read` still receives the request
ID but not an audit link they cannot open.

## 4. Target repository additions

```text
core/internal/
├── signals/                   durable monitoring signal journal/claims
├── incidents/                 rules, lifecycle, timeline, read models
├── suppressions/              maintenance and silence evaluation
├── notifications/             intents, router, retries, destination interface
│   └── mattermost/            normalized Mattermost delivery driver
└── certificates/              certificate projection/read service

adapters/webcheck/             supervised DNS/TCP/TLS/HTTP adapter

web/src/
├── lib/components/incidents/  compact incident rows, timeline, actions
├── lib/components/webpages/   availability and certificate tables/details
├── routes/(app)/alerts/
│   ├── [id]/
│   ├── history/
│   ├── rules/
│   ├── suppressions/
│   └── notifications/
└── routes/(app)/webpages/
    ├── [id]/
    ├── certificates/
    └── monitors/
```

Package names may be refined, but ownership may not blur: health evaluation emits
signals but does not own incident policy; incidents create notification intents but
do not call Mattermost; the website adapter performs checks but does not decide
incident lifecycle; Web never evaluates rules or certificate trust.

## 5. API and data contract target

### 5.1 Planned API surface

All collections use the existing cursor, limit, error, request ID, CSRF/origin,
idempotency, and `ETag` conventions.

```text
GET  /api/v1/incidents
GET  /api/v1/incidents/{id}
GET  /api/v1/incidents/{id}/timeline
POST /api/v1/incidents/{id}/acknowledge
POST /api/v1/incidents/{id}/investigate
PUT  /api/v1/incidents/{id}/owner
POST /api/v1/incidents/{id}/notes
POST /api/v1/incidents/{id}/resolve
GET  /api/v1/incident-assignees

GET  /api/v1/incident-rules
POST /api/v1/incident-rules
GET  /api/v1/incident-rules/{id}
PUT  /api/v1/incident-rules/{id}
POST /api/v1/incident-rules/preview

GET  /api/v1/maintenance-windows
POST /api/v1/maintenance-windows
PUT  /api/v1/maintenance-windows/{id}
POST /api/v1/maintenance-windows/{id}/revoke
GET  /api/v1/silences
POST /api/v1/silences
PUT  /api/v1/silences/{id}
POST /api/v1/silences/{id}/revoke

GET  /api/v1/notification-destinations
POST /api/v1/notification-destinations
PUT  /api/v1/notification-destinations/{id}
POST /api/v1/notification-destinations/{id}/test
GET  /api/v1/notification-deliveries

GET  /api/v1/website-monitors
POST /api/v1/website-monitors
GET  /api/v1/website-monitors/{id}
PUT  /api/v1/website-monitors/{id}
POST /api/v1/website-monitors/{id}/check
GET  /api/v1/webpages
GET  /api/v1/webpages/{id}
GET  /api/v1/certificates
GET  /api/v1/certificates/{id}
```

`incident-assignees` returns only opaque ID and display name for enabled Operator/
Administrator users and requires `incidents:operate`; it does not expose the user
administration model. Manual check requests are bounded asynchronous jobs and
return `202`; they do not bypass scheduling concurrency or network policy.

Incident filters are repeated `severity`, `status`, `integration`, `resource`, and
`owner` values plus `active` and bounded `from`/`to`. Timeline and delivery filters
are bounded. Website filters cover effective state and monitor; certificate filters
cover state, hostname validity, and expiry window. Stable order is newest relevant
time then ID unless an endpoint documents otherwise.

Users with `incidents:read` may inspect delivery evidence attached to incidents they
can read. The cross-incident delivery collection and all destination configuration
require `notification_destinations:manage`; read models expose display name and
destination type, never endpoint path or secret material.

### 5.2 Planned schemas and fixtures

Add checked v1 schemas, positive/negative fixtures, generated TypeScript, and
OpenAPI references for:

- monitoring signal and rule condition;
- incident summary, detail, timeline event, action request, and list;
- incident rule, maintenance window, and silence views/writes;
- notification destination redacted view/write and delivery view;
- website monitor redacted view/write, webpage summary/detail, certificate summary/
  detail; and
- additive live invalidations carrying `incident_id`, `delivery_id`, `monitor_id`,
  or `certificate_id` as appropriate.

Secret reference values, Mattermost paths/tokens, request headers, response content,
and certificate trust material do not appear in read schemas.

### 5.3 Planned migrations

Reserve migration order when Slice 2.0 begins; do not edit committed migrations
after merge. Expected groups are:

1. durable monitoring signals, incident rules/state, incidents, and timeline;
2. role-permission additions and operator idempotency records;
3. maintenance windows and silences;
4. notification destinations, intents, attempts, and retry indexes; and
5. website monitor/certificate projections and observation indexes.

Required constraints include one active incident per fingerprint, immutable
timeline and attempt evidence, valid time ranges, bounded enum checks, destination
intent uniqueness, and due-work indexes. Retention defaults keep resolved incidents
and notification metadata for at least the audit retention period; detailed website
observations follow the existing observation retention policy.

## 6. Delivery slices

Slices merge in order. A later slice may prepare additive contracts earlier, but it
must not expose navigation, capability copy, or enabled behavior before the full
read/mutation/evidence path is implemented. Every Web slice follows the
[UI guidance](../design/UI_GUIDANCE.md), [design system](../design/DESIGN_SYSTEM.md),
and Phase 2 incident wireframe in [wireframes](../design/WIREFRAMES.md).

### Slice 2.0 — Contracts, permissions, and durable work foundations

**Build**

- Record any new durable architectural choice as an ADR, especially the durable
  signal journal and in-process Mattermost-driver boundary.
- Add contract schemas/fixtures for signals, incident rules, incident reads,
  timeline events, and live invalidations. Generate TypeScript and update OpenAPI.
- Add the Phase 2 read/manage permissions to built-in roles through a forward-only
  migration without erasing custom permissions.
- Add the monitoring-signal journal, claim metadata, retry/dead-letter evidence,
  and indexes. Append signals atomically from observation ingestion and freshness.
- Implement bounded claim, completion, retry, shutdown, and replay behavior behind
  an owned worker interface. Do not start incident evaluation yet.
- Add metrics for queue depth/oldest age, claims, retries, dead letters, and
  processing latency without resource IDs as labels.

**Verify**

- Observation rollback creates no signal; a committed observation creates one.
- Duplicate observation delivery does not create duplicate signals.
- Freshness transitions append signals transactionally.
- Two workers cannot complete the same claim; crash/lease expiry replays it.
- Queue bounds, cancellation, database loss, poison work, and dead-letter behavior
  are deterministic under an injectable clock.
- Existing custom role permissions survive migration and direct `401`/`403` tests
  cover every new boundary.

**Done when:** Core can durably and idempotently carry monitoring changes across a
restart without relying on SSE, and all accepted contracts generate cleanly.

### Slice 2.1 — Automatic incident creation and read path

**Build**

- Add incident rules, persisted debounce/recovery state, incidents, and immutable
  timeline migrations and repositories.
- Implement rule validation, deterministic matching, default rule seed, fingerprint
  uniqueness, lifecycle transitions driven by signals, severity changes,
  recurrence, and recovery deadlines.
- Start one bounded incident evaluator owned by application lifecycle wiring.
- Implement incident collection/detail/timeline read services and the read-only API
  endpoints with permissions, cursors, filters, `ETag`, and safe errors.
- Extend overview with authoritative active counts and a compact newest-active list.
  Preserve compatibility by making new schema fields additive.
- Publish post-commit `incident.changed` invalidations with bounded replay.
- Replace the Alerts unavailable state with an authoritative active/history list,
  direct detail URLs, compact timeline, filters in the URL, and honest empty/error/
  disconnected states. Keep mutation controls absent until Slice 2.2.

**Verify**

- Critical, debounced warning, unknown, non-opening stale, healthy recovery, and
  out-of-order/duplicate signal cases.
- Concurrent evaluators create one incident and ordered timeline evidence.
- Restart during debounce/recovery preserves deadlines and counts.
- Active/history pagination remains stable as new incidents arrive.
- Viewer reads succeed while direct operate/manage calls remain denied.
- Dashboard and Alerts refetch after SSE replay/resync; neither invents counts.
- Alerts passes keyboard, screen-reader, contrast, 1440px, 1280px, and narrow
  viewport review and is recorded in UI review notes.

**Done when:** a sustained sample-adapter failure creates exactly one inspectable
incident and a healthy recovery changes it to recovered after a Core restart.

### Slice 2.2 — Operator incident workflow

**Build**

- Implement acknowledge, investigate, assignment, append-only note, and recovered-
  only resolution services and APIs with `If-Match` and `Idempotency-Key`.
- Add the restricted assignee lookup and validate that owners remain enabled and
  authorized; preserve historical identity text if an owner later changes.
- Commit each mutation, timeline event, and redacted audit event atomically.
- Return request/correlation receipts and expose the audit link only to authorized
  administrators.
- Add role-aware controls to incident detail, including pending, conflict,
  validation, denied, Core-unavailable, and success states. On `412`, refetch and
  ask the operator to review the newer state rather than silently overwrite it.
- Keep notes plain text and bounded; render them as text, never HTML/Markdown.

**Verify**

- Full valid transition table plus every invalid transition.
- Two Operators acting on the same version produce one success and one safe `412`.
- Idempotent retry does not duplicate timeline, note, or audit evidence.
- Viewer direct calls return `403`; UI omits controls with an honest read-only state.
- Disabled/unauthorized owner, overly long note, empty resolution note, CSRF,
  untrusted origin, and request-size failures.
- Every successful UI mutation shows a receipt; an Administrator can follow it to
  exactly one matching audit record.

**Done when:** Operators can complete the incident workflow without bypassing Core,
losing a concurrent update, or creating ambiguous audit history.

### Slice 2.3 — Rules, maintenance windows, and silences

**Build**

- Implement administrator rule list/detail/create/replace/preview endpoints with
  `ETag`, validation, overlap explanation, enable/disable, and audit receipts.
- Implement one-time maintenance windows and expiring silences with scope
  validation, UTC ranges, revocation, efficient active matching, and expiry worker.
- Integrate maintenance with effective health and incident evaluation, including
  immediate end-of-window re-evaluation. Integrate silences with notification-
  intent decisions ready for Slice 2.4.
- Add `/alerts/rules` and `/alerts/suppressions` only when their authoritative
  endpoints and receipts work. Make them real, permission-gated Alerts children in
  desktop and collapsed navigation.
- Show raw failure evidence alongside maintenance treatment; never present
  maintenance as healthy or delete a real incident.

**Verify**

- Rule precedence, boundary timestamps, concurrent replacement, invalid scopes,
  overlap preview, enable/disable, and persisted debounce state after rule changes.
- Maintenance before/during/after a failure, overlapping windows, revocation,
  Core restart at expiry, and failure still active after expiry.
- Silence match/non-match, expiry/revocation, and no change to incident status.
- Administrator mutations link to audit evidence; Operator/Viewer direct access is
  denied.
- Navigation children work by hover, focus, click/touch, `Escape`, and collapsed
  narrow-screen menu without exposing them to unauthorized sessions.

**Done when:** planned work does not create incident noise, silences do not hide
incidents, and administrators can explain exactly which rule/control applied.

### Slice 2.4 — Notification routing and Mattermost delivery

**Build**

- Add destination, intent, attempt, and retry migrations plus append-only evidence
  constraints and due-work indexes.
- Implement destination-independent intent creation in the incident transaction,
  silence evaluation, bounded router/worker ownership, retry classification,
  dead-letter state, graceful shutdown, and metrics.
- Implement the Mattermost driver with safe structured formatting, escaped operator/
  source text, mentions disabled by default, stable Espial event IDs, and incident
  links derived only from configured public URL.
- Validate destination secret references and network policy; enforce DNS pinning,
  address allowlists, HTTPS, no redirects/proxy environment, request/response caps,
  and explicit timeouts.
- Implement destination administration/test endpoints and delivery reads. A test
  delivery is labeled test and leaves an audit receipt; it cannot masquerade as an
  incident alert.
- Add incident delivery status and timeline entries. Add administrator destination
  controls at `/alerts/notifications` only with the complete read/mutate/test/audit
  path.

**Verify**

- One intent for duplicate incident events; initial/severity/recurrence/recovery
  policy; matching silence becomes terminal suppressed evidence.
- Successful delivery, timeout, connection reset, `429` with bounded `Retry-After`,
  `5xx` retry, `4xx` terminal failure, dead letter, and recovery after restart.
- Slow destination cannot block incident evaluation or unrelated destinations.
- Fake Mattermost captures correct escaped content without secrets or unrestricted
  mentions. Logs/API/audit never expose the webhook secret or body.
- SSRF tests cover userinfo, redirects, DNS rebinding, unapproved internal/public
  addresses, proxy environment, and allowed internal Mattermost deployment.
- The UI distinguishes queued, delivered, retrying, failed, dead-letter, and
  suppressed with text, time, attempts, and safe reason.

**Done when:** one incident episode produces traceable Mattermost initial and
recovery messages, controlled retries, and no duplicate database intent.

### Slice 2.5 — Website availability monitoring

**Build**

- Add website-monitor schemas and redacted API models. A website monitor is a typed
  projection of one integration-registry row using the trusted `webcheck` adapter;
  its opaque monitor ID is the integration ID. Record that boundary in an ADR and
  do not create a second configuration authority.
- Implement the trusted `webcheck` adapter, registry manifest, configuration
  validation, scheduler integration, secret-header references, and bounded exact
  content check.
- Apply the approved DNS/address/redirect policy to every connection and redirect
  hop. Bound resolution, connect, TLS, header, body, and whole-check time.
- Emit normalized resources/checks for DNS, TCP, TLS negotiation, HTTP status,
  latency, and content match. A partial failure preserves completed stage evidence
  and emits a stable safe reason code.
- Implement monitor administration and webpage read endpoints, manual bounded
  checks, audit receipts, and `webpage.changed` invalidations.
- Replace the Webpages unavailable state with authoritative availability rows and
  detail pages. Add `/webpages/monitors` only for administrators after its mutation
  path is complete.

**Verify**

- Local deterministic fixtures cover DNS failure, connect timeout/refusal, TLS
  failure, unexpected HTTP status, slow response, oversize body, content mismatch,
  redirect policy, and recovery.
- Adapter crash remains isolated and normal freshness makes data stale/unknown.
- Network policy cannot be bypassed through redirects, address families, rebinding,
  URL parsing ambiguity, userinfo, or ambient proxy variables.
- Viewer reads and Administrator writes are independently enforced; secret headers
  and response bodies never enter API, logs, audit, or observations.
- Webpages loading/empty/not-configured/stale/disconnected/Core-error and narrow
  states pass the UI acceptance contract.

**Done when:** an approved endpoint outage automatically becomes visible in
Webpages and creates one incident through the same engine as every other resource.

### Slice 2.6 — Certificate monitoring and Webpages experience

**Build**

- Add bounded certificate payload/schema fixtures and certificate observation
  projection with fingerprint/issuer change evidence and expiry indexes.
- Extend `webcheck` to emit certificate evidence even when HTTP fails after a
  successful handshake. Distinguish no certificate, untrusted chain, hostname
  mismatch, expired, and approaching expiry.
- Add validated per-monitor 30/14/7-day default thresholds and specific incident
  rules. Crossing a meaningful threshold updates the existing certificate incident
  rather than creating a new one.
- Implement certificate list/detail APIs and additive Dashboard certificate warning
  summary.
- Add `/webpages/certificates` and certificate detail to the Webpages experience
  with endpoint, status, expiry/date remaining, issuer, hostname/chain state, last
  check, source, freshness, and active incident link. Unknown values say `Unknown`
  or `Not reported`, never zero.

**Verify**

- Deterministic local CAs/certificates cover valid, 30/14/7-day boundaries,
  expired, not-yet-valid, wrong hostname, untrusted chain, replacement, issuer
  change, and clock skew under an injected clock.
- Repeated checks and threshold crossings preserve one active fingerprint and
  produce only meaningful notification updates.
- Certificate API never returns private keys, secret headers, unbounded chains, or
  sensitive raw handshake errors.
- Direct certificate URL, active incident link, filters, timestamps, keyboard use,
  and three viewport layouts work with reduced motion and live disconnect.
- Dashboard does not show a certificate count until the authoritative read succeeds;
  error/stale states remain explicit.

**Done when:** Operators can identify which approved endpoint certificate needs
attention, why, when it expires, and its linked incident using authoritative data.

### Slice 2.7 — Phase integration, hardening, and acceptance

**Build**

- Complete schema/OpenAPI/generated-type compatibility checks and update API,
  authentication, data-model, architecture index, and operator documentation.
- Add runbooks for rule rollout/rollback, incident operations, maintenance/silence,
  Mattermost destination rotation/failure, notification replay policy, website
  allowlists, certificate diagnosis, queue/dead-letter recovery, and upgrades.
- Extend backup/restore and deployment examples for Phase 2 tables, worker settings,
  Mattermost/Webcheck egress, secrets, retention, and health metrics.
- Perform a Phase 2 threat review covering rule abuse, incident spoofing, stored
  operator text, notification injection, webhook leakage, SSRF/rebinding, malicious
  HTTP/TLS peers, queue exhaustion, retry storms, audit mismatch, and permission
  escalation.
- Load-test signal/evaluation and notification backlogs at representative and
  explicitly oversized scales. Set evidence-based worker, queue, timeout, and
  retention defaults.
- Re-check every Phase 2 screen against `UI_GUIDANCE.md` and record the final
  cross-route review in UI review notes.

**Vertical acceptance scenario**

1. Start a clean stack, migrate, bootstrap an Administrator, and create an Operator
   and Viewer.
2. Configure a deterministic website monitor, default incident rule, and fake
   Mattermost destination using only secret references and approved egress.
3. Produce a debounced warning and immediate critical failure; prove one incident
   exists and one initial delivery intent is created.
4. Force retryable Mattermost failures, restart Core mid-backoff, recover the fake
   server, and prove bounded attempts end in one delivered intent.
5. As Operator, acknowledge, self-assign, investigate, and add a note; prove the
   Viewer can read but receives `403` for the same mutations.
6. Create a silence and prove a later incident remains visible while delivery is
   recorded as suppressed. Create maintenance and prove a matching failure does not
   open until it persists after the window.
7. Recover the endpoint and observe recovery notification; fail it again before
   resolution and prove the same incident reopens. Recover again, resolve with a
   note, then fail it once more and prove a new episode is created.
8. Serve deterministic valid, expiring, wrong-host, and expired certificates and
   verify exact threshold/status/incident behavior in Core and Webpages.
9. Follow each administrative UI receipt to its matching redacted audit record;
   prove stale `ETag`, duplicate idempotency key, CSRF, origin, and permission
   denials directly against Core.
10. Disconnect/reconnect SSE and restart workers with pending signal and delivery
    work; prove UI refetches authoritative state and no incident/intent is lost.
11. Exercise Alerts, Dashboard, Webpages, dropdowns, and mutations with keyboard,
    touch, 1440px, 1280px, narrow viewport, zoom, and Core-unavailable states.
12. Back up the active scenario, restore into a clean stack, and prove incidents,
    timelines, suppressions, destination metadata, delivery history, rules, website
    state, certificates, and audit correlations remain coherent.

**Done when:** the clean-stack scenario, race suite, security gates, UI review,
restore drill, and runbook dry run pass, with every production blocker assigned an
owner.

## 7. Configuration surface

Keep defaults bounded and reject unknown production keys. Representative additions:

```yaml
incidents:
  worker_concurrency: 2
  claim_batch_size: 50
  claim_lease: "30s"
  max_signal_attempts: 8
notifications:
  worker_concurrency: 2
  max_attempts: 6
  request_timeout: "10s"
  response_body_limit_bytes: 4096
  approved_hosts: []
  approved_cidrs: []
webcheck:
  executable: "/usr/local/bin/espial-webcheck-adapter"
  max_redirects: 0
  response_body_limit_bytes: 262144
  approved_hosts: []
  approved_cidrs: []
  allowed_ports: [80, 443]
certificates:
  warning_days: 30
  critical_days: 14
  escalation_days: 7
```

Exact names may change with typed configuration implementation. Committed examples
use `.invalid` hosts, empty network allowlists, and secret-file placeholders. No
default enables arbitrary egress, follows redirects, loads ambient proxy settings,
or stores a Mattermost webhook token in plain configuration.

## 8. Test strategy

| Layer | Primary tests | Required failure cases |
|---|---|---|
| Contracts | Schemas, fixtures, OpenAPI, generated TypeScript | Incompatible enum/type, secret-bearing read field, oversize bound |
| Domain unit | Rule table, lifecycle, suppression match, retry classifier | Boundary time, invalid transition, overlap, recurrence, cancellation |
| PostgreSQL | Signals, claims, fingerprints, timeline, outbox, migrations | Rollback, duplicate, lease loss, concurrent workers/actions, restore |
| API | Handler contracts, permissions, cursors, ETags, idempotency | 401/403/404/409/412/428, CSRF/origin, invalid body, safe 500 |
| Adapter | Webcheck conformance and local network fixtures | DNS/TCP/TLS/HTTP/content failures, flood, hang, redirect, SSRF |
| Notifications | Fake Mattermost and deterministic clock | timeout, 429, 4xx, 5xx, restart, duplicate remote attempt, secret leak |
| Web | Unit/component/accessibility/browser | loading, empty, denied, stale, disconnect, conflict, Core down |
| End to end | Clean stack with local TLS/HTTP/Mattermost fixtures | failure→incident→delivery→operate→recover→resolve, restart/resync |
| Operational | Compose, load, shutdown, backup/restore, scans | backlog, retry storm, partial startup, migration mismatch, egress denial |

Timing tests use injected clocks and bounded eventual assertions, not long sleeps.
Network tests use local deterministic servers and certificates; CI does not contact
public websites, DNS, or Mattermost.

## 9. Observability and operations baseline

Add bounded metrics/log fields for:

- monitoring-signal queue depth, oldest age, claim/result, and dead letter;
- rule evaluations by outcome, debounce/recovery deadline, incident creation,
  recurrence, severity change, recovery, and resolution;
- active incidents by bounded severity/status only;
- active maintenance/silence count and suppression decisions;
- notification intents/deliveries by destination type, state, attempt, safe error
  category, latency, retry queue depth, and oldest age;
- website check stage/result/latency and certificate state/expiry bucket; and
- administrative and operator mutation result by action and status class.

Resource IDs, URLs, hostnames, usernames, note text, incident titles, certificate
subjects, and provider responses are prohibited metric labels. Structured logs may
carry opaque resource/incident/destination IDs where operationally necessary but
never secrets or response content.

Readiness reports database/migration and required worker initialization. A remote
Mattermost or monitored website outage does not make Core unready; backlog age and
delivery/check state expose those failures instead.

## 10. Security and release gates

Before Phase 2 is complete:

- Every new handler has a declared permission and direct allow/deny tests.
- Operator/admin mutations enforce CSRF/origin, size bounds, `ETag` where mutable,
  idempotency where retryable, atomic audit, and visible correlation receipts.
- Operator-controlled text is bounded, stored/rendered as plain text, safely
  escaped in Mattermost, and absent from metric labels.
- Durable workers have bounded claims, leases, attempts, concurrency, payload size,
  shutdown, and dead-letter recovery.
- Destination and website egress use explicit host/address/port policy, DNS pinning,
  redirect revalidation, no ambient proxy, and deterministic SSRF/rebinding tests.
- Secrets remain references and do not enter API models, generated types, logs,
  audit, observations, notification evidence, process arguments, or screenshots.
- Notification retries cannot amplify without bound; silences and maintenance
  cannot erase incident evidence.
- Certificate validation uses approved trust roots and injected time in tests; no
  option silently disables verification in production.
- The Phase 2 threat review has no unowned critical/high finding.
- Phase 1 production blockers remain release blockers unless their owners close
  them; completing Phase 2 does not implicitly approve production deployment.

## 11. Risks and controls

| Risk | Control | Trigger to revisit |
|---|---|---|
| Signal replay creates duplicate incidents | Signal idempotency, fingerprint constraint, transactional timeline | Any duplicate in concurrency/restart acceptance |
| Rule overlap produces alert spam | Deterministic specificity/priority and preview | Operators require intentional multi-rule fan-out |
| Manual resolution hides an active fault | Recovered-only resolution and recurrence semantics | Product explicitly approves forced closure workflow |
| Maintenance hides underlying evidence | Preserve raw health and record suppressed evaluation | Operator cannot explain effective maintenance state |
| Mattermost retry creates duplicate remote posts | Unique intent, stable event ID, bounded retry | Provider adds an idempotency facility |
| Website monitoring becomes SSRF | Explicit egress policy, pinned resolution, redirect revalidation | New discovery/import feature is proposed |
| Certificate thresholds spam daily | Threshold-crossing events and one active fingerprint | Operators approve repeat reminder policy |
| Backlog outruns one Core instance | Bounded workers, age/depth metrics, load evidence | Measured oldest age breaches operational objective |
| Phase 2 UI overstates future dependency context | Only link implemented resources/incidents; label future impact planned | Phase 4 relationship API exists |

## 12. Phase 2 exit criteria

Phase 2 is complete only when all are checked and linked to evidence:

- [ ] Monitoring signals, incident evaluation, and notification intents survive
  crashes/restarts without relying on SSE or creating duplicate database work.
- [ ] Meaningful critical/warning/unknown and website/certificate failures create
  predictable, deduplicated incidents with tested debounce and recovery.
- [ ] Viewer read and Operator workflow permissions are proven directly against
  Core; concurrent mutations cannot silently overwrite each other.
- [ ] Alerts provides active/history filters, direct detail URLs, factual timeline,
  delivery evidence, and complete loading/empty/error/disconnected states.
- [ ] Maintenance prevents expected incident noise without rewriting raw health;
  silences suppress delivery without hiding incidents.
- [ ] Mattermost initial, meaningful update, and recovery delivery is traceable,
  retryable, bounded, secret-safe, and operable through documented recovery steps.
- [ ] Website checks isolate DNS/TCP/TLS/HTTP failures, respect explicit egress
  policy, and become stale/unknown when the adapter fails.
- [ ] Webpages shows authoritative availability and certificate identity/expiry,
  active incident links, freshness, and honest unknown/not-configured states.
- [ ] Dashboard incident/certificate summaries come from authoritative Core read
  models and remain correct through SSE replay/resync.
- [ ] Every administrative mutation returns a visible correlation receipt linked to
  matching redacted audit evidence for authorized users.
- [ ] Phase 2 screens pass keyboard, touch, accessibility, contrast, reduced-motion,
  1440px, 1280px, narrow viewport, and cross-route UBNetDef visual review.
- [ ] Migrations, race/concurrency tests, dependency/image scans, threat review,
  load/backlog evidence, clean-stack acceptance, and backup/restore pass.
- [ ] Operator runbooks have been dry-run by someone other than their author and all
  production blockers have named owners.

After these criteria pass, Phase 3 can add high-value infrastructure integrations
that reuse the incident and notification path without changing its Core lifecycle.
