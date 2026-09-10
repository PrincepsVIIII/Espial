import {
  hosts,
  services,
  clusters,
  snapshot,
  summarize,
  metricHistory,
  initialAlerts,
  initialAudit,
} from "./data.js";
import { renderDatacenter } from "./datacenter.js";

const app = document.querySelector("#app");
const dialog = document.querySelector("#detail");
const state = {
  scope: "all",
  metric: "cpu",
  chartTable: false,
  range: "6h",
  detailMetric: "cpu",
  alerts: structuredClone(initialAlerts),
  audit: structuredClone(initialAudit),
  alertFilter: "all",
  search: "",
  scenario: "normal",
  signedOut: false,
};
const labels = {
  dashboard: "Dashboard",
  alerts: "Alerts",
  datacenter: "Datacenter",
  hypervisor: "Hypervisor",
  webpages: "Webpages",
  audit: "Audit",
};
const metricLabels = { cpu: "CPU", memory: "Memory", storage: "Storage" };
let route = readRoute();
let previousPage = "";
let returnFocus = null;
let suppressAccountFocus = false;
let sequence = 0;
let currentSeries = [];
const fmt = (value, digits = 0) =>
  Number(value).toLocaleString("en-US", { maximumFractionDigits: digits });
const esc = (value) =>
  String(value ?? "").replace(
    /[&<>"']/g,
    (char) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        char
      ],
  );
const time = (value) =>
  new Date(value).toLocaleTimeString("en-GB", {
    timeZone: "UTC",
    hour: "2-digit",
    minute: "2-digit",
  });
const titleCase = (value) => value[0].toUpperCase() + value.slice(1);
const arrow = '<span class="arrow" aria-hidden="true">↗</span>';

