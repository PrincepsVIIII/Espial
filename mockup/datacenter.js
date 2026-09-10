const escape = (value) =>
  String(value ?? "").replace(
    /[&<>"']/g,
    (char) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        char
      ],
  );
const rackIds = ["A01", "A02", "B01"];
const number = (value, digits = 0) =>
  Number.isFinite(Number(value))
    ? Number(value).toLocaleString("en-US", { maximumFractionDigits: digits })
    : "Unknown";
const route = (rack, host, bay) => {
  const query = new URLSearchParams();
  if (rack) query.set("rack", rack);
  if (host) query.set("host", host);
  if (bay) query.set("bay", bay);
  return `#/datacenter${query.size ? `?${query}` : ""}`;
};
const isWarning = (host) =>
  host &&
  (host.name === "pve-03" ||
    Number(host.storagePercent) >= 85 ||
    String(host.state).toLowerCase() === "warning");
const hostState = (host) => (isWarning(host) ? "Capacity warning" : "Healthy");
const rackHosts = (hosts, rack) => hosts.filter((host) => host.rack === rack);
const location = (index) => (index === 0 ? "U09–10" : "U05–06");

function breadcrumbs(rack, host, bay) {
  return `<nav class="dc-breadcrumbs" aria-label="Datacenter location"><a href="#/datacenter" ${!rack ? 'aria-current="page"' : ""}>Room 01</a>${rack ? `<span aria-hidden="true">/</span><a href="${route(rack)}" ${!host ? 'aria-current="page"' : ""}>Rack ${escape(rack)}</a>` : ""}${host ? `<span aria-hidden="true">/</span><a href="${route(rack, host.id)}" ${!bay ? 'aria-current="page"' : ""}>${escape(host.name)}</a>` : ""}${bay ? `<span aria-hidden="true">/</span><span aria-current="page">Bay ${bay}</span>` : ""}</nav>`;
}

function rackFront(hosts, rack, selectedHost = null, detail = false) {
  const members = rackHosts(hosts, rack);
  const warning = members.some(isWarning);
  const fronts = Array.from({ length: 12 }, (_, index) => {
    const unit = 12 - index;
    const hostIndex = unit === 10 ? 0 : unit === 6 ? 1 : -1;
    const host = members[hostIndex];
    if ((unit === 9 && members[0]) || (unit === 5 && members[1])) return "";
    return `<span class="dc-u-label ${host ? "dc-u-double" : ""}">${String(unit).padStart(2, "0")}${host ? `<span>${String(unit - 1).padStart(2, "0")}</span>` : ""}</span>${host ? `<a class="dc-server ${selectedHost === host.id ? "dc-server-selected" : ""} ${isWarning(host) ? "dc-server-warning" : ""}" href="${route(rack, host.id)}" aria-label="Inspect ${escape(host.name)}, rack ${escape(rack)}, ${location(hostIndex)}, ${hostState(host)}" ${selectedHost === host.id ? 'aria-current="page"' : ""}><span class="dc-server-main"><strong>${escape(host.name)}</strong><span class="dc-server-state">${isWarning(host) ? "Capacity" : "Healthy"}</span></span><span class="dc-server-slats" aria-hidden="true"><i></i><i></i><i></i><i></i></span></a>` : `<div class="dc-blank" aria-hidden="true"><span></span></div>`}`;
  }).join("");
  return `<article class="dc-rack ${detail ? "dc-rack-focused" : ""}" aria-label="Rack ${escape(rack)}"><div class="dc-rack-header"><a href="${route(rack)}" class="dc-rack-title">${escape(rack)}<span aria-hidden="true">↗</span></a><span>${members.length} hosts · 12U</span></div><div class="dc-rack-frame"><div class="dc-rack-inner">${fronts}</div></div><div class="dc-rack-footer"><span class="${warning ? "dc-warning" : "dc-healthy"}">${warning ? "1 capacity warning" : "All hosts healthy"}</span><span>Row ${rack[0]}</span></div></article>`;
}

