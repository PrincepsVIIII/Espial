# Production deployment example

This directory is a hardened, single-host reference deployment. It is not a
turn-key UBNetDef production release: the host, DNS, TLS reachability, secret
manager, backup target, published Espial image digests, and current scanner
exceptions must be approved first. Those blockers are owned in the
[environment inventory](../../docs/operations/ENVIRONMENT_INVENTORY.md).

Only Caddy publishes host ports. Web, Core, the one-shot migration job, and
PostgreSQL use the private network. Runtime Core uses the least-privilege
`espial_app` database role; migrations use the separate owner role.

## Prepare and validate

1. Copy `.env.example` to a host-owned file outside the clone and replace both
   Espial image values with immutable registry digests.
2. Create the four mode-`0600` files described in [secrets/README.md](secrets/README.md).
3. Render and inspect the effective configuration:

   ```sh
   docker compose --env-file /secure/path/espial.env -f deployments/production/compose.yml config
   ```

4. Complete the [deployment preflight](../../docs/operations/DEPLOYMENT_RUNBOOK.md)
   and current vulnerability triage before starting the stack.
5. Run the migration job, start services, then bootstrap the first administrator:

   ```sh
   docker compose --env-file /secure/path/espial.env -f deployments/production/compose.yml run --rm migrate
   docker compose --env-file /secure/path/espial.env -f deployments/production/compose.yml up -d
   docker compose --env-file /secure/path/espial.env -f deployments/production/compose.yml run --rm core admin bootstrap --username admin
   ```

The password prompt does not echo or accept a password through an argument or
environment variable. Never use an example credential.

## Security properties

- immutable upstream image digests and required immutable Espial image references;
- non-root application images, read-only filesystems, dropped capabilities, and
  `no-new-privileges`;
- bounded CPU, memory, temporary storage, adapter concurrency, SSE clients, and
  JSON log rotation;
- persistent PostgreSQL and Caddy certificate data;
- TLS termination and unbuffered long-lived SSE at the proxy;
- separate migration-owner and runtime-application database credentials;
- no published Core, Web, or PostgreSQL port.
- private-only `/metrics` with fixed low-cardinality Phase 2 queue/state labels;
- bounded incident and notification worker settings backed by the Phase 2 load gate.

Use the [operations runbooks](../../docs/operations/README.md) for backup, restore,
upgrade, diagnosis, account recovery, and shutdown.
