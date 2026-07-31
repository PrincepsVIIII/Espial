# ADR-0006: Local authentication now, provider-based SSO transition later

- Status: Accepted
- Date: 2026-07-30

## Context

UBNetDef SSO is unfinished and its protocol cannot yet be confirmed. Blocking all
of Phase 1 on SSO would prevent end-to-end authorization testing. Espial must also
remain accessible to authorized administrators when SSO is the failed dependency.

## Decision

Implement a shared session and role model with pluggable identity providers. Phase
1 starts in `local` mode using an explicitly bootstrapped administrator and audited
username/password login. Supported configuration modes are:

- `local`: temporary development and initial deployment mode;
- `sso_with_local_fallback`: eventual production default;
- `sso`: available only when operators deliberately disable local fallback.

Local passwords use Argon2id hashes. Session tokens are random, stored only as
hashes, and delivered in secure, HTTP-only cookies. Every local login attempt and
all local-account changes are audited. The bootstrap command works only when no
administrator exists and reads the password without command-line or environment
exposure.

## Consequences

The temporary path is real authentication, not a hard-coded dev bypass. All roles
are enforced in Core regardless of identity provider. SSO work later implements the
provider interface and claim mapping; it does not replace sessions or authorization.
Production must rate-limit login, require TLS, review emergency account access, and
alert on successful fallback use.