function inventory(hosts, { rack = null, selected = null } = {}) {
  const visible = rack ? rackHosts(hosts, rack) : hosts;
  return `<div class="dc-inventory"><div class="section-head"><h2>Host inventory</h2><span class="muted">${visible.length} hosts</span></div><div class="dc-table-wrap"><table class="dc-table"><caption>Sample host locations and observed resource usage. Select a host to inspect its chassis.</caption><thead><tr><th scope="col">Host / location</th><th scope="col">CPU</th><th scope="col">RAM</th><th scope="col">Storage</th><th scope="col">State</th><th scope="col"><span class="dc-visually-hidden">Inspect</span></th></tr></thead><tbody>${visible.map((host) => `<tr ${selected === host.id ? 'class="dc-row-selected"' : ""}><th scope="row"><a class="dc-host-link" href="${route(host.rack, host.id)}">${escape(host.name)}</a><span class="dc-cell-secondary">${escape(host.rack)} · ${location(rackHosts(hosts, host.rack).indexOf(host))}</span></th><td class="dc-number">${number(host.cpuPercent)}%</td><td class="dc-number">${number(host.memoryPercent)}%</td><td class="dc-number ${isWarning(host) ? "dc-warning" : ""}">${number(host.storagePercent)}%</td><td><span class="${isWarning(host) ? "dc-warning" : "dc-healthy"}">${hostState(host)}</span></td><td><a class="dc-table-action" href="${route(host.rack, host.id)}" aria-label="Inspect ${escape(host.name)}"><span aria-hidden="true">↗</span></a></td></tr>`).join("")}</tbody></table></div></div>`;
}

function facts(items) {
  return `<dl class="dc-facts">${items.map(([label, value]) => `<div><dt>${label}</dt><dd>${value}</dd></div>`).join("")}</dl>`;
}

function hostChassis(host, rack, bay) {
  return `<section class="dc-chassis-section" aria-labelledby="dc-chassis-heading"><div class="section-head"><h2 id="dc-chassis-heading">${escape(host.name)} chassis</h2><span class="dc-face-label">Front · sample layout</span></div><p class="dc-description">Select a drive bay to inspect its sample health and capacity.</p><div class="dc-chassis"><div class="dc-chassis-handle" aria-hidden="true"></div><div class="dc-drive-grid">${[1, 2, 3, 4].map((slot) => `<a class="dc-drive ${bay === slot ? "dc-drive-selected" : ""}" href="${route(rack, host.id, slot)}" ${bay === slot ? 'aria-current="page"' : ""} aria-label="Inspect bay ${slot}, online, 2 TiB sample drive"><span class="dc-drive-top"><strong>0${slot}</strong><span class="dc-drive-state">Online</span></span><span class="dc-drive-capacity">2 <span>TiB</span></span><span class="dc-drive-slats" aria-hidden="true"><i></i><i></i><i></i></span><span class="dc-drive-bottom">SSD · SATA<span aria-hidden="true">↗</span></span></a>`).join("")}</div><div class="dc-chassis-handle" aria-hidden="true"></div></div><div class="dc-chassis-meta"><span>Generic 2U chassis · 4 populated bays</span><span>Power on · All drives online</span></div>${isWarning(host) ? '<p class="dc-inline-warning">Storage is approaching capacity. All four sample drives are online; no drive failure is reported.</p>' : ""}<div class="dc-drive-index" aria-label="Drive list">${[1, 2, 3, 4].map((slot) => `<a href="${route(rack, host.id, slot)}" ${bay === slot ? 'aria-current="page"' : ""}>Bay ${slot}<span>Online · 2 TiB</span></a>`).join("")}</div></section>`;
}

function driveDetail(host, rack, bay) {
  const used = Number(host.storageUsedTiB) / 4;
  return `<section class="dc-drive-detail" aria-labelledby="dc-drive-heading"><div class="section-head"><h2 id="dc-drive-heading">Drive · bay ${bay}</h2><a class="text-link" href="${route(rack, host.id)}">Close details</a></div><p class="dc-description">${escape(host.name)} / front enclosure / slot 0${bay}. Illustrative drive data.</p>${facts(
    [
      ["State", '<span class="dc-healthy">Online</span>'],
      ["Capacity", "2 TiB"],
      ["Media / protocol", "SSD / SATA"],
      ["Sample temperature", `${32 + bay} °C`],
      ["Sample lifetime remaining", `${97 - bay}%`],
      ["Sample power-on hours", number(8210 + bay * 64)],
      ["Sample allocated space", `${number(used, 2)} / 2 TiB`],
      ["Drive identity", `SAMPLE-${escape(host.name)}-0${bay}`],
    ],
  )}<p class="dc-detail-note">This is a visual inspection. Selecting a bay does not eject or operate hardware.</p></section>`;
}

