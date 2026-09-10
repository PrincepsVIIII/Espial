import type { DisplayState } from '$lib/dashboard';

export interface HypervisorHostMetric {
  id: string;
  cpu_percent: number | null;
  ram_percent: number | null;
  cpu_cores: number | null;
  ram_bytes: number | null;
  swap_percent: number | null;
  swap_indicator: string;
  health_state: string;
}

export interface HypervisorLoadNode {
  id: string;
  idle_vms: number;
  console_connections: number;
}

export interface HypervisorStorageMetric {
  id: string;
  used_bytes: number | null;
  total_bytes: number | null;
  used_percent: number | null;
  io_wait_percent: number | null;
  io_wait_indicator: string;
  health_state: string;
}

export interface HostGraphicRow extends HypervisorHostMetric {
  kind: 'compute';
  idle_vms: number;
  console_connections: number;
}

export interface StorageGraphicRow extends HypervisorStorageMetric {
  kind: 'storage';
  idle_vms: number;
  console_connections: number;
}

export type LoadGraphicRow = HostGraphicRow | StorageGraphicRow;

export interface IntegerDomain {
  min: 0;
  max: number;
  ticks: number[];
}

export function finiteNumber(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

export function normalizePercentage(
  value: number | null | undefined,
): number | null {
  const finite = finiteNumber(value);
  if (finite === null) return null;
  return Math.min(100, Math.max(0, finite));
}

export function formatPercentage(value: number | null | undefined): string {
  const normalized = normalizePercentage(value);
  return normalized === null ? 'Unknown' : `${Math.round(normalized)}%`;
}

export function formatExactPercentage(
  value: number | null | undefined,
): string {
  const normalized = normalizePercentage(value);
  if (normalized === null) return 'Unknown';
  return `${normalized.toFixed(2).replace(/\.?(?:0+)$/, '')}%`;
}

export function formatBytes(value: number | null | undefined): string {
  const finite = finiteNumber(value);
  if (finite === null || finite < 0) return 'Unknown';
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  let amount = finite;
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024;
    unit += 1;
  }
  return `${amount >= 10 ? amount.toFixed(0) : amount.toFixed(1)} ${units[unit]}`;
}

export function integerDomain(values: readonly number[]): IntegerDomain {
  const highest = values.reduce((maximum, value) => {
    if (!Number.isFinite(value)) return maximum;
    return Math.max(maximum, Math.max(0, Math.floor(value)));
  }, 0);
  const max = Math.max(1, highest);
  if (max <= 4) {
    return {
      min: 0,
      max,
      ticks: Array.from({ length: max + 1 }, (_, index) => index),
    };
  }
  const step = Math.ceil(max / 4);
  const ticks = Array.from(
    { length: Math.floor(max / step) + 1 },
    (_, index) => index * step,
  );
  if (ticks.at(-1) !== max) ticks.push(max);
  return { min: 0, max, ticks };
}

export function positionInDomain(value: number, domain: IntegerDomain): number {
  const finite = Number.isFinite(value) ? value : 0;
  return (Math.min(domain.max, Math.max(0, finite)) / domain.max) * 100;
}

export function hostGraphicRows(
  hosts: readonly HypervisorHostMetric[],
  loads: readonly HypervisorLoadNode[],
): HostGraphicRow[] {
  const hostsByID = new Map(hosts.map((host) => [host.id, host]));
  const loadsByID = new Map(
    loads.map((load) => [
      load.id,
      {
        idle_vms: nonnegativeCount(load.idle_vms),
        console_connections: nonnegativeCount(load.console_connections),
      },
    ]),
  );
  const ids = new Set([...hostsByID.keys(), ...loadsByID.keys()]);
  return [...ids]
    .sort((left, right) => left.localeCompare(right))
    .map((id) => {
      const host = hostsByID.get(id);
      return {
        kind: 'compute' as const,
        id,
        idle_vms: loadsByID.get(id)?.idle_vms ?? 0,
        console_connections: loadsByID.get(id)?.console_connections ?? 0,
        cpu_percent: finiteNumber(host?.cpu_percent),
        ram_percent: finiteNumber(host?.ram_percent),
        cpu_cores: finiteNumber(host?.cpu_cores),
        ram_bytes: finiteNumber(host?.ram_bytes),
        swap_percent: finiteNumber(host?.swap_percent),
        swap_indicator: host?.swap_indicator ?? 'unknown',
        health_state: host?.health_state ?? 'unknown',
      };
    });
}

export function storageGraphicRows(
  storage: readonly HypervisorStorageMetric[],
  loads: readonly HypervisorLoadNode[],
): StorageGraphicRow[] {
  const storageByID = new Map(storage.map((item) => [item.id, item]));
  const loadsByID = new Map(
    loads.map((load) => [
      load.id,
      {
        idle_vms: nonnegativeCount(load.idle_vms),
        console_connections: nonnegativeCount(load.console_connections),
      },
    ]),
  );
  const ids = new Set([...storageByID.keys(), ...loadsByID.keys()]);
  return [...ids]
    .sort((left, right) => left.localeCompare(right))
    .map((id) => {
      const item = storageByID.get(id);
      return {
        kind: 'storage' as const,
        id,
        idle_vms: loadsByID.get(id)?.idle_vms ?? 0,
        console_connections: loadsByID.get(id)?.console_connections ?? 0,
        used_bytes: finiteNumber(item?.used_bytes),
        total_bytes: finiteNumber(item?.total_bytes),
        used_percent: finiteNumber(item?.used_percent),
        io_wait_percent: finiteNumber(item?.io_wait_percent),
        io_wait_indicator: item?.io_wait_indicator ?? 'unknown',
        health_state: item?.health_state ?? 'unknown',
      };
    });
}

export function selectionAfterUpdate(
  selectedID: string | null,
  rows: readonly Pick<HostGraphicRow, 'id'>[],
): { selectedID: string | null; removedID: string | null } {
  if (!selectedID || rows.some((row) => row.id === selectedID)) {
    return { selectedID, removedID: null };
  }
  return { selectedID: null, removedID: selectedID };
}

export function displayState(value: string): DisplayState {
  if (
    value === 'healthy' ||
    value === 'warning' ||
    value === 'critical' ||
    value === 'unknown' ||
    value === 'stale' ||
    value === 'maintenance' ||
    value === 'disabled'
  ) {
    return value;
  }
  return 'unknown';
}

export function statusLabel(value: string): string {
  const state = displayState(value);
  return state.charAt(0).toUpperCase() + state.slice(1);
}

function nonnegativeCount(value: number): number {
  return Number.isFinite(value) ? Math.max(0, Math.floor(value)) : 0;
}
