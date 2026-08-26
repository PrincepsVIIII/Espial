<script lang="ts">
  import type { Snippet } from 'svelte';
  import StatusIndicator from '$lib/components/StatusIndicator.svelte';
  import type { DisplayState } from '$lib/dashboard';

  let {
    id,
    title,
    description,
    freshness,
    status,
    plot,
    legend,
    details,
    table,
  }: {
    id: string;
    title: string;
    description: string;
    freshness?: string;
    status?: DisplayState;
    plot: Snippet;
    legend?: Snippet;
    details?: Snippet;
    table?: Snippet;
  } = $props();
</script>

<section class="graphic-frame" aria-labelledby={`${id}-title`}>
  <header class="graphic-heading">
    <div>
      <h3 id={`${id}-title`}>{title}</h3>
      <p>{description}</p>
    </div>
    {#if status || freshness}
      <div class="graphic-state">
        {#if status}<StatusIndicator state={status} compact />{/if}
        {#if freshness}<span>{freshness}</span>{/if}
      </div>
    {/if}
  </header>

  <div class="graphic-plot">{@render plot()}</div>

  {#if legend}
    <div class="graphic-legend" aria-label="Legend">{@render legend()}</div>
  {/if}

  {#if details}
    <div class="graphic-details" aria-live="polite">{@render details()}</div>
  {/if}

  {#if table}
    <div class="graphic-table-equivalent">{@render table()}</div>
  {/if}
</section>

<style>
  .graphic-frame {
    min-width: 0;
    border: 1px solid var(--border);
    background: var(--surface-1);
  }

  .graphic-heading {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
    align-items: flex-start;
    padding: 1rem;
    border-bottom: 1px solid var(--border);
  }

  h3 {
    margin: 0;
    font-size: 1rem;
  }

  p {
    max-width: 64rem;
    margin: 0.35rem 0 0;
    color: var(--graphic-label-muted);
    line-height: 1.45;
  }

  .graphic-state {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.5rem 0.75rem;
    color: var(--graphic-label-muted);
    font-size: 0.86rem;
    text-align: right;
  }

  .graphic-plot,
  .graphic-details,
  .graphic-table-equivalent,
  .graphic-legend {
    padding: 1rem;
  }

  .graphic-details,
  .graphic-table-equivalent,
  .graphic-legend {
    border-top: 1px solid var(--border);
  }

  .graphic-legend {
    color: var(--graphic-label-muted);
    font-size: 0.86rem;
  }

  @media (max-width: 600px) {
    .graphic-heading {
      display: grid;
    }

    .graphic-state {
      justify-content: flex-start;
      text-align: left;
    }
  }
</style>
