/**
 * Fictional, fixed fixtures for the standalone Espial visual prototype.
 * These records never describe a connected environment. All times are UTC.
 * Percentages use a 0–100 domain; GiB and TiB are binary capacity units.
 */
export const snapshot = "2026-09-10T14:32:00Z";

/** @typedef {'healthy' | 'warning'} HostState */
/**
 * @typedef {Object} Host
 * @property {string} id Stable ID, also the displayed host name.
 * @property {string} name
 * @property {string} cluster Display name, matching clusters[].name.
 * @property {number} cpuCores
 * @property {number} cpuPercent
 * @property {number} memoryGiB Total physical memory.
 * @property {number} memoryUsedGiB
 * @property {number} memoryPercent
 * @property {number} storageTiB Total capacity of the primary storage target.
 * @property {number} storageUsedTiB
 * @property {number} storagePercent
 * @property {number} guests Number of running guests.
 * @property {HostState} state
 * @property {string} rack Physical rack identifier.
 * @property {string} observedAt ISO-8601 observation time.
 */

/** @type {Host[]} */
export const hosts = [
  {
    id: "pve-01",
    name: "pve-01",
    cluster: "Harriman",
    cpuCores: 32,
    cpuPercent: 24,
    memoryGiB: 128,
    memoryUsedGiB: 66.56,
    memoryPercent: 52,
    storageTiB: 8,
    storageUsedTiB: 4.96,
    storagePercent: 62,
    guests: 5,
    state: "healthy",
    rack: "A01",
    observedAt: snapshot,
  },
  {
    id: "pve-02",
    name: "pve-02",
    cluster: "Harriman",
    cpuCores: 32,
    cpuPercent: 38,
    memoryGiB: 128,
    memoryUsedGiB: 78.08,
    memoryPercent: 61,
    storageTiB: 8,
    storageUsedTiB: 5.44,
    storagePercent: 68,
    guests: 6,
    state: "healthy",
    rack: "A01",
    observedAt: snapshot,
  },
  {
    id: "pve-03",
    name: "pve-03",
    cluster: "Harriman",
    cpuCores: 32,
    cpuPercent: 78,
    memoryGiB: 128,
    memoryUsedGiB: 107.52,
    memoryPercent: 84,
    storageTiB: 8,
    storageUsedTiB: 6.88,
    storagePercent: 86,
    guests: 9,
    state: "warning",
    rack: "A02",
    observedAt: snapshot,
  },
  {
    id: "pve-04",
    name: "pve-04",
    cluster: "Harriman",
    cpuCores: 32,
    cpuPercent: 31,
    memoryGiB: 128,
    memoryUsedGiB: 75.52,
    memoryPercent: 59,
    storageTiB: 8,
    storageUsedTiB: 4.72,
    storagePercent: 59,
    guests: 6,
    state: "healthy",
    rack: "A02",
    observedAt: snapshot,
  },
  {
    id: "pve-05",
    name: "pve-05",
    cluster: "Research",
    cpuCores: 32,
    cpuPercent: 22,
    memoryGiB: 128,
    memoryUsedGiB: 61.44,
    memoryPercent: 48,
    storageTiB: 8,
    storageUsedTiB: 4.32,
    storagePercent: 54,
    guests: 4,
    state: "healthy",
    rack: "B01",
    observedAt: snapshot,
  },
  {
    id: "pve-06",
    name: "pve-06",
    cluster: "Research",
    cpuCores: 32,
    cpuPercent: 35,
    memoryGiB: 128,
    memoryUsedGiB: 87.04,
    memoryPercent: 68,
    storageTiB: 8,
    storageUsedTiB: 5.6,
    storagePercent: 70,
    guests: 6,
    state: "healthy",
    rack: "B01",
    observedAt: snapshot,
  },
];

export const clusters = [
  {
    id: "harriman",
    name: "Harriman",
    hostIds: ["pve-01", "pve-02", "pve-03", "pve-04"],
  },
  { id: "research", name: "Research", hostIds: ["pve-05", "pve-06"] },
];

const round = (value, digits = 2) => Number(value.toFixed(digits));
const snapshotMs = Date.parse(snapshot);

