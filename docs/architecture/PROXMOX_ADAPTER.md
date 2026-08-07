# Proxmox adapter authoring reference

**Status:** Authoring and discovery reference (2026-08-06); no Proxmox adapter or
user-visible Proxmox capability is implemented

**Applies to:** Phase 3 Proxmox discovery, adapter development, conformance,
security review, and future maintenance

## 1. Purpose and boundaries

This document is the working reference for students and maintainers who build or
change Espial's Proxmox VE monitoring adapter. It explains which Proxmox facts to
collect, how to examine the API, how to translate source data into Espial's
normalized adapter protocol, and which evidence is required before the adapter is
accepted.

The adapter is a standalone, read-only process. It translates Proxmox responses
into normalized resources and observations; it does not put vendor-specific logic
in Espial Core. It must reuse Core's freshness, incident, notification, and audit
paths instead of implementing a second health or alert lifecycle.

The first adapter version does not add:

- remote VM, container, node, storage, or cluster actions;
- inbound event push before durable acknowledgement and replay exist;
- service dependency relationships or affected-service calculation;
- physical inventory or rack/chassis relationships;
- a copy of Proxmox RRD history or arbitrary time-series retention; or
- a Hypervisor route or navigation child before its authoritative read path and UI
  acceptance evidence are complete.

## 2. Authoritative source material

Use the API Viewer shipped by the target Proxmox installation because it matches
the installed release:

```text
https://<proxmox-host>:8006/pve-docs/api-viewer/index.html
```

Also consult:

