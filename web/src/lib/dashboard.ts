import type {
  IntegrationAPIView,
  MonitoringOverview,
  ResourceAPIView,
} from '$lib/api/generated';

export const resourceStates = [
  'healthy',
  'warning',
  'critical',
  'unknown',
  'stale',
  'maintenance',
  'disabled',
] as const;

export type ResourceState = (typeof resourceStates)[number];
export type RuntimeState = IntegrationAPIView['runtime_state'];
export type DisplayState = ResourceState | RuntimeState;

export type DashboardFilters = {
  state: ResourceState | '';
  kind: string;
  integration: string;
  stale: 'true' | 'false' | '';
  cursor: string;
};

export type StatusPresentation = {
  label: string;
  icon: string;
  className: string;
};

const statusPresentations: Record<DisplayState, StatusPresentation> = {
  healthy: { label: 'Healthy', icon: '✓', className: 'healthy' },
  warning: { label: 'Warning', icon: '▲', className: 'warning' },
  critical: { label: 'Critical', icon: '!', className: 'critical' },
  unknown: { label: 'Unknown', icon: '?', className: 'unknown' },
  stale: { label: 'Stale', icon: '◷', className: 'stale' },
  maintenance: { label: 'Maintenance', icon: '◆', className: 'maintenance' },
  disabled: { label: 'Disabled', icon: 'Ⅱ', className: 'disabled' },
  starting: { label: 'Starting', icon: '↻', className: 'maintenance' },
  unhealthy: { label: 'Unhealthy', icon: '!', className: 'critical' },
  stopped: { label: 'Stopped', icon: '■', className: 'disabled' },
  not_started: { label: 'Not started', icon: '?', className: 'unknown' },
};

export function statusPresentation(state: DisplayState): StatusPresentation {
  return (
    statusPresentations[state] ?? {
      label: 'Unknown',
      icon: '?',
      className: 'unknown',
    }
  );
}

export function filtersFromURL(parameters: URLSearchParams): DashboardFilters {
  const rawState = parameters.get('state') ?? '';
  const state = resourceStates.includes(rawState as ResourceState)
    ? (rawState as ResourceState)
    : '';
  const rawKind = (parameters.get('kind') ?? '').trim();
  const kind = /^[a-z][a-z0-9_.-]{0,126}$/.test(rawKind) ? rawKind : '';
  const rawIntegration = parameters.get('integration') ?? '';
  const integration = uuidPattern.test(rawIntegration) ? rawIntegration : '';
  const rawStale = parameters.get('stale') ?? '';
  const stale = rawStale === 'true' || rawStale === 'false' ? rawStale : '';
  const rawCursor = parameters.get('cursor') ?? '';
  const cursor = rawCursor.length <= 2048 ? rawCursor : '';
  return { state, kind, integration, stale, cursor };
}

export function resourceQuery(filters: DashboardFilters): string {
  const query = new URLSearchParams({ limit: '50' });
  if (filters.state) query.set('state', filters.state);
  if (filters.kind) query.set('kind', filters.kind);
  if (filters.integration) query.set('integration', filters.integration);
  if (filters.stale) query.set('stale', filters.stale);
  if (filters.cursor) query.set('cursor', filters.cursor);
  return query.toString();
}

export function dashboardURL(filters: DashboardFilters, cursor = ''): string {
  const query = new URLSearchParams();
  if (filters.state) query.set('state', filters.state);
  if (filters.kind) query.set('kind', filters.kind);
  if (filters.integration) query.set('integration', filters.integration);
  if (filters.stale) query.set('stale', filters.stale);
  if (cursor) query.set('cursor', cursor);
  const encoded = query.toString();
  return encoded ? `/dashboard?${encoded}` : '/dashboard';
}

export function countFor(
  overview: MonitoringOverview | null,
  state: ResourceState,
): number {
  return (
    overview?.resource_counts.find((item) => item.state === state)?.count ?? 0
  );
}

export function totalResources(overview: MonitoringOverview | null): number {
  return (
    overview?.resource_counts.reduce((total, item) => total + item.count, 0) ??
    0
  );
}

export function dashboardState(
  overview: MonitoringOverview | null,
): ResourceState {
  if (!overview) return 'unknown';
  if (countFor(overview, 'critical') > 0) return 'critical';
  if (countFor(overview, 'warning') > 0) return 'warning';
  if (overview.stale_count > 0) return 'stale';
  if (overview.unknown_count > 0) return 'unknown';
  return totalResources(overview) > 0 ? 'healthy' : 'unknown';
}

export function resourceObservationTime(resource: ResourceAPIView): string {
  return (
    resource.health.observed_at ??
    resource.latest_observation?.observed_at ??
    resource.health.updated_at
  );
}

export function relativeTime(value: string, now = Date.now()): string {
  const time = Date.parse(value);
  if (!Number.isFinite(time)) return 'Invalid time';
  const seconds = Math.round((time - now) / 1000);
  const absolute = Math.abs(seconds);
  const formatter = new Intl.RelativeTimeFormat('en', { numeric: 'auto' });
  if (absolute < 60) return formatter.format(seconds, 'second');
  const minutes = Math.round(seconds / 60);
  if (Math.abs(minutes) < 60) return formatter.format(minutes, 'minute');
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 48) return formatter.format(hours, 'hour');
  return formatter.format(Math.round(hours / 24), 'day');
}

const uuidPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
