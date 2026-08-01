import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { problemFrom, requestJSON } from '$lib/api/client';
import type {
  IncidentAPISummary,
  IncidentRuleAPIView,
  IntegrationAPIView,
  MaintenanceWindow,
  NotificationSilence,
  ResourceAPIView,
} from '$lib/api/generated';

export const load = (async ({ depends, fetch, url }) => {
  depends('espial:monitoring');
  try {
    const [windows, silences, resources, integrations, incidents, rules] =
      await Promise.all([
        requestJSON<{ items: MaintenanceWindow[] }>(
          fetch,
          '/api/v1/maintenance-windows',
        ),
        requestJSON<{ items: NotificationSilence[] }>(
          fetch,
          '/api/v1/silences',
        ),
        requestJSON<{ items: ResourceAPIView[] }>(
          fetch,
          '/api/v1/resources?limit=200',
        ),
        requestJSON<{ items: IntegrationAPIView[] }>(
          fetch,
          '/api/v1/integrations?limit=200',
        ),
        requestJSON<{ items: IncidentAPISummary[] }>(
          fetch,
          '/api/v1/incidents?active=true&limit=200',
        ),
        requestJSON<{ items: IncidentRuleAPIView[] }>(
          fetch,
          '/api/v1/incident-rules',
        ),
      ]);
    return {
      windows: windows.items,
      silences: silences.items,
      resources: resources.items,
      integrations: integrations.items,
      incidents: incidents.items,
      rules: rules.items,
      problem: null,
    };
  } catch (error) {
    const problem = problemFrom(error);
    if (problem.status === 401)
      redirect(303, `/login?returnTo=${encodeURIComponent(url.pathname)}`);
    return {
      windows: [],
      silences: [],
      resources: [],
      integrations: [],
      incidents: [],
      rules: [],
      problem,
    };
  }
}) satisfies PageLoad;
