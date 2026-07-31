# Repository instructions

## UI changes

Before planning, implementing, or reviewing any user-interface change, read
`docs/design/UI_GUIDANCE.md` in full and treat it as the repository's UI acceptance
contract. This includes changes to routes, navigation, layouts, components, copy,
CSS, visual assets, loading/empty/error states, and responsive behavior.

Also follow any more specific design document linked from that guidance. If an
older planning document conflicts with `UI_GUIDANCE.md`, the guidance takes
precedence unless a later explicit user instruction supersedes it. Re-check the
guidance before considering UI work complete.

Preserve the current UBNetDef direction: use the supplied logo unmodified; keep a
near-black detached canvas and floating rounded-rectangle top navigation; use cyan
sparingly and a short white navigation underline; avoid decorative icons, badges,
glows, gradients, and filled hover tiles; and reserve ambitious animation for
purposeful spatial or integration views. Desktop dropdowns must work on hover,
focus, click/touch, and `Escape`, with an equivalent collapsed narrow-screen menu.
