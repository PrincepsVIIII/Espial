# Initial deployment baseline

The accepted topology is a single dedicated management host with separately
lifecycle-managed services.

```text
Operator browser
      |
      | HTTPS
      v
Reverse proxy (only published service)
      |----------------------|
      v                      v
Espial Web               Espial Core
                             |  |-------- supervised adapter processes
                             |  |-------- SSE clients through proxy
                             v
                         PostgreSQL
```

## Environments

- **Developer:** Web and Core may run on the host; PostgreSQL and the sample adapter
  may run in containers. Local authentication is allowed and secure cookies may be
  relaxed only on loopback.
- **Test/CI:** ephemeral PostgreSQL, deterministic sample adapter, fixed clock where
  needed, and no external infrastructure calls.
- **Production:** Compose-compatible pinned images on the management host, private
  service network, TLS at the reverse proxy, persistent database volume, secret
  files/store, resource limits, and external heartbeat.

## Network boundaries

Publish only HTTPS. PostgreSQL is never host-public. Core accepts Web traffic and
admin health checks on the private network. Each adapter receives only required
destination access. Core's Mattermost client additionally requires exact host,
resolved-CIDR, and port allowlists, pins validated DNS answers, uses HTTPS, and
ignores ambient proxies and redirects. The production `private` network is internal;
an internal Mattermost service must join an explicitly managed reachable network.
Future sandboxing should deny filesystem and network access by default where the
runtime supports it. The proxy preserves the originating address only from trusted
proxy hops.

## Configuration and secrets

Non-secret configuration comes from one validated file plus environment overrides.
Secret configuration is represented by references to mounted files or an approved
secret provider. Startup logs list sources and overridden key names, never values.
Core fails closed on unknown configuration keys in production. Mattermost tokens are
individual files beneath `/run/secrets`; destination JSON stores only the opaque file
name. Empty notification host/CIDR allowlists disable destination configuration.

## Startup and shutdown

PostgreSQL readiness precedes migrations. Core migrates/checks schema, initializes
repositories, starts adapters and workers, then reports ready. Web reports ready
only when its server is running; it presents an upstream-unavailable state if Core
is down. On termination, Core stops accepting mutations, closes SSE streams,
cancels scheduled work, requests adapter shutdown, flushes audit writes, and closes
the database within a bounded deadline.

## Production readiness checklist

- Host and failure domain selected and recorded in the environment inventory.
- TLS certificate and trusted proxy configuration tested.
- Local fallback administrator bootstrapped and recovery procedure tested.
- PostgreSQL backup schedule plus restore drill completed.
- Images pinned by immutable tag or digest and dependency scans enabled.
- CPU, memory, open-file, log-size, and adapter restart limits set.
- SSE buffering disabled and reconnect tested through the proxy.
- External heartbeat alerts when the entire Espial host is unreachable.
- All `Production blocker` items in the environment inventory closed.

High availability and cross-host failover are out of MVP scope. A restore procedure
and last-known data durability are not.
