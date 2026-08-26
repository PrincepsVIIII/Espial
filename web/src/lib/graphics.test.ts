import { describe, expect, it } from 'vitest';
import {
  formatBytes,
  formatPercentage,
  hostGraphicRows,
  integerDomain,
  normalizePercentage,
  positionInDomain,
  selectionAfterUpdate,
} from './graphics';

describe('graphic value normalization', () => {
  it('clamps percentages without turning unknown values into zero', () => {
    expect(normalizePercentage(-4)).toBe(0);
    expect(normalizePercentage(42.5)).toBe(42.5);
    expect(normalizePercentage(140)).toBe(100);
    expect(normalizePercentage(Number.NaN)).toBeNull();
    expect(formatPercentage(Number.POSITIVE_INFINITY)).toBe('Unknown');
    expect(formatBytes(Number.NaN)).toBe('Unknown');
    expect(formatBytes(0)).toBe('0.0 B');
  });

  it('builds bounded integer domains for zero, all-equal, and outlier data', () => {
    expect(integerDomain([])).toEqual({ min: 0, max: 1, ticks: [0, 1] });
    expect(integerDomain([0, 0])).toEqual({
      min: 0,
      max: 1,
      ticks: [0, 1],
    });
    expect(integerDomain([3, 3])).toEqual({
      min: 0,
      max: 3,
      ticks: [0, 1, 2, 3],
    });
    const outlier = integerDomain([1, 2, 128]);
    expect(outlier.ticks.length).toBeLessThanOrEqual(5);
    expect(outlier.ticks.at(-1)).toBe(128);
    expect(positionInDomain(64, outlier)).toBe(50);
  });
});

describe('Hypervisor host graphic data', () => {
  const hosts = [
    {
      id: 'pve-b',
      cpu_percent: 20,
      ram_percent: 30,
      cpu_cores: 8,
      ram_bytes: 1024,
      swap_percent: 0,
      swap_indicator: 'healthy',
      health_state: 'healthy',
    },
    {
      id: 'pve-a',
      cpu_percent: 60,
      ram_percent: 70,
      cpu_cores: 16,
      ram_bytes: 2048,
      swap_percent: 12,
      swap_indicator: 'warning',
      health_state: 'warning',
    },
  ];

  it('joins metrics and counts with stable ordering and explicit zero load', () => {
    expect(
      hostGraphicRows(hosts, [{ id: 'pve-a', active_vms: 3 }]).map(
        ({ id, active_vms }) => ({ id, active_vms }),
      ),
    ).toEqual([
      { id: 'pve-a', active_vms: 3 },
      { id: 'pve-b', active_vms: 0 },
    ]);
  });

  it('keeps load-only hosts visible with unknown compute metrics', () => {
    const rows = hostGraphicRows(hosts, [
      { id: 'pve-c', active_vms: 2 },
      { id: 'pve-a', active_vms: Number.NaN },
    ]);
    expect(rows.map((row) => row.id)).toEqual(['pve-a', 'pve-b', 'pve-c']);
    expect(rows[0].active_vms).toBe(0);
    expect(rows[2].cpu_percent).toBeNull();
    expect(rows[2].health_state).toBe('unknown');
  });

  it('preserves selection while a host exists and reports removal', () => {
    const rows = hostGraphicRows(hosts, []);
    expect(selectionAfterUpdate('pve-a', rows)).toEqual({
      selectedID: 'pve-a',
      removedID: null,
    });
    expect(selectionAfterUpdate('pve-c', rows)).toEqual({
      selectedID: null,
      removedID: 'pve-c',
    });
  });
});
