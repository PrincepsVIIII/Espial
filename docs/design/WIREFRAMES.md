# Phase 0 wireframes

These low-fidelity wireframes validate hierarchy and data requirements. They do not
prescribe final spacing or visual styling.

## Local login and SSO-ready state

```text
┌──────────────────────────────────────────────────────────────┐
│ Espial / UBNetDef Infrastructure Operations                  │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│                 Sign in with a local account                 │
│                 Username [________________]                  │
│                 Password [________________]                  │
│                          [ Sign in ]                         │
│                                                              │
│  Local access is audited. SSO will appear here when enabled. │
└──────────────────────────────────────────────────────────────┘
```

In `sso_with_local_fallback` mode, the primary button is “Continue with UBNetDef
SSO”; the local form moves behind “Emergency local access” with an audit warning.

## Overview

```text
┌──────────────┬──────────────────────────────────────────────────────────┐
│ Espial       │ Overview                         Live ●  Updated 12s ago │
│──────────────│──────────────────────────────────────────────────────────│
│ Overview     │ CRITICAL 2   WARNING 4   STALE 3   UNKNOWN 1            │
│ Incidents    │──────────────────────────────────────────────────────────│
│ Services     │ Active incidents                                        │
│ Infra        │ [CRIT] Storage pool degraded        8m  Unacknowledged  │
│ Certificates │ [WARN] Host packet loss             3m  Investigating   │
│ History      │──────────────────────────────────────────────────────────│
│ Integrations │ Monitoring coverage                                     │
│ Admin        │ Proxmox Healthy  TrueNAS Warning  iDRAC Stale           │
│              │──────────────────────────────────────────────────────────│
│ user / role  │ Resources needing attention                             │
│ Sign out     │ Status  Resource       Source       Last observation     │
│              │ Stale   node-03        iDRAC        17m ago              │
└──────────────┴──────────────────────────────────────────────────────────┘
```

Phase 1 initially renders the counts, integration health, and resource table. The
incident area is an explicit empty/not-yet-enabled state until Phase 2.

## Incident list and detail (Phase 2 input)

```text
┌──────────────┬──────────────────────────────────────────────────────────┐
│ Navigation   │ Incidents  [Active] [History]  Severity ▾ Owner ▾       │
│              │───────────────────────────────┬──────────────────────────│
│              │ CRIT Storage pool degraded   │ Storage pool degraded    │
│              │      TrueNAS • 8m • Open      │ Critical • Detected 8m  │
│              │ WARN Host packet loss         │ [Acknowledge] [Assign]  │
│              │      Proxmox • 3m • Ack       │ Resource / services     │
│              │                               │ Timeline                │
│              │                               │ 12:01 Detected          │
│              │                               │ 12:02 Alert delivered   │
└──────────────┴───────────────────────────────┴──────────────────────────┘
```

The list remains usable with the detail pane closed, and direct URLs open a specific
incident. Mutation controls are omitted or disabled with a reason when unauthorized.

## Infrastructure list and device detail

```text
┌──────────────┬──────────────────────────────────────────────────────────┐
│ Navigation   │ Infrastructure  List | Rack | Room   Filters [Stale ×] │
│              │────────────────────────────────────┬─────────────────────│
│              │ State Resource Type Source Updated│ node-03             │
│              │ Stale node-03 Host iDRAC 17m      │ Status: Stale       │
│              │ Crit  pool-a  Pool TrueNAS 2m     │ Location: R2 / U18  │
│              │ Healthy vm-01 VM Proxmox 1m       │ Incidents: 1        │
│              │                                    │ Checks / links      │
│              │                                    │ [Full device page]  │
└──────────────┴────────────────────────────────────┴─────────────────────┘
```

Rack and room tabs may show “planned” until their roadmap phases. The same resource
URL and drawer fields survive the later visualization work.

## Responsive and state requirements

- Large display prioritizes counts and unacknowledged failures.
- Laptop retains navigation, table filters, and detail drawer.
- Narrow screens stack the detail below the list; login and basic inspection remain
  usable.
- Every view defines loading skeleton, empty state, permission denied, Core
  unavailable, and live stream disconnected states.
- Status icon/label and accessible table semantics remain present at every size.
