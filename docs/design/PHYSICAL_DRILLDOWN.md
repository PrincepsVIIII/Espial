# Physical infrastructure drill-down

This specification defines the intended authenticated path from a data-center room
to an individual drive. Delivery remains staged across Roadmap Phases 5–7, but the
interaction model is fixed early so the inventory schema, routes, chassis templates,
and animations converge on one experience.

## Operator path

```text
Room overview → Rack front → Server/chassis front → Drive bay → Drive details
```

Each selection updates the URL and browser history. Back, breadcrumbs, and `Escape`
return one level without discarding room/rack context. Direct links must open the
same focused state without replaying every animation:

```text
/datacenter/rooms/{room_id}
/datacenter/racks/{rack_id}
/datacenter/devices/{device_id}?face=front
/datacenter/drives/{drive_id}
```

Identifiers are opaque Core IDs. The UI never hardcodes hostnames, rack positions,
server geometry, or drive locations.

## Room overview

The authenticated Datacenter entry view presents the selected data-center room
as a subdued spatial scene:

- floor, walls, aisles, and unselected racks use neutral grays with dark-blue chrome;
- operational status colors are reserved for rack outlines, lights, and explicit
  labels rather than filling the entire room;
- rack placement, orientation, row, and dimensions come from inventory data;
- hover or keyboard focus adds a high-contrast outline and a slight visual scale
  increase without moving neighboring elements;
- the focus treatment exposes rack name, row, status, affected-resource count, and
  last trustworthy observation; and
- clicking, pressing `Enter`, or tapping a rack selects the identical action.

The grayscale scene keeps status highlights legible and prevents the room itself
from competing with the equipment needing attention. A table/list alternative
provides the same racks, states, locations, and navigation targets.

## Rack focus

Selecting a rack performs a short camera transition that moves toward the rack and
settles on a straight-on front view. A small rotation may correct the room-view
angle, but the result must be an orthographic or low-perspective inspection view,
not a cinematic orbit.

The rack view shows:

- rack name, room/row, orientation, total rack units, and current status;
- a visible U scale and data-driven occupied/empty positions;
- server/device labels, height in U, and front/rear availability;
- affected-service and incident indicators when that data exists; and
- a persistent breadcrumb plus a control to return to the room.

Hover/focus slightly enlarges a server within an overlay layer, adds an outline, and
raises its label. It must not reflow rack units or obscure a neighboring critical
indicator. Selection moves closer to the server while retaining enough rack context
to preserve physical location.

## Server and chassis focus

Selecting a server transitions to its chassis front. The view is generated from a
versioned chassis template selected by device model; a generic rectangular template
is used when no approved model-specific template exists.

Templates define:

- front and rear dimensions and aspect ratio;
- rack height, bezel, controls, power/health indicator positions, and labels;
- drive-bay count, geometry, numbering, orientation, and occupancy;
- selectable front/rear faces;
- supported bay-inspection motion (`slide`, `hinge`, `carrier`, or `none`); and
- safe animation distance/axis metadata that describes the visual model only.

This is an inspection interface, not a hardware-control surface. An animated open
bay does not imply or issue a physical eject/unlock action.

## Indicator semantics

Power, overall health, and drive state are separate signals; one light must not
conflate them.

| Indicator | States | Treatment |
|---|---|---|
| Power | On, Standby, Off, Unknown | Blue/white/gray light plus visible label and power icon |
| Device health | Healthy, Warning, Critical, Unknown, Stale | Semantic color, distinct icon/shape, visible label |
| Drive state | Online, Warning, Failed, Rebuilding, Missing, Unknown | Per-bay light, icon/pattern, visible state in focus/detail |
| Drive activity | Active, Idle, Unknown | Optional secondary light; motion disabled under reduced-motion |

An unlit indicator is never assumed to mean healthy. Unknown and stale remain
explicit. Color-blind and screen-reader users receive the same state through shape,
text, and accessible names.

## Drive-bay selection and details

Hover/focus on a bay adds a small scale/outline treatment and reveals slot plus
state. Selecting an occupied bay runs its chassis-template inspection motion, then
opens a stable details panel. Selecting an empty/missing bay shows its slot and
state without inventing drive data.

The details panel displays available authoritative fields only:

- server, enclosure, face, slot, and bay label;
- drive serial number, model, capacity, media type, and protocol;
- health/state, temperature, wear/lifetime, power-on hours, and error counters;
- pool, vdev/array, controller, replacement, and rebuild association;
- last observation, source integration, freshness, and source-system deep link; and
- linked incident/service context when later phases supply it.

Unavailable values render as `Unknown` or `Not reported`, never a fabricated zero.
The panel remains usable while the animation is disabled or still completing.

## Motion contract

- Hover/focus scale is subtle (normally `1.015`–`1.04`) and never changes layout.
- Room-to-rack and rack-to-server transitions target 220–400 ms.
- Bay inspection targets 160–280 ms and uses the chassis template's motion metadata.
- Easing communicates spatial movement without bounce, overshoot, blur, or glow.
- Input is never locked behind decorative sequencing; skip/back works immediately.
- `prefers-reduced-motion` replaces transforms with an instant state change or a
  brief opacity transition.
- Deep links and browser restoration open directly at the requested state.

## Performance and accessibility gates

- Maintain an equivalent table/list path for every room, rack, device, and drive.
- Hover, focus, tap, and keyboard selection expose equivalent information/actions.
- Virtualize or simplify off-screen geometry; do not render a high-poly scene when
  simple CSS/SVG/canvas geometry is sufficient.
- Keep status text and interaction targets in the DOM even if geometry uses canvas.
- Test large display, laptop, narrow layout, zoom, high contrast, keyboard-only, and
  reduced-motion behavior with representative full-room inventory.
- The room remains navigable when animations, WebGL, or optional model assets fail.

## Delivery boundary

- Phase 5 establishes room/rack inventory, the neutral room scene, rack focus, and
  device selection.
- Phase 6 adds model-aware chassis faces, indicators, drive bays, bay inspection,
  and drive details.
- Phase 7 adds the room editor, richer overlays, transition refinement, and visual
  polish after the non-animated inspection path is reliable.
