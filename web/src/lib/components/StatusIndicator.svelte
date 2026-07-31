<script lang="ts">
  import { statusPresentation, type DisplayState } from '$lib/dashboard';

  let { state, compact = false }: { state: DisplayState; compact?: boolean } =
    $props();
  let presentation = $derived(statusPresentation(state));
</script>

<span
  class={`status-indicator status-${presentation.className}`}
  class:compact
  aria-label={presentation.label}
>
  <span class="status-icon" aria-hidden="true">{presentation.icon}</span>
  <span>{presentation.label}</span>
</span>
