# Operational graphics contract

This contract governs every Espial meter, plot, diagram, topology, physical view,
and future chart. It extends the [general UI guidance](UI_GUIDANCE.md) and
[design system](DESIGN_SYSTEM.md); those documents remain authoritative when a
rule overlaps.

## Purpose before rendering

Every graphic specification and review records:

1. the operator question it answers;
2. the authoritative data source and normalized fields it consumes;
3. units, domain, aggregation, and treatment of missing or non-finite values;
4. observation freshness and disconnected, stale, empty, and error behavior;
5. each visual encoding, including how status is conveyed without color alone;
6. pointer, keyboard, touch, and reduced-motion interaction; and
7. an always-available textual equivalent containing the same operational facts.

A graphic is not decorative content. If it cannot answer a named question with
authoritative data, use prose, a definition list, or a table instead.

## Shared composition

Operational graphics use the shared `GraphicFrame` structure:

- visible title followed by a concise description;
- visible freshness and status treatment where applicable;
- bounded plot region;
- optional compact legend only when an encoding needs explanation;
- linked selected-detail region for exploratory graphics; and
- a table or list equivalent that remains available without hover or animation.

Linear meters use `MetricMeter`: a zero-based flat track, flat neutral fill,
printed value and unit, and accessible progress semantics. Compact aggregate
utilization may use `SemicircleGauge` when the larger arc improves at-a-glance
scanning and an explicit value remains centered in the gauge. Both forms expose
accessible progress semantics. Unknown, missing, or non-finite values render as
`Unknown`; they never silently become zero. Values outside a percentage domain are
clamped for rendering while the authoritative formatted value policy is tested.

The shared graphic tokens are `--graphic-grid`, `--graphic-track`,
`--graphic-mark`, `--graphic-selection`, `--graphic-focus`,
`--graphic-label-muted`, and `--graphic-transition-duration`. Cyan communicates
focus and selection only. Green, amber, red, stale orange, and gray retain their
operational meanings. Status always includes visible text and may add an outline,
shape, or pattern; color alone is insufficient.

## Reactivity and motion

Svelte owns component lifecycle and DOM reactivity. Graphic components receive
typed props and do not fetch data. Route loads and the authenticated live-event
invalidation path remain authoritative; a live event prompts a full read rather
than becoming a second data store.

Layouts and ordering are deterministic. Stable IDs preserve selection across data
replacement while the selected object exists. If it disappears, selection clears
and a polite visible message explains why. Initial rendering is static. Subsequent
value and position changes may transition for 160–220 ms. Continuous pulsing,
decorative movement, bounce, gradients, glows, and animation that delays data are
prohibited. Reduced-motion mode applies changes immediately.

Hover and focus may preview a value. Click, tap, `Enter`, and `Space` commit the
same selection; `Escape` clears it. Every hover disclosure has a focus and visible
detail equivalent. Scroll-constrained graphics keep focused objects in view.

## Accessibility and textual equivalence

Visible descriptions and structured HTML carry the meaning; SVG `<title>` alone is
not an adequate tooltip or alternative. SVG and Canvas regions have an accessible
name and a DOM-based text or table equivalent. Tables use captions and scoped
headers. Interactive marks are native controls where practical, expose their
selected state, and retain a visible focus outline.

Review graphics with keyboard, pointer, and touch input; reduced motion; 200%
zoom-equivalent width; dark-theme contrast; long labels; zero, one, many, and
outlier values; and stale, disconnected, empty, and error states. Serious or
critical automated accessibility findings block acceptance.

## Renderer policy

- Native HTML, CSS, and inline SVG are the default for meters, small categorical
  plots, status diagrams, racks, chassis, and other bounded DOM-scale views.
- LayerCake is the preferred future Svelte-native framework when a coordinate-based
  SVG or Canvas chart genuinely needs shared scales, axes, or layered rendering.
- uPlot is reserved for authoritative high-density time series after Espial has a
  documented time-series API, sampling, and retention contract.
