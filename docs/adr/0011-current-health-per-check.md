# ADR 0011: Current health is scoped to a resource and check type

- Status: Accepted
- Date: 2026-08-27

## Context

Some adapters emit several independent observations for one normalized resource.
For example, Proxmox storage has availability and capacity checks. The former has
no capacity measurements, so a single resource-level current-health row allowed a
same-time or newer availability observation to replace the capacity observation.
Read ordering then made the storage display intermittently show zero used and
total bytes even when the capacity check was current.

## Decision

`current_health` is keyed by `(resource_id, check_type)`. Ingestion, freshness
transitions, durable freshness signals, incident evaluation, and check-scoped
maintenance all preserve that scope. A check is current until a later observation
for the same resource and check type arrives.

Generic resource reads use the `current_resource_health` database view. It chooses
one deterministic state per resource by severity, then observation time, update
time, and check type. Check-specific read models query the exact current check;
the Proxmox storage view always obtains bytes from
`proxmox.storage.capacity`.

## Consequences

- Independent health dimensions cannot overwrite each other.
- Generic resource lists remain one row per resource and surface the most urgent
  applicable state.
- A resource with no observations has no current-check row and is presented as
  unknown by generic reads.
- ADR 0010's separate certificate resource remains useful for certificate identity
  and its bounded projection, but is no longer required merely to avoid a
  current-health overwrite.
