# Phase 2 threat review and release triage

**Reviewed:** 2026-08-01  
**Scope:** monitoring signals, incident rules and workflow, maintenance/silence,
Mattermost delivery, website/TLS monitoring, certificate projections, Phase 2 UI,
workers, metrics, deployment, and restore.  
**Triage owner:** Espial maintainer. UBNetDef Security and Operations retain final
production acceptance.

| Threat | Implemented controls | Residual owner / release decision |
|---|---|---|
| Rule abuse or overlap alert storms | Bounded declarative conditions, deterministic precedence, preview, ETag/idempotency, audited replacement, debounce/recovery bounds | Operations owns production rule review; broad rollout requires preview evidence |
| Incident spoofing or evidence rewrite | Signals are transactionally journaled; fingerprints are unique; timelines and attempts are append-only; operator actions require Core authorization and version checks | Database owner remains privileged; backup/admin activity is an operational audit responsibility |
| Stored operator-text injection | Length bounds, strict JSON, plain-text Web rendering, escaped Mattermost formatting, mentions disabled, no operator text in metric labels | Operators must keep credentials and response bodies out of notes |
| Notification injection or webhook leakage | Structured payloads, write-only secret references, bounded response evidence, HTTPS, no redirects or ambient proxy, stable event IDs | Security owns secret-store and token-rotation approval |
| Website SSRF and DNS rebinding | Exact host/CIDR/port policy, validation plus pinned dialing, every redirect revalidated, URL userinfo/query secrets rejected, protected headers do not cross origins | Each service owner supplies exact production targets and all valid DNS answers |
| Malicious HTTP/TLS peer | Whole-stage timeouts, header/body caps, minimum TLS, approved roots, explicit hostname and chain verification, bounded certificate evidence, no raw bodies/chains | Operations owns trust-root and monitored-endpoint inventory |
| Queue exhaustion and poison work | Bounded batch, lease, attempts, concurrency, payload sizes, dead letters, oldest-age/depth metrics, disposable 1×/10× backlog profiles | Site scale remains an environment input; rerun load gate when it changes |
| Retry amplification | Maximum six notification attempts, capped exponential delay and Retry-After, unique database intent, terminal suppression/failure/dead-letter states | Remote delivery remains at least once; operators recognize duplicate stable event IDs |
| Audit mismatch or mutation replay | State/timeline/audit committed atomically, correlation receipts, idempotency request hashes, stale ETag rejection, redacted summaries | Administrator follows receipts during acceptance and change review |
| Permission escalation | Explicit Viewer/Operator/Administrator permissions, direct handler 401/403 tests, Core enforcement independent of navigation | Identity/group ownership remains a production input; SSO is outside Phase 2 |
| Metrics disclosure or cardinality abuse | `/metrics` is outside `/api`, excluded from the production public proxy, fixed label enums only, and no resource IDs, URLs, hostnames, users, subjects, or text labels | Operations must expose it only on the private management network |
| Backup disclosure or incoherent restore | Secret references rather than secret values, encrypted-target requirement, isolated restore, schema check, Phase 2 correlation checklist | Operations must name the real target, retention, restore owner, and cadence |

## Automated evidence

- Contract/OpenAPI/generated-type checks reject incompatible or secret-bearing
  models; direct API tests cover permission, origin, CSRF, ETag, idempotency, and
  safe-error boundaries.
- PostgreSQL race suites cover concurrent claims, fingerprints, workflow conflicts,
  append-only evidence, notification retries, certificate projection, and migration
  compatibility.
- Mattermost and webcheck tests cover redirects, address policy, rebinding,
  userinfo, proxy environment, timeouts, response caps, TLS failures, and secret
  absence.
- Browser tests cover role-aware navigation, receipts, live reconnect/resync,
  Core-unavailable states, keyboard/touch equivalents, reduced motion, three
  viewports, and serious/critical accessibility findings.
- `npm run load:phase2` proves bounded representative and explicitly oversized
  backlog drain with no lost signal or delivery.
- The existing security workflow runs production dependency audits, `govulncheck`,
  repository secret/misconfiguration scanning, and release-shaped image scans.

On 2026-08-01 the production Node audits, `govulncheck`, pinned Trivy repository
scan, and release-shaped Core/Web image scans completed locally with no blocking
finding. Hosted CI repeats the same gates on `main`, pull requests, and its weekly
schedule.

## Residual production blockers

No Phase 2 critical/high finding lacks an owning role. Production remains blocked
until the environment inventory names accountable people and closes or explicitly
accepts: the real secret store, encrypted backup target/retention/restore cadence,
external heartbeat, exact website/Mattermost egress, representative site scale and
capacity, role ownership, and current Caddy/PostgreSQL image findings recorded in
the Phase 1 review. Phase 2 implementation does not waive those blockers.
