<script lang="ts">
  import { relativeTime } from '$lib/dashboard';

  let { value }: { value: string } = $props();
  let parsed = $derived(new Date(value));
  let valid = $derived(!Number.isNaN(parsed.getTime()));
  let utc = $derived(valid ? parsed.toISOString() : 'Invalid timestamp');
  let local = $derived(valid ? parsed.toLocaleString() : 'Invalid timestamp');
</script>

{#if valid}
  <span class="timestamp-wrap">
    <button
      class="timestamp"
      type="button"
      aria-label={`${relativeTime(value)}. UTC ${utc}. Local ${local}.`}
      >{relativeTime(value)}</button
    >
    <span class="timestamp-detail" role="tooltip">
      <time datetime={value}>UTC {utc}</time>
      <span>Local {local}</span>
    </span>
  </span>
{:else}
  <span class="timestamp">Unavailable</span>
{/if}
