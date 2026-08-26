<script lang="ts">
  import { displayState, normalizePercentage } from '$lib/graphics';

  let {
    label,
    value,
    secondaryValue,
    description,
    state,
    stateText,
    reading,
  }: {
    label: string;
    value: number | null | undefined;
    secondaryValue?: number | null;
    description: string;
    state: string;
    stateText: string;
    reading: string;
  } = $props();

  let primary = $derived(normalizePercentage(value));
  let secondary = $derived(normalizePercentage(secondaryValue));
  let visualState = $derived(displayState(state));
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
    <span class={`bar-marker marker-${visualState}`} aria-hidden="true"></span>
  </div>
  <div class="bar-meta">
    <span class={`state-text resource-state-${visualState}`}>{stateText}</span>
    <span class="reading">{reading}</span>
  </div>
</div>

<style>
  .resource-bar {
    display: grid;
    gap: 0.38rem;
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
    grid-template-columns: minmax(0, 1fr) 0.8rem;
    gap: 0.55rem;
    align-items: center;
  }

  .bar-track {
    position: relative;
    height: 1.15rem;
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

  .bar-marker {
    width: 0.72rem;
    height: 0.72rem;
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

  .bar-meta {
    display: grid;
    grid-template-columns: minmax(0, 1fr) max-content;
    gap: 0.75rem;
    padding-right: 1.35rem;
    color: var(--graphic-label-muted);
    font-size: 0.78rem;
    line-height: 1.35;
  }

  .state-text {
    overflow: hidden;
    font-weight: 700;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .resource-state-healthy {
    color: var(--healthy);
  }

  .resource-state-warning {
    color: var(--warning);
  }

  .resource-state-critical {
    color: var(--critical);
  }

  .resource-state-stale {
    color: var(--stale);
  }

  .resource-state-maintenance {
    color: var(--maintenance);
  }

  .resource-state-disabled,
  .resource-state-unknown {
    color: var(--unknown);
  }

  .reading {
    color: var(--text);
    font-family: var(--font-mono);
    text-align: right;
    white-space: nowrap;
  }

  @media (max-width: 430px) {
    .bar-meta {
      grid-template-columns: 1fr;
      gap: 0.15rem;
    }

    .reading {
      text-align: left;
      white-space: normal;
    }
  }
</style>
