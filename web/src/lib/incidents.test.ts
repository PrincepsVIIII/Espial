import { describe, expect, it } from 'vitest';
import {
  incidentFilters,
  incidentPageURL,
  incidentQuery,
  timelineKindLabel,
  type IncidentFilters,
} from './incidents';

describe('incident navigation', () => {
  it('accepts supported filters and rejects unsupported values', () => {
    expect(
      incidentFilters(
        new URLSearchParams('severity=critical&status=open&cursor=next'),
      ),
    ).toEqual({ severity: 'critical', status: 'open', cursor: 'next' });
    expect(
      incidentFilters(new URLSearchParams('severity=urgent&status=closed')),
    ).toEqual({ severity: '', status: '', cursor: '' });
  });

  it('keeps active mode authoritative and cursors filter-bound', () => {
    const filters: IncidentFilters = {
      severity: 'warning',
      status: '',
      cursor: '',
    };
    expect(incidentQuery(filters, true)).toContain('active=true');
    expect(incidentPageURL(filters, true, 'next')).toBe(
      '/alerts/history?severity=warning&cursor=next',
    );
  });

  it('presents machine timeline kinds as readable copy', () => {
    expect(timelineKindLabel('severity_changed')).toBe('Severity Changed');
  });
});
