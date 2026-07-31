import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import {
  problemFrom,
  requestJSON,
  type ClientProblem,
  type IntegrationListResponse,
  type ResourceListResponse,
} from '$lib/api/client';
import type { MonitoringOverview } from '$lib/api/generated';
import { dashboardURL, filtersFromURL, resourceQuery } from '$lib/dashboard';

type Loaded<T> = { value: T | null; problem: ClientProblem | null };

export const load = (async ({ depends, fetch, url }) => {
  depends('espial:monitoring');
  const filters = filtersFromURL(url.searchParams);
  const [overview, resources, integrations] = await Promise.all([
    loaded(() => requestJSON<MonitoringOverview>(fetch, '/api/v1/overview')),
    loaded(() =>
      requestJSON<ResourceListResponse>(
        fetch,
        `/api/v1/resources?${resourceQuery(filters)}`,
      ),
    ),
    loaded(() =>
      requestJSON<IntegrationListResponse>(
        fetch,
        '/api/v1/integrations?limit=200',
      ),
    ),
  ]);
  const authenticationFailure = [overview, resources, integrations].find(
    (result) => result.problem?.status === 401,
  );
  if (authenticationFailure) {
    redirect(
      303,
      `/login?returnTo=${encodeURIComponent(url.pathname + url.search)}`,
    );
  }
  return {
    filters,
    overview: overview.value,
    resources: resources.value,
    integrations: integrations.value,
    problems: {
      overview: overview.problem,
      resources: resources.problem,
      integrations: integrations.problem,
    },
    nextPageURL: resources.value?.next_cursor
      ? dashboardURL(filters, resources.value.next_cursor)
      : '',
  };
}) satisfies PageLoad;

async function loaded<T>(operation: () => Promise<T>): Promise<Loaded<T>> {
  try {
    return { value: await operation(), problem: null };
  } catch (error) {
    return { value: null, problem: problemFrom(error) };
  }
}
