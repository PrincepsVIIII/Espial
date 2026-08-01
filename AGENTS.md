# Repository instructions

## Phase 2 implementation slices

Before planning, implementing, or reviewing any Phase 2 slice, read
`docs/plans/PHASE_2_IMPLEMENTATION.md` in full and treat it as the execution contract
for scope, slice order, dependencies, invariants, verification, and completion.

Implement slices in the documented order unless a later explicit user instruction
changes the sequence. Do not expose a later-slice route, navigation child,
capability claim, or enabled behavior from preparatory code. When a slice is
accepted, update the plan's implementation-progress table with dated evidence and
update any architecture, API, operations, security, and design records named by
that slice. Code presence without the slice's required tests and evidence is not
completion.

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

Do not infer a user-visible capability from a permission, backend method, schema,
plan, or placeholder. Before UI copy says an operator can perform or verify an
action, trace and test the complete path: discoverable navigation, authoritative
read, authorized mutation where applicable, success/error feedback, and audit
evidence. Label incomplete work as planned or unavailable. Administrative mutations
must expose a correlation receipt that links to the matching audit record.
