# Espial Frontend and UI Plan

All UI work must first follow the authoritative
[general UI guidance](design/UI_GUIDANCE.md). This plan expands that contract for
individual product areas.

## 1. Purpose

The frontend will be a standalone SvelteKit application designed for a technical UBNetDef audience. It should help infrastructure administrators, instructors, and TAs quickly identify failures, understand affected systems, and reach useful diagnostic details.

The interface should feel like a modern network operations center rather than a generic SaaS or AI-generated dashboard.

## 2. Frontend goals

- Run independently from Espial Core, the Go backend service.
- Consume a versioned REST API.
- Receive live status and incident changes through server-sent events or WebSockets.
- Present dense technical information without becoming visually confusing.
- Make warning, critical, unknown, and stale states immediately distinguishable.
- Reuse a stable component system across integrations.
- Allow custom Svelte pages for advanced integrations without changing central frontend logic.
- Use a deliberately designed dark theme throughout Phase 1; do not mix in isolated
  light cards or defer the theme until polish.
- Establish a reusable UBNetDef visual language for future internal tools.

## 2.1 Frontend/backend boundary

SvelteKit is a presentation and browser-session layer; it must not duplicate Espial Core domain rules. Health evaluation, incident state transitions, authorization, auditing, dependency resolution, and adapter management belong in Go. The frontend should consume versioned API models and treat live events as prompts to refresh authoritative state rather than as the only stored truth.

Use generated or shared TypeScript types from the API schema where practical. Role-aware controls improve usability, but every permission must still be enforced by Espial Core.

## 3. Visual direction

### 3.1 Brand character

The product name is **Espial**. UBNetDef should remain clearly visible as the organization and visual identity behind the product. Authenticated views may use a compact lockup such as `Espial / UBNetDef Infrastructure Operations` rather than repeatedly describing the application as a generic dashboard.

The visual identity should combine:

- the supplied UBNetDef logo, used unmodified, with its cyan as the primary accent
- a near-black canvas that visually recedes behind deep navy application chrome
- dark neutral surfaces
- clear operational status colors
- compact information density
- strong typography
- restrained static chrome and high-impact purposeful spatial motion
- clear diagrams and physical layouts

UBNetDef attribution is part of the product chrome, not footer-only decoration.
Login and persistent navigation use the supplied UBNetDef mark with `Espial`
identified alongside it as the product. The top navigation is a padded-down,
floating dark rectangle with restrained rounded corners. Cyan is the restrained
selection/action accent, white is the hover/current underline, and green is
operational status only.

### 3.2 Avoid

- gradients, ambient color fields, or textured backgrounds
- excessive glow or neon effects
- oversized rounded cards
- fake terminal decorations
- chatbot-style elements
- generated illustrations
- large empty marketing layouts in authenticated views
- red and green as the only status distinction
- vague summaries that hide source data
- glassmorphism, pastel ambient backgrounds, floating islands, and soft shadows
- default SaaS card grids, excessive pills, and large corner radii on every object
- generic product copy, fake metrics, fake activity feeds, and placeholder charts
- sparkle motifs, chatbot composition, or decorative icons without state/action meaning

These are acceptance constraints, not taste suggestions. A screen that falls into
these patterns is incomplete even when its routes and data loading work.

### 3.3 Status system

Each status should use more than color alone.

| State       | Visual treatment                                     |
| ----------- | ---------------------------------------------------- |
| Healthy     | visible label and calm success color                 |
| Warning     | visible label and amber treatment                    |
| Critical    | visible label and strong high-contrast treatment     |
| Unknown     | visible label with neutral treatment                 |
| Stale       | visible label, timestamp emphasis, and muted warning |
| Maintenance | visible label and explicit maintenance treatment     |

Icons and shapes are optional cues for dense views, not mandatory decoration.

## 4. Information architecture

