# Design system baseline

Espial uses a dark, dense NOC interface anchored by University at Buffalo blue.
This baseline defines semantics, not a finished component library.

The [general UI guidance](UI_GUIDANCE.md) is the authoritative product and
navigation contract. This document supplies its concrete tokens and component rules.

## Brand use

UB Blue is `#005bbb` and Hayes Hall White is `#ffffff`, following the official
[University at Buffalo web color palette](https://www.buffalo.edu/brand/creative/color/color-palette.html).
Espial uses the text lockup `Espial / UBNetDef Infrastructure Operations` or the
compact `Espial / UBNetDef Operations` in persistent navigation. `Espial` is the
product; `UBNetDef` is the operating organization and must remain visible on login,
the authenticated shell, and operator-facing exports.

UB Blue and Hayes Hall White remain the official brand references, but Espial's
application frame is intentionally darker than UB Blue. Harriman Blue (`#002f56`)
and adjacent deep navy tones carry the top navigation and spatial-view chrome; UB
Blue is reserved for selection, focus, and primary action. Secondary colors are
reserved for operational meaning. Espial currently uses a plain typographic product
lockup with a blue rule; it does not invent a university or departmental logo.
Official UB marks must use approved assets and must never be reconstructed, altered,
or combined into a new logo.

## Token baseline

```css
:root {
  color-scheme: dark;

  --ub-blue: #005bbb;
  --ub-blue-bright: #2f79d0;
  --harriman-blue: #002f56;
  --nav-blue-start: #02182b;
  --nav-blue-end: #063860;
  --hayes-white: #ffffff;
  --canvas: #07111d;
  --surface-1: #0c1826;
  --surface-2: #112235;
  --surface-3: #172b40;
  --border: #29445f;
  --border-strong: #3c5d7d;
  --text: #f4f7fb;
  --text-muted: #a9bacb;

  --healthy: #72d6a0;
  --warning: #f2b84b;
  --critical: #ffaaa3;
  --unknown: #a9bacb;
  --stale: #d3945b;
  --maintenance: #78a9e6;
  --disabled: #718092;

  --space-1: 0.25rem;
  --space-2: 0.5rem;
  --space-3: 0.75rem;
  --space-4: 1rem;
  --space-6: 1.5rem;
  --space-8: 2rem;

  --radius-sm: 0.2rem;
  --radius-md: 0.3rem;
  --font-sans: Inter, ui-sans-serif, system-ui, sans-serif;
  --font-mono: "IBM Plex Mono", ui-monospace, monospace;
}
```

Exact foreground/background pairs must pass automated WCAG contrast checks during
frontend implementation. Status colors are never used as body text until their
contrast is verified in context.

Phase 1 is dark-only. A later light theme must be designed as a complete semantic
token set; components must not introduce one-off light surfaces in the meantime.

## Anti-template constraints

Espial must look like a specific UBNetDef operations tool, not a generic SaaS
starter or AI-generated dashboard. Reviews reject:

- pastel, multicolor, radial, or ambient gradients, glass effects, glows, and
  floating blobs. One restrained linear gradient between the two dark navigation
  tokens is allowed only on structural chrome, with a flat fallback;
- a grid of oversized rounded cards where a table, definition list, or bordered
  operational section communicates the hierarchy better;
- excessive pills, uniformly rounded containers, decorative shadows, or icons that
  do not encode an action or state;
- oversized marketing headlines, vague aspirational copy, fake metrics, invented
  activity, or placeholder charts presented as real monitoring;
- chatbot patterns, sparkle motifs, generated illustrations, fake terminals, and
  controls included only to make a screen look populated; and
- green as a brand color. Green is reserved for explicit healthy/success semantics.

Prefer square or lightly radiused geometry, a persistent top bar, contextual rails
only where a task needs them, thin dividers, compact labels, monospaced values where
useful, and data-dense tables. Empty space should clarify hierarchy without turning
authenticated pages into marketing layouts.

## Visual review gate

Every frontend slice must be reviewed at 1440px, 1280px, and a narrow viewport
before it is complete. The review confirms:

1. UBNetDef is visibly attributed and UB Blue remains the dominant accent.
2. The screen answers a named operator question with implemented data only.
3. Dark surfaces, focus states, and semantic status pairs pass contrast checks.
4. Status never relies on color alone and keyboard landmarks remain usable.
5. No anti-template pattern above has entered through a new component.

Record material reviews in [UI review notes](UI_REVIEW.md); do not defer visual
coherence until the end of Phase 1.

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
- Borders and spacing establish hierarchy. Only the documented dark-blue structural
  navigation gradient is permitted; content backgrounds remain flat.
- Cards are compact summaries, not oversized containers for every section.
- Primary navigation is a persistent top bar with accessible click/keyboard
  dropdowns on desktop and one collapsed menu on smaller screens.
- Keyboard focus is visible on every interactive element.
- Hover-only information has a focus and non-pointer equivalent.
- Physical visualizations always have a table/list representation.
- Motion is brief, respects `prefers-reduced-motion`, and never delays data.

## Phase 1 component set

Implement only what the first vertical slice uses: application shell, navigation,
status badge, integration health row, resource table, timestamp, empty/error/loading
states, local login form, role-aware control wrapper, and live-connection indicator.
Broader component inventory stays in the [frontend plan](../FRONTEND_UI_PLAN.md).
