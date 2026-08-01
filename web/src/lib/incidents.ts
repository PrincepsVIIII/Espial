import type {
  IncidentAPIDetail,
  IncidentAPISummary,
  IncidentTimelineEvent,
} from '$lib/api/generated';

export type IncidentListResponse = {
  items: IncidentAPISummary[];
  next_cursor?: string;
};

export type IncidentTimelineResponse = {
  items: IncidentTimelineEvent[];
  next_cursor?: string;
};

export type IncidentDetailResponse = IncidentAPIDetail;
export type IncidentSeverity = IncidentAPISummary['severity'];
export type IncidentStatus = IncidentAPISummary['status'];

export type IncidentFilters = {
  severity: IncidentSeverity | '';
  status: IncidentStatus | '';
  cursor: string;
};

const severities: IncidentSeverity[] = ['warning', 'critical'];
const statuses: IncidentStatus[] = [
  'open',
  'acknowledged',
  'investigating',
  'recovered',
  'resolved',
];

export function incidentFilters(parameters: URLSearchParams): IncidentFilters {
  const rawSeverity = parameters.get('severity') ?? '';
  const rawStatus = parameters.get('status') ?? '';
  const rawCursor = parameters.get('cursor') ?? '';
  return {
    severity: severities.includes(rawSeverity as IncidentSeverity)
      ? (rawSeverity as IncidentSeverity)
      : '',
    status: statuses.includes(rawStatus as IncidentStatus)
      ? (rawStatus as IncidentStatus)
      : '',
    cursor: rawCursor.length <= 2048 ? rawCursor : '',
  };
}

export function incidentQuery(
  filters: IncidentFilters,
  active: boolean,
): string {
  const query = new URLSearchParams({ limit: '50', active: String(active) });
  if (filters.severity) query.set('severity', filters.severity);
  if (filters.status) query.set('status', filters.status);
  if (filters.cursor) query.set('cursor', filters.cursor);
  return query.toString();
}

export function incidentPageURL(
  filters: IncidentFilters,
  history: boolean,
  cursor = '',
): string {
  const query = new URLSearchParams();
  if (filters.severity) query.set('severity', filters.severity);
  if (filters.status) query.set('status', filters.status);
  if (cursor) query.set('cursor', cursor);
  const path = history ? '/alerts/history' : '/alerts';
  return query.size ? `${path}?${query}` : path;
}

export function incidentStatusLabel(status: IncidentStatus): string {
  return status.charAt(0).toUpperCase() + status.slice(1);
}

export function timelineKindLabel(kind: string): string {
  return kind
    .split('_')
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}
