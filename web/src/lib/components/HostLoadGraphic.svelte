<script lang="ts">
  import { tick } from 'svelte';
  import GraphicFrame from '$lib/components/GraphicFrame.svelte';
  import MetricMeter from '$lib/components/MetricMeter.svelte';
  import StatusIndicator from '$lib/components/StatusIndicator.svelte';
  import Timestamp from '$lib/components/Timestamp.svelte';
  import {
    displayState,
    formatBytes,
    formatPercentage,
    hostGraphicRows,
    integerDomain,
    positionInDomain,
    selectionAfterUpdate,
    statusLabel,
    type HypervisorHostMetric,
    type HypervisorLoadNode,
  } from '$lib/graphics';

  let {
    id,
    clusterName,
    clusterState,
    freshness,
    lastObservedAt,
    hosts,
    loadNodes,
  }: {
    id: string;
    clusterName: string;
    clusterState: string;
    freshness: string;
    lastObservedAt?: string | null;
    hosts: HypervisorHostMetric[];
    loadNodes: HypervisorLoadNode[];
  } = $props();

  let selectedID = $state<string | null>(null);
  let previewID = $state<string | null>(null);
  let selectionNotice = $state('');
  let rows = $derived(hostGraphicRows(hosts, loadNodes));
  let domain = $derived(integerDomain(rows.map((row) => row.active_vms)));
  let activeID = $derived(previewID ?? selectedID);
  let activeRow = $derived(rows.find((row) => row.id === activeID) ?? null);

  $effect(() => {
    const next = selectionAfterUpdate(selectedID, rows);
    if (next.removedID) {
      selectedID = null;
      if (previewID === next.removedID) previewID = null;
      selectionNotice = `${next.removedID} is no longer present in the latest cluster observation. Selection was cleared.`;
    }
  });

  function plotRowID(hostID: string): string {
    return `${id}-plot-${encodeURIComponent(hostID).replaceAll('%', '-')}`;
  }

  function preview(hostID: string) {
    previewID = hostID;
  }

  function stopPreview(hostID: string) {
    if (previewID === hostID) previewID = null;
  }

  async function commit(hostID: string, revealPlot = false) {
    selectedID = hostID;
    selectionNotice = `${hostID} selected.`;
    if (revealPlot) {
      await tick();
      document
        .getElementById(plotRowID(hostID))
        ?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
    }
  }

  function clearSelection(event: KeyboardEvent) {
    if (event.key !== 'Escape' || !selectedID) return;
    event.preventDefault();
    event.stopPropagation();
    selectedID = null;
    previewID = null;
    selectionNotice = 'Host selection cleared.';
  }

  function freshnessLabel(value: string): string {
    if (value === 'fresh') return 'Fresh observation';
    return `${statusLabel(value)} observation`;
  }

  function vmLabel(count: number): string {
    return `${count} active ${count === 1 ? 'VM' : 'VMs'}`;
  }
</script>

<GraphicFrame
  {id}
  title="Active VMs by host"
  description={`Which ${clusterName} host carries the most active VMs, and what is its current compute health? Select a host to inspect the authoritative host metrics.`}
  freshness={freshnessLabel(freshness)}
  status={displayState(clusterState)}
