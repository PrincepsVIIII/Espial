<script lang="ts">
  import UnavailableSection from '$lib/components/UnavailableSection.svelte';
</script>

<svelte:head><title>Hypervisor · Espial</title></svelte:head>

<UnavailableSection
  title="Hypervisor"
  phase="Phase 3"
  description="No hypervisor adapter is configured. Hosts, clusters, and virtual machines will appear only after an integration reports authoritative state."
  planned={[
    'Host and cluster health',
    'Virtual-machine inventory and current state',
    'Resource relationships and affected services',
  ]}
/>
