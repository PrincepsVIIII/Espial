import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import {
  problemFrom,
  requestJSON,
  requestJSONWithMetadata,
} from '$lib/api/client';
import type {
  IncidentDetailResponse,
  IncidentTimelineResponse,
} from '$lib/incidents';

export const load = (async ({ depends, fetch, params, url }) => {
  depends('espial:monitoring');
  try {
    const [detail, timeline] = await Promise.all([
      requestJSONWithMetadata<IncidentDetailResponse>(
        fetch,
        `/api/v1/incidents/${params.id}`,
      ),
      requestJSON<IncidentTimelineResponse>(
        fetch,
        `/api/v1/incidents/${params.id}/timeline?limit=100`,
      ),
    ]);
    return {
      incident: detail.data,
      etag: detail.etag,
      timeline,
      problem: null,
    };
  } catch (error) {
    const problem = problemFrom(error);
    if (problem.status === 401)
      redirect(303, `/login?returnTo=${encodeURIComponent(url.pathname)}`);
    return { incident: null, etag: undefined, timeline: null, problem };
  }
}) satisfies PageLoad;