>
  {#snippet plot()}
    <div class="load-plot">
      <div class="plot-axis" aria-hidden="true">
        <span></span>
        <div class="tick-track">
          {#each domain.ticks as tick}
            <span
              class="tick"
              style={`left: ${positionInDomain(tick, domain)}%`}
            >
              <span>{tick}</span>
            </span>
          {/each}
        </div>
        <span>VMs</span>
        <span>Health</span>
      </div>
      <div class:many={rows.length > 12} class="plot-rows">
        {#each rows as row (row.id)}
          <button
            id={plotRowID(row.id)}
            type="button"
            class="plot-row"
            class:selected={selectedID === row.id}
            aria-pressed={selectedID === row.id}
            aria-label={`${row.id}, ${vmLabel(row.active_vms)}, health ${statusLabel(row.health_state)}`}
            onclick={() => commit(row.id)}
            onkeydown={clearSelection}
            onpointerenter={() => preview(row.id)}
            onpointerleave={() => stopPreview(row.id)}
            onfocus={() => preview(row.id)}
            onblur={() => stopPreview(row.id)}
          >
            <code title={row.id}>{row.id}</code>
            <span class="load-track" aria-hidden="true">
              {#each domain.ticks as tick}
                <span
                  class="grid-line"
                  style={`left: ${positionInDomain(tick, domain)}%`}
                ></span>
              {/each}
              <span
                class="marker"
                style={`left: ${positionInDomain(row.active_vms, domain)}%`}
              >
                <span
                  class={`marker-shape shape-${displayState(row.health_state)}`}
                ></span>
              </span>
            </span>
            <span class="vm-count">{row.active_vms}</span>
            <span
              class={`health-label health-${displayState(row.health_state)}`}
              >{statusLabel(row.health_state)}</span
            >
          </button>
        {/each}
      </div>
    </div>
  {/snippet}

  {#snippet legend()}
    <p class="legend-copy">
      Marker position encodes active VM count from zero. Marker outline and
      shape reinforce the visible health label. Cyan outlines indicate the
      committed selection, not operational health.
    </p>
  {/snippet}

  {#snippet details()}
    {#if activeRow}
      <section class="host-detail" aria-labelledby={`${id}-detail-title`}>
        <div class="detail-heading">
          <div>
            <h4 id={`${id}-detail-title`}>{activeRow.id}</h4>
            <p>
              {previewID && previewID !== selectedID
                ? 'Previewed host'
                : 'Selected host'}
              · {vmLabel(activeRow.active_vms)}
            </p>
          </div>
          <StatusIndicator
            state={displayState(activeRow.health_state)}
            compact
          />
        </div>
        <div class="detail-meters">
          <MetricMeter label="CPU" value={activeRow.cpu_percent} />
          <MetricMeter label="RAM" value={activeRow.ram_percent} />
          <MetricMeter label="Swap" value={activeRow.swap_percent} />
        </div>
        <dl>
          <div>
            <dt>CPU capacity</dt>
            <dd>
              {activeRow.cpu_cores === null
                ? 'Unknown'
                : `${activeRow.cpu_cores.toFixed(0)} cores`}
            </dd>
          </div>
          <div>
            <dt>RAM capacity</dt>
            <dd>{formatBytes(activeRow.ram_bytes)}</dd>
          </div>
          <div>
            <dt>Observation</dt>
            <dd>
              {#if lastObservedAt}
                <Timestamp value={lastObservedAt} />
              {:else}
                Unknown
              {/if}
            </dd>
          </div>
        </dl>
      </section>
    {:else}
      <p class="detail-prompt">
        Select a host in the plot or table to compare active VM load with CPU,
        RAM, swap, health, and observation freshness.
      </p>
    {/if}
    <p class="selection-notice" aria-live="polite">{selectionNotice}</p>
  {/snippet}

  {#snippet table()}
    <div
      class="table-scroll"
      role="region"
      aria-label={`${clusterName} host metrics table`}
    >
      <table>
        <caption> Host metrics equivalent to the active VM plot </caption>
        <thead>
          <tr>
            <th scope="col">Host</th>
            <th scope="col">Active VMs</th>
            <th scope="col">CPU</th>
            <th scope="col">RAM</th>
            <th scope="col">Swap</th>
            <th scope="col">Health</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.id)}
            <tr class:selected={selectedID === row.id}>
              <th scope="row">
                <button
                  type="button"
                  class="table-host"
                  aria-pressed={selectedID === row.id}
                  onclick={() => commit(row.id, true)}
                  onkeydown={clearSelection}
                  onpointerenter={() => preview(row.id)}
                  onpointerleave={() => stopPreview(row.id)}
                  onfocus={() => preview(row.id)}
                  onblur={() => stopPreview(row.id)}>{row.id}</button
                >
              </th>
              <td>{row.active_vms}</td>
              <td>{formatPercentage(row.cpu_percent)}</td>
              <td>{formatPercentage(row.ram_percent)}</td>
              <td>{formatPercentage(row.swap_percent)}</td>
              <td>
                <StatusIndicator
                  state={displayState(row.health_state)}
                  compact
                />
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/snippet}
</GraphicFrame>

<style>
  .load-plot {
    min-width: 0;
  }

  .plot-axis,
  .plot-row {
    display: grid;
    grid-template-columns:
      minmax(7rem, 12rem) minmax(9rem, 1fr) minmax(2.7rem, 3.5rem)
      minmax(5.7rem, 7rem);
    gap: 0.7rem;
    align-items: center;
  }

  .plot-axis {
    min-height: 2.2rem;
    color: var(--graphic-label-muted);
    font-size: 0.75rem;
    font-weight: 650;
  }

  .tick-track,
  .load-track {
    position: relative;
  }

  .tick-track {
    height: 1.8rem;
  }

  .tick {
    position: absolute;
    inset-block: 0;
    width: 1px;
    background: var(--graphic-grid);
  }

  .tick span {
    position: absolute;
    top: 0;
    transform: translateX(-50%);
    font-family: var(--font-mono);
  }

  .plot-rows {
    min-width: 0;
  }

  .plot-rows.many {
    max-height: min(34rem, 70vh);
    overflow-y: auto;
    scrollbar-gutter: stable;
  }

  .plot-row {
    width: 100%;
    min-height: 3.1rem;
    padding: 0.45rem 0.35rem;
    border: 1px solid transparent;
    border-top-color: var(--graphic-grid);
    border-radius: 0;
    background: transparent;
    color: var(--text);
    text-align: left;
  }

  .plot-row:hover,
  .plot-row:active {
    border-color: var(--border-strong);
    background: transparent;
  }

  .plot-row.selected {
    border-color: var(--graphic-selection);
  }

  .plot-row code {
    overflow: hidden;
    color: inherit;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .load-track {
    height: 0.65rem;
    border: 1px solid var(--graphic-grid);
    background: var(--graphic-track);
  }

  .grid-line {
    position: absolute;
    top: -0.32rem;
    width: 1px;
    height: 1.28rem;
    background: var(--graphic-grid);
  }

  .marker {
    position: absolute;
    top: 50%;
    width: 1rem;
    height: 1rem;
    transform: translate(-50%, -50%);
    transition: left var(--graphic-transition-duration) ease-out;
  }

  .marker-shape {
    display: block;
    width: 100%;
    height: 100%;
    border: 2px solid var(--unknown);
    background: var(--surface-1);
    border-radius: 50%;
  }

  .shape-healthy {
    border-color: var(--healthy);
  }

  .shape-warning {
    border-color: var(--warning);
    border-radius: 0;
    transform: rotate(45deg) scale(0.78);
  }

  .shape-critical {
    border-color: var(--critical);
    border-radius: 0;
    clip-path: polygon(
      30% 0,
      70% 0,
      100% 30%,
      100% 70%,
      70% 100%,
      30% 100%,
      0 70%,
      0 30%
    );
  }

  .shape-stale,
  .shape-unknown {
    border-style: dashed;
  }

  .shape-maintenance,
  .shape-disabled {
    border-color: var(--maintenance);
    border-radius: 0;
  }

  .vm-count {
    font-family: var(--font-mono);
    text-align: right;
  }

  .health-label {
    font-size: 0.82rem;
    font-weight: 700;
  }

  .health-healthy {
    color: var(--healthy);
  }

  .health-warning {
    color: var(--warning);
  }

  .health-critical {
    color: var(--critical);
  }

  .health-stale {
    color: var(--stale);
  }

  .health-maintenance {
    color: var(--maintenance);
  }

  .health-disabled,
  .health-unknown {
    color: var(--unknown);
  }

  .legend-copy,
  .detail-prompt,
  .selection-notice {
    margin: 0;
    color: var(--graphic-label-muted);
    line-height: 1.45;
  }

  .host-detail {
    display: grid;
    gap: 1rem;
  }

  .detail-heading {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
    align-items: flex-start;
  }

  .detail-heading h4 {
    margin: 0;
  }

  .detail-heading p {
    margin: 0.25rem 0 0;
    color: var(--graphic-label-muted);
  }

  .detail-meters {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 1rem;
  }

  dl {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 1rem;
    margin: 0;
  }

  dl > div {
    border-left: 2px solid var(--border-strong);
    padding-left: 0.65rem;
  }

  dt {
    color: var(--graphic-label-muted);
    font-size: 0.82rem;
  }

  dd {
    margin: 0.2rem 0 0;
    font-family: var(--font-mono);
  }

  .selection-notice {
    min-height: 1.4em;
    margin-top: 0.75rem;
  }

  .table-scroll {
    overflow-x: auto;
  }

  table {
    width: 100%;
    min-width: 42rem;
    border-collapse: collapse;
  }

  caption {
    padding: 0 0 0.75rem;
    color: var(--graphic-label-muted);
    font-weight: 650;
    text-align: left;
  }

  th,
  td {
    padding: 0.6rem;
    border-bottom: 1px solid var(--border);
    text-align: left;
  }

  thead th {
    color: var(--graphic-label-muted);
    font-size: 0.78rem;
    text-transform: uppercase;
  }

  tbody tr.selected {
    outline: 2px solid var(--graphic-selection);
    outline-offset: -2px;
  }

  .table-host {
    min-height: 2rem;
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--text);
    font-family: var(--font-mono);
    text-align: left;
    text-decoration: underline;
    text-decoration-color: transparent;
    text-underline-offset: 0.2rem;
  }

  .table-host:hover,
  .table-host:active {
    background: transparent;
    color: var(--netdef-cyan);
    text-decoration-color: currentColor;
  }

  @media (max-width: 700px) {
    .plot-axis,
    .plot-row {
      grid-template-columns:
        minmax(6.5rem, 1fr) minmax(7rem, 1.5fr) 2.4rem
        minmax(4.8rem, 0.8fr);
      gap: 0.4rem;
    }

    .detail-meters,
    dl {
      grid-template-columns: 1fr;
    }
  }
</style>
