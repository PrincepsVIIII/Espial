# Phase 1 threat review and vulnerability triage

**Reviewed:** 2026-07-31  
**Scope:** public entry, local authentication/session/RBAC, REST/SSE, audit writes,
PostgreSQL, supervised adapters, local and production-example containers.  
**Triage owner:** Espial maintainer. Production release acceptance remains with the
UBNetDef Security/Operations owner named before deployment.

| Threat | Phase 1 controls | Residual decision |
|---|---|---|
| Credential theft and local fallback abuse | Argon2id, 15–128-character denylist-checked passwords, hidden bootstrap/reset input, generic failures, lockout/rate limit, audited local use, no default password | Review local administrators each semester; SSO remains external |
| Session fixation, CSRF, and origin spoofing | Random hashed session tokens, rotation on login, idle/absolute expiry, HttpOnly/Secure/SameSite cookies, CSRF token plus exact trusted origin | TLS/public URL are production preflight inputs |
| Authorization escalation | Direct Core permission checks and 401/403 tests; audited create/role/enable/reset; role changes revoke sessions | Named role/group owner required for SSO |
| Secret leakage | File references, redacted config/logs/API models, isolated adapter environment, ignored secret paths, secret scanner | Production secret manager remains TBD (Security/Operations) |
| Adapter escape or output abuse | No shell, configured trusted executable, bounded NDJSON message/output/time/concurrency/restarts, restricted environment, cancellation and crash tests | Container/OS sandbox per real adapter remains an owned production decision |
| SSRF/config misuse | No Phase 1 arbitrary HTTP adapter; strict config parsing/bounds; production adapter egress is deny-by-default per integration | Concrete allowlists required before each real adapter |
| Audit tampering | Transactions fail closed for protected mutations; database trigger rejects audit UPDATE/DELETE; app role receives SELECT/INSERT only | Backup administrators remain privileged and audited operationally |
| Denial of service | Login limits/lockout, bounded HTTP bodies/cursors, adapter deadlines/message size/concurrency/backoff, scheduler coalescing, SSE client/replay caps, DB pool and container resource/log limits | Site scale/load target remains TBD; measured baseline provides initial headroom |

## Automated evidence

- production dependency audits: root `0`, Web `0` known vulnerabilities at install;
- `govulncheck v1.6.0`: no reachable Go vulnerability in Espial code;
- current Trivy `0.72.0`: repository high/critical misconfiguration/secret gate
  passed; Espial Core and Web images contain zero high/critical findings;
- the acceptance harness proves hidden bootstrap, anonymous `401`, Viewer `403`,
  lifecycle/audit evidence, stale/unknown/recovery, bounded shutdown, and restore;
- PostgreSQL integration tests prove audit insert succeeds and update/delete fail.

## Upstream production-image blockers

The newest official images available during review are pinned, but the 2026-07-31
Trivy database reports fixed-version findings in components not yet rebuilt by
their publishers:

- **Caddy 2.11.4-alpine:** high findings in Alpine `c-ares`/`curl`, Go standard
  library, `x/text`, and gRPC. Owner: Espial maintainer to rescan and repin; Security
  owner to decide reachability/risk. **Production blocked until an updated clean
  image is pinned or a written, time-bounded exception is approved.**
- **PostgreSQL 17.10-alpine3.23:** one critical and high findings are confined to
  `/usr/local/bin/gosu`'s embedded Go toolchain, not the PostgreSQL server. Owner:
  Espial maintainer to repin/rescan or publish a reviewed non-root image without
  `gosu`; Security owner validates reachability. **Production blocked until clean
  or explicitly accepted.**

No critical/high finding is unowned. Scanner ignores must cite this record, exact
CVE, affected image digest, reachability, approver, and expiry; Phase 1 adds no
blanket ignore file.
