<script lang="ts">
  import { formatPercentage, normalizePercentage } from '$lib/graphics';

  let {
    label,
    value,
    description,
  }: {
    label: string;
    value: number | null | undefined;
    description?: string;
  } = $props();

  let normalized = $derived(normalizePercentage(value));
  let formatted = $derived(formatPercentage(value));
</script>

<div class="metric-meter">
  <div class="metric-heading">
    <span>{label}</span>
    <output>{formatted}</output>
  </div>
  <div
    class="meter-track"
    class:unknown={normalized === null}
    role="progressbar"
    aria-label={label}
    aria-valuemin="0"
    aria-valuemax="100"
    aria-valuenow={normalized ?? undefined}
    aria-valuetext={formatted}
  >
    {#if normalized !== null}
      <span class="meter-fill" style={`width: ${normalized}%`}></span>
    {/if}
  </div>
  {#if description}
    <small>{description}</small>
  {/if}
</div>

<style>
  .metric-meter {
    display: grid;
    gap: 0.38rem;
    min-width: 0;
  }

  .metric-heading {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
    align-items: baseline;
  }

  .metric-heading > span {
    color: var(--graphic-label-muted);
    font-size: 0.86rem;
    font-weight: 650;
  }

  output {
    color: var(--text);
    font-family: var(--font-mono);
    font-weight: 720;
  }

  .meter-track {
    height: 0.7rem;
    overflow: hidden;
    border: 1px solid var(--graphic-grid);
    background: var(--graphic-track);
  }

  .meter-track.unknown {
    border-style: dashed;
  }

  .meter-fill {
    display: block;
    height: 100%;
    background: var(--graphic-mark);
    transition: width var(--graphic-transition-duration) ease-out;
  }

  small {
    color: var(--graphic-label-muted);
    line-height: 1.35;
  }
</style>