- `d3-force` is reserved for data that contains genuine nodes and edges. Non-trivial
  layouts run in a cancellable worker and cache stable positions.
- Cytoscape, XYFlow, ECharts, Pixi, and Three.js require a documented scale,
  topology, editing, or physical-rendering need before adoption.

Do not add a charting dependency for a data shape that semantic HTML, CSS, or a
small inline SVG can represent more clearly.

This policy follows Grafana's separation of normalized
[data frames](https://grafana.com/developers/plugin-tools/key-concepts/data-frames),
its specialized
[Node Graph](https://github.com/grafana/grafana/tree/main/public/app/plugins/panel/nodeGraph)
and
[time-series](https://github.com/grafana/grafana/blob/main/public/app/plugins/panel/timeseries/TimeSeriesPanel.tsx)
renderers, and the bounded layout/caching behavior in its
[node layout implementation](https://github.com/grafana/grafana/blob/main/public/app/plugins/panel/nodeGraph/layout.ts).
Future renderer reviews use the primary
[LayerCake guide](https://layercake.graphics/guide),
[uPlot repository](https://github.com/leeoniya/uPlot), and
[`d3-force` documentation](https://d3js.org/d3-force). Accessibility review uses
the [W3C SVG accessibility note](https://www.w3.org/TR/SVG/access) and
[GOV.UK complex-image guidance](https://design-system.service.gov.uk/styles/images/).

## Hypervisor pilot

The Hypervisor pilot asks: “Which Proxmox host currently carries the most idle VMs,
which has guests with an open Proxmox console, and what is that host's current
compute health?” It uses the existing cluster read model: host metrics, cluster
freshness, running-guest counts, and active-console counts by host.

The summary surface is flat and divided horizontally. On wide screens the compact
host/storage tracks, vertically stacked utilization gauges, and Load distribution
view share one row, with the Load distribution column receiving the most space;
narrower screens reflow them without changing source order. Load distribution omits
the redundant observation timestamp; the current node state is conveyed directly by
each node's semantic ring.
Resource rows use matched-width, zero-based tracks; the state marker follows the
track on the same horizontal axis, exposes swap or IO wait on hover, focus, and
tap, and the exact capacity reading aligns to the track's far edge. Repeated inline
health sentences are omitted because Dashboard and Alerts own that triage. CPU,
RAM, and storage totals use large native-SVG semicircles with explicit values and
no gradient, glow, or ornamental animation.

The host comparison is titled “Load distribution”; it is not topology. It renders
a deterministic compute-node field from stable ID-sorted host/count pairs, with
circle size encoding its idle-VM count: running guests minus guests with an open
Proxmox console proxy. For up to 12 hosts, each node control sits on the matching
radar vertex; the solid idle series is a proportionally filled polygon, while the
dashed console-active series appears only when Proxmox permits the active-task
read. The vertex layout derives from the actual host count, including clusters with
fewer than seven hosts. If the active-task read is unavailable, the graphic says so
rather than presenting zero as a count, while the running-guest inventory continues
to determine idle capacity. The node controls remain the textual equivalent and
source of selection. Each node shows its host name by default and switches to the
idle and console-active VM counts on hover or focus; its tooltip and selected detail
repeat both counts with CPU, RAM, and swap context. The semantic ring conveys the
overall node state without a repeated status word. Selecting a node expands it without introducing a
cyan selection ring; clicking unoccupied field space or pressing `Escape` clears the
selection. The interactive nodes and selected detail are the textual equivalent for
this intentionally compact comparison; the former duplicate host-metrics table is
omitted.

Console-active means that Proxmox reports an open `vncproxy` task for a running
VM/CT. It is not a general in-guest login or CPU-activity claim. The graphic draws
no edges because the source contains no relationship data. Individual VM placement,
migration paths, traffic, pan/zoom, drag, and layout controls remain out of scope
until Core exposes authoritative relationships.