Desktop/laptop navigation uses a top bar rather than a permanent primary sidebar.
The bar keeps the Espial/UBNetDef lockup on the left and session state, user menu,
and sign-out access on the right. The primary items and order are fixed:

```text
Espial / UBNetDef  |  Dashboard  Alerts  Datacenter  Hypervisor  Webpages  |  Live  User ▾
```

These target `/dashboard`, `/alerts`, `/datacenter`, `/hypervisor`, and `/webpages`.
Dashboard is the default destination after login; Datacenter remains one primary
navigation action away. Dropdowns contain real destinations or actions. On desktop
they reveal on pointer hover and keyboard focus, also support click/touch activation,
arrow-key movement, `Escape` to close, visible focus, and route-aware selected state.
Hovered, focused, and current labels receive a short white underline instead of a
filled highlight. Unimplemented areas show an honest unavailable or not-configured
state rather than fake content. On narrow screens the same items collapse behind
one menu button without changing their order.

### 4.1 Dashboard

The dashboard should answer:

- What is wrong now?
- What changed recently?
- Which services are affected?
- Which alerts need acknowledgement?
- Is any monitoring data stale?

Suggested content:

- current critical and warning count
- active incidents
- recently recovered incidents
- affected services
- certificate warnings
- integration health
- stale-data warnings
- optional quick links to Grafana, Proxmox, iDRAC, TrueNAS, and other source systems

### 4.2 Incidents

Features:

- active and historical incident views
- severity, status, source, service, and owner filters
- acknowledgement and assignment
- operator notes
- timeline of observations and notifications
- recovery and resolution state
- links to affected devices, services, and source tools

### 4.3 Services

The service view should show logical services such as SSO and the infrastructure they depend upon.

Features:

- service status summary
- dependency list or graph
- active incidents
- supporting VMs, hosts, storage, network, and certificates
- historical availability
- “show affected services” behavior from infrastructure views

### 4.4 Infrastructure

Infrastructure should support multiple perspectives:

- list view
- resource-type view
- room layout
- rack grid
- device detail
- chassis and drive view

### 4.5 Certificates

Display:

- endpoint
- current status
- expiration date
- days remaining
- issuer
- hostname match
- chain status
- last check
- linked service
- active incident

### 4.6 Integrations and administration

Administrators should be able to:

- install or register adapters
- review adapter manifests
- configure integrations
- test configuration
- enable or disable integrations
- configure thresholds and schedules
- approve plugins
- manage notification destinations
- manage users and roles
- manage room, rack, device, and chassis metadata
- view audit events

## 5. Core component library

The frontend should build a small, reusable component system.

### Operational components

- status badge
- severity indicator
- stale-data indicator
- metric card
- status summary card
- resource table
- incident row
- incident timeline
- notification delivery status
- dependency list
- dependency graph
- time-series chart wrapper
- source-system deep link

### Interaction components

- device detail drawer
- filter chips
- hover tooltips
- searchable command or navigation palette
- confirmation dialog
- maintenance or silence control
- live-update indicator
- timestamp with relative and absolute formats

### Physical visualization components

- room layout component
- rack grid component
- rack unit scale
- device block
- chassis component
- drive bay component
- front/rear view toggle
- incident highlight overlay
- affected-services toggle

## 6. Rack grid component

The rack grid is the first practical physical visualization.

Requirements:

- render racks vertically by rack unit
- size devices according to height in U
- support front and rear orientation
- show empty and occupied positions
- color and label devices by health state
- highlight devices with active incidents
- show summary tooltips on hover or focus
- open the device detail drawer on selection
- filter by device type, status, service, room, or integration
- remain readable on large displays and laptops

The component should be data-driven. Rack placement must come from backend inventory, not hardcoded coordinates.

The rack is normally reached by selecting it in the room overview. Selection uses
the shared spatial transition and route model in the
[physical drill-down specification](design/PHYSICAL_DRILLDOWN.md).

## 7. Room layout component

The room layout is a later visual layer above rack views.

