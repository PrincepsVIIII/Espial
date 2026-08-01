import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { problemFrom, requestJSON } from '$lib/api/client';
import type {
  IncidentRuleAPIView,
  IntegrationAPIView,
  ResourceAPIView,
} from '$lib/api/generated';

type RuleList = { items: IncidentRuleAPIView[] };
type ResourceList = { items: ResourceAPIView[] };
type IntegrationList = { items: IntegrationAPIView[] };

export const load = (async ({ depends, fetch, url }) => {
  depends('espial:monitoring');
  try {
    const [rules, resources, integrations] = await Promise.all([
      requestJSON<RuleList>(fetch, '/api/v1/incident-rules'),
      requestJSON<ResourceList>(fetch, '/api/v1/resources?limit=200'),
      requestJSON<IntegrationList>(fetch, '/api/v1/integrations?limit=200'),
    ]);
    return {
      rules: rules.items,
      resources: resources.items,
      integrations: integrations.items,
      problem: null,
    };
  } catch (error) {
    const problem = problemFrom(error);
    if (problem.status === 401)
      redirect(303, `/login?returnTo=${encodeURIComponent(url.pathname)}`);
    return { rules: [], resources: [], integrations: [], problem };
  }
}) satisfies PageLoad;
