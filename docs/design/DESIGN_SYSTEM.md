# Design system baseline

Espial uses a dark, dense NOC interface anchored by the supplied UBNetDef logo's
cyan and white.
This baseline defines semantics, not a finished component library.

The [general UI guidance](UI_GUIDANCE.md) is the authoritative product and
navigation contract. This document supplies its concrete tokens and component rules.

## Brand use

The supplied UBNetDef logo is the primary visual mark and must be used unmodified.
Its cyan (`#55d6e2`) and Hayes Hall White (`#ffffff`) establish the product accent
and navigation rule. `Espial` is the product and appears as adjacent text where
context is needed; `UBNetDef` is the operating organization and must remain visible
on login, the authenticated shell, and operator-facing exports.

Harriman Blue (`#002f56`) remains a supporting institutional reference, while
near-black navy tones carry the top navigation and spatial-view chrome. UBNetDef
cyan is reserved for selection, focus, links, and primary action. Secondary colors
are reserved for operational meaning. Approved marks must never be reconstructed,
altered, recolored, or combined into a new logo.

## Token baseline

```css
:root {
  color-scheme: dark;

  --ub-blue: #005bbb;
  --ub-blue-bright: #2f79d0;
  --netdef-cyan: #55d6e2;
  --harriman-blue: #002f56;
  --nav-blue: #09131c;
  --hayes-white: #ffffff;
  --canvas: #05080b;
  --surface-1: #0b1218;
  --surface-2: #101a22;
  --surface-3: #16232d;
  --border: #253540;
  --border-strong: #3b5361;
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

  --radius-sm: 0.25rem;
  --radius-md: 0.7rem;
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

- gradients, glass effects, glows, textures, and floating color fields;
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

1. UBNetDef is visibly attributed and its cyan remains the dominant accent.
2. The supplied UBNetDef logo remains intact and the screen answers a named
   operator question with implemented data only.
3. Dark surfaces, focus states, and semantic status pairs pass contrast checks.
4. Status never relies on color alone and keyboard landmarks remain usable.
5. No anti-template pattern above has entered through a new component.

Record material reviews in [UI review notes](UI_REVIEW.md); do not defer visual
coherence until the end of Phase 1.

## Status semantics

| State       | Optional cue        | Required label | Meaning                                     |
| ----------- | ------------------- | -------------- | ------------------------------------------- |
| Healthy     | check               | Healthy        | Fresh observation meets expectations        |
| Warning     | triangle            | Warning        | Degraded or approaching a threshold         |
| Critical    | octagon/exclamation | Critical       | Failed or beyond a critical threshold       |
| Unknown     | question/disconnect | Unknown        | No trustworthy current determination        |
| Stale       | clock               | Stale          | Last known data exceeded its refresh window |
| Maintenance | wrench              | Maintenance    | Explicitly suppressed operational period    |
| Disabled    | pause               | Disabled       | Collection intentionally disabled           |

Every instance includes a visible label and accessible name. Add the optional cue
only where it makes dense operational data easier to scan. `Unknown` and `stale`
must never inherit healthy styling. Timestamps show relative time with the
absolute UTC and local time available on focus/hover.

## Layout and component rules

- 16px minimum body text; dense tables may use 14px with adequate line height.
- Borders, spacing, and flat tonal contrast establish hierarchy; backgrounds remain
  flat.
- Cards are compact summaries, not oversized containers for every section.
- Primary navigation is a floating top bar with restrained rounded corners. Desktop
  dropdowns open on hover and focus, support click/touch, and close with `Escape`;
  smaller screens use one collapsed menu.
- Every page, section, card, form, and state heading starts with the heading itself;
  eyebrow, kicker, overline, and decorative category copy are not used in any view.
  Useful explanation belongs below the heading as body copy, while meaningful state
  belongs in the heading or an established status component.
- Preserve restrained cyan rhythm with a short edge on standalone headings or a
  short segment attached to a real divider. Do not use cyan decorative text as a
  substitute.
- Primary labels are links and activate their own route. A separate, accessible
  child-menu trigger is used when touch input needs to open a real submenu.
- Hovered, focused, and current navigation labels use a short white underline, not
  a filled tile or decorative glow.
- Keyboard focus is visible on every interactive element.
- Hover-only information has a focus and non-pointer equivalent.
- Physical visualizations always have a table/list representation.
- Motion is brief, respects `prefers-reduced-motion`, and never delays data. Richer
  motion is reserved for spatial inspection and future integration visualizations.

## Phase 1 component set

The established set includes application shell, navigation, status text,
integration health row, resource table, timestamp, empty/error/loading states,
local login, role-aware controls, audit table and filters, administrative user table,
mutation receipt, and disconnected/reconnecting notice. Broader component inventory
stays in the [frontend plan](../FRONTEND_UI_PLAN.md).
