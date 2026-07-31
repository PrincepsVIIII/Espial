<script lang="ts">
  import '../styles.css';
  let { children } = $props();
</script>

<svelte:head><title>Espial</title></svelte:head>
{@render children()}
