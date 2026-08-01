import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { problemFrom, requestJSON } from '$lib/api/client';
import {
  incidentFilters,
  incidentPageURL,
  incidentQuery,
  type IncidentListResponse,
} from '$lib/incidents';

export const load = (async ({ depends, fetch, url }) => {
  depends('espial:monitoring');
  const filters = incidentFilters(url.searchParams);
  try {
    const incidents = await requestJSON<IncidentListResponse>(
      fetch,
      `/api/v1/incidents?${incidentQuery(filters, true)}`,
    );
    return {
      filters,
      incidents,
      problem: null,
      nextPageURL: incidents.next_cursor
        ? incidentPageURL(filters, false, incidents.next_cursor)
        : '',
    };
  } catch (error) {
    const problem = problemFrom(error);
    if (problem.status === 401)
      redirect(
        303,
        `/login?returnTo=${encodeURIComponent(url.pathname + url.search)}`,
      );
    return { filters, incidents: null, problem, nextPageURL: '' };
  }
}) satisfies PageLoad;
