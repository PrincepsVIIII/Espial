# General UI guidance

This is Espial's authoritative UI acceptance contract. Read it before planning,
implementing, or reviewing any UI change. More detailed specifications may extend
it, but must not contradict it. A later explicit product decision should update
this file and the affected detailed documents together.

## Product structure

Espial has one public experience and one authenticated application.

### Public page

The unauthenticated root (`/`) is a concise explanation of what Espial does. It is
a temporary public presentation, not an operational status page.

It must:

- identify Espial and UBNetDef;
- explain infrastructure monitoring, operational awareness, and physical hardware
  inspection in plain language;
- keep `Log in` visible at the top right;
- contain no live monitoring data, internal names, topology, incidents, counts,
  addresses, integration details, or other environment metadata; and
- use the same color, typography, and top-bar language as the application without
  pretending to be an authenticated operator screen.

### Authenticated application

After login, the default destination is the **Dashboard**. The top navigation
is present across every authenticated page in this order:

```text
Espial / UBNetDef  |  Dashboard  Alerts  Datacenter  Hypervisor  Webpages  |  Live  User ▾
```

- **Dashboard** summarizes health, incidents, freshness, and monitoring coverage.
- **Alerts** owns active and historical alerts, acknowledgement, assignment, and
  the incident context that explains what requires attention.
- **Datacenter** owns the room → rack → server → drive inspection flow and remains
  one primary-navigation action away from Dashboard.
- **Hypervisor** owns virtual-machine, host, cluster, and related virtualization
  views.
- **Webpages** owns monitored-site availability, response, and certificate views.
- **User** contains identity, role, settings made available to that role, and sign
  out. Administrative functions may live here or in a permission-gated secondary
  menu rather than becoming another permanent primary item.

The canonical route targets are `/dashboard`, `/alerts`, `/datacenter`,
`/hypervisor`, and `/webpages`. Until a section's backend phase is implemented, its
route must show an honest unavailable, empty, or not-configured state—never
fabricated operational content. The existing `/overview` route may redirect to
`/dashboard` during the navigation migration.

## Navigation

Use one consistent top navigation bar. Do not use a permanent primary sidebar.
Contextual secondary controls may appear as tabs, breadcrumbs, a details drawer, or
a temporary task-specific rail when the content genuinely needs them, but they must
not duplicate or replace the primary top navigation.

The top bar must:

- remain visually and structurally consistent across authenticated routes;
- visibly identify the current section;
- support pointer, keyboard, and touch input with visible focus;
- use dropdowns only for real child routes or user actions, never as decoration;
- open dropdowns on activation rather than hover alone and support `Escape`; and
- collapse into one clearly labeled menu on narrow screens while preserving the
  same item order and access to sign out.

## Color and surfaces

Use a consistent dark-blue and blackish-gray scheme:

- near-black blue/gray for the page canvas;
- deep navy or Harriman Blue for the top bar and structural chrome;
- flat charcoal-blue surfaces for content regions;
- restrained UB Blue for primary actions, focus, links, and selected state; and
- semantic green, yellow/amber, red, gray, and stale-orange only for operational
  status.

Content surfaces should normally be flat. One restrained linear gradient between
adjacent dark navy values is allowed in the top navigation if it improves depth and
has a flat-color fallback. Do not use bright, pastel, multicolor, radial, ambient,
or content-card gradients. Do not use green as a product-brand color.

Every text/background and focus-state pair must meet the contrast requirements in
the [design system](DESIGN_SYSTEM.md). Status must always include text and/or an
icon or shape; color alone is insufficient.

## Composition and visual character

Espial should look like a specific, serious infrastructure operations tool. Avoid
UI patterns commonly associated with generic AI-generated products:

- glassmorphism, blurred panels, glowing borders, floating color blobs, and soft
  ambient backgrounds;
- grids of oversized rounded cards when a table, split view, definition list, or
  bordered operational section communicates the information better;
- excessive pills, large corner radii, decorative shadows, and arbitrary whitespace;
- enormous marketing headings inside the authenticated application;
- sparkle motifs, chatbot composition, fake terminals, decorative activity feeds,
  meaningless charts, and icons with no state or action meaning;
- vague copy, invented metrics, fabricated events, and placeholder data presented
  as real; and
- one-off page aesthetics that break the shared top bar, palette, geometry, or
  typography.

Prefer compact information hierarchy, thin dividers, lightly radiused or square
geometry, factual labels, useful tables, direct status language, and monospaced
technical values where appropriate. Public-page copy may be more explanatory, but
it must remain factual and restrained.

## Datacenter interaction

The Datacenter view follows the
[physical infrastructure drill-down](PHYSICAL_DRILLDOWN.md):

```text
Room overview → Rack front → Server/chassis front → Drive bay → Drive details
```

The room and hardware are primarily gray so selection and operational status remain
prominent. Hover and keyboard focus may slightly enlarge and outline selectable
equipment. Click, tap, Enter, and Space must perform equivalent selection. Spatial
animations are brief, interruptible, reduced-motion safe, and never delay details.
Every physical view has a direct URL and a non-spatial list/table equivalent.

## Required states and review

Every page must deliberately handle loading, empty/not-configured, permission
denied, stale data, disconnected live updates, Core unavailable, and unexpected
error states where applicable. These states use the same shell and visual language
as successful content.

Before a UI change is complete, verify:

1. The page follows the public/authenticated structure and exact primary navigation.
2. The dark-blue/blackish-gray palette remains consistent and accessible.
3. No permanent primary sidebar or generic AI-looking pattern was introduced.
4. Content is authoritative or explicitly labeled unavailable—not fabricated.
5. Keyboard, touch, narrow-screen, and reduced-motion behavior remain usable.
6. Relevant detailed contracts, wireframes, and visual tests were updated.

Record material changes or exceptions in [UI review notes](UI_REVIEW.md).
