# UI review notes

This log records material visual-direction reviews so UI quality remains a delivery
constraint instead of end-of-phase polish.

## 2026-07-30 — Pre-Slice 1.3 foundation review

### Scope

Reviewed the implemented local-login page, protected shell, overview placeholder,
[design system](DESIGN_SYSTEM.md), [frontend plan](../FRONTEND_UI_PLAN.md), and
[Phase 1 plan](../plans/PHASE_1_IMPLEMENTATION.md) before starting Slice 1.3.

### Findings

- The documentation called for a dark UB-blue NOC interface, but the implemented
  Slice 1.2 UI used a light canvas, pale green gradient, green product mark, soft
  shadow, and oversized rounded card.
- `UBNetDef` was absent from the rendered login and authenticated shell even though
  the accepted brand lockup required organizational attribution.
- The protected placeholder was a generic single-card success message. It did not
  establish the compact, divided information structure needed by later monitoring.
- The implementation therefore resembled a generic SaaS starter more than the
  specified UBNetDef operations product.

### Resolution

- Replaced the light/green treatment with a dark canvas, Harriman Blue branded
  areas, UB Blue actions/selection, Hayes Hall White text, and restrained semantic
  status colors.
- Added a plain typographic `UBNetDef Infrastructure Operations` lockup with a UB
  Blue rule to login and persistent navigation, without reconstructing or misusing
  an official UB mark.
- Removed the gradient, shadow, large radius, and isolated white card. The login now
  uses a bordered operational frame; the overview uses divided factual fields.
- Kept the placeholder honest: it reports authentication/session readiness and
  explicitly says monitoring data begins in the next slice. It contains no fake
  health counts, activity, charts, or integrations.
- Added explicit anti-template rules and a per-slice visual review gate to the design
  system, frontend plan, and Phase 1 delivery plan.

### Gate for subsequent UI work

Before a later slice adds monitoring UI, capture desktop, laptop, and narrow
viewport evidence and review it against the five design-system gates. New views
must extend the same tokens and geometry; they must not introduce their own light
cards, gradients, product colors, or generic dashboard composition.

Official brand colors and mark restrictions remain governed by the linked
University at Buffalo brand sources in the [design system](DESIGN_SYSTEM.md).

### Verification evidence

- Svelte diagnostics: zero errors and zero warnings.
- Frontend unit suite: 15 tests, including official anchor-token assertions, seven
  WCAG AA text-pair checks, UBNetDef attribution in login/shell, and rejection of
  gradient, shadow, and backdrop-filter regressions.
- Production SvelteKit build completed successfully.
- Documentation link validation passed for the updated design and plan indexes.

The monitoring UI is not implemented yet, so data-density and status-state viewport
captures remain a required Slice 1.7 gate rather than fabricated evidence here.

## 2026-07-30 — Navigation and physical-interaction direction

### Updated product direction

- The unauthenticated root becomes a temporary, factual explanation of Espial with
  sign-in at the top right. It is not a public operational status surface and must
  expose no environment data.
- The authenticated target shell uses a darker-than-UB-Blue top bar with Dashboard,
  Alerts, Datacenter, Hypervisor, and Webpages in that order. Dashboard is the
  default post-login destination and Datacenter remains one click away. The current
  permanent side rail remains valid transitional Slice 1.2 UI, but it is not the
  Slice 1.7 navigation target.
- A restrained dark-blue linear gradient is acceptable in structural navigation
  chrome when it improves depth and has a flat fallback. Ambient, multicolor,
  content-card, glow, and glass treatments remain disallowed.
- The physical experience follows one consistent selection path: gray room, rack
  front, server/chassis front, drive bay, and drive details. Hover/focus provides a
  restrained highlight and slight enlargement; click, tap, or keyboard activation
  performs selection.

### Phase boundary

Only the public root and top navigation enter Phase 1. The physical interaction is
documented now so inventory and template contracts support it, but delivery remains
split across Phases 5–7. See the
[general UI guidance](UI_GUIDANCE.md),
[physical drill-down contract](PHYSICAL_DRILLDOWN.md) and updated
[wireframes](WIREFRAMES.md).

### Review gate

When Slice 1.7 replaces the transitional rail, capture public, authenticated,
dropdown-open, and narrow-navigation states. When physical views arrive, capture
every drill-down level with pointer, keyboard focus, reduced motion, and unavailable
or stale data. Decorative motion cannot conceal, delay, or fabricate state.

## 2026-07-30 — Webpage skeleton implementation review

### Scope

Reviewed the public root, local login, authenticated shell, Dashboard, Alerts,
Datacenter, Hypervisor, Webpages, Core-unavailable state, and `/overview` migration
against the [general UI guidance](UI_GUIDANCE.md).

