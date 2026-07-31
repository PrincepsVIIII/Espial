# Espial deployments

Deployment artifacts will implement the accepted
[single-host topology](../docs/architecture/DEPLOYMENT.md): reverse proxy, Espial
Web, Espial Core, PostgreSQL, and isolated adapter processes/containers.

Phase 1 includes a [local developer stack](local/README.md) for PostgreSQL and Core.
A later slice adds a pinned production example with
secret references, health checks, resource limits, persistent database storage,
and backup/restore guidance. No real secret belongs in this directory.
