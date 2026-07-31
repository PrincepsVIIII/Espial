import { redirect } from '@sveltejs/kit';
import type { LayoutLoad } from './$types';
import { ApiFailure, requestJSON } from '$lib/api/client';
import type { SessionResponse } from '$lib/auth';

export const load = (async ({ fetch, url }) => {
  try {
    const session = await requestJSON<SessionResponse>(
      fetch,
      '/api/v1/auth/session',
    );
    return { session, loadError: '' };
  } catch (error) {
    if (
      error instanceof ApiFailure &&
      (error.problem.status === 401 || error.problem.status === 403)
    ) {
      const returnTo = url.pathname + url.search;
      redirect(303, `/login?returnTo=${encodeURIComponent(returnTo)}`);
    }
    const requestID =
      error instanceof ApiFailure ? error.problem.request_id : undefined;
    return {
      session: null,
      loadError: requestID
        ? `Espial Core is unavailable. Request ID: ${requestID}`
        : 'Espial Core is unavailable. Session state could not be verified.',
    };
  }
}) satisfies LayoutLoad;
