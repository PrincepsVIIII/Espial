<script lang="ts">
  import { page } from '$app/state';
  import { alertNavigationItems } from '$lib/navigation';

  let { permissions }: { permissions: readonly string[] } = $props();
  const items = $derived(alertNavigationItems(permissions));
</script>

<nav class="section-navigation" aria-label="Alert views">
  {#each items as item}
    <a
      class:active={page.url.pathname === item.href}
      aria-current={page.url.pathname === item.href ? 'page' : undefined}
      href={item.href}>{item.label}</a
    >
  {/each}
</nav>
