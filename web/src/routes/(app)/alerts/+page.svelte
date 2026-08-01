<script lang="ts">
  import { navigating } from '$app/state';
  import IncidentList from '$lib/components/IncidentList.svelte';
  let { data } = $props();
</script>

<svelte:head><title>Alerts · Espial</title></svelte:head>

<header class="page-header">
  <div>
    <p class="eyebrow">Operational attention</p>
    <h1>Alerts</h1>
    <p class="page-description">
      Authoritative incidents detected from resource health. This view is
      read-only in Phase 2.1.
    </p>
  </div>
</header>

<nav class="section-navigation" aria-label="Alert views">
  <a class="active" aria-current="page" href="/alerts">Active</a>
  <a href="/alerts/history">History</a>
</nav>

{#if navigating.to?.url.pathname.startsWith('/alerts')}
  <div class="refresh-notice" aria-live="polite">Refreshing incidents…</div>
{/if}

<IncidentList {...data} history={false} />