- [current official API Viewer](https://pve.proxmox.com/pve-docs/api-viewer/index.html);
- [Proxmox VE API overview](https://pve.proxmox.com/wiki/Proxmox_VE_API);
- [Proxmox VE Administration Guide](https://pve.proxmox.com/pve-docs/pve-admin-guide.pdf),
  especially the limited monitoring-token and `pvesh` sections; and
- Espial's [adapter protocol](ADAPTER_PROTOCOL.md), machine-readable
  [API contracts](../../api/README.md), and
  [Phase 3 pre-planning gates](../plans/PHASE_3_PREPLANNING.md).

Do not design against an example response from a different Proxmox major version
without checking it against the target cluster and a version-matched fixture.

## 3. Discovery record

Before writing live collection code, record the following without including any
password, token secret, private key, or unsanitized internal address:

| Fact | Required evidence |
|---|---|
| Proxmox release | `pveversion --verbose` and `/version` output |
| Topology | Single node or cluster, node count, and cluster/quorum behavior |
| Scale | QEMU VM, LXC container, and storage counts at representative and expected maximum size |
| Optional subsystems | Whether Proxmox HA and Ceph are used and whether they are in the first adapter scope |
| API entry point | Approved HTTPS hostname/port and whether one endpoint or bounded failover is required |
| TLS trust | Public CA or named deployment-managed private trust bundle; verification may not be disabled |
| Identity | Stable configured deployment ID plus observed node, VMID, container ID, and storage identity behavior |
| Credentials | Dedicated user/token owner, privilege-separated token, ACL paths, expiry/rotation owner, and secret-reference mechanism |
| Collection policy | Interval, deadlines, response limits, and representative/oversized payload sizes |
| Health policy | Expected-running guests and site-owned storage thresholds |
| Source links | Safe UI base URL and version-checked deep-link forms |
| Egress | Exact approved hostname/address/port rules from the adapter deployment environment |

Useful discovery commands are:

```console
pveversion --verbose
pvecm status
pvesh get /version --output-format json
pvesh get /cluster/status --output-format json
pvesh get /cluster/resources --output-format json
pvesh get /nodes --output-format json
```

`pvecm status` is relevant only to clustered installations. For each node being
assessed, also inspect:

```console
pvesh get /nodes/<NODE>/status --output-format json
pvesh get /nodes/<NODE>/storage --output-format json
```

Sanitized fixtures replace internal names and addresses consistently while
preserving JSON keys, types, nullability, omitted fields, source status strings,
and realistic collection cardinality. A sanitized fixture must never contain an
API token, authorization header, certificate private key, storage credential, or
other secret.

## 4. Authentication and least privilege

Use a dedicated Proxmox service identity and a privilege-separated API token.
Proxmox documents `PVEAuditor` as its standard read-only role and includes a
limited API-token example for monitoring in the Administration Guide. Token
permissions are a subset of the backing user's permissions, so verify both:

```console
pveum user permissions <USER@REALM>
pveum user token permissions <USER@REALM> <TOKEN-ID>
```

The Proxmox request header has this form:

```text
Authorization: PVEAPIToken=<user>@<realm>!<token-id>=<secret>
```

The proposed Espial integration configuration treats the API URL, deployment ID,
and token ID as non-secret values. The token secret is declared in the manifest's
`secret_fields` and supplied only through a Core-resolved secret reference. The
adapter must never persist or log the combined authorization header, token secret,
raw credential error, or resolved secret value.

Start with `PVEAuditor` on only the required ACL paths. If the initial role exposes
more than the approved endpoints require, define and document a narrower custom
read-only role after the endpoint list is fixed. No mutation privilege is accepted.

## 5. Initial API surface

Start with the smallest source surface that can provide useful health:

| Endpoint | Initial use |
|---|---|
| `GET /api2/json/version` | Version and compatibility evidence |
| `GET /api2/json/cluster/status` | Cluster membership and quorum evidence |
| `GET /api2/json/cluster/resources` | Cluster-wide node, QEMU, LXC, storage, and current status evidence |
| `GET /api2/json/nodes/{node}/status` | Additional bounded node evidence when the cluster resource response is insufficient |

Do not add guest configuration, task logs, RRD history, replication history, or
event polling merely because the API exposes them. Each additional endpoint needs
an operational question, least-privilege proof, bounded response fixture, and
normalized mapping.

The author should first determine whether `/cluster/resources` alone provides the
required first-version state. Proxmox responses normally wrap results under
`data`; the adapter should validate required fields and their types while tolerating
unrelated additive response fields.

## 6. Resource identity and mapping

Use an administrator-configured `deployment_id` as the stable prefix. Do not use a
human-readable cluster display name as the only deployment identity.

| Proxmox object | Proposed Espial kind | Proposed external ID |
|---|---|---|
| Cluster/deployment | `hypervisor_cluster` | `proxmox:<deployment-id>:cluster` |
| Node | `hypervisor_node` | `proxmox:<deployment-id>:node:<node-id>` |
| QEMU virtual machine | `virtual_machine` | `proxmox:<deployment-id>:qemu:<vmid>` |
| LXC container | `container` | `proxmox:<deployment-id>:lxc:<vmid>` |
| Storage view | `storage` | `proxmox:<deployment-id>:storage:<scope>:<storage-id>` |

Proxmox VMIDs are cluster-wide identifiers and are suitable for guest identity
when combined with the guest type and Espial deployment ID. Node and storage
identity must be confirmed against the actual deployment. Local storage with the
same storage ID on several nodes may require the node in `<scope>`; shared storage
must not be duplicated or collapsed until fixtures establish its semantics.

A guest's current node may be stored as a bounded attribute for source context,
but that is not a Phase 4 dependency relationship. Resource `source_url` values
must be assembled only from the approved UI base and known version-checked path
forms. Never copy an arbitrary response URL into `source_url`.

## 7. Check taxonomy and health semantics

The first implementation should prefer a few explainable checks:

| Check type | Suggested behavior |
|---|---|
| `proxmox.cluster.quorum` | `critical` when authoritative cluster evidence says quorum is lost; otherwise `healthy` |
| `proxmox.node.availability` | `critical` for an explicitly offline node; `healthy` for online |
| `proxmox.guest.runtime` | `healthy` when running; `disabled` when stopped unless that typed guest ID is configured as expected-running |
| `proxmox.storage.availability` | `critical` when authoritative source evidence says unavailable; otherwise `healthy` |
| `proxmox.storage.capacity` | `warning`/`critical` only at validated site-configured thresholds |

An expected-running guest that is stopped may be `critical`. A guest without that
policy must not create an incident merely because it is intentionally stopped.
Templates and intentionally unmonitored objects should use factual `disabled`
evidence or be excluded by a documented bounded filter.

API unavailability, authentication failure, timeout, malformed data, or an adapter
crash is an integration/collection failure. The adapter must not manufacture a
critical observation for every last-known Proxmox resource; Core's existing
freshness path makes their old observations stale and then unknown.

Current CPU, memory, disk, and uptime values may be emitted as a small set of
bounded measurements when they explain a selected check. The first version should
not retain RRD history or add CPU/memory alerts without an explicit site policy and
a decision that the signal is not already authoritative elsewhere.

## 8. Proposed adapter manifest and configuration

The initial manifest should declare approximately:

```text
adapter_id: org.ubnetdef.espial.proxmox
protocol_versions: ["1.0"]
integration_category: hypervisor
capabilities: ["collect"]
read_only: true
resource_types: [hypervisor_cluster, hypervisor_node, virtual_machine, container, storage]
check_types: [proxmox.cluster.quorum, proxmox.node.availability,
              proxmox.guest.runtime, proxmox.storage.availability,
              proxmox.storage.capacity]
secret_fields: [token_secret]
```

A bounded first configuration can include:

```text
base_url
deployment_id
token_id
token_secret                  # secret reference only
expected_refresh_seconds
request_timeout_seconds
expected_running_guests      # typed values such as qemu:100 or lxc:200
storage_warning_percent
storage_critical_percent
```

An approved TLS trust bundle, target host/address/port allowlist, response-size
cap, and any future endpoint-failover list belong to trusted deployment/adapter
policy. An integration API must not be able to select arbitrary executable paths,
CA files, proxy settings, or egress targets.

## 9. Espial process behavior

The executable follows adapter protocol v1.0:

1. Write exactly one `ready` notification after local initialization.
2. Answer `manifest` with identity, capability, resource/check types, and bounded
   configuration schema.
3. Answer `validate_config` only after structural, threshold, URL, and immutable
   policy validation succeeds.
4. On `collect`, authenticate to Proxmox, retrieve the bounded endpoint set, map it
   to resources/observations, and return exactly one terminal response.
5. Answer `health` about the adapter process, not Proxmox resource health.
6. Stop new work and exit cleanly after `shutdown`.

Every stdout line is protocol output. Diagnostics go to bounded stderr and must be
safe after secret redaction. The adapter respects the request envelope deadline
and cancels network work when it expires. Push `event` notifications remain
disabled.

## 10. HTTP and data safety

The Proxmox client must enforce:

- HTTPS with normal hostname and chain verification;
- exact approved host, resolved address, and port policy;
- no redirects and no ambient proxy environment;
- explicit connect, TLS, response-header, and whole-request deadlines bounded by
  the Espial envelope deadline;
- bounded response headers, body bytes, decoded records, and output size;
- no credential, authorization header, raw body, or internal server error in logs,
  protocol errors, observations, or audit evidence;
- stable safe error categories for unauthorized, forbidden, throttled, timeout,
  temporary upstream failure, malformed data, and oversized response; and
- deterministic behavior for missing resources and partial source responses.

Do not disable TLS verification for a self-signed Proxmox certificate. Mount and
approve the Proxmox cluster CA or another deployment-managed trust bundle instead.

## 11. Fixtures and verification

Before a live integration is enabled, retain sanitized fixtures for:

- normal single-node or clustered operation at representative size;
- node offline;
- quorum lost;
- expected-running QEMU guest stopped;
- expected-running LXC guest stopped;
- intentionally stopped or template guest;
- storage unavailable and storage warning/critical capacity boundaries;
- API `401`, `403`, throttling, and retryable server failure;
- connect/TLS/response timeout;
- malformed, partial, and unexpected-type response;
- oversized source collection and oversized normalized output; and
- additive unknown Proxmox response fields.

Build and test in this order:

1. Parse version-matched sanitized fixtures.
2. Implement pure Proxmox-to-Espial identity and health mapping functions.
3. Implement the bounded authenticated HTTP client.
4. Add the NDJSON protocol loop and manifest/config validation.
5. Run Espial's black-box conformance and process-failure matrix.
6. Test stale/unknown transitions, incident deduplication/recovery, redaction, and
   restart isolation through Core.
7. Run a live read-only non-production collection using the approved token and
   compare it with the sanitized fixtures.
8. Verify representative and oversized collection time, payload size, database
   work, and scheduler behavior before setting defaults.

## 12. Known Core preparation

The current Core runtime has an immutable adapter registry, but application wiring
registers only the sample and webcheck executables explicitly. Phase 3 needs a
secure generic trusted-adapter registration mechanism so adding or upgrading the
Proxmox executable does not require vendor-specific Core code.

The v1.0 collection response includes optional `next_cursor`, but current Core does
not request subsequent adapter collection pages. Until bounded pagination is
implemented and tested, one normalized Proxmox collection must fit in one protocol
line below the 1 MiB limit. Actual cluster cardinality and oversized fixtures decide
whether pagination is required before the adapter can be enabled.

These are Core/platform responsibilities, not reasons for the adapter to bypass
the protocol, split data nondeterministically, start child processes, or write
directly to Espial's database.

## 13. Change checklist for future students

When changing the adapter:

1. Name the Proxmox versions and deployment topology the change supports.
2. Link the version-matched API Viewer endpoint or official documentation used.
3. Add or update sanitized positive, degraded, failure, and oversized fixtures.
4. Document any resource identity, check taxonomy, threshold, or retention change.
5. Prove the token still has only the required read privileges.
6. Keep secrets, raw responses, and unbounded vendor data out of Espial models.
7. Run protocol conformance, unit, fault, restart, redaction, Core integration, and
   load tests.
8. Update this reference, the environment inventory, operations guidance, and any
   affected schema/ADR before acceptance.
9. Do not add a route, navigation child, or capability statement until the
   authoritative read path, permissions, states, source links, and UI acceptance
   evidence all exist.

An adapter binary being present is not completion. Acceptance requires a bounded
read-only source contract, repeatable fixtures, conformance/security evidence,
operational ownership, and honest rendering through Espial's standard monitoring
path.
