import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { problemFrom, requestJSON } from '$lib/api/client';
import type { CertificateSummary } from '$lib/api/generated';

type CertificateList = { items: CertificateSummary[]; next_cursor?: string };

export const load = (async ({ depends, fetch, parent, url }) => {
  depends('espial:monitoring');
  const parentData = await parent();
  const canManage =
    parentData.session?.user.permissions.includes('website_monitors:manage') ??
    false;
  const state = ['healthy', 'warning', 'critical', 'unknown'].includes(
    url.searchParams.get('state') ?? '',
  )
    ? (url.searchParams.get('state') ?? '')
    : '';
  const hostnameValid = ['true', 'false'].includes(
    url.searchParams.get('hostname_valid') ?? '',
  )
    ? (url.searchParams.get('hostname_valid') ?? '')
    : '';
  const rawExpiry = url.searchParams.get('expiry_days') ?? '';
  const expiryDays =
    /^\d{1,4}$/.test(rawExpiry) && Number(rawExpiry) <= 3650 ? rawExpiry : '';
  const cursor = (url.searchParams.get('cursor') ?? '').slice(0, 2048);
  const query = new URLSearchParams({ limit: '50' });
  if (state) query.set('state', state);
  if (hostnameValid) query.set('hostname_valid', hostnameValid);
  if (expiryDays) query.set('expiry_days', expiryDays);
  if (cursor) query.set('cursor', cursor);
  try {
    const result = await requestJSON<CertificateList>(
      fetch,
      `/api/v1/certificates?${query}`,
    );
    return {
      certificates: result.items,
      nextCursor: result.next_cursor ?? '',
      filters: { state, hostnameValid, expiryDays },
      canManage,
      problem: null,
    };
  } catch (error) {
    const problem = problemFrom(error);
    if (problem.status === 401)
      redirect(
        303,
        `/login?returnTo=${encodeURIComponent(url.pathname + url.search)}`,
      );
    return {
      certificates: [],
      nextCursor: '',
      filters: { state, hostnameValid, expiryDays },
      canManage,
      problem,
    };
  }
}) satisfies PageLoad;
