<script lang="ts">
  import UnavailableSection from '$lib/components/UnavailableSection.svelte';
</script>

<svelte:head><title>Datacenter · Espial</title></svelte:head>

<UnavailableSection
  eyebrow="Physical infrastructure"
  title="Datacenter"
  phase="Phases 5–7"
  description="Room and hardware inventory are not configured. The spatial view will remain empty until Core can provide authoritative rooms, racks, devices, and drive locations."
  planned={[
    'Neutral-gray room overview with selectable racks',
    'Rack and model-aware server inspection',
    'Drive-bay status and authoritative drive details',
  ]}
/>
