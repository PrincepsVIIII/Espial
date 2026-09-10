# Espial design preview

A separate, interactive front-end concept for Espial. The existing application,
routes, styles, dependencies, and backend are unchanged. The original UBNetDef
logo is copied byte-for-byte from the existing `BrandLogo.svelte` component.

## Run

From the repository root, with Node.js 20 or newer:

```sh
npm --prefix mockup start
```

Open **http://localhost:4175**. No installation, build, Docker, login, Core, or
database is needed. Stop with `Ctrl+C`. Choose another port with
`PORT=4180 npm --prefix mockup start`.

The server only serves local static files. The UI uses native HTML, CSS, JavaScript,
and SVG, with no external assets, font requests, trackers, or API requests. The
directory can also be served by any ordinary static server. Open it over HTTP;
browser ES modules cannot reliably run from a `file://` URL.

## Explore

- **Dashboard:** CPU, memory, storage, and compute balance are immediately visible.
  Select a resource for history and exact values; select a host to inspect it.
- **Host distribution:** switch CPU / Memory / Storage, compare on a consistent
  0–100% scale, or use the equivalent data table. Scope to either cluster.
- **History:** choose 1h / 6h / 24h; point at the plot or scrub with the slider.
  The slider also supports arrow keys, Home, and End. Exact observations are
  available in a table.
- **Alerts:** acknowledge a warning, assign it to yourself, and save a note. Every
  demo change returns a correlation receipt linked to its matching Audit entry.
  Acknowledging leaves the underlying condition active.
- **Hypervisor:** compare capacity, inspect hosts, and open their sample VM lists.
- **Datacenter:** explore room → rack → chassis → drive. Browser Back, breadcrumbs,
  and Escape preserve the physical context. List links provide the same access.
- **Webpages:** inspect sample availability, connection-stage timings, individual
  checks, and certificate validity.
- **Audit:** search sample events, inspect receipts, and view a sample user roster.
- **Preview states:** the footer lets you try stale, disconnected, empty,
  permission-denied, unavailable, and unexpected-error screens.

The account menu supports pointer, keyboard, and touch. Narrow screens use a
collapsed menu. Reset sample data or sign out of the demo from the account menu.
Escape dismisses menus and details; focus returns to the relevant control.

## Data and boundaries

All data is fictional and fixed at **10 September 2026, 14:32 UTC**. Values do not
pretend to stream. Changes live only in memory and reset on reload; receipt links
for newly generated events belong to the current tab session. Cluster selection
and inspection context are carried in the URL.

Six sample hosts have 192 CPU cores, 768 GiB memory, 48 TiB storage, and 36 running
VMs. Aggregate utilization is weighted by capacity: 38% CPU, 62% memory, and 66.5%
storage. Historical aggregates use the same host samples as host-level histories.
An empty dataset is never shown as zero utilization.

“Load balancing” means **compute workload distribution**, not network traffic or
an automatic rebalancing feature. This concept flags a CPU spread greater than
30 percentage points as uneven. The host chart emphasizes values at or above 75%;
the storage incident's sample warning threshold is 85%. The text provides exact
values and states alongside every color encoding.

The datacenter is a proposed physical-inspection concept, not a claim about
currently available inventory APIs. Rack geometry and drive details are explicitly
illustrative. VM migration, hardware control, integration setup, credential changes,
and user administration remain in their existing applications. This preview does
not change production capability or advance a Phase 2 implementation slice.

## Verify

The optional browser checks use the repository's existing Playwright and axe
development dependencies. From a checkout with `web/node_modules` installed:

```sh
npm --prefix mockup test
```

The test script starts and stops its own local static server. Screenshots and the
verification report are written to `mockup/test-results/` (ignored by Git).
See [UI review](UI_REVIEW.md) for the design rationale and validation record.
