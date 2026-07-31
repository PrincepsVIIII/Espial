# Temporary local authentication runbook

Local authentication is Espial's controlled bootstrap and fallback path while the
UBNetDef SSO contract is unfinished. It is real authentication—not a development
bypass—and every successful use emits `auth.local.used`.

## Start and bootstrap locally

For a new clone, run the one-command initializer from the repository root:

```sh
npm install
npm --prefix web ci
npm run init
```

It creates local secret files when absent, starts PostgreSQL, runs migrations,
prompts for the first administrator, and starts the stack. For an existing `.env`,
start with `npm run dev` and bootstrap in another terminal if needed:

```sh
docker compose --env-file .env -f deployments/local/compose.yml run --rm core admin bootstrap --username admin
```

The command reads the password twice without echoing it. Use 15–128 Unicode
characters and avoid common passwords. It does not accept a password in an argument
or environment variable. Once one Administrator assignment exists, every further
bootstrap attempt fails transactionally, including concurrent attempts.

Open `http://localhost:5173/login`, sign in, and confirm the Dashboard appears.
Core remains available on loopback port `18080` for health checks.

## Audited user and role administration

All commands prompt for passwords without echoing them and require database access.

```sh
docker compose --env-file .env -f deployments/local/compose.yml run --rm core admin user create --username operator --role Viewer
docker compose --env-file .env -f deployments/local/compose.yml run --rm core admin user role --username operator --role Operator
docker compose --env-file .env -f deployments/local/compose.yml run --rm core admin user password --username operator
docker compose --env-file .env -f deployments/local/compose.yml run --rm core admin user disable --username operator
docker compose --env-file .env -f deployments/local/compose.yml run --rm core admin user enable --username operator
```

Create/role/reset/enable/disable operations write audit events. Role, password, or
enabled-state changes revoke that user's sessions. There is no default password and
no command returns a password.

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
  hatch. Use the audited password command for recovery; keep at least two reviewed
  local administrators before relying on this as emergency access.
- Direct database edits are not a supported password reset procedure. They bypass
  auditing and must not become a runbook shortcut.
- Disabling, resetting, or changing a user's role revokes existing sessions.
- Setting `ESPIAL_AUTH_MODE` to `sso` or `sso_with_local_fallback` currently stops
  startup with a clear validation error. Do not enable either until a real provider
  completes the SSO readiness contract.
- Before any shared deployment, benchmark Argon2id on the production Core hardware,
  set HTTPS on `ESPIAL_PUBLIC_URL`, replace example database credentials, and define
  who reviews `auth.login.failed`, `auth.local.used`, and session-revocation events.

The durable security contract is in
[Authentication and authorization](../architecture/AUTHENTICATION.md).