### Resolution

- Replaced the root redirect with a factual public page containing no API-derived
  operational content and keeping `Log in` at the top right.
- Replaced the permanent primary side rail with the required top navigation in the
  exact order Dashboard, Alerts, Datacenter, Hypervisor, and Webpages.
- Made Dashboard the default safe post-login destination and retained `/overview`
  only as a compatibility redirect.
- Added all five canonical route skeletons. Dashboard reports only implemented auth
  and session capabilities; the other domains explicitly report unavailable or
  not-configured state until their authoritative Core data exists.
- Added session-verification loading and Core-unavailable states using the same dark
  shell language.
- Kept the user menu as a real identity/sign-out control with `Escape` behavior. On
  narrow screens, navigation and account access share the labeled menu rather than
  introducing a sidebar or overflowing the viewport.

### Visual review

Rendered and inspected representative public, login, Dashboard, and Datacenter
states at 1440px, 1280px, and a narrow 500px viewport against a temporary read-only
mock session. The first narrow capture exposed account-control overflow; the account
actions were moved into the collapsed navigation and the follow-up capture showed a
two-column header with correctly wrapping content. The reviewed screens use flat
dark-blue/blackish-gray surfaces, thin dividers, restrained UB Blue selection, and
no gradients, glass, glow, shadows, oversized authenticated headings, or fabricated
metrics.

### Verification evidence

- Svelte diagnostics: zero errors and zero warnings.
- Frontend suite: 25 tests, including exact navigation order, public entry, route
  availability honesty, sidebar rejection, contrast tokens, and safe return paths.
- Production SvelteKit build completed successfully.
- Full repository verification passed: formatting, 10 contract fixtures, links in
  38 Markdown files, generated-type diff, Core vet/tests, Svelte diagnostics,
  frontend tests, and the production build.

## 2026-07-31 — Slice 1.7 operational Dashboard review

### Scope

Reviewed the implemented Dashboard, status semantics, resource filters/table,
integration rows, live-connection states, error/permission/empty states, desktop
user menu, and collapsed navigation against the [general UI guidance](UI_GUIDANCE.md)
and [design system](DESIGN_SYSTEM.md).

The review used deterministic test-only Core responses. Those fixtures validate
layout and behavior but are never compiled into or displayed by the production UI.

### Material decisions

- Kept initial session and monitoring reads in SvelteKit loaders so authenticated
  SSR uses the same-origin Core proxy and generated contracts. The browser does not
  contact infrastructure sources or persist operational payloads.
- Used one divided state-count strip, one compact resource table, and divided
  integration rows. This avoids an oversized rounded-card dashboard while keeping
  current health, freshness, coverage, reason, and observation time scannable.
- Every state uses a visible icon and label. Healthy, warning, critical, unknown,
  stale, maintenance, and disabled remain visually and textually distinct.
- Converted the resource table to labeled stacked rows at 500px rather than hiding
  columns or requiring horizontal scrolling. The same resource, state,
  integration, kind, observation time, and reason remain available.
- Kept Alerts and Datacenter honest and unavailable. Slice 1.7 does not pull the
  physical room/rack interaction forward from Phases 5–7.
- Exposed Live, Reconnecting, and Disconnected in shared top chrome. Live events
  only invalidate; successful REST reloads remain the displayed authority.

### Viewport findings

- At 1440px, the full resource table and integration coverage row fit without
  clipped text or ornamental whitespace. The attention state is visible beside the
  Dashboard heading and does not rely on color.
- At 1280px, the same columns and state strip remain readable with the established
  top navigation intact.
- At 500px, the top navigation becomes the required labeled menu, preserves all
  five items in order, exposes Live and sign out, and the monitoring content uses
  full-width divided rows. No primary sidebar is introduced.
- Public and Dashboard axe scans reported no violations on the public page and no
  serious/critical violations on Dashboard. Keyboard Escape returns focus to the
  user-menu trigger; filters and narrow navigation remain keyboard operable.
- Reduced-motion emulation confirms the global motion cap, while SSE reconnect and
  resync remain functional without animation.

### Verification evidence

- Svelte diagnostics: zero errors and zero warnings.
- Unit/contract suite: 36 tests covering API errors, filter normalization, status
  semantics, timestamps, SSE parsing/replay headers, and existing UI contracts.
- Chromium suite: 11 tests covering public disclosure, axe, filters, keyboard
  menus, 401/403/outage behavior, live reconnect, full resync, reduced motion, and
  large/laptop/narrow captures.
- Production SvelteKit SSR build completed successfully.
