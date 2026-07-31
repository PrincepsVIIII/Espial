# Espial deployments

Deployment artifacts will implement the accepted
[single-host topology](../docs/architecture/DEPLOYMENT.md): reverse proxy, Espial
Web, Espial Core, PostgreSQL, and isolated adapter processes/containers.

Phase 1 includes a [local developer stack](local/README.md) and a hardened,
[pinned production example](production/README.md) with secret references, health
checks, resource limits, a private application network, persistent storage, and
SSE-aware TLS proxying. No real secret belongs in this directory.
