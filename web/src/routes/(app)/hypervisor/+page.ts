import { dev } from '$app/environment';
import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { problemFrom, requestJSON, type ClientProblem } from '$lib/api/client';
import type { ProxmoxClusterCard } from '$lib/api/generated';

type Loaded<T> = { value: T | null; problem: ClientProblem | null };
type ProxmoxClusterList = { items: ProxmoxClusterCard[] };

export const load = (async ({ depends, fetch, parent, url }) => {
  depends('espial:monitoring');
  const parentData = await parent();
  const clusters = await loaded(() =>
    requestJSON<ProxmoxClusterList>(
      fetch,
      '/api/v1/hypervisor/proxmox/clusters',
    ),
  );
  if (clusters.problem?.status === 401) {
    redirect(
      303,
      `/login?returnTo=${encodeURIComponent(url.pathname + url.search)}`,
    );
  }
  const items = clusters.value?.items ?? [];
  return {
    clusters:
      dev && items.length === 0
        ? { items: exampleClusters.items }
        : clusters.value,
    problem: clusters.problem,
    canManage:
      parentData.session?.user.permissions.includes('integrations:manage') ??
      false,
    demo: dev && items.length === 0 && !clusters.problem,
  };
}) satisfies PageLoad;

async function loaded<T>(operation: () => Promise<T>): Promise<Loaded<T>> {
  try {
    return { value: await operation(), problem: null };
  } catch (error) {
    return { value: null, problem: problemFrom(error) };
  }
}

const exampleClusters: ProxmoxClusterList = {
  items: [
    {
      id: 'example',
      integration_id: 'example',
      name: 'Example',
      state: 'healthy',
      freshness: 'fresh',
      last_observed_at: new Date().toISOString(),
      hosts: [
        {
          id: 'pve-a',
          cpu_percent: 42,
          ram_percent: 64,
          cpu_cores: 16,
          ram_bytes: 68_719_476_736,
          swap_percent: 3,
          swap_indicator: 'healthy',
          health_state: 'healthy',
        },
        {
          id: 'pve-b',
          cpu_percent: 68,
          ram_percent: 71,
          cpu_cores: 24,
          ram_bytes: 137_438_953_472,
          swap_percent: 13,
          swap_indicator: 'warning',
          health_state: 'warning',
        },
      ],
      storage: [
        {
          id: 'local-zfs',
          scope: 'pve-a',
          used_bytes: 1_900_000_000_000,
          total_bytes: 4_000_000_000_000,
          used_percent: 47.5,
          io_wait_percent: 1.5,
          io_wait_indicator: 'healthy',
          health_state: 'healthy',
        },
        {
          id: 'shared-vmstore',
          scope: 'shared',
          used_bytes: 8_400_000_000_000,
          total_bytes: 12_000_000_000_000,
          used_percent: 70,
          io_wait_percent: 7,
          io_wait_indicator: 'warning',
          health_state: 'warning',
        },
      ],
      totals: {
        cpu_percent: 57.6,
        ram_percent: 68.7,
        storage_percent: 63.6,
      },
      load_nodes: [
        { id: 'pve-a', active_vms: 3 },
        { id: 'pve-b', active_vms: 7 },
      ],
    },
  ],
};
