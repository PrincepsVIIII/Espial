# Temporary local authentication runbook

Local authentication is Espial's controlled bootstrap and fallback path while the
UBNetDef SSO contract is unfinished. It is real authentication—not a development
bypass—and every successful use emits `auth.local.used`.

## Start and bootstrap locally

From the repository root:

```sh
cp .env.example .env
npm install
npm --prefix web ci
npm run dev
```

In another terminal, create the first administrator:

```sh
docker compose --env-file .env -f deployments/local/compose.yml run --rm core admin bootstrap --username admin
```

The command reads the password twice without echoing it. Use 15–128 Unicode
characters and avoid common passwords. It does not accept a password in an argument
or environment variable. Once one Administrator assignment exists, every further
bootstrap attempt fails transactionally, including concurrent attempts.

Open `http://localhost:5173/login`, sign in, and confirm the protected Overview
page appears. Core remains available on loopback port `18080` for health checks.

## Operational controls

| Control | Default |
|---|---|
| Source-address login attempts | 10 per minute |
| Account lockout | Five failed passwords for 15 minutes |
| Session idle timeout | 30 minutes |
| Session absolute lifetime | 12 hours |
| Session cleanup | Every 15 minutes while Core runs |

Session and CSRF values are random 256-bit secrets. The database stores only their
SHA-256 digests. The session cookie is HttpOnly and both cookies are Secure outside
loopback development, SameSite Lax, and scoped to `/`.

## Recovery and transition cautions

- There is deliberately no second-bootstrap or password-in-environment escape
  hatch. Before production use, add the audited user/password and role-management
  flows planned after this foundational slice.
- Direct database edits are not a supported password reset procedure. They can
  bypass auditing and must not become a runbook shortcut.
- Disabling a user immediately makes existing sessions unusable. Role reads are
  refreshed on each request; role-management code must also rotate or revoke the
  affected sessions.
- Setting `ESPIAL_AUTH_MODE` to `sso` or `sso_with_local_fallback` currently stops
  startup with a clear validation error. Do not enable either until a real provider
  completes the SSO readiness contract.
- Before any shared deployment, benchmark Argon2id on the production Core hardware,
  set HTTPS on `ESPIAL_PUBLIC_URL`, replace example database credentials, and define
  who reviews `auth.login.failed`, `auth.local.used`, and session-revocation events.

The durable security contract is in
[Authentication and authorization](../architecture/AUTHENTICATION.md).
