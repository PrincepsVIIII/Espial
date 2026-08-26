<script lang="ts">
  import { formatPercentage, normalizePercentage } from '$lib/graphics';

  let {
    label,
    value,
    tone = 'cyan',
  }: {
    label: string;
    value: number | null | undefined;
    tone?: 'cyan' | 'amber' | 'neutral';
  } = $props();

  let normalized = $derived(normalizePercentage(value));
  let formatted = $derived(formatPercentage(value));
</script>

<div
  class={`semicircle-gauge tone-${tone}`}
  role="progressbar"
  aria-label={label}
  aria-valuemin="0"
  aria-valuemax="100"
  aria-valuenow={normalized ?? undefined}
  aria-valuetext={formatted}
>
  <svg viewBox="0 0 220 132" aria-hidden="true">
    <path
      class="gauge-track"
      d="M 22 112 A 88 88 0 0 1 198 112"
      pathLength="100"
    />
    {#if normalized !== null}
      <path
        class="gauge-value"
        d="M 22 112 A 88 88 0 0 1 198 112"
        pathLength="100"
        stroke-dasharray={`${normalized} 100`}
      />
    {/if}
  </svg>
  <div class="gauge-copy">
    <output>{formatted}</output>
    <span>{label}</span>
  </div>
</div>

<style>
  .semicircle-gauge {
    position: relative;
    width: 100%;
    min-width: 0;
    max-width: 15rem;
    margin-inline: auto;
    color: #1aa6bb;
  }

  svg {
    display: block;
    width: 100%;
    overflow: visible;
  }

  path {
    fill: none;
    stroke-width: 18;
    stroke-linecap: butt;
  }

  .gauge-track {
    stroke: var(--surface-3);
  }

  .gauge-value {
    stroke: currentColor;
    transition: stroke-dasharray var(--graphic-transition-duration) ease-out;
  }

  .tone-amber {
    color: #b99231;
  }

  .tone-neutral {
    color: var(--graphic-mark);
  }

  .gauge-copy {
    position: absolute;
    inset: auto 0 0.25rem;
    display: grid;
    justify-items: center;
    gap: 0.12rem;
  }

  output {
    color: var(--text);
    font-family: var(--font-mono);
    font-size: clamp(1.25rem, 2vw, 1.7rem);
    font-weight: 760;
  }

  span {
    color: var(--graphic-label-muted);
    font-size: 0.78rem;
    font-weight: 700;
    letter-spacing: 0.035em;
    text-transform: uppercase;
  }
</style>