Requirements:

- position racks within a room
- show rack-level health summaries
- permit zooming or focused selection
- highlight groups of affected racks
- support network, power, cooling, or service overlays later
- link directly into the selected rack

The first version can use a simple grid-based room editor instead of a full freeform drawing tool.

The operator-facing room view precedes the editor. It renders the room and
unselected equipment primarily in grays, then uses outlines, labels, and restrained
status color to highlight affected or focused racks. Hover/focus slightly enlarges
the target without changing layout; selection moves into a straight-on rack view.

## 8. Device detail drawer

Selecting a device should open a side drawer rather than always navigating away.

Suggested sections:

- display name and hostname
- device role and type
- health status
- room, rack, and rack-unit location
- model, serial number, and asset tag
- linked incidents
- recent checks and metrics
- linked services
- VMs, datasets, pools, or other contained resources
- source-system links
- chassis and drive view
- last successful update and stale state

The drawer may link to a full device page for deeper history.

## 9. Chassis and drive visualization

The UI should support reusable templates for the main UBNetDef server types.

### 9.1 Chassis templates

Templates define:

- server dimensions
- front and rear faces
- drive-bay positions
- bay labels
- optional power supply, NIC, or controller locations
- supported animations

Devices reference a template ID and fill it with inventory data.

### 9.2 Drive bays

Each bay should display or expose:

- slot number
- occupancy
- drive serial number
- model and capacity
- health state
- pool or array association
- rebuild or replacement status
- last observed time

A failed or degraded drive should be visually obvious without relying only on color.

### 9.3 Motion and opening animations

The spatial sequence is a product requirement, delivered progressively after the
non-animated inspection routes work:

- room selection zooms toward a rack and gently corrects its angle to a front view
- server selection retains rack context, then moves to a model-aware chassis front
- drive selection runs the bay motion defined by that server's chassis template
- the drive details panel becomes available immediately and does not wait on motion

Motion remains brief, functional, interruptible, and reduced-motion safe. Exact
routes, indicator separation, drive fields, animation bounds, and fallbacks are in
the [physical drill-down specification](design/PHYSICAL_DRILLDOWN.md).

## 10. Incident highlighting

Incident highlighting should work consistently across views.

Examples:

- an active incident adds an indicator to a device block
- selecting an incident highlights its affected resources
- room view highlights affected racks
- rack view highlights affected devices
- chassis view highlights affected bays or components
- service view highlights degraded dependencies

The same incident identifier should connect all views.

## 11. “Show affected services” toggle

This toggle should be available in infrastructure and incident views.

When enabled, it should:

- list services directly dependent on a selected resource
- show dependency paths where available
- highlight affected service cards
- distinguish direct dependencies from indirect dependencies
- avoid implying certainty when a relationship is incomplete

No numerical risk score is required. The UI should show transparent relationships and current health state.

## 12. Filters and tooltips

### 12.1 Filter chips

Useful filters include:

- status
- severity
- site
- room
- rack
- resource type
- integration
- affected service
- incident state
- owner
- stale data

Active filters should be visible, removable, and reflected in the URL where practical.

### 12.2 Tooltips

Tooltips should expose quick information but never hide critical content exclusively.

Useful tooltip fields:

- hostname
- model
- role
- health state
- active incident count
- last updated time
- rack location
- selected drive serial and slot

Keyboard focus must reveal the same information as mouse hover.

## 13. Frontend extension model

Most integrations should use standard normalized widgets and require no custom frontend code.

Standard presentation options:

- status cards
- metric cards
- time-series charts
- tables
- certificate lists
- incident timelines
- dependency relationships

Advanced integrations may add a custom Svelte page or component. Adding one may require rebuilding the frontend, but it should not require changes to:

- authentication
- incident logic
- notification logic
- shared status rules
- core navigation patterns
- backend normalization

Custom pages should use the shared component library and design tokens.

## 14. Data fetching and state

Recommended approach:

