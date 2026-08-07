# Espial adapters

Adapters are isolated processes that translate vendor/service APIs into normalized
Espial resources and observations. They do not run in Espial Core and do not receive
monitoring-derived action authority.

Phase 1 begins with a deterministic sample adapter that implements the
[NDJSON adapter protocol](../docs/architecture/ADAPTER_PROTOCOL.md) and serves as a
conformance fixture. Phase 2 adds the trusted `webcheck` adapter for bounded DNS,
TCP, TLS, HTTP status/latency, redirect, and exact-content availability checks. It
uses explicit host/address/port policy, pinned connections, no ambient proxy, and
secret references; it never emits response bodies or protected header values.
Machine-readable contracts live under [`api/`](../api/README.md).

Phase 3 begins its infrastructure-adapter work with Proxmox while UBNetDef SSO is
deferred in development sequencing. Students building or changing that adapter
must follow the [Proxmox adapter authoring reference](../docs/architecture/PROXMOX_ADAPTER.md).
The reference is guidance and discovery scope; it does not claim that a Proxmox
adapter or Hypervisor capability is implemented.