function readRoute() {
  const [path, query = ""] = (location.hash.slice(1) || "/dashboard").split(
    "?",
  );
  return {
    page: path.split("/")[1] || "dashboard",
    child: path.split("/")[2] || "",
    params: new URLSearchParams(query),
  };
}
function href(params = {}, page = route.page) {
  const query = new URLSearchParams(
    page === route.page ? route.params : undefined,
  );
  for (const [key, value] of Object.entries(params)) query.set(key, value);
  if (params.detail && page === "hypervisor") query.delete("host");
  return `#/${page}${page === route.page && route.child ? `/${route.child}` : ""}${query.size ? `?${query}` : ""}`;
}
function selectedHosts() {
  return hosts.filter(
    (host) => state.scope === "all" || host.cluster === state.scope,
  );
}
function selectedAlerts() {
  return state.alerts.filter((alert) =>
    selectedHosts().some((host) => host.id === alert.hostId),
  );
}
function status(value, text = titleCase(value)) {
  return `<span class="health ${value === "healthy" ? "" : esc(value)}">${esc(text)}</span>`;
}
function meter(value) {
  return `<span class="segmented-meter" aria-hidden="true">${Array.from({ length: 30 }, (_, index) => `<i class="${index < Math.round(value * 0.3) ? "filled" : ""}"></i>`).join("")}</span>`;
}
function pageHeading(title, description, controls = "") {
  return `<div class="page-heading"><div><h1 tabindex="-1">${title}</h1><p>${description}</p></div>${controls ? `<div class="page-controls">${controls}</div>` : ""}</div>`;
}
function scopeControl() {
  return `<label class="field-inline"><span>View</span><select id="scope" aria-label="Cluster scope"><option value="all">All clusters</option>${clusters.map((cluster) => `<option ${state.scope === cluster.name ? "selected" : ""}>${cluster.name}</option>`).join("")}</select></label>`;
}
function shell(body) {
  return `<header class="topbar">
    <a class="brand" href="#/dashboard" aria-label="Espial Dashboard"><img src="./assets/ubnetdef.png" alt="UBNetDef" width="173" height="69"><span class="product-name">Espial</span></a>
    <button class="menu-toggle" data-action="menu" aria-expanded="false" aria-controls="primary-nav">Menu <span class="chevron" aria-hidden="true"></span></button>
    <nav class="primary-nav" id="primary-nav" aria-label="Primary">${Object.entries(
      labels,
    )
      .map(
        ([key, label]) =>
          `<a class="nav-link" href="#/${key}" ${route.page === key ? 'aria-current="page"' : ""}>${label}</a>`,
      )
      .join(
        "",
      )}<div class="mobile-account"><span>Demo operator<br><small class="muted">Administrator</small></span><div><button data-action="reset">Reset data</button><button data-action="signout">Sign out of demo</button></div></div></nav>
    <div class="account"><button class="account-trigger" data-action="account" aria-expanded="false" aria-controls="account-menu">Operator<span class="chevron" aria-hidden="true"></span></button><div class="dropdown" id="account-menu" hidden><strong>Demo operator</strong><p>Administrator · sample session</p><button data-action="reset">Reset sample data</button><button data-action="signout">Sign out of demo</button></div></div>
  </header><main class="content" id="main">${body}<footer class="footer"><span>Espial / UBNetDef <span aria-hidden="true">·</span> Design preview. All data is fictional.</span><label class="footer-controls">Preview state <select class="preview-state" id="scenario" aria-label="Preview state">${Object.entries(
    {
      normal: "Sample snapshot",
      stale: "Stale data",
      disconnected: "Disconnected",
      empty: "No data",
      denied: "Permission denied",
      unavailable: "Core unavailable",
      error: "Unexpected error",
    },
  )
    .map(
      ([key, text]) =>
        `<option value="${key}" ${state.scenario === key ? "selected" : ""}>${text}</option>`,
    )
    .join("")}</select></label></footer></main>`;
}
function resourceStrip(list) {
  const total = summarize(list);
  const spread =
    Math.max(...list.map((host) => host.cpuPercent)) -
    Math.min(...list.map((host) => host.cpuPercent));
  return `<section class="resource-strip" aria-label="Resource overview">
    ${["cpu", "memory", "storage"]
      .map((metric) => {
        const value = total[`${metric}Percent`];
        const note =
          metric === "cpu"
            ? `${total.cpuCores} cores · ${list.length} hosts`
            : metric === "memory"
              ? `${fmt(total.memoryUsedGiB)} / ${total.memoryGiB} GiB used`
              : `${fmt(total.storageUsedTiB, 1)} / ${total.storageTiB} TiB used`;
        return `<a class="resource" href="${href({ detail: metric })}" aria-label="Inspect ${metricLabels[metric]}: ${fmt(value, 1)} percent used"><div class="resource-label">${metricLabels[metric]} utilization ${arrow}</div><div class="resource-value">${fmt(value, 1)}<span class="unit">%</span></div>${meter(value)}<p class="resource-note">${note}</p></a>`;
      })
      .join("")}
    <a class="resource" href="${href({ detail: "balance" })}" aria-label="Inspect load balancing: ${spread > 30 ? "uneven" : "balanced"} compute distribution"><div class="resource-label">Load balancing ${arrow}</div><div class="resource-value balance-value ${spread > 30 ? "warning" : ""}">${spread > 30 ? "Uneven" : "Balanced"}</div><span class="balance-meter" aria-hidden="true">${list.map((host) => `<i class="${host.cpuPercent > 70 ? "hot" : ""}" style="height:${host.cpuPercent}%"></i>`).join("")}</span><p class="resource-note">${Math.min(...list.map((host) => host.cpuPercent))}–${Math.max(...list.map((host) => host.cpuPercent))}% CPU across hosts</p></a>
  </section>`;
}
function hostTable(list, metricOnly = false) {
  return `<div class="table-wrap"><table><caption>Sample host utilization · ${time(snapshot)} UTC</caption><thead><tr><th scope="col">Host</th><th scope="col">CPU</th><th scope="col">Memory</th><th scope="col">Storage</th>${metricOnly ? "" : '<th scope="col">VMs</th><th scope="col">State</th>'}</tr></thead><tbody>${list.map((host) => `<tr><td><a href="${href({ detail: host.id })}" class="mono">${host.name}</a><span class="table-secondary">${host.cluster}</span></td><td class="mono">${host.cpuPercent}%</td><td class="mono">${host.memoryPercent}%</td><td class="mono">${host.storagePercent}%</td>${metricOnly ? "" : `<td class="mono">${host.guests}</td><td>${status(host.state)}</td>`}</tr>`).join("")}</tbody></table></div>`;
}
function hostChart(list, title = "Host distribution") {
  const metric = state.metric;
  return `<section aria-label="${title}"><div class="section-head"><div><h2>${title}</h2></div><div class="segmented-control" aria-label="Host comparison metric">${Object.entries(
    metricLabels,
  )
    .map(
      ([key, label]) =>
        `<button data-action="metric" data-value="${key}" aria-pressed="${metric === key}">${label}</button>`,
    )
    .join("")}</div></div>
  ${
    state.chartTable
      ? hostTable(list)
      : `<div class="host-chart"><div class="chart-axis" aria-hidden="true"><span>HOST</span><div class="axis-ticks"><span>0</span><span>25</span><span>50</span><span>75</span><span>100%</span></div><span></span></div>${list
          .map((host) => {
            const value = host[`${metric}Percent`];
            return `<a class="host-row" href="${href({ detail: host.id })}" aria-label="Inspect ${host.name}, ${metricLabels[metric]} ${value} percent, ${host.state}"><div class="host-id">${host.name}<span class="host-cluster">${host.cluster}${host.state === "warning" ? " · Warning" : ""}</span></div><div class="rail" aria-hidden="true">${[0, 25, 50, 75, 100].map((tick) => `<i class="gridline" style="left:${tick}%"></i>`).join("")}<span class="bar ${value >= 75 ? "hot" : ""}" style="width:${value}%"><i class="endpoint"></i></span></div><span class="rail-value ${value >= 75 ? "warning" : ""}">${value}<span class="muted">%</span></span></a>`;
          })
          .join("")}</div>`
  }<div class="chart-foot"><span>Utilization / host capacity · Select a host to inspect</span><button data-action="table">${state.chartTable ? "Show distribution" : "Show data table"}</button></div></section>`;
}
function attention() {
  const alerts = selectedAlerts();
  return `<aside class="attention-panel" aria-label="Attention and service checks"><section><div class="section-head"><h2>Needs attention</h2><span class="count">${String(alerts.length).padStart(2, "0")}</span></div>${alerts.length ? alerts.map((alert) => `<a class="attention-item" href="${href({ detail: alert.id }, "alerts")}"><div><h3>${alert.title}</h3><p>${alert.hostId} · ${alert.metric === "storage" ? `${alert.value}% used · 1.12 TiB remaining` : `${alert.value}% CPU · cluster average 42.75%`}</p><p>${alert.acknowledgedBy ? "Acknowledged · Demo operator" : "Warning · awaiting review"}</p></div>${arrow}</a>`).join("") : '<p class="empty-inline">No active alerts in this cluster.</p>'}<a class="text-link attention-link" href="#/alerts">Review alerts <span aria-hidden="true">→</span></a></section>
  <section class="service-summary"><h2>Webpages <span class="muted small">/ 3 reachable</span></h2>${services.map((service) => `<a class="service-mini" href="${href({ detail: service.id }, "webpages")}"><span>${service.name}</span><span class="health"><span class="mono">${service.latencyMs} ms</span><span class="sr-only">Healthy</span></span></a>`).join("")}</section></aside>`;
}
function dashboard() {
  const list = selectedHosts();
  const total = summarize(list);
  return `${pageHeading("Dashboard", "Capacity and health across your clusters.", scopeControl())}
  <div class="system-line"><div class="system-summary">${status(state.scenario === "stale" || state.scenario === "disconnected" ? "stale" : "healthy", state.scenario === "stale" || state.scenario === "disconnected" ? "Last known: all hosts reachable" : "All hosts reachable")}<a href="#/alerts" class="muted">${total.warningCount ? "1 host needs attention" : "No hosts need attention"} <span aria-hidden="true">→</span></a></div><div class="snapshot"><span>Sample snapshot</span><time datetime="${snapshot}" title="${snapshot}">${time(snapshot)} UTC · 10 Sep 2026</time></div></div>
  ${resourceStrip(list)}<div class="dashboard-grid">${hostChart(list)}${attention()}</div>
  <div class="coverage"><strong>${total.hostCount} hosts <span>/ ${total.guests} running VMs</span></strong><span>${state.scope === "all" ? "2 clusters" : state.scope} · Proxmox</span><span>All ${total.hostCount} sample hosts reporting</span><a href="#/hypervisor" class="text-link">Inspect infrastructure <span aria-hidden="true">→</span></a></div>`;
}
function hypervisor() {
  const list = selectedHosts();
  return `${pageHeading("Hypervisor", "Compute, storage, and virtual-machine placement.", scopeControl())}${resourceStrip(list)}${hostChart(list, "Compare hosts")}<section class="wide-section"><div class="section-head"><div><h2>Cluster inventory</h2><p>Capacity and running guests at the sample snapshot.</p></div><a class="text-link" href="#/datacenter">Locate hardware <span aria-hidden="true">→</span></a></div><div class="table-wrap"><table><thead><tr><th scope="col">Cluster</th><th scope="col">Hosts</th><th scope="col">CPU cores</th><th scope="col">Memory</th><th scope="col">Storage</th><th scope="col">Running VMs</th></tr></thead><tbody>${clusters
    .filter((cluster) => state.scope === "all" || state.scope === cluster.name)
    .map((cluster) => {
      const sum = summarize(
        hosts.filter((host) => host.cluster === cluster.name),
      );
      return `<tr><td>${cluster.name}</td><td class="mono">${sum.hostCount}</td><td class="mono">${sum.cpuCores}</td><td class="mono">${sum.memoryGiB} GiB</td><td class="mono">${sum.storageTiB} TiB</td><td class="mono">${sum.guests}</td></tr>`;
    })
    .join("")}</tbody></table></div></section>`;
}
function alertsPage() {
  const history = route.child === "history";
  const alerts = selectedAlerts().filter(
    (alert) =>
      state.alertFilter === "all" ||
      (state.alertFilter === "unacknowledged"
        ? !alert.acknowledgedBy
        : alert.acknowledgedBy),
  );
  return `${pageHeading("Alerts", "Active conditions, investigation notes, and ownership.", scopeControl())}<nav class="subnav" aria-label="Alert views"><a href="#/alerts" ${history ? "" : 'aria-current="page"'}>Active <span class="mono muted">${selectedAlerts().length}</span></a><a href="#/alerts/history" ${history ? 'aria-current="page"' : ""}>History</a></nav>${history ? (state.scope === "Research" ? '<p class="empty-inline">No recovered sample incidents in Research.</p>' : `<div class="table-wrap"><table><caption>One fictional recovered incident</caption><thead><tr><th scope="col">Incident</th><th scope="col">Resource</th><th scope="col">Recovered</th><th scope="col">Evidence</th></tr></thead><tbody><tr><td>CPU utilization recovered<span class="table-secondary">Resolved · 28% at recovery</span></td><td class="mono">pve-02</td><td class="mono">13:48 UTC</td><td><a class="text-link" href="#/audit?receipt=DEMO-0846">Acknowledgement →</a></td></tr></tbody></table></div>`) : `<div class="filter-bar"><p class="small muted">${selectedAlerts().length} active warnings · no critical incidents</p><label class="field-inline">Show<select id="alert-filter"><option value="all">All active</option><option value="unacknowledged" ${state.alertFilter === "unacknowledged" ? "selected" : ""}>Needs acknowledgement</option><option value="acknowledged" ${state.alertFilter === "acknowledged" ? "selected" : ""}>Acknowledged</option></select></label></div>${alerts.length ? alerts.map((alert) => `<a class="alert-row" href="${href({ detail: alert.id }, "alerts")}">${status("warning")}<div><h2>${alert.title}</h2><p>${alert.hostId} · ${alert.metric === "storage" ? "86% of 8 TiB used" : "78% CPU utilization"}</p></div><span class="muted">${alert.acknowledgedBy ? "Acknowledged" : "Needs review"}</span><span class="alert-owner muted">${time(alert.startedAt)} UTC</span>${arrow}</a>`).join("") : '<p class="empty-inline">No alerts match this view.</p>'}`}`;
}
function webpages() {
  const certificates = route.child === "certificates";
  return `${pageHeading("Webpages", "Availability first. Connection detail when you need it.")}<nav class="subnav" aria-label="Webpage views"><a href="#/webpages" ${certificates ? "" : 'aria-current="page"'}>Availability</a><a href="#/webpages/certificates" ${certificates ? 'aria-current="page"' : ""}>Certificates</a></nav>${certificates ? `<div class="table-wrap"><table><caption>Sample certificate inventory · evaluated at ${time(snapshot)} UTC</caption><thead><tr><th scope="col">Webpage</th><th scope="col">Validity</th><th scope="col">Remaining</th><th scope="col">Inspect</th></tr></thead><tbody>${services.map((service) => `<tr><td>${service.name}<span class="table-secondary">${service.url}</span></td><td>${status("healthy", "Valid")}</td><td class="mono">${service.certificateDays} days</td><td><a class="text-link" href="${href({ detail: service.id }, "webpages")}">Certificate details →</a></td></tr>`).join("")}</tbody></table></div>` : `<div class="section-head"><div><h2>3 of 3 reachable</h2><p>25 hourly sample checks per webpage · last 24 hours</p></div><span class="health">Healthy</span></div><div class="service-list">${services.map((service) => `<a class="service-row" href="${href({ detail: service.id })}"><div><h2>${service.name}</h2><p>${service.url.replace("https://", "")}</p></div><div class="history-cell"><div class="check-history" aria-hidden="true">${service.history.map(() => "<i></i>").join("")}</div><div class="check-history-caption">25 / 25 checks successful · HTTP 200</div></div><div><span class="mono">${service.latencyMs} ms</span><p>Response</p></div><div class="certificate-cell"><span class="mono">${service.certificateDays} days</span><p>Certificate remaining</p></div>${arrow}</a>`).join("")}</div><p class="sample-caption">Each mark is one successful sample check, not continuous uptime. Select a webpage for timings and exact observations.</p>`}`;
}
function auditPage() {
  const receipt = route.params.get("receipt");
  const users = route.child === "users";
  let events = state.audit.filter((event) =>
    receipt
      ? event.correlationId === receipt
      : `${event.action} ${event.target} ${event.actor} ${event.correlationId}`
          .toLowerCase()
          .includes(state.search.toLowerCase()),
  );
  return `${pageHeading("Audit", "A traceable record of every change in this demo.")}<nav class="subnav" aria-label="Audit views"><a href="#/audit" ${users ? "" : 'aria-current="page"'}>Event history</a><a href="#/audit/users" ${users ? 'aria-current="page"' : ""}>Users</a></nav>${users ? `<div class="notice"><p>User administration remains in the existing application. This preview shows a sample roster.</p></div><div class="table-wrap"><table><thead><tr><th scope="col">User</th><th scope="col">Role</th><th scope="col">Access</th></tr></thead><tbody><tr><td>Demo operator<span class="table-secondary">operator@example.com</span></td><td>Administrator</td><td>${status("healthy", "Enabled")}</td></tr><tr><td>Demo observer<span class="table-secondary">observer@example.com</span></td><td>Viewer</td><td>${status("healthy", "Enabled")}</td></tr></tbody></table></div>` : `<div class="filter-bar"><label class="sr-only" for="audit-search">Search audit events</label><input id="audit-search" type="search" placeholder="Search events, resources, receipts…" value="${receipt ? "" : esc(state.search)}"><span class="small muted">${events.length} ${receipt ? "matching" : "sample"} events ${receipt ? '<a class="text-link" href="#/audit">Clear receipt filter</a>' : ""}</span></div>${receipt ? `<div class="receipt"><strong>Correlation receipt <span class="mono">${esc(receipt)}</span></strong><p>This filtered record is the evidence for the linked demo action.</p></div>` : ""}<div class="table-wrap"><table><caption>Session-only sample audit log · all timestamps UTC</caption><thead><tr><th scope="col">Time</th><th scope="col">Action / resource</th><th scope="col">Actor</th><th scope="col">Receipt</th></tr></thead><tbody>${events.map((event) => `<tr><td class="mono">${time(event.timestamp)}</td><td><strong>${esc(event.action)}</strong><span class="table-secondary">${esc(event.target)}</span></td><td>${esc(event.actor)}</td><td><a href="${href({ detail: event.id })}" class="text-link mono">${esc(event.correlationId)} ↗</a></td></tr>`).join("")}${events.length ? "" : '<tr><td colspan="4">No audit events match your search.</td></tr>'}</tbody></table></div>`}`;
}
function seriesFor(metric, hostId) {
  if (hostId) return metricHistory(metric, state.range, hostId);
  const list = selectedHosts();
  const weightKey = {
    cpu: "cpuCores",
    memory: "memoryGiB",
    storage: "storageTiB",
  }[metric];
  const series = list.map((host) => ({
    weight: host[weightKey],
    points: metricHistory(metric, state.range, host.id),
  }));
  const weightTotal = series.reduce((sum, item) => sum + item.weight, 0);
  return series[0].points.map((point, index) => ({
    timestamp: point.timestamp,
    value:
      series.reduce(
        (sum, item) => sum + item.points[index].value * item.weight,
        0,
      ) / weightTotal,
  }));
}
function trend(metric, hostId) {
  const points = seriesFor(metric, hostId);
  currentSeries = points;
  const path = points
    .map(
      (point, index) =>
        `${index ? "L" : "M"} ${42 + index * 21} ${180 - point.value * 1.5}`,
    )
    .join(" ");
  const latest = points.at(-1);
  return `<section class="detail-section"><div class="section-head"><div><h3>${metricLabels[metric]} history</h3><p>Sample values · percent of capacity</p></div><div class="segmented-control" aria-label="History period">${["1h", "6h", "24h"].map((range) => `<button data-action="range" data-value="${range}" aria-pressed="${state.range === range}">${range}</button>`).join("")}</div></div><div class="trend"><svg viewBox="0 0 568 212" role="img" aria-label="${metricLabels[metric]} sample history from ${time(points[0].timestamp)} to ${time(latest.timestamp)} UTC. Values are available with the slider and data table.">${[0, 25, 50, 75, 100].map((tick) => `<line x1="42" x2="546" y1="${180 - tick * 1.5}" y2="${180 - tick * 1.5}" stroke="#2d3b46" stroke-width="1"/><text x="29" y="${184 - tick * 1.5}" text-anchor="end" fill="#a7b2bc" font-family="monospace" font-size="10">${tick}</text>`).join("")}<path d="${path}" fill="none" stroke="#bac8d2" stroke-width="2" stroke-linejoin="round"/><line id="chart-guide" x1="546" x2="546" y1="24" y2="180" stroke="#55d6e2" stroke-width="1" stroke-dasharray="3 4"/><circle id="chart-point" cx="546" cy="${180 - latest.value * 1.5}" r="4" fill="#55d6e2"/><text x="42" y="205" fill="#a7b2bc" font-family="monospace" font-size="10">${time(points[0].timestamp)}</text><text x="546" y="205" text-anchor="end" fill="#a7b2bc" font-family="monospace" font-size="10">${time(latest.timestamp)} UTC</text></svg><label><span class="sr-only">Inspect history sample</span><input class="chart-scrubber" id="chart-scrubber" type="range" min="0" max="24" value="24" aria-valuetext="${time(latest.timestamp)} UTC, ${fmt(latest.value, 1)} percent"></label><output class="chart-reading" id="chart-reading" for="chart-scrubber">${time(latest.timestamp)} UTC <span class="muted">/</span> ${fmt(latest.value, 1)}%</output><div class="trend-summary"><span>Min ${fmt(Math.min(...points.map((point) => point.value)), 1)}% · Max ${fmt(Math.max(...points.map((point) => point.value)), 1)}%</span><span>25 fixed observations</span></div><details><summary class="text-link">View exact sample values</summary><div class="table-wrap"><table><caption>${metricLabels[metric]} sample history</caption><thead><tr><th scope="col">Timestamp (UTC)</th><th scope="col">Utilization</th></tr></thead><tbody>${points.map((point) => `<tr><td class="mono">${point.timestamp.replace("T", " ").replace(".000Z", "").replace("Z", "")}</td><td class="mono">${fmt(point.value, 2)}%</td></tr>`).join("")}</tbody></table></div></details></div></section>`;
}
function detailHeader(title, description) {
  return `<div class="detail-head"><div><h2 id="detail-title" tabindex="-1">${esc(title)}</h2><p>${description}</p>${["stale", "disconnected"].includes(state.scenario) ? '<p class="warning">Last-known sample · collection ' + (state.scenario === "stale" ? "overdue" : "disconnected") + ". Readings and states are not current.</p>" : ""}</div><button class="close-button" data-action="close" aria-label="Close details">×</button></div>`;
}
function metricDetail(metric) {
  const list = selectedHosts();
  const total = summarize(list);
  const note =
    metric === "cpu"
      ? `${total.cpuCores} physical cores across ${total.hostCount} hosts`
      : metric === "memory"
        ? `${fmt(total.memoryUsedGiB, 1)} GiB used of ${total.memoryGiB} GiB`
        : `${fmt(total.storageUsedTiB, 2)} TiB used of ${total.storageTiB} TiB`;
  return `${detailHeader(`${metricLabels[metric]} utilization`, `${state.scope === "all" ? "All clusters" : state.scope} · sample snapshot · ${time(snapshot)} UTC`)}<div class="detail-body"><div class="detail-stat"><div><div class="resource-value">${fmt(total[`${metric}Percent`], 1)}<span class="unit">%</span></div><p>${note}</p></div><span class="muted small">Capacity weighted</span></div>${meter(total[`${metric}Percent`])}${trend(metric)}<section class="detail-section"><h3>By host</h3>${hostTable(
    [...list].sort((a, b) => b[`${metric}Percent`] - a[`${metric}Percent`]),
    true,
  )}</section></div>`;
}
function hostDetail(host) {
  const cpu = state.detailMetric;
  return `${detailHeader(host.name, `${host.cluster} / Rack ${host.rack} · sample host`)}<div class="detail-body"><div class="section-head">${status(host.state)}<span class="mono muted small">${host.guests} running VMs</span></div><div class="mini-stats">${["cpu", "memory", "storage"].map((metric) => `<div class="mini-stat"><p>${metricLabels[metric]}</p><strong>${host[`${metric}Percent`]}%</strong><small>${metric === "cpu" ? `${host.cpuCores} cores` : metric === "memory" ? `${fmt(host.memoryUsedGiB, 1)} / ${host.memoryGiB} GiB` : `${host.storageUsedTiB} / ${host.storageTiB} TiB`}</small></div>`).join("")}</div>${host.id === "pve-03" ? `<div class="receipt"><strong class="warning">Storage is approaching capacity.</strong><p>1.12 TiB remaining. The sample warning threshold is 85%.</p><a class="text-link" href="#/alerts?detail=alert-storage-03">Open related alert →</a></div>` : ""}<div class="segmented-control" aria-label="Host history metric">${Object.entries(
    metricLabels,
  )
    .map(
      ([key, label]) =>
        `<button data-action="detail-metric" data-value="${key}" aria-pressed="${cpu === key}">${label}</button>`,
    )
    .join(
      "",
    )}</div>${trend(cpu, host.id)}<section class="detail-section"><h3>Host context</h3><dl class="fact-list"><div><dt>Source</dt><dd>Proxmox · sample fixture</dd></div><div><dt>Last observation</dt><dd class="mono">${time(snapshot)} UTC</dd></div><div><dt>Cluster</dt><dd>${host.cluster}</dd></div><div><dt>Placement</dt><dd>Rack ${host.rack}</dd></div></dl><div class="action-row"><a class="button" href="#/datacenter?rack=${host.rack}&host=${host.id}">Locate in datacenter →</a><a class="text-link" href="${href({ detail: `guests-${host.id}` }, "hypervisor")}">Inspect ${host.guests} VMs →</a></div></section></div>`;
}
function guestsDetail(host) {
  return `${detailHeader(`Virtual machines / ${host.name}`, "Sample guest inventory · running guests only")}<div class="detail-body"><p class="detail-copy">${host.guests} running virtual machines are placed on this host. Placement is displayed for inspection.</p><div class="detail-section table-wrap"><table><caption>Fictional VM names and allocations</caption><thead><tr><th scope="col">VM</th><th scope="col">vCPUs</th><th scope="col">Allocated memory</th><th scope="col">State</th></tr></thead><tbody>${Array.from({ length: host.guests }, (_, index) => `<tr><td class="mono">lab-${host.id.slice(-2)}-${String(index + 1).padStart(2, "0")}</td><td class="mono">${index % 3 === 0 ? 4 : 2}</td><td class="mono">${index % 3 === 0 ? 8 : 4} GiB</td><td>${status("healthy", "Running")}</td></tr>`).join("")}</tbody></table></div><div class="action-row"><a class="text-link" href="${href({ detail: host.id }, "hypervisor")}">← Back to ${host.name}</a></div><p class="sample-caption">Migration and VM controls remain in Proxmox. No infrastructure actions are connected in this preview.</p></div>`;
}
function balanceDetail() {
  const list = selectedHosts();
  const busy = [...list].sort((a, b) => b.cpuPercent - a.cpuPercent)[0];
  const available = [...list].sort((a, b) => a.cpuPercent - b.cpuPercent)[0];
  return `${detailHeader("Compute load balancing", "Compare placement and headroom · sample snapshot")}<div class="detail-body"><p class="detail-copy">${busy.name} has the highest CPU utilization at <strong>${busy.cpuPercent}%</strong>. ${available.name} is at <strong>${available.cpuPercent}%</strong>. Compare memory and guest counts before reviewing placement within each cluster.</p><div class="detail-section">${hostTable(list)}</div><div class="receipt"><strong>Workload inspection only</strong><p>This is host workload distribution, not a network traffic balancer. Espial does not move workloads in this preview.</p></div><a class="text-link" href="#/hypervisor">Open Hypervisor →</a></div>`;
}
function receiptMarkup(alert) {
  return alert.correlationId
    ? `<div class="receipt" role="status"><strong>Demo change saved</strong><p>${alert.lastAction || "Alert updated"}. Stored only in this tab.</p><a class="text-link" href="#/audit?receipt=${alert.correlationId}">View audit receipt ${alert.correlationId} →</a></div>`
    : "";
}
function alertDetail(alert) {
  return `${detailHeader(alert.title, `${alert.hostId} · opened ${time(alert.startedAt)} UTC · sample incident`)}<div class="detail-body"><div class="section-head">${status("warning")}<span class="muted small">${alert.acknowledgedBy ? "Acknowledged by Demo operator" : "Awaiting acknowledgement"}</span></div><p class="detail-copy">${alert.description}</p>${receiptMarkup(alert)}<div class="action-row"><button class="button primary" data-action="acknowledge" data-id="${alert.id}" ${alert.acknowledgedBy ? "disabled" : ""}>${alert.acknowledgedBy ? "Acknowledged" : "Acknowledge alert"}</button><button class="button" data-action="assign" data-id="${alert.id}" ${alert.owner ? "disabled" : ""}>${alert.owner ? "Assigned to you" : "Assign to me"}</button></div><section class="detail-section"><h3>Observed condition</h3><dl class="fact-list"><div><dt>Current ${metricLabels[alert.metric].toLowerCase()}</dt><dd class="mono">${alert.value}%</dd></div><div><dt>${alert.threshold ? "Warning threshold" : "Cluster average"}</dt><dd class="mono">${alert.threshold || alert.clusterAverage}%</dd></div><div><dt>First observed</dt><dd class="mono">${time(alert.startedAt)} UTC</dd></div><div><dt>Owner</dt><dd>${alert.owner || "Unassigned"}</dd></div></dl><a class="text-link attention-link" href="${href({ detail: alert.hostId }, "hypervisor")}">Inspect ${alert.hostId} →</a></section><section class="detail-section"><h3>Investigation notes</h3><form class="note-form" id="note-form" data-id="${alert.id}"><label for="note">Add context for the next operator</label><textarea id="note" name="note" required maxlength="1200" placeholder="What did you find?"></textarea><button class="button" type="submit">Save demo note</button></form><div class="notes">${(alert.notes || []).map((note) => `<div class="note"><small>Demo operator · ${time(note.timestamp)} UTC</small>${esc(note.text)}</div>`).join("")}</div></section><p class="sample-caption">Acknowledgement records a response; it does not resolve an active condition. Reloading resets all demo changes.</p></div>`;
}
function serviceDetail(service) {
  const stages = [
    { name: "DNS lookup", value: 12 },
    { name: "TCP connection", value: 18 },
    { name: "TLS handshake", value: 24 },
    { name: "HTTP response", value: service.latencyMs - 54 },
  ];
  return `${detailHeader(service.name, `${service.url} · sample check`)}<div class="detail-body"><div class="section-head">${status("healthy", "Reachable")}<span class="mono muted small">HTTP ${service.status}</span></div><div class="detail-stat"><div><div class="resource-value">${service.latencyMs}<span class="unit">ms</span></div><p>Total response time</p></div><p>${time(snapshot)} UTC</p></div><section class="detail-section"><h3>Where the time goes</h3><p class="detail-copy">Sequential sample stages, adding up to the total response time.</p><div class="host-chart" style="margin-top:18px">${stages.map((stage) => `<div class="host-row"><span class="small">${stage.name}</span><span class="rail" aria-hidden="true"><span class="bar" style="width:${(stage.value / service.latencyMs) * 100}%"></span></span><span class="rail-value">${stage.value} ms</span></div>`).join("")}</div></section><section class="detail-section"><h3>Certificate</h3><dl class="fact-list"><div><dt>Validity</dt><dd>${status("healthy", "Valid")}</dd></div><div><dt>Remaining at snapshot</dt><dd class="mono">${service.certificateDays} days</dd></div><div><dt>Common name</dt><dd>${service.url.replace("https://", "")}</dd></div><div><dt>Expires (UTC)</dt><dd class="mono">${new Date(Date.parse(snapshot) + service.certificateDays * 86400000).toISOString().slice(0, 10)}</dd></div></dl></section><section class="detail-section"><h3>Last 24 hours</h3><p class="detail-copy">All 25 hourly samples returned HTTP 200. These samples do not establish continuous availability.</p><details class="wide-section"><summary class="text-link">View individual checks</summary><div class="table-wrap"><table><caption>Fixed hourly observations</caption><thead><tr><th scope="col">Time (UTC)</th><th scope="col">HTTP</th><th scope="col">Response</th></tr></thead><tbody>${service.history.map((point) => `<tr><td class="mono">${point.timestamp.slice(0, 10)} ${time(point.timestamp)}</td><td class="mono">${point.status}</td><td class="mono">${point.latencyMs} ms</td></tr>`).join("")}</tbody></table></div></details></section></div>`;
}
function auditDetail(event) {
  return `${detailHeader(event.action, `${event.correlationId} · sample audit event`)}<div class="detail-body"><p class="detail-copy">${esc(event.description)}</p><section class="detail-section"><h3>Event record</h3><dl class="fact-list"><div><dt>Actor</dt><dd>${esc(event.actor)}</dd></div><div><dt>Outcome</dt><dd>${status("healthy", "Success")}</dd></div><div><dt>Target</dt><dd>${esc(event.target)}</dd></div><div><dt>Time (UTC)</dt><dd>${esc(event.timestamp.replace("T", " ").replace("Z", ""))}</dd></div><div><dt>Correlation ID</dt><dd class="mono">${event.correlationId}</dd></div><div><dt>Storage</dt><dd>Current demo session only</dd></div></dl></section></div>`;
}
function detailContent(id) {
  if (metricLabels[id]) return metricDetail(id);
  if (id === "balance") return balanceDetail();
  const host = hosts.find((host) => host.id === id);
  if (host) return hostDetail(host);
  const guestHost = hosts.find((host) => `guests-${host.id}` === id);
  if (guestHost) return guestsDetail(guestHost);
  const alert = state.alerts.find((alert) => alert.id === id);
  if (alert) return alertDetail(alert);
  const service = services.find((service) => service.id === id);
  if (service) return serviceDetail(service);
  const audit = state.audit.find((event) => event.id === id);
  if (audit) return auditDetail(audit);
  return `${detailHeader("Record unavailable", "This record is not included in the sample dataset.")}<div class="detail-body"><button class="button" data-action="close">Return to overview</button></div>`;
}
function blockedState() {
  const copy = {
    empty: [
      "No resources in this view",
      "Connect an integration to see host health and capacity here. This empty state is a preview; integration setup stays in the existing application.",
    ],
    denied: [
      "This view requires permission",
      "Your example role cannot read this section. The sample data is hidden until you return to the default preview.",
    ],
    unavailable: [
      "Core is unavailable",
      "Espial cannot read the monitoring snapshot. Resource values are withheld until a connection can be verified.",
    ],
    error: [
      "This view could not be loaded",
      "An unexpected error interrupted the read. Try the sample again to return to the overview.",
    ],
  }[state.scenario];
  return `${pageHeading(labels[route.page] || "Espial", "Design preview · simulated application state")}<section class="state-screen"><h2>${copy[0]}</h2><p>${copy[1]}</p><button class="button" data-action="retry">${state.scenario === "empty" ? "Load sample data" : "Retry sample view"}</button></section>`;
}
function render({ focus = false } = {}) {
  const oldRoute = route;
  route = readRoute();
  // Keep shared links and refreshes tied to the same cluster as the visible data.
  if (["dashboard", "hypervisor", "alerts"].includes(route.page)) {
    const requestedScope = route.params.get("scope");
    if (
      requestedScope &&
      clusters.some((cluster) => cluster.name === requestedScope)
    ) {
      state.scope = requestedScope;
    }
    if (state.scope !== "all") route.params.set("scope", state.scope);
    else route.params.delete("scope");
    const canonical = `#/${route.page}${route.child ? `/${route.child}` : ""}${route.params.size ? `?${route.params}` : ""}`;
    if (location.hash !== canonical) history.replaceState(null, "", canonical);
  }
  const wasDialogOpen = dialog.open;
  const hasBlockedState = ["empty", "denied", "unavailable", "error"].includes(
    state.scenario,
  );
  const renderPage = {
    dashboard,
    hypervisor,
    alerts: alertsPage,
    webpages,
    audit: auditPage,
    datacenter: () =>
      `${pageHeading("Datacenter", "A physical path from room to drive. Sample layout concept.")}${renderDatacenter(hosts, Object.fromEntries(route.params))}`,
  }[route.page];
  let body = hasBlockedState
    ? blockedState()
    : renderPage
      ? renderPage()
      : `${pageHeading("Page not found", "This route is not part of the design preview.")}<a class="text-link" href="#/dashboard">Return to Dashboard →</a>`;
  if (state.scenario === "stale" || state.scenario === "disconnected")
    body = `<div class="notice" role="status"><p>${state.scenario === "stale" ? "Stale sample: collection is overdue. All values below are last known, not current." : "Sample connection interrupted. Last-known data remains visible; updates are disconnected."}</p><button class="button" data-action="retry">Restore sample</button></div>${body}`;
  if (state.signedOut)
    body = `<section class="login-preview"><img src="./assets/ubnetdef.png" alt="UBNetDef"><h1 tabindex="-1">Espial design preview</h1><p>You have left the sample session. Explore infrastructure health, capacity, and hardware inspection with fictional data.</p><button class="button primary" data-action="signin">Enter demo</button></section>`;
  app.innerHTML = state.signedOut
    ? `<main class="content" id="main">${body}</main>`
    : shell(body);
  document.title = `${labels[route.page] || "Espial"} · Espial design preview`;
  const id =
    route.params.get("detail") ||
    (route.page === "hypervisor" ? route.params.get("host") : null);
  if (id && !hasBlockedState && !state.signedOut) {
    dialog.innerHTML = detailContent(id);
    if (!dialog.open) dialog.showModal();
    if (focus || !wasDialogOpen) dialog.querySelector("#detail-title").focus();
  } else {
    if (dialog.open) dialog.close();
    if (wasDialogOpen)
      (
        (returnFocus && document.querySelector(returnFocus)) ||
        app.querySelector("h1")
      )?.focus({ preventScroll: true });
    else if (focus && route.page === "datacenter") {
      const oldHref = `#/datacenter${oldRoute.params.size ? `?${oldRoute.params}` : ""}`;
      const parentTarget =
        oldRoute.page === "datacenter" &&
        oldRoute.params.size > route.params.size
          ? app.querySelector(`a[href="${CSS.escape(oldHref)}"]`)
          : null;
      const target =
        parentTarget ||
        (route.params.has("bay")
          ? app.querySelector("#dc-drive-heading")
          : route.params.has("host")
            ? app.querySelector("#dc-chassis-heading")
            : route.params.has("rack")
              ? app.querySelector(".dc-rack-information h2")
              : app.querySelector("h1"));
      if (target) {
        if (target.matches("h1, h2, h3")) target.tabIndex = -1;
        target.focus({ preventScroll: false });
      }
    } else if (focus && previousPage !== route.page)
      app.querySelector("h1")?.focus({ preventScroll: true });
  }
  if (previousPage && previousPage !== route.page) window.scrollTo(0, 0);
  previousPage = route.page;
  bindAccount();
}
function renderDetail() {
  const id = route.params.get("detail") || route.params.get("host");
  const active = document.activeElement;
  const action = active?.dataset.action;
  const value = active?.dataset.value;
  const scroll = dialog.scrollTop;
  dialog.innerHTML = detailContent(id);
  dialog.scrollTop = scroll;
  const target =
    action &&
    dialog.querySelector(
      `[data-action="${action}"]${value ? `[data-value="${value}"]` : ""}`,
    );
  if (target && !target.disabled) target.focus({ preventScroll: true });
  else dialog.querySelector("#detail-title").focus({ preventScroll: true });
}
function closeDetails() {
  const params = new URLSearchParams(route.params);
  params.delete("detail");
  if (route.page === "hypervisor") params.delete("host");
  location.hash = `#/${route.page}${route.child ? `/${route.child}` : ""}${params.size ? `?${params}` : ""}`;
}
function bindAccount() {
  const account = document.querySelector(".account");
  if (!account) return;
  account.addEventListener("pointerenter", (event) => {
    if (event.pointerType === "mouse") setAccount(true);
  });
  account.addEventListener("pointerleave", () => {
    if (!account.contains(document.activeElement)) setAccount(false);
  });
  account.addEventListener("focusin", (event) => {
    if (!suppressAccountFocus) {
      if (event.target.matches(".account-trigger"))
        event.target.dataset.justFocused = "true";
      setAccount(true);
    }
  });
  account.addEventListener("focusout", (event) => {
    if (!account.contains(event.relatedTarget)) setAccount(false);
  });
}
function setAccount(open) {
  const button = document.querySelector(".account-trigger");
  if (!button) return;
  button.setAttribute("aria-expanded", String(open));
  document.querySelector("#account-menu").hidden = !open;
}
function announce(message) {
  document.querySelector("#announcer").textContent = message;
}
function saveAction(alert, action, description) {
  sequence += 1;
  const correlationId = `DEMO-${String(1000 + sequence)}`;
  const timestamp = new Date(
    Date.parse(snapshot) + sequence * 60000,
  ).toISOString();
  state.audit.unshift({
    id: `audit-${sequence}`,
    timestamp,
    actor: "Demo operator",
    action,
    target: alert.hostId,
    outcome: "Success",
    correlationId,
    description,
  });
  alert.correlationId = correlationId;
  alert.lastAction = action;
  announce(`${action}. Demo change saved. Audit receipt ${correlationId}.`);
  render();
  dialog.querySelector(".receipt")?.scrollIntoView({ block: "nearest" });
  dialog.querySelector(".receipt a")?.focus({ preventScroll: true });
}

