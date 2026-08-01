import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { problemFrom, requestJSON } from '$lib/api/client';
import type { WebpageAvailabilitySummary } from '$lib/api/generated';

type WebpageList = {
  items: WebpageAvailabilitySummary[];
  next_cursor?: string;
};

export const load = (async ({ depends, fetch, parent, url }) => {
  depends('espial:monitoring');
  const parentData = await parent();
  const canManage =
    parentData.session?.user.permissions.includes('website_monitors:manage') ??
    false;
  try {
    const cursor = url.searchParams.get('cursor');
    const query = new URLSearchParams({ limit: '50' });
    if (cursor) query.set('cursor', cursor);
    const result = await requestJSON<WebpageList>(
      fetch,
      `/api/v1/webpages?${query}`,
    );
    return {
      webpages: result.items,
      nextCursor: result.next_cursor ?? '',
      canManage,
      problem: null,
    };
  } catch (error) {
    const problem = problemFrom(error);
    if (problem.status === 401)
      redirect(303, `/login?returnTo=${encodeURIComponent(url.pathname)}`);
    return { webpages: [], nextCursor: '', canManage, problem };
  }
}) satisfies PageLoad;