/**
 * Capacity-weighted totals; an empty collection has null percentages, not 0%.
 * cpuUsedCores is a utilization equivalent, not an allocation or guest count.
 * @param {Host[]} hostList
 */
export function summarize(hostList) {
  const totals = hostList.reduce(
    (result, host) => {
      result.cpuCores += host.cpuCores;
      result.cpuUsedCores += (host.cpuCores * host.cpuPercent) / 100;
      result.memoryGiB += host.memoryGiB;
      result.memoryUsedGiB += host.memoryUsedGiB;
      result.storageTiB += host.storageTiB;
      result.storageUsedTiB += host.storageUsedTiB;
      result.guests += host.guests;
      result.healthyCount += Number(host.state === "healthy");
      result.warningCount += Number(host.state === "warning");
      return result;
    },
    {
      hostCount: hostList.length,
      cpuCores: 0,
      cpuUsedCores: 0,
      memoryGiB: 0,
      memoryUsedGiB: 0,
      storageTiB: 0,
      storageUsedTiB: 0,
      guests: 0,
      healthyCount: 0,
      warningCount: 0,
    },
  );

  return {
    ...totals,
    cpuUsedCores: round(totals.cpuUsedCores),
    memoryUsedGiB: round(totals.memoryUsedGiB),
    storageUsedTiB: round(totals.storageUsedTiB),
    cpuPercent: totals.cpuCores
      ? round((totals.cpuUsedCores / totals.cpuCores) * 100)
      : null,
    memoryPercent: totals.memoryGiB
      ? round((totals.memoryUsedGiB / totals.memoryGiB) * 100)
      : null,
    storagePercent: totals.storageTiB
      ? round((totals.storageUsedTiB / totals.storageTiB) * 100)
      : null,
  };
}

/**
 * Generate a repeatable fictional trend, including the exact snapshot reading.
 * Aggregates are capacity weighted from the same per-host samples, so inspecting
 * an individual host and the fleet never produces contradictory observations.
 * The three ranges share an identical absolute-time curve where they overlap.
 * @param {'cpu'|'memory'|'storage'} metric
 * @param {'1h'|'6h'|'24h'} range
 * @param {string} [hostId] Omit for all six hosts.
 * @returns {{timestamp: string, value: number}[]} 25 uniformly spaced points.
 */
export function metricHistory(metric, range, hostId) {
  const field = {
    cpu: "cpuPercent",
    memory: "memoryPercent",
    storage: "storagePercent",
  }[metric];
  const capacityField = {
    cpu: "cpuCores",
    memory: "memoryGiB",
    storage: "storageTiB",
  }[metric];
  const hours = { "1h": 1, "6h": 6, "24h": 24 }[range];
  if (!field || !hours)
    throw new RangeError(
      "Use cpu, memory, or storage with a 1h, 6h, or 24h range.",
    );
  const selectedHosts = hostId
    ? hosts.filter((host) => host.id === hostId)
    : hosts;
  if (!selectedHosts.length)
    throw new RangeError(`Unknown sample host: ${hostId}`);

  return Array.from({ length: 25 }, (_, index) => {
    const hoursAgo = (hours * (24 - index)) / 24;
    let weightedValue = 0;
    let capacity = 0;
    for (const host of selectedHosts) {
      const hostIndex = hosts.findIndex(
        (candidate) => candidate.id === host.id,
      );
      const phase = hostIndex * 0.91;
      let value = host[field];
      if (metric === "storage") {
        value -=
          hoursAgo * (host.id === "pve-03" ? 0.19 : 0.035 + hostIndex * 0.008);
      } else {
        const amplitude = metric === "cpu" ? 6.5 : 2.2;
        value +=
          amplitude * (Math.sin(hoursAgo * 1.7 + phase) - Math.sin(phase));
        value +=
          amplitude *
          0.35 *
          (Math.sin(hoursAgo * 5.1 + phase) - Math.sin(phase));
        if (host.id === "pve-03")
          value -= Math.min(hoursAgo, 9) * (metric === "cpu" ? 2.7 : 0.9);
      }
      value = Math.min(100, Math.max(0, value));
      weightedValue += value * host[capacityField];
      capacity += host[capacityField];
    }
    return {
      timestamp: new Date(snapshotMs - hoursAgo * 3_600_000).toISOString(),
      value: round(weightedValue / capacity),
    };
  });
}

