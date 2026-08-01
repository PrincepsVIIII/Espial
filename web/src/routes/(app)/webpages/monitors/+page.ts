import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { problemFrom, requestJSON } from '$lib/api/client';
import type { RedactedWebsiteMonitorAPIView } from '$lib/api/generated';

type MonitorList = {
  items: RedactedWebsiteMonitorAPIView[];
  next_cursor?: string;
};

export const load = (async ({ depends, fetch, url }) => {
  depends('espial:monitoring');
  try {
    const cursor = url.searchParams.get('cursor');
    const query = new URLSearchParams({ limit: '50' });
    if (cursor) query.set('cursor', cursor);
    const result = await requestJSON<MonitorList>(
      fetch,
      `/api/v1/website-monitors?${query}`,
    );
    return {
      monitors: result.items,
      nextCursor: result.next_cursor ?? '',
      problem: null,
    };
  } catch (error) {
    const problem = problemFrom(error);
    if (problem.status === 401)
      redirect(303, `/login?returnTo=${encodeURIComponent(url.pathname)}`);
    return { monitors: [], nextCursor: '', problem };
  }
}) satisfies PageLoad;
