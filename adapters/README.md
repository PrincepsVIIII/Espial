# Espial adapters

Adapters are isolated processes that translate vendor/service APIs into normalized
Espial resources and observations. They do not run in Espial Core and do not receive
monitoring-derived action authority.

Phase 1 begins with a deterministic sample adapter that implements the
[NDJSON adapter protocol](../docs/architecture/ADAPTER_PROTOCOL.md) and serves as a
conformance fixture. Machine-readable contracts live under [`api/`](../api/README.md).
Vendor adapters follow after the monitoring foundation is trustworthy.
