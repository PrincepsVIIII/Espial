<script lang="ts">
  import { navigating } from '$app/state';
  import AlertNavigation from '$lib/components/AlertNavigation.svelte';
  import IncidentList from '$lib/components/IncidentList.svelte';
  let { data } = $props();
</script>

<svelte:head><title>Alerts · Espial</title></svelte:head>

<header class="page-header">
  <div>
    <h1>Alerts</h1>
    <p class="page-description">
      Authoritative incidents detected from resource health. This view is backed
      by durable evaluation and lifecycle evidence.
    </p>
  </div>
</header>

<AlertNavigation permissions={data.session?.user.permissions ?? []} />

{#if navigating.to?.url.pathname.startsWith('/alerts')}
  <div class="refresh-notice" aria-live="polite">Refreshing incidents…</div>
{/if}

<IncidentList {...data} history={false} />
