# Espial mockup UI review

Date: 2026-09-10

Scope: a standalone, explicitly fictional front-end concept requested by the user.
This does not implement or accept a Phase 2 slice. The production application and
pre-existing uncommitted work are preserved.

## Design decisions

The [UI guidance](../docs/design/UI_GUIDANCE.md),
[design system](../docs/design/DESIGN_SYSTEM.md),
[graphics contract](../docs/design/GRAPHICS.md), and
[physical drill-down](../docs/design/PHYSICAL_DRILLDOWN.md) were read in full.

The first screen answers “Where is capacity tight, and what needs attention?”
Four resource summaries share one divided surface. A host comparison and a short
attention list carry the next level of information. Historical charts, VM details,
connection timings, and investigation actions open on demand.

The unchanged supplied logo anchors a detached near-black canvas and a floating
nav bar. White navigation underlines, flat surfaces, fine dividers, and aligned
technical values provide structure. Cyan marks actual interaction and selection;
amber and green communicate labeled operational conditions. There are no gradients,
glows, ornamental icons, permanent sidebar, remote fonts, or generated artwork.

Native DOM and SVG are sufficient for this bounded dataset. All plots expose
printed values and a textual equivalent. The physical view alone uses small spatial
transitions; reduced-motion mode removes them.

## Explicit prototype exceptions

- The user's request authorizes hardcoded operational fixtures in this separate
  mockup. Persistent sample labels distinguish this from authenticated real data.
- Room, rack, chassis, and drive inspection preview future interaction without
  adding production routes, permissions, APIs, or capability claims.
- Alert response actions are session-only demonstrations with matching simulated
  audit records. Administrative writes are not offered in the prototype.
- This isolated review record belongs to the prototype so the existing design and
  implementation records are not rewritten as an acceptance of production work.

## Review coverage

Desktop and narrow-screen review covers the overview, drill-down panels, original
logo, typography, meaningful data graphics, and responsive navigation. Browser
verification covers route navigation, resource aggregation and scope, chart period
and keyboard inspection, alert-to-audit receipts, datacenter selection and return,
stale/error/empty states, local-only requests, reduced motion, and axe accessibility.

Review findings corrected include lost keyboard focus during physical inspection,
detail links dropping child-page context, retained audit search hiding a receipt,
history ignoring cluster scope, missing stale disclosure in drawers, and overly
small essential capacity labels on mobile.

The reproducible verification command and generated evidence are documented in the
[README](README.md). Test results reflect this prototype only; no production test
or backend verification is claimed.

## Verified results

- `npm --prefix mockup test`: **20/20 browser checks passed** in Chromium.
- No axe violations in the six main routes, tested detail dialogs, and sampled
  error/empty/narrow-screen states; no browser errors or external/API requests.
- Overview screenshots reviewed at 1440×900, 1280×800, and 390×844.
- Datacenter room, rack, chassis, and drive inspected at 1440, 1280, and 390px:
  all 12 combinations have zero axe violations and no document overflow.
- Datacenter keyboard, touch, Escape, browser Back, direct-link reload, and
  reduced-motion checks passed.
- Original logo byte equality, JavaScript syntax, formatting, Git whitespace, and
  repository Markdown link checks passed.

Generated evidence: `test-results/report.json`, individual `axe-*.json` reports,
`dc-review.json`, `dc-interactions.json`, and desktop/mobile PNG captures.
