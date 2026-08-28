<script lang="ts">
  import {
    displayState,
    formatBytes,
    formatPercentage,
    hostGraphicRows,
    selectionAfterUpdate,
    storageGraphicRows,
    statusLabel,
    type HypervisorHostMetric,
    type HypervisorLoadNode,
    type HypervisorStorageMetric,
    type LoadGraphicRow,
  } from '$lib/graphics';

  let {
    id,
    clusterName,
    hosts,
    loadNodes,
    storage,
    storageLoadNodes,
    consoleActivityAvailable,
    storageAttributionAvailable,
  }: {
    id: string;
    clusterName: string;
    hosts: HypervisorHostMetric[];
    loadNodes: HypervisorLoadNode[];
    storage: HypervisorStorageMetric[];
    storageLoadNodes: HypervisorLoadNode[];
    consoleActivityAvailable: boolean;
    storageAttributionAvailable: boolean;
  } = $props();

  let view = $state<'compute' | 'storage'>('compute');
  let selectedID = $state<string | null>(null);
  let selectionNotice = $state('');
  let computeRows = $derived(hostGraphicRows(hosts, loadNodes));
  let storageRows = $derived(storageGraphicRows(storage, storageLoadNodes));
  let rows = $derived(view === 'compute' ? computeRows : storageRows);
  let highestIdleLoad = $derived(
    Math.max(1, ...rows.map((row) => row.idle_vms)),
  );
  let highestSeriesValue = $derived(
    Math.max(1, ...rows.flatMap((row) => [row.idle_vms, row.active_vms])),
  );
  let totalActiveVMs = $derived(
    rows.reduce((total, row) => total + Math.max(0, row.active_vms), 0),
  );
  let activeRow = $derived(rows.find((row) => row.id === selectedID) ?? null);

  $effect(() => {
    const next = selectionAfterUpdate(selectedID, rows);
    if (next.removedID) {
      selectedID = null;
      selectionNotice = `${next.removedID} is no longer present in the latest ${view === 'compute' ? 'compute' : 'storage'} observation. Selection was cleared.`;
    }
  });

  function nodeID(rowID: string): string {
    return `${id}-${view}-node-${encodeURIComponent(rowID).replaceAll('%', '-')}`;
  }

  function tooltipID(rowID: string): string {
    return `${nodeID(rowID)}-tooltip`;
  }

  function nodeSize(row: LoadGraphicRow): string {
    const loadSize =
      3.75 + (Math.max(0, row.idle_vms) / highestIdleLoad) * 1.35;
    const longestLabelPart = row.id
      .split(/[\s_-]+/)
      .reduce((longest, part) => Math.max(longest, part.length), 0);
    // Keep a three-line name readable before idle-VM utilization expands it.
    // The cap preserves the bounded radar field for exceptionally long IDs.
    const labelSize = Math.min(5.75, 1.25 + longestLabelPart * 0.45);
    return `${Math.max(loadSize, labelSize)}rem`;
  }

  function commit(event: MouseEvent, rowID: string) {
    // The button fills its layout cell for keyboard focus, but only the visible
    // disc is a pointer target. Blank field space must always dismiss selection.
    if (
      event.detail !== 0 &&
      !(event.target as HTMLElement).closest('.node-disc, .node-list')
    ) {
      return;
    }
    event.stopPropagation();
    selectedID = rowID;
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
    selectionNotice = `${view === 'compute' ? 'Compute-node' : 'Storage'} selection cleared.`;
  }

  function selectView(nextView: 'compute' | 'storage') {
    if (view === nextView) return;
    view = nextView;
    selectedID = null;
    selectionNotice = '';
  }

  function description(): string {
    return view === 'compute'
      ? `Idle VMs by compute host in ${clusterName}.`
      : `Idle VMs by storage backing each running guest's main disk in ${clusterName}. EFI, ISO, and auxiliary volumes are excluded.`;
  }

  function groupLabel(): string {
    return `${clusterName} ${view === 'compute' ? 'compute nodes' : 'main-disk storage'}`;
  }

  function rowContext(row: LoadGraphicRow): string {
    if (row.kind === 'compute') {
      return `CPU ${formatPercentage(row.cpu_percent)} · RAM ${formatPercentage(row.ram_percent)} · Swap ${formatPercentage(row.swap_percent)}`;
    }
    return `Used ${formatPercentage(row.used_percent)} · I/O wait ${formatPercentage(row.io_wait_percent)}`;
  }

  function accessibleRowLabel(row: LoadGraphicRow): string {
    return `${row.id}, ${idleVMLabel(row.idle_vms)}, ${consoleActivityAvailable ? activeVMLabel(row.active_vms) : 'console activity unavailable'}, health ${statusLabel(row.health_state)}`;
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

  function radarSeries(metric: 'idle_vms'): string {
    return rows
      .map((row, index) => radarPoint(index, row[metric] / highestSeriesValue))
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
      <p>{description()}</p>
    </div>
    <div
      class="load-view-toggle"
      role="group"
      aria-label="Load distribution view"
      data-view={view}
    >
      <button
        type="button"
        class:current={view === 'compute'}
        aria-pressed={view === 'compute'}
        onclick={() => selectView('compute')}>Compute</button
      >
      <button
        type="button"
        class:current={view === 'storage'}
        aria-pressed={view === 'storage'}
        onclick={() => selectView('storage')}>Storage</button
      >
      <span class="load-view-selector" aria-hidden="true"></span>
    </div>
  </header>

  {#if view === 'storage' && !storageAttributionAvailable}
    <p class="unavailable-storage">
      Storage attribution is unavailable because Espial could not read every
      running guest's disk configuration. Compute distribution remains
      available.
    </p>
  {:else if rows.length > 2 && rows.length <= 12}
    <div
      class="radar-comparison"
      aria-label={`Idle and console-active VM comparison by ${view}`}
    >
      <div class="radar-stage">
        <svg class="load-radar" viewBox="0 0 100 100" aria-hidden="true">
          {#if rows.length > 2}
            {#each [0.2, 0.4, 0.6, 0.8, 1] as ratio}
              <polygon class="radar-grid" points={radarGrid(ratio)} />
            {/each}
          {/if}
          {#each rows as row, index}
            <line
              x1="50"
              y1="50"
              x2={radarPoint(index, 1).split(',')[0]}
              y2={radarPoint(index, 1).split(',')[1]}
            />
            {#if consoleActivityAvailable && row.active_vms > 0}
              <line
                class="radar-active"
                x1="50"
                y1="50"
                x2={radarPoint(
                  index,
                  row.active_vms / highestSeriesValue,
                ).split(',')[0]}
                y2={radarPoint(
                  index,
                  row.active_vms / highestSeriesValue,
                ).split(',')[1]}
              />
            {/if}
          {/each}
          {#if rows.length > 2}
            <polygon class="radar-idle" points={radarSeries('idle_vms')} />
          {/if}
        </svg>

        <div class="node-field" role="group" aria-label={groupLabel()}>
          {#each rows as row, index (row.id)}
            {@const position = radarNodePosition(index)}
            <div
              class="node-item"
              class:selected={selectedID === row.id}
              style={`--node-size: ${nodeSize(row)}; --node-x: ${position.x}; --node-y: ${position.y}`}
            >
              <button
                id={nodeID(row.id)}
                type="button"
                class="node-button"
                class:selected={selectedID === row.id}
                aria-pressed={selectedID === row.id}
                aria-describedby={tooltipID(row.id)}
                aria-label={accessibleRowLabel(row)}
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
                <span>{rowContext(row)}</span>
              </span>
            </div>
          {/each}
        </div>
      </div>
    </div>
  {:else if rows.length > 0 && rows.length <= 2}
    <div class="compact-node-field" role="group" aria-label={groupLabel()}>
      {#each rows as row (row.id)}
        <div
          class="node-item compact"
          class:selected={selectedID === row.id}
          style={`--node-size: ${nodeSize(row)}`}
        >
          <button
            id={nodeID(row.id)}
            type="button"
            class="node-button"
            class:selected={selectedID === row.id}
            aria-pressed={selectedID === row.id}
            aria-describedby={tooltipID(row.id)}
            aria-label={accessibleRowLabel(row)}
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
            <span>{rowContext(row)}</span>
          </span>
        </div>
      {/each}
    </div>
  {:else}
    <p class="too-many-nodes">
      {rows.length > 12
        ? `Load distribution is available in the ${view === 'compute' ? 'compute-node' : 'storage'} list below for clusters with more than 12 targets.`
        : `No ${view === 'compute' ? 'compute nodes' : 'storage targets'} are present in the current cluster observation.`}
    </p>
    <div
      class="node-field node-list many"
      role="group"
      aria-label={groupLabel()}
    >
      {#each rows as row (row.id)}
        <button
          type="button"
          class="node-button"
          class:selected={selectedID === row.id}
          aria-pressed={selectedID === row.id}
          aria-label={accessibleRowLabel(row)}
          onclick={(event) => commit(event, row.id)}
          onkeydown={clearSelection}
          ><code>{row.id}</code><span>{idleVMLabel(row.idle_vms)}</span></button
        >
      {/each}
    </div>
  {/if}

  {#if rows.length > 0 && !(view === 'storage' && !storageAttributionAvailable)}
    <p class="console-summary">
      {#if consoleActivityAvailable}
        {activeVMLabel(totalActiveVMs)}
      {:else}
        Console activity unavailable
      {/if}
    </p>
  {/if}

  {#if activeRow}
    <section class="node-detail" aria-labelledby={`${id}-${view}-detail-title`}>
      <div>
        <h4 id={`${id}-${view}-detail-title`}>{activeRow.id}</h4>
        <p>
          {idleVMLabel(activeRow.idle_vms)} · {consoleActivityAvailable
            ? activeVMLabel(activeRow.active_vms)
            : 'Console activity unavailable'}
        </p>
      </div>
      <dl>
        {#if activeRow.kind === 'compute'}
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
        {:else}
          <div>
            <dt>Used</dt>
            <dd>{formatPercentage(activeRow.used_percent)}</dd>
          </div>
          <div>
            <dt>I/O wait</dt>
            <dd>{formatPercentage(activeRow.io_wait_percent)}</dd>
          </div>
          <div>
            <dt>Capacity</dt>
            <dd>
              {formatBytes(activeRow.used_bytes)} / {formatBytes(
                activeRow.total_bytes,
              )}
            </dd>
          </div>
        {/if}
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

  .compute-heading {
    display: flex;
    flex-wrap: wrap;
    align-items: start;
    justify-content: space-between;
    gap: 0.75rem 1rem;
  }

  .load-view-toggle {
    position: relative;
    display: grid;
    flex: 0 0 8rem;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    width: 8rem;
    padding-bottom: 0.3rem;
    font-weight: 700;
  }

  .load-view-toggle button {
    padding: 0.1rem 0.25rem;
    border: 0;
    background: transparent;
    color: var(--graphic-label-muted);
    font-size: 0.78rem;
    font-weight: 700;
  }

  .load-view-toggle button.current {
    color: var(--text);
  }

  .load-view-toggle button:focus-visible {
    outline: 2px solid var(--graphic-focus);
    outline-offset: 2px;
  }

  .load-view-selector {
    position: absolute;
    bottom: 0;
    left: 0;
    width: 50%;
    height: 2px;
    background: var(--graphic-selection);
    transform: translateX(0);
    transition: transform var(--graphic-transition-duration) ease-out;
  }

  .load-view-toggle[data-view='storage'] .load-view-selector {
    transform: translateX(100%);
  }

  .unavailable-storage {
    margin-top: 1rem;
    color: var(--graphic-label-muted);
    font-size: 0.84rem;
    line-height: 1.45;
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

  .load-radar line.radar-active {
    fill: none;
    stroke: var(--graphic-console);
    stroke-linecap: round;
    stroke-width: 1.8;
  }

  .console-summary {
    text-align: center;
    margin-top: 0.75rem;
    color: var(--graphic-console);
    font-size: 0.7rem;
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
    box-sizing: border-box;
    width: var(--node-size);
    height: var(--node-size);
    border: 3px solid var(--unknown);
    border-radius: 50%;
    background: #0a1820;
    transition:
      transform var(--graphic-transition-duration) ease-out,
      width var(--graphic-transition-duration) ease-out,
      height var(--graphic-transition-duration) ease-out;
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
    overflow-wrap: break-word;
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

  .compact-node-field {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 1.5rem 2.25rem;
    min-height: 8rem;
    margin-top: 1.1rem;
  }

  .node-item.compact {
    position: relative;
    top: auto;
    left: auto;
    width: calc(var(--node-size) + 2rem);
    pointer-events: auto;
    transform: none;
  }

  .node-item.compact .node-button {
    width: 100%;
  }

  .node-item.compact .node-tooltip {
    width: min(14rem, 90vw);
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
    .compute-heading {
      align-items: stretch;
    }

    .load-view-toggle {
      justify-content: end;
    }

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
