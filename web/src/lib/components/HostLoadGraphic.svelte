<script lang="ts">
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
    hosts,
    loadNodes,
    consoleActivityAvailable,
  }: {
    id: string;
    clusterName: string;
    hosts: HypervisorHostMetric[];
    loadNodes: HypervisorLoadNode[];
    consoleActivityAvailable: boolean;
  } = $props();

  let selectedID = $state<string | null>(null);
  let selectionNotice = $state('');
  let rows = $derived(hostGraphicRows(hosts, loadNodes));
  let highestLoad = $derived(Math.max(1, ...rows.map((row) => row.idle_vms)));
  let activeRow = $derived(rows.find((row) => row.id === selectedID) ?? null);

  $effect(() => {
    const next = selectionAfterUpdate(selectedID, rows);
    if (next.removedID) {
      selectedID = null;
      selectionNotice = `${next.removedID} is no longer present in the latest cluster observation. Selection was cleared.`;
    }
  });

  function nodeID(hostID: string): string {
    return `${id}-node-${encodeURIComponent(hostID).replaceAll('%', '-')}`;
  }

  function tooltipID(hostID: string): string {
    return `${nodeID(hostID)}-tooltip`;
  }

  function nodeSize(idleVMs: number): string {
    return `${3.25 + (Math.max(0, idleVMs) / highestLoad) * 1.25}rem`;
  }

  function commit(event: MouseEvent, hostID: string) {
    // The button fills its layout cell for keyboard focus, but only the visible
    // disc is a pointer target. Blank field space must always dismiss selection.
    if (
      event.detail !== 0 &&
      !(event.target as HTMLElement).closest('.node-disc, .node-list')
    ) {
      return;
    }
    event.stopPropagation();
    selectedID = hostID;
    selectionNotice = '';
  }

  function clearSelectionFromMap(event: MouseEvent) {
    if ((event.target as HTMLElement).closest('.node-disc')) return;
    if (!selectedID) return;
    selectedID = null;
    selectionNotice = '';
  }

  function clearSelection(event: KeyboardEvent) {
    if (event.key !== 'Escape' || !selectedID) return;
    event.preventDefault();
    event.stopPropagation();
    selectedID = null;
    selectionNotice = 'Compute-node selection cleared.';
  }

  function idleVMLabel(count: number): string {
    return `${count} idle ${count === 1 ? 'VM' : 'VMs'}`;
  }

  function activeVMLabel(count: number): string {
    return `${count} console-active ${count === 1 ? 'VM' : 'VMs'}`;
  }

  function radarPoint(index: number, ratio: number): string {
    const angle = -Math.PI / 2 + (Math.PI * 2 * index) / rows.length;
    const radius = 34 * Math.max(0, Math.min(1, ratio));
    return `${50 + Math.cos(angle) * radius},${50 + Math.sin(angle) * radius}`;
  }

  function radarNodePosition(index: number): { x: number; y: number } {
    const [x, y] = radarPoint(index, 1).split(',').map(Number);
    return { x, y };
  }

  function radarGrid(ratio: number): string {
    return rows.map((_, index) => radarPoint(index, ratio)).join(' ');
  }

  function radarSeries(metric: 'idle_vms' | 'active_vms'): string {
    return rows
      .map((row, index) => radarPoint(index, row[metric] / highestLoad))
      .join(' ');
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<section
  class="compute-map"
  aria-labelledby={`${id}-title`}
  onclick={clearSelectionFromMap}
>
  <header class="compute-heading">
    <div>
      <h3 id={`${id}-title`}>Load distribution</h3>
      <p>Idle VMs by host in {clusterName}.</p>
    </div>
  </header>

  {#if rows.length > 1 && rows.length <= 12}
    <div
      class="radar-comparison"
      aria-label="Idle and console-active VM comparison"
    >
      <div class="radar-stage">
        <svg class="load-radar" viewBox="0 0 100 100" aria-hidden="true">
          {#if rows.length > 2}
            {#each [0.2, 0.4, 0.6, 0.8, 1] as ratio}
              <polygon class="radar-grid" points={radarGrid(ratio)} />
            {/each}
          {/if}
          {#each rows as _, index}
            <line
              x1="50"
              y1="50"
              x2={radarPoint(index, 1).split(',')[0]}
              y2={radarPoint(index, 1).split(',')[1]}
            />
          {/each}
          {#if rows.length > 2}
            <polygon class="radar-idle" points={radarSeries('idle_vms')} />
            {#if consoleActivityAvailable}
              <polygon
                class="radar-active"
                points={radarSeries('active_vms')}
              />
            {/if}
          {/if}
        </svg>

        <div
          class="node-field"
          role="group"
          aria-label={`${clusterName} compute nodes`}
        >
          {#each rows as row, index (row.id)}
            {@const position = radarNodePosition(index)}
            <div
              class="node-item"
              class:selected={selectedID === row.id}
              style={`--node-size: ${nodeSize(row.idle_vms)}; --node-x: ${position.x}; --node-y: ${position.y}`}
            >
              <button
                id={nodeID(row.id)}
                type="button"
                class="node-button"
                class:selected={selectedID === row.id}
                aria-pressed={selectedID === row.id}
                aria-describedby={tooltipID(row.id)}
                aria-label={`${row.id}, ${idleVMLabel(row.idle_vms)}, ${consoleActivityAvailable ? activeVMLabel(row.active_vms) : 'console activity unavailable'}, health ${statusLabel(row.health_state)}`}
                onclick={(event) => commit(event, row.id)}
                onkeydown={clearSelection}
              >
                <span
                  class={`node-disc disc-${displayState(row.health_state)}`}
                  aria-hidden="true"
                >
                  <code class="node-name" title={row.id}>{row.id}</code>
                  <span class="node-load">
                    <strong>{row.idle_vms}</strong>
                    <small>Idle VMs</small>
                    {#if consoleActivityAvailable}
                      <span>{row.active_vms} active</span>
                    {/if}
                  </span>
                </span>
              </button>
              <span class="node-tooltip" id={tooltipID(row.id)} role="tooltip">
                <strong
                  >{idleVMLabel(row.idle_vms)} · {consoleActivityAvailable
                    ? activeVMLabel(row.active_vms)
                    : 'Console activity unavailable'}</strong
                >
                <span
                  >CPU {formatPercentage(row.cpu_percent)} · RAM {formatPercentage(
                    row.ram_percent,
                  )} · Swap {formatPercentage(row.swap_percent)}</span
                >
              </span>
            </div>
          {/each}
        </div>
      </div>
      <p>
        <span class="idle-key">Idle VMs</span>
        {#if consoleActivityAvailable}
          <span class="active-key">Console-active VMs</span>
        {:else}
          <span>Console activity unavailable</span>
        {/if}
      </p>
    </div>
  {:else}
    <p class="too-many-nodes">
      {rows.length > 12
        ? 'Load distribution is available in the node list below for clusters with more than 12 hosts.'
        : 'One compute node is available.'}
    </p>
    <div
      class="node-field node-list many"
      role="group"
      aria-label={`${clusterName} compute nodes`}
    >
      {#each rows as row (row.id)}
        <button
          type="button"
          class="node-button"
          class:selected={selectedID === row.id}
          aria-pressed={selectedID === row.id}
          aria-label={`${row.id}, ${idleVMLabel(row.idle_vms)}, ${consoleActivityAvailable ? activeVMLabel(row.active_vms) : 'console activity unavailable'}, health ${statusLabel(row.health_state)}`}
          onclick={(event) => commit(event, row.id)}
          onkeydown={clearSelection}
          ><code>{row.id}</code><span>{idleVMLabel(row.idle_vms)}</span></button
        >
      {/each}
    </div>
  {/if}

  {#if activeRow}
    <section class="node-detail" aria-labelledby={`${id}-detail-title`}>
      <div>
        <h4 id={`${id}-detail-title`}>{activeRow.id}</h4>
        <p>
          {idleVMLabel(activeRow.idle_vms)} · {consoleActivityAvailable
            ? activeVMLabel(activeRow.active_vms)
            : 'Console activity unavailable'}
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
          <dt>Console-active</dt>
          <dd>
            {consoleActivityAvailable ? activeRow.active_vms : 'Unavailable'}
          </dd>
        </div>
      </dl>
    </section>
  {/if}

  {#if selectionNotice}
    <p class="selection-notice" aria-live="polite">{selectionNotice}</p>
  {/if}
</section>

<style>
  .compute-map {
    container-name: load-distribution;
    container-type: inline-size;
    min-width: 0;
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

  .radar-stage {
    position: relative;
    width: min(27rem, 100%);
    aspect-ratio: 1;
  }

  .radar-comparison {
    display: grid;
    justify-items: center;
    margin-top: 0.7rem;
  }

  .load-radar {
    width: 100%;
    height: 100%;
    overflow: visible;
  }

  .load-radar line,
  .radar-grid {
    fill: none;
    stroke: var(--graphic-grid);
    stroke-width: 0.5;
  }

  .radar-idle,
  .radar-active {
    stroke-width: 1.4;
  }

  .radar-idle {
    stroke: var(--text);
    fill: #7894a8;
    fill-opacity: 0.32;
  }

  .radar-active {
    fill: none;
    stroke: var(--graphic-label-muted);
    stroke-dasharray: 2 1.3;
  }

  .radar-comparison p {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 0.75rem;
    margin-top: 0.25rem;
    color: var(--graphic-label-muted);
    font-size: 0.7rem;
  }

  .radar-comparison p span::before {
    display: inline-block;
    width: 1rem;
    margin-right: 0.3rem;
    border-top: 2px solid currentColor;
    content: '';
    vertical-align: middle;
  }

  .active-key::before {
    border-top-style: dashed !important;
  }

  .node-field {
    position: absolute;
    inset: 0;
    pointer-events: none;
  }

  .node-item {
    position: relative;
    z-index: 1;
    position: absolute;
    top: calc(var(--node-y) * 1%);
    left: calc(var(--node-x) * 1%);
    width: calc(var(--node-size) + 1.5rem);
    pointer-events: none;
    transform: translate(-50%, -50%);
  }

  .node-button {
    display: grid;
    justify-items: center;
    gap: 0.3rem;
    width: 100%;
    min-height: 0;
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--text);
    pointer-events: auto;
  }

  .node-button:hover,
  .node-button:active {
    background: transparent;
  }

  .node-disc {
    display: grid;
    place-content: center;
    width: var(--node-size);
    aspect-ratio: 1;
    border: 3px solid var(--unknown);
    border-radius: 50%;
    background: #0a1820;
    transition:
      transform var(--graphic-transition-duration) ease-out,
      width var(--graphic-transition-duration) ease-out;
  }

  .node-button:hover .node-disc,
  .node-button:focus-visible .node-disc {
    transform: scale(1.035);
  }

  .node-button.selected .node-disc {
    transform: scale(1.18);
  }

  .node-button.selected:hover .node-disc,
  .node-button.selected:focus-visible .node-disc {
    transform: scale(1.18);
  }

  .node-item.selected {
    z-index: 2;
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

  .node-name,
  .node-load {
    grid-area: 1 / 1;
    align-self: center;
    justify-self: center;
    transition: opacity var(--graphic-transition-duration) ease-out;
  }

  .node-name {
    display: -webkit-box;
    width: calc(var(--node-size) - 1rem);
    overflow: hidden;
    color: var(--text);
    font-size: 0.72rem;
    font-weight: 720;
    line-height: 1.25;
    overflow-wrap: anywhere;
    text-align: center;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 3;
    line-clamp: 3;
  }

  .node-load {
    display: grid;
    place-items: center;
    opacity: 0;
  }

  .node-button:hover .node-name,
  .node-button:focus-visible .node-name {
    opacity: 0;
  }

  .node-button:hover .node-load,
  .node-button:focus-visible .node-load {
    opacity: 1;
  }

  .node-load strong {
    font-family: var(--font-mono);
    font-size: 1.35rem;
  }

  .node-load small {
    margin-top: 0.1rem;
    color: var(--text);
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .node-load span {
    margin-top: 0.25rem;
    color: var(--text-muted);
    font-size: 0.62rem;
    font-weight: 700;
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
    top: calc(100% + 0.45rem);
    left: 50%;
    display: grid;
    gap: 0.2rem;
    box-sizing: border-box;
    width: 100%;
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
    grid-template-columns: repeat(4, minmax(0, 1fr));
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
    min-height: 0;
    margin-top: 0.65rem;
    color: var(--graphic-label-muted);
    font-size: 0.8rem;
  }

  .too-many-nodes {
    margin-top: 1rem;
    color: var(--graphic-label-muted);
    font-size: 0.8rem;
  }

  .node-list {
    position: relative;
    inset: auto;
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(11rem, 1fr));
    gap: 0.45rem;
    margin-top: 0.65rem;
    max-height: min(34rem, 70vh);
    overflow-y: auto;
    pointer-events: auto;
    scrollbar-gutter: stable;
  }

  .node-list .node-button {
    display: flex;
    justify-content: space-between;
    gap: 0.75rem;
    min-height: 2.2rem;
    padding: 0.45rem 0.55rem;
    border: 1px solid var(--border);
    border-radius: 0.2rem;
    background: var(--surface);
    color: var(--text);
    text-align: left;
  }

  .node-list .node-button.selected {
    border-color: var(--graphic-selection);
  }

  .node-list code {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .node-list span {
    color: var(--graphic-label-muted);
    font-size: 0.8rem;
    white-space: nowrap;
  }

  @container load-distribution (max-width: 34rem) {
    .radar-stage {
      width: min(22rem, 100%);
    }

    .node-detail {
      grid-template-columns: 1fr;
    }

    dl {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>
