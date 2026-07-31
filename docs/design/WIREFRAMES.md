# Product wireframes

These low-fidelity wireframes validate hierarchy, navigation, and data requirements.
They do not prescribe final spacing, illustration detail, or visual styling. The
physical sequence is governed by the
[physical drill-down contract](PHYSICAL_DRILLDOWN.md).

## Temporary public entry (Phase 1)

```text
┌────────────────────────────────────────────────────────────────────────────┐
│ Espial / UBNetDef                                      About   [ Sign in ] │
├────────────────────────────────────────────────────────────────────────────┤
│                                                                            │
│  INFRASTRUCTURE OPERATIONS, MADE VISIBLE                                   │
│  Espial brings health, inventory, incidents, and physical location into    │
│  one operator view.                                                        │
│                                                                            │
│  Monitor current health  │  Understand impact  │  Locate affected hardware │
│                                                                            │
│  Temporary product overview. No live operational data is shown here.       │
└────────────────────────────────────────────────────────────────────────────┘
```

The root page is informational, not a public status page. It must not reveal
hostnames, locations, integrations, health counts, incidents, or environment
metadata. The top-right sign-in action remains visible at supported widths.

## Local login and SSO-ready state

```text
┌────────────────────────────────────────────────────────────────────────────┐
│ Espial / UBNetDef                                            Back to home │
├────────────────────────────────────────────────────────────────────────────┤
│                                                                            │
│                    Sign in with a local account                            │
│                    Username [________________]                             │
│                    Password [________________]                             │
│                             [ Sign in ]                                    │
│                                                                            │
│       Local access is audited. SSO will appear here when enabled.          │
└────────────────────────────────────────────────────────────────────────────┘
```

In `sso_with_local_fallback` mode, the primary button is “Continue with UBNetDef
SSO”; the local form moves behind “Emergency local access” with an audit warning.

## Authenticated top navigation

```text
┌────────────────────────────────────────────────────────────────────────────┐
│ Espial / UBNetDef  Dashboard  Alerts  Datacenter  Hypervisor  Webpages     │
│                 Audit ▾                                      user / role ▾ │
└────────────────────────────────────────────────────────────────────────────┘
```

Dashboard is selected after login. The six items retain this order everywhere;
Datacenter and Audit are each one primary-navigation action away. Audit is
permission-gated and its submenu contains the implemented Users destination only
when the session has `users:manage`.
Primary labels navigate directly. Desktop dropdowns reveal on hover or keyboard
focus and remain available through click/touch activation. A short white rule
appears beneath the hovered, focused, or current label. Escape closes an open menu
and returns focus to its trigger. At narrow widths, the same permitted items and
real children move into one labeled navigation control without changing their order
or hiding sign-out. Normal connection health does not occupy the navigation;
reconnection or disconnection appears as a temporary content notice.

## Dashboard (Phase 1)

```text
┌────────────────────────────────────────────────────────────────────────────┐
│ Authenticated top navigation                                               │
├────────────────────────────────────────────────────────────────────────────┤
│ Dashboard                                            Updated 12 seconds ago │
│────────────────────────────────────────────────────────────────────────────│
│ CRITICAL 2        WARNING 4        STALE 3        UNKNOWN 1                │
│────────────────────────────────────────────────────────────────────────────│
│ Active incidents                                                            │
│ [CRIT] Storage pool degraded          8m   Unacknowledged                  │
│ [WARN] Host packet loss               3m   Investigating                   │
│────────────────────────────────────────────────────────────────────────────│
│ Monitoring coverage: Proxmox Healthy | TrueNAS Warning | iDRAC Stale       │
│────────────────────────────────────────────────────────────────────────────│
│ Resources needing attention                                                │
│ Status     Resource        Source         Last observation                 │
│ Stale      node-03         iDRAC          17m ago                          │
└────────────────────────────────────────────────────────────────────────────┘
```

Phase 1 initially renders the counts, integration health, and resource table. The
incident area is an explicit empty/not-yet-enabled state until Phase 2.

## Alerts list and incident detail (Phase 2 input)

```text
┌────────────────────────────────────────────────────────────────────────────┐
│ Authenticated top navigation                                               │
├────────────────────────────────────────────────────────────────────────────┤
│ Alerts     [Active] [History]  Severity ▾ Owner ▾                          │
│──────────────────────────────────┬─────────────────────────────────────────│
│ CRIT Storage pool degraded       │ Storage pool degraded                   │
│      TrueNAS • 8m • Open         │ Critical • Detected 8m                  │
│ WARN Host packet loss            │ [Acknowledge] [Assign]                  │
│      Proxmox • 3m • Ack          │ Resource / services                     │
│                                  │ Timeline                                │
│                                  │ 12:01 Detected                          │
│                                  │ 12:02 Alert delivered                   │
└──────────────────────────────────┴─────────────────────────────────────────┘
```

