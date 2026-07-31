<script lang="ts">
  import UnavailableSection from '$lib/components/UnavailableSection.svelte';
</script>

<svelte:head><title>Webpages · Espial</title></svelte:head>

<UnavailableSection
  eyebrow="Availability and certificates"
  title="Webpages"
  phase="Phase 2"
  description="Website checks are not configured. Availability, response timing, and certificate state will remain absent until Core has trustworthy observations."
  planned={[
    'Website availability and response state',
    'TLS certificate validity and expiry',
    'Check history and linked alerts',
  ]}
/>
