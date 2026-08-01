# Phase 3 pre-planning: core infrastructure integrations

**Status:** Pre-planning only (2026-08-01); no implementation slice is active  
**Inputs:** [Roadmap](../ROADMAP.md),
[adapter protocol](../architecture/ADAPTER_PROTOCOL.md),
[authentication contract](../architecture/AUTHENTICATION.md),
[environment inventory](../operations/ENVIRONMENT_INVENTORY.md), and
[Phase 2 implementation record](PHASE_2_IMPLEMENTATION.md)

## 1. Authorization and purpose

Phase 2 is complete for development sequencing. Its independent operator dry run
and named production-owner sign-off were explicitly deferred on 2026-08-01; they
remain production-release gates and are not treated as completed evidence.

This record starts only Phase 3 discovery and sequencing. It does not authorize an
adapter implementation, SSO activation, schema migration, route, navigation child,
permission, or user-visible capability. A full implementation plan must follow the
discovery evidence below before a Phase 3 slice starts.

## 2. Intended outcome and boundaries

Phase 3 should connect high-value UBNetDef infrastructure sources to the existing
monitoring, incident, notification, and audit paths while keeping vendor logic out
of Espial Core.

The phase is read-only monitoring and source context. It does not add:

- service dependency relationships or affected-service calculation (Phase 4);
- physical room, rack, chassis, or drive-slot inventory (Phases 5 and 6);
- spatial editing or visualization polish (Phase 7);
- arbitrary metric retention that duplicates an authoritative metrics platform;
- inbound adapter events until durable acknowledgement/replay is designed; or
- remote or approval-based actions (Phase 9).

UBNetDef SSO is an authentication-provider integration, not a monitoring adapter.
It must preserve the provider-neutral session/RBAC boundary and the deliberate
local emergency-access path described by the authentication contract.

## 3. Reusable baseline and known gaps

Phase 3 can reuse:

- supervised, restart-isolated NDJSON adapter processes and v1 conformance tests;
- bounded integration configuration with secret references;
- normalized resources, observations, current health, freshness, and source URLs;
- durable signals, incident rules, maintenance/silence, notification delivery, and
  audit evidence; and
- standard Dashboard/resource/integration states without inventing vendor health in
  Web.

Current protocol v1.0 accepts resource and observation collections. Before a vendor
adapter relies on inventory, relationships, event push, or richer typed evidence,
the implementation plan must decide whether bounded existing fields are sufficient
or an additive protocol minor and schema fixtures are required. Vendor response
blobs must not become the central data model.

The environment inventory still lacks the SSO protocol/claims, source versions,
API mechanisms, scale, rate limits, stable identifiers, least-privilege credential
scope, and production egress for every Phase 3 candidate. Those are discovery
inputs, not values to guess in code.

## 4. Provisional priority and decision gates

The roadmap order remains provisional until owners provide evidence:

| Candidate | Pre-planning gate | Earliest useful result |
|---|---|---|
| UBNetDef SSO | Confirm protocol, issuer, registration, claims/groups, logout, outage behavior, test tenant, and role-mapping owner | Approved provider contract and deterministic non-production login fixtures |
| Proxmox | Confirm cluster/version, token scope, API/event availability, stable node/VM/container IDs, rate limits, scale, and deep-link format | A sanitized collection fixture and mapping to normalized resource/check types |
| TrueNAS | Confirm edition/version, authentication, REST/WebSocket/alert support, stable pool/dataset identifiers, scale, and deep links | A sanitized fixture and a bounded pool/system health mapping |
| iDRAC | Inventory versions/licenses and confirm Redfish resources, event support, credential scope, scale, and console deep links | A smallest-common Redfish profile plus explicit vendor/version exceptions |
| Grafana or metric source | Identify the authoritative source and alert interface; define which health signals Espial needs without copying full time series | A narrow alert/health/deep-link contract and retention decision |
| pfSense | Confirm edition/version and approved API, SNMP, or syslog mechanism, credential scope, stable IDs, and rate limits | A selected read-only mechanism and normalized health mapping |

SSO discovery stays first because it is already a production input. Proxmox remains
the tentative first vendor monitoring adapter because it is first in the roadmap,
but neither ordering decision authorizes implementation. If an earlier candidate
lacks its owner-supplied evidence, discovery may continue in parallel without
shipping a later candidate's route or capability claim.

## 5. Questions every monitoring integration must answer

Before implementation planning, each candidate needs a short evidence record that
answers:

1. Which exact product versions and deployment topology are in scope?
2. Which API or event mechanism is authoritative and supported there?
3. What is the least-privilege read-only credential and how is its secret supplied?
4. Which identifiers remain stable across rename, restart, migration, and cluster
   membership changes?
5. Which bounded resources and checks provide operational value, and which metrics
   intentionally remain in the source platform?
6. What creates `warning`, `critical`, `unknown`, recovery, and freshness, and which
   thresholds are site configuration rather than adapter policy?
7. What pagination, rate, payload, timeout, concurrency, and collection-size limits
   apply at representative and oversized UBNetDef scale?
8. Which source URLs are safe and useful, and how are their scheme/host/path values
   validated?
9. Which failure and malformed-data fixtures prove isolation, redaction, restart,
   and stale/unknown behavior?
10. Can standard resource and integration UI components present the result
    honestly, or is a later purpose-specific view justified by authoritative data?

## 6. Provisional planning shape

A later implementation plan should preserve this order unless new evidence changes
it:

1. **Discovery and contracts:** close environment inputs, define resource/check
   taxonomy, assess protocol gaps, and publish adapter-authoring guidance.
2. **SSO provider:** only after the SSO readiness contract is fully answered and
   emergency local access, session rotation, group mapping, outage behavior, and
   audit evidence have deterministic acceptance tests.
3. **One monitoring adapter at a time:** start with the highest-value ready source;
   land its contracts, adapter, conformance/security tests, operations record, and
   standard UI evidence before exposing it.
4. **Cross-integration hardening:** prove concurrent adapter isolation, scheduler and
   database capacity, incident/rule behavior, secret/egress policy, upgrade/restore,
   and documentation suitable for student adapter authors.

No phase-wide schema should be designed from all vendor possibilities at once. The
first evidence-backed integration should establish only the smallest reusable
addition, and the next integration must prove that addition is genuinely common.

## 7. Ready to write the implementation plan when

- an owner has completed the SSO discovery checklist or explicitly sequenced it
  behind a ready monitoring source;
- the environment inventory names versions, scale, credentials, egress, and owners
  for the proposed first adapter;
- sanitized happy-path, degraded, unavailable, unauthorized, throttled, malformed,
  and oversized source fixtures exist;
- the v1.0 resource/observation fit and any additive protocol/schema gap are
  documented;
- resource identity, freshness, thresholds, source links, redaction, and retention
  are agreed; and
- the first slice has measurable backend, security, operational, and UI acceptance
  evidence without claiming Phase 4+ capability.