export function renderDatacenter(
  hosts,
  { rack = null, host: hostId = null, bay = null } = {},
) {
  const selectedHost = hosts.find(
    (entry) => entry.id === hostId || entry.name === hostId,
  );
  const selectedRack =
    selectedHost?.rack || (rackIds.includes(rack) ? rack : null);
  const selectedBay =
    selectedHost && [1, 2, 3, 4].includes(Number(bay)) ? Number(bay) : null;
  const warningCount = hosts.filter(isWarning).length;
  const prefix = `${breadcrumbs(selectedRack, selectedHost, selectedBay)}<div class="dc-context"><p>Room 01 · UBNetDef lab</p><span>Sample physical layout · hardcoded data</span></div>`;
  if (!selectedRack) {
    return `<div class="dc-view">${prefix}<section class="dc-room" aria-labelledby="dc-room-heading"><div class="section-head"><h2 id="dc-room-heading">Rack overview</h2><span class="muted">3 racks · ${hosts.length} hosts · ${warningCount} capacity warning${warningCount === 1 ? "" : "s"}</span></div><p class="dc-description">Select a rack or go directly to a host. Capacity and hardware health are shown separately.</p><div class="dc-room-racks">${rackIds.map((id) => rackFront(hosts, id)).join("")}</div><div class="dc-room-index" aria-label="Rack list">${rackIds.map((id) => `<a href="${route(id)}">Rack ${id}<span>${rackHosts(hosts, id).length} hosts · ${rackHosts(hosts, id).some(isWarning) ? "Capacity warning" : "Healthy"}<span aria-hidden="true">↗</span></span></a>`).join("")}</div></section>${inventory(hosts)}</div>`;
  }
  if (!selectedHost) {
    const members = rackHosts(hosts, selectedRack);
    return `<div class="dc-view">${prefix}<div class="dc-rack-inspection"><div class="dc-rack-context">${rackFront(hosts, selectedRack, null, true)}<a class="dc-back text-link" href="#/datacenter">← Back to room</a></div><div class="dc-rack-information"><div class="section-head"><h2>Rack ${escape(selectedRack)}</h2><span class="muted">Front face</span></div><p class="dc-description">A straight-on inspection of the sample rack. Select a host to open its chassis.</p>${facts(
      [
        ["Location", `Room 01 / row ${selectedRack[0]}`],
        ["Occupied", `${members.length * 2} of 12 U`],
        [
          "Compute",
          `${number(members.reduce((sum, h) => sum + Number(h.cpuCores), 0))} CPU cores`,
        ],
        [
          "Memory",
          `${number(members.reduce((sum, h) => sum + Number(h.memoryGiB), 0))} GiB`,
        ],
      ],
    )}${inventory(hosts, { rack: selectedRack })}</div></div></div>`;
  }
  return `<div class="dc-view">${prefix}<div class="dc-host-inspection"><aside class="dc-host-context" aria-label="Host location"><h2>Rack ${escape(selectedRack)}</h2><p class="dc-description">${location(rackHosts(hosts, selectedRack).indexOf(selectedHost))} · Front face</p>${rackFront(hosts, selectedRack, selectedHost.id, true)}<a class="dc-back text-link" href="${route(selectedRack)}">← Back to rack</a></aside><div class="dc-host-content">${hostChassis(selectedHost, selectedRack, selectedBay)}${
    selectedBay
      ? driveDetail(selectedHost, selectedRack, selectedBay)
      : `<section class="dc-host-summary" aria-labelledby="dc-host-summary-heading"><div class="section-head"><h2 id="dc-host-summary-heading">Host resources</h2><span class="${isWarning(selectedHost) ? "dc-warning" : "dc-healthy"}">${hostState(selectedHost)}</span></div>${facts(
          [
            [
              "CPU",
              `${number(selectedHost.cpuPercent)}% / ${number(selectedHost.cpuCores)} cores`,
            ],
            [
              "Memory",
              `${number(selectedHost.memoryUsedGiB)} / ${number(selectedHost.memoryGiB)} GiB`,
            ],
            [
              "Storage",
              `${number(selectedHost.storageUsedTiB, 2)} / ${number(selectedHost.storageTiB, 2)} TiB`,
            ],
            ["Running guests", number(selectedHost.guests)],
          ],
        )}<a class="dc-hypervisor-link text-link" href="#/hypervisor?host=${encodeURIComponent(selectedHost.id)}">View ${escape(selectedHost.name)} in Hypervisor <span aria-hidden="true">↗</span></a></section>`
  }</div></div></div>`;
}