The list remains usable with the detail pane closed, and direct URLs open a specific
incident. Mutation controls are omitted or disabled with a reason when unauthorized.

## Datacenter inventory list and device detail

```text
┌────────────────────────────────────────────────────────────────────────────┐
│ Authenticated top navigation                                               │
├────────────────────────────────────────────────────────────────────────────┤
│ Datacenter       List | Room | Rack                  Filters [Stale ×]     │
│──────────────────────────────────────────┬─────────────────────────────────│
│ State     Resource  Type  Source Updated │ node-03                         │
│ Stale     node-03   Host  iDRAC  17m     │ Status: Stale                   │
│ Critical  pool-a    Pool  TrueNAS 2m     │ Location: Room A / R2 / U18     │
│ Healthy   vm-01     VM    Proxmox 1m     │ Incidents: 1                    │
│                                          │ Checks / links                  │
│                                          │ [Full device page]              │
└──────────────────────────────────────────┴─────────────────────────────────┘
```

Room and rack controls may show “planned” until their roadmap phases. The same
resource URLs and detail fields survive the later visualization work.

## Physical drill-down sequence (Phases 5–7)

### 1. Room overview

```text
Room A  /  Overview                              [List equivalent] [Filters ▾]
┌────────────────────────────────────────────────────────────────────────────┐
│  ▒▒ R1 ▒▒       ▒▒ R2 ▒▒       ▒▒ R3 ▒▒       ▒▒ R4 ▒▒                  │
│                 ┌────────┐                                                 │
│                 │ R2     │  hover/focus: slight scale + status outline     │
│                 └────────┘                                                 │
│  Neutral gray room and equipment; status color is reserved for signals.    │
└────────────────────────────────────────────────────────────────────────────┘
```

Selecting R2 moves the camera toward the rack and rotates to a straight-on front
view. Reduced-motion mode swaps the transition for an immediate state change.

### 2. Rack front

```text
Room A  ›  Rack R2                                      Front  |  Rear
┌────────┬───────────────────────────────────────────────────────────────────┐
│ U42    │ [ switch-01                                      ● healthy ]      │
│ ...    │                                                                   │
│ U20    │ [ server-07                         ● on  ◇ warning ]  ← selected │
│ U19    │ [ server-07                                                    ]  │
│ ...    │                                                                   │
│ U01    │ [ reserved                                                    ]  │
└────────┴───────────────────────────────────────────────────────────────────┘
```

Selecting a server focuses its front or rear chassis representation. Rack geometry
does not reflow when an item is hovered or focused.

### 3. Chassis and drive bays

```text
Room A  ›  Rack R2  ›  server-07                         Front  |  Rear
┌────────────────────────────────────────────────────────────────────────────┐
│ Power ● on       Device health ◇ warning       Updated 12 seconds ago      │
│                                                                            │
│ [00 ●] [01 ●] [02 ◇] [03 ○] [04 ●] [05 ●] [06 ●] [07 ●]                 │
│                 ^ selected drive                                           │
│                                                                            │
│ ● healthy/active   ◇ warning/rebuild   ○ empty/off   (always with labels)  │
└────────────────────────────────────────────────────────────────────────────┘
```

Bay numbering and movement come from the chassis template. Selecting bay 02 slides,
hinges, or exposes its carrier only when that motion matches the server type.

### 4. Drive inspection

```text
Room A  ›  Rack R2  ›  server-07  ›  Drive 02
┌───────────────────────────────────────┬────────────────────────────────────┐
│ Chassis front                         │ Drive 02                           │
│ [00] [01] [02 ─────►] [03] ...       │ Serial: Z...                       │
│               open carrier            │ Model / capacity / media / protocol│
│                                       │ Health: Warning                    │
│                                       │ Pool / vdev / controller           │
│                                       │ Temperature / wear / error counts  │
│                                       │ Rebuild state / data freshness     │
└───────────────────────────────────────┴────────────────────────────────────┘
```

The animation communicates selection only; it never implies remote eject or any
physical control. The selected drive also opens directly through its URL, and the
same details remain available in a non-spatial list/table mode.

## Responsive, input, and state requirements

- Large displays prioritize status and spatial context without adding decoration.
- Laptops retain top navigation, filters, breadcrumbs, and the details pane.
- Narrow screens stack details below content and provide a non-spatial list/table
  equivalent before attempting a compressed room rendering.
- Hover and focus may enlarge a selectable physical item by roughly 1.015–1.04,
  but selection must also work through click, tap, and Enter/Space.
- Every view defines loading, empty, permission denied, Core unavailable, stale-data,
  and live-stream-disconnected states.
- Status icon/label, focus visibility, and accessible table semantics remain present
  at every size. Motion is never the only indication of a state change.
