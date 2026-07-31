# Authentication and authorization

This contract lets Phase 1 ship safely before UBNetDef SSO is ready while preserving
the required emergency access path.

## Modes and transition

| Mode | Interactive providers | Intended use |
|---|---|---|
| `local` | Local username/password | Phase 1 development and controlled initial rollout |
| `sso_with_local_fallback` | SSO plus deliberate local route | Eventual production default |
| `sso` | SSO only | Optional only after operators accept the lockout risk |

The provider authenticates an identity. A provider-neutral Core service creates the
session, loads roles, enforces permissions, and records audit events. Frontend code
never makes an authorization decision on behalf of Core.

## Phase 1 local login

- Route: `POST /api/v1/auth/local/login` with `username` and `password`.
- Bootstrap: `espial admin bootstrap --username NAME` reads a password twice from
  an attached terminal or stdin. It fails once any administrator exists.
- Storage: normalized usernames and Argon2id password hashes; no recoverable
  password and no password in arguments, logs, or environment variables.
- Response: a generic authentication error for unknown users, wrong passwords, and
  disabled users.
- Controls: a per-address rate limit, timed per-account lockout, constant-time hash comparison,
  request-size limits, and structured audit outcomes without submitted usernames
  beyond the normalized attempted identity.
- UI: the login page clearly labels local access. Once SSO exists, local fallback is
  behind an “Emergency local access” disclosure rather than presented as equal.

## Sessions

- Generate at least 256 bits of randomness; store only a SHA-256 digest server-side.
- Cookie name: `espial_session`; `HttpOnly`, `Secure` outside loopback development,
  `SameSite=Lax`, `Path=/`, and no JavaScript access.
- Idle timeout: 30 minutes by default; absolute lifetime: 12 hours by default.
- Rotate the token through the session service; role-management work must rotate or
  revoke affected sessions when it changes privileges.
- Revoke on logout, account disable, role change, expiration, and administrator
  action.
- Protect state-changing requests with allowed-origin checks and a session-bound
  CSRF token. `GET`, `HEAD`, and `OPTIONS` remain side-effect free.

## Initial roles

Permissions are named actions so roles can evolve without route-specific checks.

| Permission | Viewer | Operator | Administrator | Action Approver |
|---|:---:|:---:|:---:|:---:|
| `overview:read`, `resources:read`, `integrations:read` | Yes | Yes | Yes | Yes |
| `audit:read` | No | No | Yes | No |
| `incidents:operate` | No | Yes | Yes | No |
| `integrations:manage`, `users:manage`, `roles:manage` | No | No | Yes | No |
| `actions:approve` | No | No | No | Reserved |

Phase 1 implements the first row and administrator management boundaries. Incident
and action permissions exist as reserved names until their roadmap phases.

## Provider interface

The Go boundary should be small and protocol-independent:

```text
IdentityProvider
  Name() string
  BeginLogin(context, returnURL) -> redirect or local-form instruction
  CompleteLogin(context, callback data) -> ExternalIdentity

ExternalIdentity
  provider, subject, display_name, email, groups
```

Local authentication may use a dedicated credential verifier rather than pretending
to redirect, but it must return the same internal identity result. SSO claim/group
mapping is configuration, validated before enabling SSO.

## SSO readiness contract

The SSO team must supply protocol and issuer, client registration process, callback
and logout behavior, stable subject claim, display/email claims, group claims,
signing-key discovery and rotation, clock-skew guidance, test environment, and
expected outage behavior. OIDC Authorization Code with PKCE is preferred if offered;
the implementation must follow the actual confirmed protocol.

## Required audit events

`auth.login.succeeded`, `auth.login.failed`, `auth.logout`, `auth.session.rotated`, `auth.session.revoked`,
`auth.local.bootstrap`, `auth.local.used`, `auth.user.disabled`, and
`auth.roles.changed` include actor/session when known, source address, result,
correlation ID, and target identity. They never include passwords, cookies, tokens,
or raw SSO assertions.