- server-render or load initial page data through SvelteKit
- use a typed API client generated or maintained from backend schemas
- subscribe to live event updates after page load
- update local views based on event identifiers
- display connection state and last successful refresh
- cache only non-sensitive UI state in the browser
- avoid making the browser contact infrastructure systems directly

The backend remains the single authority for authorization and normalized state.

## 15. Accessibility and usability

The dashboard should support:

- keyboard navigation
- visible focus states
- sufficient contrast
- status labels in addition to color
- reduced-motion preferences
- readable timestamps
- responsive layouts
- large-display NOC use
- laptop administration use
- screen-reader labels for diagrams and controls

Physical diagrams should have an accessible list or table equivalent.

## 16. Responsive strategy

### Large NOC display

- summary-first layout
- active incidents prominent
- dense service and infrastructure health
- minimal interaction required

### Desktop/laptop

- full navigation
- drawers and multi-column layouts
- filters and detailed inspection

### Mobile

Mobile support can be secondary, but incident acknowledgement and basic inspection should remain usable.

## 17. Public entry page

The root route is a public informational page before login. Its first version is an
explicitly temporary explanation of what Espial does and will be replaced by a more
complete public presentation later.

Required first-version structure:

- the same dark top bar and Espial/UBNetDef attribution as the authenticated app;
- a visible `Sign in` action at the top right, linking to `/login` with a safe local
  return target when applicable;
- concise factual sections explaining infrastructure monitoring, freshness/stale
  state, incident direction, physical drill-down direction, and audited access;
- no authenticated session requirement and no automatic redirect to login; and
- an honest temporary-page label so it is not mistaken for final marketing copy.

The page is informational, not a public status page. It exposes no live health,
internal hostnames, addresses, versions, serials, incidents, topology, user data, or
API-derived operational payload. The later replacement may add approved copy and
media, but operational data remains in the authenticated application unless a
separate public-status product is explicitly approved.

## 18. Frontend implementation order

1. Establish design tokens, status semantics, UBNetDef attribution, and automated
   contrast checks.
2. Build the public informational entry, exact five-section top navigation, and
   authenticated shell, then complete the visual review gate before feature pages.
3. Implement authentication states and role-aware controls without introducing a
   separate generic login-page aesthetic.
4. Build Dashboard as the post-login summary. Establish Alerts and Datacenter as
   adjacent primary routes with honest unavailable/not-configured states until their
   domain data is available.
5. Build incident list, detail, acknowledgement, and timeline.
6. Build service and dependency views.
7. Build inventory lists and device detail drawer.
8. Build certificate monitoring views.
9. Build integration administration screens.
10. Build the neutral-gray room view, rack grid, and room-to-rack selection.
11. Add incident highlighting and affected-services context in physical views.
12. Build chassis templates, device indicators, drive bays, and drive details.
13. Add the controlled rack/server/bay transitions defined by the physical drill-down.
14. Add custom integration pages as real needs appear.

## 19. Frontend success criteria

The frontend is successful when:

- an operator can identify current critical issues within seconds
- stale and unknown data cannot be mistaken for healthy data
- incidents connect clearly to affected resources and services
- common integrations render without custom pages
- rack and device views are generated from inventory data
- a failed drive can be located by server, chassis, slot, and serial number
- the design remains consistent across student-built extensions
- the interface looks intentionally UBNetDef, technical, and operational
- dark-blue branding remains coherent across login, shell, loading, empty, error,
  and feature states rather than only on the happy-path overview
- visual review finds no generic AI-dashboard patterns listed in Section 3.2
- the public root explains Espial without disclosing operations and keeps sign-in
  available in the top-right navigation
- every authenticated page uses Dashboard, Alerts, Datacenter, Hypervisor, and
  Webpages in that order, with Dashboard as the post-login destination
- room, rack, server, and drive selection form one deep-linkable and accessible
  drill-down with motion-independent fallbacks
