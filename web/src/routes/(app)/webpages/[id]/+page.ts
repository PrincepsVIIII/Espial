import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { problemFrom, requestJSON } from '$lib/api/client';
import type { WebpageAvailabilityDetail } from '$lib/api/generated';

export const load = (async ({ depends, fetch, params, url }) => {
  depends('espial:monitoring');
  try {
    const webpage = await requestJSON<WebpageAvailabilityDetail>(
      fetch,
      `/api/v1/webpages/${params.id}`,
    );
    return { webpage, problem: null };
  } catch (error) {
    const problem = problemFrom(error);
    if (problem.status === 401)
      redirect(303, `/login?returnTo=${encodeURIComponent(url.pathname)}`);
    return { webpage: null, problem };
  }
}) satisfies PageLoad;