document.addEventListener("click", (event) => {
  const link = event.target.closest("a");
  if (link?.closest("#primary-nav")) {
    document.querySelector("#primary-nav").classList.remove("open");
    document
      .querySelector(".menu-toggle")
      ?.setAttribute("aria-expanded", "false");
    if (link.getAttribute("href") === location.hash)
      app.querySelector("h1")?.focus();
  }
  if (link?.getAttribute("href") === "#main") {
    event.preventDefault();
    app.querySelector("h1")?.focus();
    return;
  }
  if (link && !dialog.contains(link))
    returnFocus = `a[href="${CSS.escape(link.getAttribute("href"))}"]`;
  const button = event.target.closest("[data-action]");
  if (!button) {
    if (!event.target.closest(".account")) setAccount(false);
    return;
  }
  const action = button.dataset.action;
  if (action === "close") closeDetails();
  if (action === "menu") {
    const open = button.getAttribute("aria-expanded") !== "true";
    button.setAttribute("aria-expanded", String(open));
    document.querySelector("#primary-nav").classList.toggle("open", open);
  }
  if (action === "account") {
    const openedByFocus = button.dataset.justFocused === "true";
    button.dataset.justFocused = "false";
    setAccount(
      openedByFocus || button.getAttribute("aria-expanded") !== "true",
    );
  }
  if (action === "metric") {
    state.metric = button.dataset.value;
    render();
    document
      .querySelector(`[data-action="metric"][data-value="${state.metric}"]`)
      ?.focus({ preventScroll: true });
  }
  if (action === "table") {
    state.chartTable = !state.chartTable;
    render();
    document
      .querySelector('[data-action="table"]')
      ?.focus({ preventScroll: true });
  }
  if (action === "range") {
    state.range = button.dataset.value;
    renderDetail();
  }
  if (action === "detail-metric") {
    state.detailMetric = button.dataset.value;
    renderDetail();
  }
  if (action === "retry") {
    state.scenario = "normal";
    render({ focus: true });
    announce("Sample view restored.");
  }
  if (action === "signout" || action === "signin") {
    state.signedOut = action === "signout";
    render({ focus: true });
    app.querySelector("h1")?.focus();
  }
  if (action === "reset") {
    state.alerts = structuredClone(initialAlerts);
    state.audit = structuredClone(initialAudit);
    state.scenario = "normal";
    state.search = "";
    sequence = 0;
    render();
    announce("Sample data reset.");
    document.querySelector(".account-trigger")?.focus();
  }
  if (action === "acknowledge" || action === "assign") {
    const alert = state.alerts.find((item) => item.id === button.dataset.id);
    if (action === "acknowledge") {
      alert.acknowledgedBy = "Demo operator";
      alert.acknowledgedAt = snapshot;
      saveAction(
        alert,
        "Alert acknowledged",
        `Demo operator acknowledged ${alert.title}. The underlying warning remains active.`,
      );
    } else {
      alert.owner = "Demo operator";
      saveAction(
        alert,
        "Alert assigned",
        `${alert.title} assigned from Unassigned to Demo operator.`,
      );
    }
  }
});
document.addEventListener("change", (event) => {
  const element = event.target;
  if (element.id === "scope") {
    state.scope = element.value;
    if (state.scope === "all") route.params.delete("scope");
    else route.params.set("scope", state.scope);
    history.replaceState(
      null,
      "",
      `#/${route.page}${route.child ? `/${route.child}` : ""}${route.params.size ? `?${route.params}` : ""}`,
    );
    render();
    document.querySelector("#scope")?.focus();
    announce(
      `Showing ${state.scope === "all" ? "all clusters" : state.scope}.`,
    );
  }
  if (element.id === "alert-filter") {
    state.alertFilter = element.value;
    render();
    document.querySelector("#alert-filter")?.focus();
  }
  if (element.id === "scenario") {
    state.scenario = element.value;
    render();
    document.querySelector("#scenario")?.focus();
    announce(`Preview state: ${element.selectedOptions[0].textContent}.`);
  }
});
document.addEventListener("input", (event) => {
  if (event.target.id === "audit-search") {
    const start = event.target.selectionStart;
    state.search = event.target.value;
    // Starting a new search deliberately exits the exact-receipt view.
    if (route.params.has("receipt")) {
      route.params.delete("receipt");
      history.replaceState(
        null,
        "",
        `#/audit${route.params.size ? `?${route.params}` : ""}`,
      );
    }
    render();
    const input = document.querySelector("#audit-search");
    input.focus();
    input.setSelectionRange(start, start);
  }
  if (event.target.id === "chart-scrubber")
    updateChartReading(Number(event.target.value));
});
function updateChartReading(index) {
  const point = currentSeries[index];
  if (!point) return;
  const input = dialog.querySelector("#chart-scrubber");
  input.value = index;
  input.setAttribute(
    "aria-valuetext",
    `${time(point.timestamp)} UTC, ${fmt(point.value, 1)} percent`,
  );
  dialog.querySelector("#chart-reading").innerHTML =
    `${time(point.timestamp)} UTC <span class="muted">/</span> ${fmt(point.value, 1)}%`;
  const x = 42 + index * 21;
  const guide = dialog.querySelector("#chart-guide");
  guide.setAttribute("x1", x);
  guide.setAttribute("x2", x);
  const marker = dialog.querySelector("#chart-point");
  marker.setAttribute("cx", x);
  marker.setAttribute("cy", 180 - point.value * 1.5);
}
dialog.addEventListener("pointermove", (event) => {
  const svg = event.target.closest(".trend svg");
  if (!svg || event.pointerType === "touch") return;
  const bounds = svg.getBoundingClientRect();
  const index = Math.max(
    0,
    Math.min(
      24,
      Math.round(
        (((event.clientX - bounds.left) / bounds.width) * 568 - 42) / 21,
      ),
    ),
  );
  updateChartReading(index);
});
document.addEventListener("submit", (event) => {
  if (event.target.id !== "note-form") return;
  event.preventDefault();
  const text = new FormData(event.target).get("note").trim();
  if (!text) {
    event.target
      .querySelector("textarea")
      .setCustomValidity("Enter a note before saving.");
    event.target.querySelector("textarea").reportValidity();
    return;
  }
  const alert = state.alerts.find(
    (item) => item.id === event.target.dataset.id,
  );
  alert.notes ||= [];
  alert.notes.push({
    text,
    timestamp: new Date(
      Date.parse(snapshot) + (sequence + 1) * 60000,
    ).toISOString(),
  });
  saveAction(
    alert,
    "Investigation note added",
    `A note was attached to ${alert.title}: ${text}`,
  );
});
document.addEventListener("input", (event) => {
  if (event.target.id === "note") event.target.setCustomValidity("");
});
dialog.addEventListener("cancel", (event) => {
  event.preventDefault();
  closeDetails();
});
dialog.addEventListener("click", (event) => {
  if (event.target === dialog) {
    const rect = dialog.getBoundingClientRect();
    if (
      event.clientX < rect.left ||
      event.clientX > rect.right ||
      event.clientY < rect.top ||
      event.clientY > rect.bottom
    )
      closeDetails();
  }
});
document.addEventListener("keydown", (event) => {
  if (event.key === " " && event.target.matches('a[href^="#/"]')) {
    event.preventDefault();
    event.target.click();
  }
  if (event.key !== "Escape" || dialog.open) return;
  const account = document.querySelector(".account-trigger");
  if (account?.getAttribute("aria-expanded") === "true") {
    setAccount(false);
    suppressAccountFocus = true;
    account.focus();
    suppressAccountFocus = false;
    return;
  }
  const menu = document.querySelector(".menu-toggle");
  if (menu?.getAttribute("aria-expanded") === "true") {
    menu.click();
    menu.focus();
    return;
  }
  if (route.page === "datacenter" && route.params.size) {
    const params = new URLSearchParams(route.params);
    if (params.has("bay")) params.delete("bay");
    else if (params.has("host")) params.delete("host");
    else params.delete("rack");
    location.hash = `#/datacenter${params.size ? `?${params}` : ""}`;
  }
});
window.addEventListener("hashchange", () => render({ focus: true }));
render();
