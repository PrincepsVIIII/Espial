# Design system baseline

Espial uses a dark, dense NOC interface anchored by University at Buffalo blue.
This baseline defines semantics, not a finished component library.

## Brand use

UB Blue is `#005bbb` and Hayes Hall White is `#ffffff`, following the official
[University at Buffalo web color palette](https://www.buffalo.edu/brand/creative/color/color-palette.html).
Espial may use the text lockup `Espial / UBNetDef Infrastructure Operations`.
Official UB marks must use approved assets and must not be reconstructed or combined
into a new logo.

## Token baseline

```css
:root {
  color-scheme: dark;

  --color-brand: #005bbb;
  --color-brand-hover: #2f79d0;
  --color-canvas: #0b1118;
  --color-surface-1: #111a24;
  --color-surface-2: #182330;
  --color-border: #334252;
  --color-text: #f4f7fb;
  --color-text-muted: #a9b7c6;

  --color-healthy: #45b97c;
  --color-warning: #f2b84b;
  --color-critical: #ef6a6a;
  --color-unknown: #a9b7c6;
  --color-stale: #d3945b;
  --color-maintenance: #78a9e6;
  --color-disabled: #718092;

  --space-1: 0.25rem;
  --space-2: 0.5rem;
  --space-3: 0.75rem;
  --space-4: 1rem;
  --space-6: 1.5rem;
  --space-8: 2rem;

  --radius-sm: 0.25rem;
  --radius-md: 0.5rem;
  --font-sans: Inter, ui-sans-serif, system-ui, sans-serif;
  --font-mono: "IBM Plex Mono", ui-monospace, monospace;
}
```

Exact foreground/background pairs must pass automated WCAG contrast checks during
frontend implementation. Status colors are never used as body text until their
contrast is verified in context.

## Status semantics

| State | Icon concept | Required label | Meaning |
|---|---|---|---|
| Healthy | check | Healthy | Fresh observation meets expectations |
| Warning | triangle | Warning | Degraded or approaching a threshold |
| Critical | octagon/exclamation | Critical | Failed or beyond a critical threshold |
| Unknown | question/disconnect | Unknown | No trustworthy current determination |
| Stale | clock | Stale | Last known data exceeded its refresh window |
| Maintenance | wrench | Maintenance | Explicitly suppressed operational period |
| Disabled | pause | Disabled | Collection intentionally disabled |

Every instance includes icon, visible label, and accessible name. `Unknown` and
`stale` must never inherit healthy styling. Timestamps show relative time with the
absolute UTC and local time available on focus/hover.

## Layout and component rules

- 16px minimum body text; dense tables may use 14px with adequate line height.
- Borders and spacing establish hierarchy; avoid decorative glows and gradients.
- Cards are compact summaries, not oversized containers for every section.
- Navigation is persistent on desktop and collapses predictably on smaller screens.
- Keyboard focus is visible on every interactive element.
- Hover-only information has a focus and non-pointer equivalent.
- Physical visualizations always have a table/list representation.
- Motion is brief, respects `prefers-reduced-motion`, and never delays data.

## Phase 1 component set

Implement only what the first vertical slice uses: application shell, navigation,
status badge, integration health row, resource table, timestamp, empty/error/loading
states, local login form, role-aware control wrapper, and live-connection indicator.
Broader component inventory stays in the [frontend plan](../FRONTEND_UI_PLAN.md).
