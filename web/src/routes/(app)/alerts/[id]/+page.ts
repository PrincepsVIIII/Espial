import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import {
  problemFrom,
  requestJSON,
  requestJSONWithMetadata,
} from '$lib/api/client';
import type {
  EligibleIncidentAssignee,
  IncidentDetailResponse,
  IncidentTimelineResponse,
} from '$lib/incidents';
import type { NotificationDeliveryEvidence } from '$lib/api/generated';

type AssigneeList = { items: EligibleIncidentAssignee[]; next_cursor?: string };
type DeliveryList = { items: NotificationDeliveryEvidence[] };

export const load = (async ({ depends, fetch, params, parent, url }) => {
  depends('espial:monitoring');
  try {
    const parentData = await parent();
    const canOperate =
      parentData.session?.user.permissions.includes('incidents:operate') ??
      false;
    const [detail, timeline, deliveries, assigneesResult] = await Promise.all([
      requestJSONWithMetadata<IncidentDetailResponse>(
        fetch,
        `/api/v1/incidents/${params.id}`,
      ),
      requestJSON<IncidentTimelineResponse>(
        fetch,
        `/api/v1/incidents/${params.id}/timeline?limit=100`,
      ),
      requestJSON<DeliveryList>(
        fetch,
        `/api/v1/incidents/${params.id}/deliveries?limit=100`,
      ),
      canOperate
        ? requestJSON<AssigneeList>(
            fetch,
            '/api/v1/incident-assignees?limit=200',
          )
            .then((value) => ({ value, problem: null }))
            .catch((error) => ({ value: null, problem: problemFrom(error) }))
        : Promise.resolve({ value: null, problem: null }),
    ]);
    return {
      incident: detail.data,
      etag: detail.etag,
      timeline,
      deliveries: deliveries.items,
      canOperate,
      assignees: assigneesResult.value?.items ?? [],
      assigneeProblem: assigneesResult.problem,
      problem: null,
    };
  } catch (error) {
    const problem = problemFrom(error);
    if (problem.status === 401)
      redirect(303, `/login?returnTo=${encodeURIComponent(url.pathname)}`);
    return {
      incident: null,
      etag: undefined,
      timeline: null,
      deliveries: [],
      canOperate: false,
      assignees: [],
      assigneeProblem: null,
      problem,
    };
  }
}) satisfies PageLoad;
