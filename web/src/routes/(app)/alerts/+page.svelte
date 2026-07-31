<script lang="ts">
  import UnavailableSection from '$lib/components/UnavailableSection.svelte';
</script>

<svelte:head><title>Alerts · Espial</title></svelte:head>

<UnavailableSection
  eyebrow="Operational attention"
  title="Alerts"
  phase="Phase 2"
  description="The incident state machine and alert API are not implemented. Espial will not invent alert counts or activity while that source is unavailable."
  planned={[
    'Active and historical alerts',
    'Acknowledgement, assignment, and resolution',
    'Incident context, timelines, and delivery state',
  ]}
/>
