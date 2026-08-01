<script lang="ts">
  import IncidentList from '$lib/components/IncidentList.svelte';
  let { data } = $props();
</script>

<svelte:head><title>Alert history · Espial</title></svelte:head>

<header class="page-header">
  <div>
    <p class="eyebrow">Operational record</p>
    <h1>Alert history</h1>
    <p class="page-description">
      Recovered and resolved incidents, backed by immutable lifecycle timelines.
    </p>
  </div>
</header>
<nav class="section-navigation" aria-label="Alert views">
  <a href="/alerts">Active</a><a
    class="active"
    aria-current="page"
    href="/alerts/history">History</a
  >
</nav>
<IncidentList {...data} history={true} />
