import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { problemFrom, requestJSON } from '$lib/api/client';
import type { CertificateDetail } from '$lib/api/generated';

export const load = (async ({ depends, fetch, params, url }) => {
  depends('espial:monitoring');
  try {
    return {
      certificate: await requestJSON<CertificateDetail>(
        fetch,
        `/api/v1/certificates/${params.id}`,
      ),
      problem: null,
    };
  } catch (error) {
    const problem = problemFrom(error);
    if (problem.status === 401)
      redirect(303, `/login?returnTo=${encodeURIComponent(url.pathname)}`);
    return { certificate: null, problem };
  }
}) satisfies PageLoad;
