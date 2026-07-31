import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { problemFrom, requestJSON, type ClientProblem } from '$lib/api/client';
import type { ManagedUser, RoleView } from '$lib/api/generated';

type Loaded<T> = { value: T | null; problem: ClientProblem | null };

export const load = (async ({ fetch, url }) => {
  const cursor = url.searchParams.get('cursor') ?? '';
  const userQuery = new URLSearchParams({ limit: '200' });
  if (cursor) userQuery.set('cursor', cursor);
  const [users, roles] = await Promise.all([
    loaded(() =>
      requestJSON<{ items: ManagedUser[]; next_cursor?: string }>(
        fetch,
        `/api/v1/users?${userQuery}`,
      ),
    ),
    loaded(() => requestJSON<{ items: RoleView[] }>(fetch, '/api/v1/roles')),
  ]);
  const authenticationFailure = [users, roles].find(
    (result) => result.problem?.status === 401,
  );
  if (authenticationFailure) {
    redirect(
      303,
      `/login?returnTo=${encodeURIComponent(url.pathname + url.search)}`,
    );
  }
  return {
    users: users.value?.items ?? [],
    roles: roles.value?.items ?? [],
    problem: users.problem ?? roles.problem,
    nextPageURL: users.value?.next_cursor
      ? `/audit/users?cursor=${encodeURIComponent(users.value.next_cursor)}`
      : '',
    continuation: Boolean(cursor),
  };
}) satisfies PageLoad;

async function loaded<T>(operation: () => Promise<T>): Promise<Loaded<T>> {
  try {
    return { value: await operation(), problem: null };
  } catch (error) {
    return { value: null, problem: problemFrom(error) };
  }
}
