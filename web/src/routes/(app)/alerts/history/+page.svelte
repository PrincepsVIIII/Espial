<script lang="ts">
  import AlertNavigation from '$lib/components/AlertNavigation.svelte';
  import IncidentList from '$lib/components/IncidentList.svelte';
  let { data } = $props();
</script>

<svelte:head><title>Alert history · Espial</title></svelte:head>

<header class="page-header">
  <div>
    <h1>Alert history</h1>
    <p class="page-description">
      Recovered and resolved incidents, backed by immutable lifecycle timelines.
    </p>
  </div>
</header>
<AlertNavigation permissions={data.session?.user.permissions ?? []} />
<IncidentList {...data} history={true} />
