# Espial deployments

Deployment artifacts will implement the accepted
[single-host topology](../docs/architecture/DEPLOYMENT.md): reverse proxy, Espial
Web, Espial Core, PostgreSQL, and isolated adapter processes/containers.

Phase 1 adds a local developer stack first, then a pinned production example with
secret references, health checks, resource limits, persistent database storage,
and backup/restore guidance. No real secret belongs in this directory.
