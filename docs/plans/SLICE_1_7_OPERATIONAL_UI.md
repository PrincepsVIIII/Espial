# Slice 1.7 implementation record: SvelteKit operational UI

**Status:** Implemented  
**Completed:** 2026-07-31  
**Parent:** [Phase 1 implementation plan](PHASE_1_IMPLEMENTATION.md)  
**UI contract:** [General UI guidance](../design/UI_GUIDANCE.md)  
**API contract:** [Slice 1.6 REST/SSE record](SLICE_1_6_REST_SSE_API.md)

## Outcome

Slice 1.7 replaces the authenticated Dashboard placeholder with authoritative
Phase 1 monitoring. An operator can identify unhealthy, stale, and unknown
resources; inspect integration coverage and current reasons; filter resources; and
see whether live invalidations are connected within seconds.

The accepted public root, local login, exact five-item top navigation, Dashboard
post-login destination, user menu, and honest unavailable states remain intact.
Alerts, Datacenter, Hypervisor, and Webpages still show only implemented domain
availability; no incident, topology, room, device, VM, or website data is invented.

## Data and trust boundary

The authenticated layout now verifies the session through a SvelteKit loader.
Dashboard's loader concurrently reads overview, the filtered resource page, and the
first stable integration page through the same-origin `/api/v1` proxy. It uses the
TypeScript models generated from shared Slice 1.6 schemas.

Each request includes same-origin credentials and a request ID. Structured Core
errors are reduced to bounded safe code/message/request-ID data. A `401` redirects
to login with a local validated return path; `403` renders a permission explanation;
Core/network failures retain the authenticated shell when the session is already
known and replace old operational rows with an unavailable state.

Neither loaders nor the live client store credentials, sessions, resources,
integrations, filters, or operational responses in local storage. The browser only
contacts Web's same-origin proxy; Core remains the authorization and domain source.

## Dashboard composition

The page uses compact divided operational structures rather than a generic card
grid:

- a state-count strip for healthy, warning, critical, unknown, stale, maintenance,
  and disabled resources;
- a resource table with visible state icon/label, integration, kind, authoritative
  observation timestamp, and current reason;
- a filter bar for state, kind, integration, and exact stale selection;
- a stable next-page link that preserves filters and returns Core's opaque cursor;
- divided integration rows with runtime state, resource/stale/unknown coverage, and
  last collection result/time; and
- explicit loading/refresh, no-match, no-resource, no-integration, permission,
  partial failure, and Core-unavailable states.

Filters are reflected in the Dashboard URL and normalized before being sent to
Core. Applying new filters omits the previous cursor. The Web layer does not copy
health rules: counts, current states, reasons, and runtime state all come from v1.
The only derived value is a presentation priority for the page label—critical,
warning, stale, unknown, then healthy.

Status indicators always contain a visible symbol and label. Unknown and stale use
distinct words, symbols, borders, and semantic colors. Relative timestamps expose
absolute UTC and local values through a focusable/hoverable detail.

## Live invalidation behavior

The shell starts one authenticated fetch-based SSE reader after hydration. Unlike a
native `EventSource`, the reader can explicitly preserve numeric `Last-Event-ID`
while using bounded exponential reconnect with ±20% jitter and a 30-second cap.

The top bar exposes `Live`, `Reconnecting`, or `Disconnected`. Domain events are
coalesced over a short interval and invalidate the SvelteKit monitoring dependency;
the page then refetches authoritative REST state. A `resync_required` event clears
the replay cursor and refreshes every visible monitoring read. Invalid JSON and
unknown frames do not enter UI state. `401`/`403` closes the reader and returns to
login; component teardown and logout abort the request and pending timers.

The last-successful-refresh value changes only when overview REST data succeeds.
An outage therefore never labels retained data as newly refreshed.

## Responsive and accessibility behavior

The existing top navigation remains unchanged on desktop and collapses to one
labeled menu below the established breakpoint. The collapsed menu preserves
Dashboard, Alerts, Datacenter, Hypervisor, and Webpages in order and includes the
current role, live state, and sign out.

At 500px, resource table rows become compact labeled records without dropping any
critical column. Integration measurements wrap below identity/state. All controls
retain visible focus; the user menu closes on Escape and returns focus to its
trigger. Reduced-motion CSS caps transitions and animations without changing live
connection behavior.

See the [UI review record](../design/UI_REVIEW.md) for the inspected 1440px, 1280px,
and 500px findings.

## Verification

- 36 Vitest tests cover generated-client safety, URL normalization, status
  semantics, timestamp presentation, SSE chunk parsing and replay headers, and the
  existing public/navigation contracts.
- 11 Playwright Chromium tests use a deterministic out-of-process mock Core to test
  real SSR and browser behavior: public disclosure, filters, keyboard menus,
  permission/outage/unauthenticated states, reconnect, full resync, reduced motion,
  and all three required viewports.
- axe reports no public-page violations and no serious or critical Dashboard
  violations.
- Svelte diagnostics report zero errors and warnings; the adapter-node production
  SSR build succeeds.
- The mock Core and sample records live exclusively under `web/tests` and are not
  imported by application code or emitted in the production build.

## Slice 1.8 handoff

Slice 1.8 should run the real clean-stack acceptance sequence through this UI:

1. Bootstrap and sign in without a default password.
2. Create/enable the registered sample integration and observe Dashboard coverage.
3. Crash and recover the adapter while verifying live-driven REST refresh, then
   stale and unknown transitions.
4. Confirm a Viewer sees monitoring but receives a safe denial for administrator
   APIs.
5. Validate reverse-proxy SSE buffering/timeouts, secure cookies, production
   origins, graceful shutdown, backup/restore, and resource limits.
