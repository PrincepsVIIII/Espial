import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { problemFrom, requestJSON, type ClientProblem } from '$lib/api/client';
import type { RedactedAuditEventView } from '$lib/api/generated';

type AuditList = {
  items: RedactedAuditEventView[];
  next_cursor?: string;
  from: string;
  to: string;
};

const filterNames = [
  'action',
  'result',
  'target_type',
  'correlation_id',
] as const;

export const load = (async ({ fetch, url }) => {
  const query = new URLSearchParams({ limit: '50' });
  const filters = Object.fromEntries(
    filterNames.map((name) => [name, url.searchParams.get(name) ?? '']),
  ) as Record<(typeof filterNames)[number], string>;
  for (const name of filterNames) {
    if (filters[name]) query.set(name, filters[name]);
  }
  const cursor = url.searchParams.get('cursor');
  if (cursor) query.set('cursor', cursor);

  let audit: AuditList | null = null;
  let problem: ClientProblem | null = null;
  try {
    audit = await requestJSON<AuditList>(fetch, `/api/v1/audit?${query}`);
  } catch (error) {
    problem = problemFrom(error);
    if (problem.status === 401) {
      redirect(
        303,
        `/login?returnTo=${encodeURIComponent(url.pathname + url.search)}`,
      );
    }
  }

  const nextPageURL = audit?.next_cursor
    ? auditURL(filters, audit.next_cursor)
    : '';
  return { audit, problem, filters, nextPageURL };
}) satisfies PageLoad;

function auditURL(
  filters: Record<(typeof filterNames)[number], string>,
  cursor: string,
): string {
  const query = new URLSearchParams();
  for (const name of filterNames) {
    if (filters[name]) query.set(name, filters[name]);
  }
  query.set('cursor', cursor);
  return `/audit?${query}`;
}
