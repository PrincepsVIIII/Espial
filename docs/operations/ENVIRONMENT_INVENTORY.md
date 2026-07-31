# Environment and integration inventory

This is the Phase 0 discovery record. It distinguishes known product targets from
deployment facts that UBNetDef operators still need to supply. Do not put secrets,
tokens, internal passwords, or private keys in this file.

## Scale baseline

| Fact | Current value | Owner | Needed by |
|---|---|---|---|
| Physical hosts and management endpoints | TBD (Infrastructure) | Infrastructure lead | First real infrastructure adapter |
| VMs and containers | TBD (Virtualization) | Proxmox owner | Proxmox adapter sizing |
| Storage systems/pools | TBD (Storage) | TrueNAS owner | TrueNAS adapter sizing |
| Websites and TLS endpoints | TBD (Service owners) | Operations | Phase 2 certificate checks |
| Expected users by role | TBD (Administration) | Project owner | Production authorization review |
| Sites, rooms, and racks | TBD (Infrastructure) | Inventory owner | Phase 5 |

These values are implementation inputs, not reasons to hard-code sample assumptions.

## Initial integration inventory

| Source/destination | Direction | Candidate mechanisms | Phase | Current decision or question |
|---|---|---|---|---|
| UBNetDef SSO | Auth inbound | OIDC preferred if supported; protocol TBD | 1/3 | External dependency. Need issuer/protocol, claims, groups, logout, test tenant. Local auth is Phase 1 fallback. **Production blocker for SSO mode.** |
| Sample adapter | Health inbound | Supervised NDJSON stdio | 1 | Deterministic reference implementation; accepted transport |
| Proxmox | Health/inventory inbound | REST API, event/log facilities TBD | 3 | Confirm cluster version, token scope, events, rate limits, deep-link format |
| TrueNAS | Health/inventory inbound | REST/WebSocket/alerts depending edition | 3 | Confirm edition/version, API auth, events, pool and drive identifiers |
| iDRAC | Health/inventory inbound | Redfish preferred; SNMP/events if useful | 3 | Inventory versions/licenses and Redfish/event availability |
| Grafana or metric source | Health inbound/deep links | HTTP APIs, Alertmanager/webhook, source API | 3 | Identify authoritative data source; avoid duplicating all metrics |
| pfSense | Health/inventory inbound | API package, SNMP, syslog TBD | 3 | Confirm edition/version and approved integration method |
| Website/TLS endpoints | Scheduled checks | DNS/TCP/TLS/HTTP from Core or built-in adapter | 2 | Need endpoint list, expected status/content, ownership, discovery method |
| Mattermost | Notifications outbound | Incoming webhook or bot API | 2 | Confirm destination model, retry expectations, secret ownership |

## SSO discovery checklist

- Protocol and issuer/metadata endpoint
- Client registration and allowed redirect URIs
- Stable subject, display name, email, and group/role claims
- Group-to-Espial-role ownership and default-deny behavior
- Signing/encryption algorithms, key rotation, clock skew, and token lifetimes
- Login, callback, refresh (if applicable), front/back-channel logout behavior
- Test environment and non-production identities for every role
- Expected behavior during partial outage and how service health is monitored

Until answered, Core runs in `local` mode. No placeholder SSO endpoint or unsigned
token behavior is allowed.

## Deployment discovery checklist

| Decision | Current baseline | Close before production? |
|---|---|:---:|
| Deployment topology | Dedicated single management host, separate containers/processes | Yes—name host/failure domain |
| Container runtime and reverse proxy | Compose-compatible and proxy-neutral | Yes |
| Database backup destination and retention | TBD (Operations) | Yes |
| Secret file/store mechanism | TBD (Security/Operations) | Yes |
| External heartbeat destination | TBD (Operations) | Yes |
| Adapter network egress policy | Least privilege; concrete rules TBD per integration | Yes per adapter |
| Six-month audit capacity estimate | TBD after user/event counts | Yes |

## Phase 1 non-blocking sample values

The reference adapter may expose three deterministic resources—healthy, warning,
and stale-transition—to build the vertical slice. IDs and fixtures are clearly
labeled `sample`; migrations and production configuration do not seed them.

## Change process

An integration owner updates this record during discovery, links vendor/API
documentation, records version and capability evidence, and opens an ADR only when
the fact changes a cross-component architecture decision.
