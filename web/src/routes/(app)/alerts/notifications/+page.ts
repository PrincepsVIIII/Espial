import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { problemFrom, requestJSON } from '$lib/api/client';
import type {
  NotificationDeliveryEvidence,
  RedactedNotificationDestinationAPIView,
} from '$lib/api/generated';

type DestinationList = { items: RedactedNotificationDestinationAPIView[] };
type DeliveryList = { items: NotificationDeliveryEvidence[] };

export const load = (async ({ depends, fetch, url }) => {
  depends('espial:monitoring');
  try {
    const [destinations, deliveries] = await Promise.all([
      requestJSON<DestinationList>(fetch, '/api/v1/notification-destinations'),
      requestJSON<DeliveryList>(
        fetch,
        '/api/v1/notification-deliveries?limit=100',
      ),
    ]);
    return {
      destinations: destinations.items,
      deliveries: deliveries.items,
      problem: null,
    };
  } catch (error) {
    const problem = problemFrom(error);
    if (problem.status === 401)
      redirect(303, `/login?returnTo=${encodeURIComponent(url.pathname)}`);
    return { destinations: [], deliveries: [], problem };
  }
}) satisfies PageLoad;
