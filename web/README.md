# Espial Web

This directory contains the standalone SvelteKit/TypeScript browser application.
It presents Core state and sessions; authorization and domain decisions remain in
Core.

The implemented authentication and webpage skeleton includes:

- a local login page with password-manager-compatible fields and generic errors;
- a public informational root with no operational data and a top-right login action;
- server-advertised auth capabilities, with unfinished SSO hidden;
- local-only return-path validation;
- an authoritative `/api/v1/auth/session` check around the application route group;
- a Dashboard-first protected shell with Dashboard, Alerts, Datacenter, Hypervisor,
  and Webpages in a responsive top navigation bar;
- honest unavailable/not-configured states for monitoring domains that Core does not
  expose yet, plus an explicit Core-unavailable shell state;
- an `/overview` compatibility redirect to `/dashboard` and session-bound CSRF
  logout; and
- a same-origin server proxy to Core, configured by `ESPIAL_CORE_URL`.

No password, session token, CSRF token, or role decision is persisted in local
storage. The browser receives the session in an HttpOnly cookie and the CSRF secret
in a separate readable cookie.

## Visual foundation

Web is dark-first and visibly attributed to UBNetDef. Deep navy/Harriman Blue
anchors the product chrome, UB Blue is reserved for selection and action, and green
is reserved for healthy/success state. A restrained dark-blue linear gradient is
permitted only in structural navigation chrome; content surfaces stay flat and do
not use ambient gradients, glass effects, generic dashboard composition, or
invented data.

The protected shell uses Dashboard, Alerts, Datacenter, Hypervisor, and Webpages in
that order. Dashboard is the post-login destination, with Datacenter one click away.
The primary navigation collapses into a labeled menu on narrow screens; account and
sign-out access move into that menu instead of forcing horizontal overflow.

Read the [general UI guidance](../docs/design/UI_GUIDANCE.md) before any UI change,
use the [design system](../docs/design/DESIGN_SYSTEM.md) as its token/component
contract, and record material reviews in the [UI review log](../docs/design/UI_REVIEW.md).
Official University at Buffalo marks may only come from approved assets; the current
interface uses a plain typographic product lockup.

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
