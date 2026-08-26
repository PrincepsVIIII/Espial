<script lang="ts">
  import { displayState, normalizePercentage } from '$lib/graphics';

  let {
    label,
    value,
    secondaryValue,
    description,
    state: statusState,
    markerDetail,
    reading,
  }: {
    label: string;
    value: number | null | undefined;
    secondaryValue?: number | null;
    description: string;
    state: string;
    markerDetail: string;
    reading: string;
  } = $props();

  let primary = $derived(normalizePercentage(value));
  let secondary = $derived(normalizePercentage(secondaryValue));
  let visualState = $derived(displayState(statusState));
  let tooltipOpen = $state(false);

  function handleMarkerKeydown(event: KeyboardEvent) {
    if (event.key !== 'Escape') return;
    tooltipOpen = false;
    event.currentTarget instanceof HTMLElement && event.currentTarget.blur();
  }
</script>

<div class="resource-bar">
  <code title={label}>{label}</code>
  <div class="bar-line">
    <div
      class="bar-track"
      class:unknown={primary === null}
      role="img"
      aria-label={description}
    >
      {#if primary !== null}
        <span
          class:split={secondary !== null}
          class="bar-fill primary-fill"
          style={`width: ${primary}%`}
        ></span>
      {/if}
      {#if secondary !== null}
        <span class="bar-fill secondary-fill" style={`width: ${secondary}%`}
        ></span>
      {/if}
    </div>
    <button
      type="button"
      class="marker-wrap"
      class:open={tooltipOpen}
      aria-label={`${label}: ${markerDetail}`}
      aria-expanded={tooltipOpen}
      onclick={() => (tooltipOpen = !tooltipOpen)}
      onkeydown={handleMarkerKeydown}
      onblur={() => (tooltipOpen = false)}
    >
      <span class={`bar-marker marker-${visualState}`} aria-hidden="true"
      ></span>
      <span class="bar-tooltip" role="tooltip">{markerDetail}</span>
    </button>
  </div>
  <div class="bar-meta">
    <span class="reading">{reading}</span>
  </div>
</div>

<style>
  .resource-bar {
    display: grid;
    gap: 0.28rem;
    min-width: 0;
  }

  code {
    overflow: hidden;
    color: var(--text);
    font-size: 0.88rem;
    font-weight: 650;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .bar-line {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 0.7rem;
    gap: 0.45rem;
    align-items: center;
  }

  .bar-track {
    position: relative;
    height: 0.82rem;
    overflow: hidden;
    border: 1px solid var(--graphic-grid);
    background: var(--graphic-track);
  }

  .bar-track.unknown {
    border-style: dashed;
  }

  .bar-fill {
    position: absolute;
    inset-block: 0;
    left: 0;
    background: #0d8498;
    transition: width var(--graphic-transition-duration) ease-out;
  }

  .bar-fill.split {
    bottom: 50%;
  }

  .secondary-fill {
    top: 50%;
    background: #a5852d;
  }

  .marker-wrap {
    position: relative;
    display: grid;
    place-items: center;
    width: 0.7rem;
    height: 0.7rem;
    min-width: 0;
    min-height: 0;
    padding: 0;
    border: 0;
    border-radius: 50%;
    background: transparent;
    color: inherit;
  }

  .marker-wrap:hover,
  .marker-wrap:active {
    background: transparent;
  }

  .marker-wrap:focus-visible {
    outline: 3px solid var(--graphic-focus);
    outline-offset: 3px;
  }

  .bar-marker {
    width: 0.62rem;
    height: 0.62rem;
    border: 2px solid var(--unknown);
    border-radius: 50%;
    background: transparent;
  }

  .marker-healthy {
    border-color: var(--healthy);
  }

  .marker-warning {
    border-color: var(--warning);
  }

  .marker-critical {
    border-color: var(--critical);
  }

  .marker-stale {
    border-color: var(--stale);
    border-style: dashed;
  }

  .marker-maintenance {
    border-color: var(--maintenance);
  }

  .marker-disabled {
    border-color: var(--disabled);
  }

  .bar-tooltip {
    position: absolute;
    z-index: 6;
    right: -0.35rem;
    bottom: calc(100% + 0.55rem);
    width: max-content;
    max-width: 14rem;
    padding: 0.45rem 0.55rem;
    border: 1px solid var(--border-strong);
    background: var(--nav-blue);
    color: var(--text);
    font-size: 0.75rem;
    font-weight: 650;
    line-height: 1.35;
    opacity: 0;
    pointer-events: none;
    transform: translateY(0.2rem);
    transition:
      opacity var(--graphic-transition-duration) ease-out,
      transform var(--graphic-transition-duration) ease-out;
  }

  .marker-wrap:hover .bar-tooltip,
  .marker-wrap:focus .bar-tooltip,
  .marker-wrap.open .bar-tooltip {
    opacity: 1;
    transform: translateY(0);
  }

  .bar-meta {
    padding-right: 1.15rem;
    color: var(--graphic-label-muted);
    font-size: 0.78rem;
    line-height: 1.35;
  }

  .reading {
    display: block;
    color: var(--text);
    font-family: var(--font-mono);
    text-align: right;
    white-space: nowrap;
  }

  @media (max-width: 430px) {
    .reading {
      white-space: normal;
    }
  }
</style>
