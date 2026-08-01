import { describe, expect, it } from 'vitest';
import {
  dashboardState,
  dashboardURL,
  filtersFromURL,
  relativeTime,
  resourceQuery,
  statusPresentation,
  totalResources,
} from './dashboard';
import type { MonitoringOverview } from './api/generated';

const overview: MonitoringOverview = {
  generated_at: '2026-07-31T12:00:00Z',
  resource_counts: [
    { state: 'healthy', count: 4 },
    { state: 'stale', count: 1 },
  ],
  integration_counts: [{ state: 'healthy', count: 1 }],
  stale_count: 1,
  unknown_count: 0,
  certificate_warnings: { warning: 0, critical: 0, unknown: 0 },
  recent_state_changes: [],
};

describe('Dashboard query contract', () => {
  it('normalizes accepted URL filters and keeps them in API and page queries', () => {
    const filters = filtersFromURL(
      new URLSearchParams(
        'state=stale&kind=server&integration=60000000-0000-4000-8000-000000000001&stale=true',
      ),
    );
    expect(resourceQuery(filters)).toContain('state=stale');
    expect(resourceQuery(filters)).toContain('limit=50');
    expect(dashboardURL(filters, 'opaque')).toContain('cursor=opaque');
  });

  it('drops invalid filters instead of forwarding arbitrary query material', () => {
    const filters = filtersFromURL(
      new URLSearchParams('state=excellent&kind=%2Fetc&integration=nope'),
    );
    expect(filters).toEqual({
      state: '',
      kind: '',
      integration: '',
      stale: '',
      cursor: '',
    });
  });
});

describe('Dashboard monitoring semantics', () => {
  it('uses stale as an attention state and computes only API-backed totals', () => {
    expect(totalResources(overview)).toBe(5);
    expect(dashboardState(overview)).toBe('stale');
  });

  it('keeps unknown and stale labels/icons distinct from healthy', () => {
    expect(statusPresentation('healthy')).toMatchObject({ label: 'Healthy' });
    expect(statusPresentation('unknown')).not.toEqual(
      statusPresentation('healthy'),
    );
    expect(statusPresentation('stale')).not.toEqual(
      statusPresentation('unknown'),
    );
  });

  it('formats absolute data as concise relative time', () => {
    expect(
      relativeTime('2026-07-31T11:55:00Z', Date.parse(overview.generated_at)),
    ).toBe('5 minutes ago');
  });
});
