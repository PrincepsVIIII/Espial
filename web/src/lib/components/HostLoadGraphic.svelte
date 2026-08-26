<script lang="ts">
  import { tick } from 'svelte';
  import StatusIndicator from '$lib/components/StatusIndicator.svelte';
  import Timestamp from '$lib/components/Timestamp.svelte';
  import {
    displayState,
    formatBytes,
    formatPercentage,
    hostGraphicRows,
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
  let highestLoad = $derived(Math.max(1, ...rows.map((row) => row.active_vms)));
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

  function nodeID(hostID: string): string {
    return `${id}-node-${encodeURIComponent(hostID).replaceAll('%', '-')}`;
  }

  function tooltipID(hostID: string): string {
    return `${nodeID(hostID)}-tooltip`;
  }

  function nodeSize(activeVMs: number): string {
    return `${5.2 + (Math.max(0, activeVMs) / highestLoad) * 2.6}rem`;
  }

  function preview(hostID: string) {
    previewID = hostID;
  }

  function stopPreview(hostID: string) {
    if (previewID === hostID) previewID = null;
  }

  async function commit(hostID: string, revealNode = false) {
    selectedID = hostID;
    selectionNotice = `${hostID} selected.`;
    if (revealNode) {
      await tick();
      document
        .getElementById(nodeID(hostID))
        ?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
    }
  }

  function clearSelection(event: KeyboardEvent) {
    if (event.key !== 'Escape' || !selectedID) return;
    event.preventDefault();
    event.stopPropagation();
    selectedID = null;
    previewID = null;
    selectionNotice = 'Compute-node selection cleared.';
  }

  function freshnessLabel(value: string): string {
    if (value === 'fresh') return 'Fresh observation';
    return `${statusLabel(value)} observation`;
  }

  function vmLabel(count: number): string {
    return `${count} ${count === 1 ? 'VM' : 'VMs'} online`;
  }
</script>

<section class="compute-map" aria-labelledby={`${id}-title`}>
  <header class="compute-heading">
    <div>
      <h3 id={`${id}-title`}>Compute nodes</h3>
      <p>Active VM distribution and current host health for {clusterName}.</p>
    </div>
    <div class="compute-state">
      <StatusIndicator state={displayState(clusterState)} compact />
      <span>{freshnessLabel(freshness)}</span>
    </div>
  </header>

  <div
    class="node-field"
    class:many={rows.length > 12}
    role="list"
    aria-label={`${clusterName} compute nodes`}
  >
    {#each rows as row (row.id)}
      <div
        class="node-item"
        role="listitem"
        style={`--node-size: ${nodeSize(row.active_vms)}`}
      >
        <button
          id={nodeID(row.id)}
          type="button"
          class="node-button"
          class:selected={selectedID === row.id}
          aria-pressed={selectedID === row.id}
          aria-describedby={tooltipID(row.id)}
          aria-label={`${row.id}, ${vmLabel(row.active_vms)}, health ${statusLabel(row.health_state)}`}
          onclick={() => commit(row.id)}
          onkeydown={clearSelection}
          onpointerenter={() => preview(row.id)}
          onpointerleave={() => stopPreview(row.id)}
          onfocus={() => preview(row.id)}
          onblur={() => stopPreview(row.id)}
        >
          <span
            class={`node-disc disc-${displayState(row.health_state)}`}
            aria-hidden="true"
          >
            <strong>{row.active_vms}</strong>
            <small>VMs</small>
          </span>
          <code title={row.id}>{row.id}</code>
          <span class={`node-health health-${displayState(row.health_state)}`}
            >{statusLabel(row.health_state)}</span
          >
        </button>
        <span class="node-tooltip" id={tooltipID(row.id)} role="tooltip">
          <strong>{vmLabel(row.active_vms)}</strong>
          <span>
            CPU {formatPercentage(row.cpu_percent)} · RAM {formatPercentage(
              row.ram_percent,
            )}
          </span>
        </span>
      </div>
    {/each}
  </div>

  {#if activeRow}
    <section class="node-detail" aria-labelledby={`${id}-detail-title`}>
      <div>
        <h4 id={`${id}-detail-title`}>{activeRow.id}</h4>
        <p>
          {previewID && previewID !== selectedID ? 'Preview' : 'Selected'} ·
          {vmLabel(activeRow.active_vms)}
        </p>
      </div>
      <dl>
        <div>
          <dt>CPU</dt>
          <dd>{formatPercentage(activeRow.cpu_percent)}</dd>
        </div>
        <div>
          <dt>RAM</dt>
          <dd>{formatPercentage(activeRow.ram_percent)}</dd>
        </div>
        <div>
          <dt>Swap</dt>
          <dd>{formatPercentage(activeRow.swap_percent)}</dd>
        </div>
        <div>
          <dt>Capacity</dt>
          <dd>
            {activeRow.cpu_cores === null
              ? 'Unknown'
              : `${activeRow.cpu_cores.toFixed(0)} cores`}
            / {formatBytes(activeRow.ram_bytes)}
          </dd>
        </div>
        <div>
          <dt>Observed</dt>
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
  {/if}

  <p class="selection-notice" aria-live="polite">{selectionNotice}</p>

  <details class="host-data">
    <summary onkeydown={clearSelection}>Host metrics table</summary>
    <div
      class="table-scroll"
      role="region"
      aria-label={`${clusterName} host metrics table`}
    >
      <table>
        <caption>Host metrics equivalent to the compute-node graphic</caption>
        <thead>
          <tr>
            <th scope="col">Host</th>
            <th scope="col">VMs online</th>
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
  </details>
</section>

<style>
  .compute-map {
    min-width: 0;
    padding-block: 1.2rem 0;
    border-top: 1px solid var(--border);
  }

  .compute-heading {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
    align-items: flex-start;
  }

  h3,
  h4,
  p {
    margin: 0;
  }

  .compute-heading p,
  .node-detail p {
    margin-top: 0.3rem;
    color: var(--graphic-label-muted);
    line-height: 1.4;
  }

  .compute-state {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.5rem 0.8rem;
    color: var(--graphic-label-muted);
    font-size: 0.82rem;
  }

  .node-field {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));
    gap: 1.5rem;
    align-items: end;
    margin-top: 1.35rem;
    padding: 0.5rem;
  }

  .node-field.many {
    max-height: min(34rem, 70vh);
    overflow-y: auto;
    scrollbar-gutter: stable;
  }

  .node-item {
    position: relative;
    display: grid;
    justify-items: center;
    min-width: 0;
  }

  .node-button {
    display: grid;
    justify-items: center;
    gap: 0.35rem;
    min-width: min(100%, 10rem);
    min-height: 0;
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--text);
  }

  .node-button:hover,
  .node-button:active {
    background: transparent;
  }

  .node-disc {
    display: grid;
    place-content: center;
    width: min(var(--node-size), 100%);
    aspect-ratio: 1;
    border: 3px solid var(--unknown);
    border-radius: 50%;
    background: #0a1820;
    transition: transform var(--graphic-transition-duration) ease-out;
  }

  .node-button:hover .node-disc,
  .node-button:focus-visible .node-disc {
    transform: scale(1.035);
  }

  .node-button.selected .node-disc {
    outline: 3px solid var(--graphic-selection);
    outline-offset: 4px;
  }

  .disc-healthy {
    border-color: var(--healthy);
  }

  .disc-warning {
    border-color: var(--warning);
  }

  .disc-critical {
    border-color: var(--critical);
  }

  .disc-stale {
    border-color: var(--stale);
    border-style: dashed;
  }

  .disc-maintenance {
    border-color: var(--maintenance);
  }

  .disc-disabled {
    border-color: var(--disabled);
  }

  .node-disc strong {
    font-family: var(--font-mono);
    font-size: 1.35rem;
  }

  .node-disc small {
    margin-top: 0.1rem;
    color: var(--text);
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .node-button code {
    width: 100%;
    overflow: hidden;
    color: var(--text);
    font-weight: 700;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .node-health {
    font-size: 0.75rem;
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

  .node-tooltip {
    position: absolute;
    z-index: 4;
    top: 1.2rem;
    left: 50%;
    display: grid;
    gap: 0.2rem;
    width: max-content;
    max-width: 14rem;
    padding: 0.55rem 0.65rem;
    border: 1px solid var(--border-strong);
    background: var(--nav-blue);
    color: var(--text);
    font-size: 0.76rem;
    line-height: 1.35;
    opacity: 0;
    pointer-events: none;
    transform: translate(-50%, -0.25rem);
    transition:
      opacity var(--graphic-transition-duration) ease-out,
      transform var(--graphic-transition-duration) ease-out;
  }

  .node-tooltip span {
    color: var(--graphic-label-muted);
  }

  .node-item:hover .node-tooltip,
  .node-item:focus-within .node-tooltip {
    opacity: 1;
    transform: translate(-50%, 0);
  }

  .node-detail {
    display: grid;
    grid-template-columns: minmax(11rem, 0.8fr) minmax(0, 2fr);
    gap: 1.5rem;
    align-items: start;
    margin-top: 1.4rem;
    padding-block: 1rem;
    border-top: 1px solid var(--border);
    border-bottom: 1px solid var(--border);
  }

  dl {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: 1rem;
    margin: 0;
  }

  dt {
    color: var(--graphic-label-muted);
    font-size: 0.72rem;
    font-weight: 700;
    text-transform: uppercase;
  }

  dd {
    margin: 0.25rem 0 0;
    font-family: var(--font-mono);
    font-size: 0.84rem;
  }

  .selection-notice {
    min-height: 1.4em;
    margin-top: 0.65rem;
    color: var(--graphic-label-muted);
    font-size: 0.8rem;
  }

  .host-data {
    margin-top: 0.2rem;
    border-top: 1px solid var(--border);
  }

  summary {
    width: max-content;
    padding: 0.8rem 0;
    color: var(--netdef-cyan);
    font-weight: 700;
    cursor: pointer;
  }

  summary:focus-visible {
    outline: 3px solid var(--graphic-focus);
    outline-offset: 2px;
  }

  .table-scroll {
    overflow-x: auto;
    padding-bottom: 0.5rem;
  }

  table {
    width: 100%;
    min-width: 42rem;
    border-collapse: collapse;
  }

  caption {
    padding: 0 0 0.65rem;
    color: var(--graphic-label-muted);
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
    font-size: 0.74rem;
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
  }

  .table-host:hover,
  .table-host:active {
    background: transparent;
    color: var(--netdef-cyan);
  }

  @media (max-width: 800px) {
    .compute-heading,
    .node-detail {
      grid-template-columns: 1fr;
    }

    .compute-heading {
      display: grid;
    }

    .compute-state {
      justify-content: flex-start;
    }

    dl {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>