/** Each point is a successful hourly sample over the displayed 24-hour window. */
function checkHistory(baseLatency, phase) {
  return Array.from({ length: 25 }, (_, index) => ({
    timestamp: new Date(snapshotMs - (24 - index) * 3_600_000).toISOString(),
    state: "healthy",
    status: 200,
    latencyMs:
      index === 24
        ? baseLatency
        : Math.round(
            baseLatency +
              Math.sin(index * 1.37 + phase) * 12 +
              Math.cos(index * 0.7 + phase) * 7,
          ),
  }));
}

/**
 * Reserved example.com URLs are display-only; the mockup does not poll them.
 * availabilityPercent describes the supplied successful samples, not an SLA.
 */
export const services = [
  {
    id: "site-main",
    name: "Public website",
    url: "https://www.example.com",
    state: "healthy",
    status: 200,
    latencyMs: 142,
    availabilityPercent: 100,
    certificateDays: 67,
    observedAt: snapshot,
    history: checkHistory(142, 0),
  },
  {
    id: "site-docs",
    name: "Documentation",
    url: "https://docs.example.com",
    state: "healthy",
    status: 200,
    latencyMs: 86,
    availabilityPercent: 100,
    certificateDays: 42,
    observedAt: snapshot,
    history: checkHistory(86, 1),
  },
  {
    id: "site-status",
    name: "Status page",
    url: "https://status.example.com",
    state: "healthy",
    status: 200,
    latencyMs: 113,
    availabilityPercent: 100,
    certificateDays: 89,
    observedAt: snapshot,
    history: checkHistory(113, 2),
  },
];

/** Mutations are local demo state only. UI acknowledgements should add an audit receipt. */
export const initialAlerts = [
  {
    id: "alert-storage-03",
    hostId: "pve-03",
    severity: "warning",
    status: "active",
    title: "Storage approaching capacity",
    description:
      "pve-03 primary storage is 86% full: 6.88 of 8 TiB used. The sample warning threshold is 85%.",
    metric: "storage",
    value: 86,
    threshold: 85,
    unit: "%",
    startedAt: "2026-09-10T13:52:00Z",
    observedAt: snapshot,
    acknowledgedAt: null,
    acknowledgedBy: null,
    correlationId: null,
  },
  {
    id: "alert-cpu-03",
    hostId: "pve-03",
    severity: "warning",
    status: "active",
    title: "Uneven compute load",
    description:
      "pve-03 is using 78% CPU while the Harriman cluster averages 42.75%. Review placement across its four hosts.",
    metric: "cpu",
    value: 78,
    clusterAverage: 42.75,
    unit: "%",
    startedAt: "2026-09-10T14:18:00Z",
    observedAt: snapshot,
    acknowledgedAt: null,
    acknowledgedBy: null,
    correlationId: null,
  },
];

/** Explicit sample events: no production actor, token, or authoritative audit log. */
export const initialAudit = [
  {
    id: "audit-sample-003",
    timestamp: "2026-09-10T14:24:00Z",
    actor: "Demo operator",
    action: "Sample session opened",
    target: "Espial mockup",
    outcome: "Success",
    correlationId: "DEMO-0924",
    description:
      "A fictional operator session for exploring this front-end prototype.",
  },
  {
    id: "audit-sample-002",
    timestamp: "2026-09-10T13:46:00Z",
    actor: "Demo operator",
    action: "Sample alert acknowledged",
    target: "pve-02 · previous CPU warning",
    outcome: "Success",
    correlationId: "DEMO-0846",
    description:
      "Historical example acknowledgement; the earlier CPU warning is resolved and is not one of the current active alerts.",
  },
  {
    id: "audit-sample-001",
    timestamp: "2026-09-10T12:10:00Z",
    actor: "Demo administrator",
    action: "Sample check enabled",
    target: "Documentation",
    outcome: "Success",
    correlationId: "DEMO-0710",
    description:
      "Example audit event showing the intended receipt format. No actual monitoring configuration was changed.",
  },
];
