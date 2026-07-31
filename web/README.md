# Espial Web

This directory contains the standalone SvelteKit/TypeScript browser application.
It presents Core state and sessions; authorization and domain decisions remain in
Core.

Slice 1.2 includes:

- a local login page with password-manager-compatible fields and generic errors;
- server-advertised auth capabilities, with unfinished SSO hidden;
- local-only return-path validation;
- an authoritative `/api/v1/auth/session` check around the application route group;
- a protected overview placeholder and session-bound CSRF logout; and
- a same-origin server proxy to Core, configured by `ESPIAL_CORE_URL`.

No password, session token, CSRF token, or role decision is persisted in local
storage. The browser receives the session in an HttpOnly cookie and the CSRF secret
in a separate readable cookie.

## Commands

From this directory:

```sh
npm install
npm run dev
npm run check
npm run test
npm run build
```

The local Compose stack sets `ESPIAL_CORE_URL=http://core:8080`. A host development
server defaults to `http://127.0.0.1:8080` unless that environment variable is set.
UI direction lives in the [frontend plan](../docs/FRONTEND_UI_PLAN.md).
