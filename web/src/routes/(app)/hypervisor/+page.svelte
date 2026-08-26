<script lang="ts">
  import { invalidate } from '$app/navigation';
  import { readCookie } from '$lib/auth';
  import StatusIndicator from '$lib/components/StatusIndicator.svelte';
  import Timestamp from '$lib/components/Timestamp.svelte';
  import type { DisplayState } from '$lib/dashboard';
  import {
    ApiFailure,
    problemFrom,
    requestJSON,
  } from '$lib/api/client';
  import type {
    AdministrativeMutationReceipt,
    ProxmoxClusterCreateRequest,
  } from '$lib/api/generated';

  let { data } = $props();
  let modalOpen = $state(false);
  let saving = $state(false);
  let formProblem = $state('');
  let receipt = $state<AdministrativeMutationReceipt | null>(null);

  async function createCluster(event: SubmitEvent) {
    event.preventDefault();
    const form = new FormData(event.currentTarget as HTMLFormElement);
    const body: ProxmoxClusterCreateRequest = {
      display_name: text(form, 'display_name'),
      base_url: text(form, 'base_url'),
      approve_target: true,
      deployment_id: text(form, 'deployment_id'),
      token_id: text(form, 'token_id'),
      token_secret: text(form, 'token_secret'),
      interval_seconds: integer(form, 'interval_seconds', 300),
      expected_refresh_seconds: integer(form, 'expected_refresh_seconds', 300),
      request_timeout_seconds: integer(form, 'request_timeout_seconds', 10),
      storage_warning_percent: numberValue(form, 'storage_warning_percent', 80),
      storage_critical_percent: numberValue(form, 'storage_critical_percent', 90),
      swap_warning_percent: numberValue(form, 'swap_warning_percent', 10),
      swap_critical_percent: numberValue(form, 'swap_critical_percent', 25),
      io_wait_warning_percent: numberValue(form, 'io_wait_warning_percent', 5),
      io_wait_critical_percent: numberValue(form, 'io_wait_critical_percent', 15),
      expected_running_guests: text(form, 'expected_running_guests')
        .split(',')
        .map((value) => value.trim())
        .filter(Boolean),
    };
    saving = true;
    formProblem = '';
    receipt = null;
    try {
      receipt = await requestJSON<AdministrativeMutationReceipt>(
        fetch,
        '/api/v1/hypervisor/proxmox/clusters',
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'X-CSRF-Token': readCookie('espial_csrf'),
          },
          body: JSON.stringify(body),
        },
      );
      modalOpen = false;
      await invalidate('espial:monitoring');
    } catch (error) {
      const problem = error instanceof ApiFailure ? error.problem : problemFrom(error);
      formProblem = formatProblem(problem);
    } finally {
      saving = false;
    }
  }

  function formatProblem(problem: ReturnType<typeof problemFrom>): string {
    const fields = problem.fields?.map((field) => `${field.field}: ${field.code}`).join(', ');
    return [
      problem.message,
      fields ? `Fields: ${fields}` : '',
      problem.request_id ? `Request ID: ${problem.request_id}` : '',
    ]
      .filter(Boolean)
      .join(' ');
  }

  function text(form: FormData, key: string): string {
    return String(form.get(key) ?? '').trim();
  }

  function integer(form: FormData, key: string, fallback: number): number {
    const value = Number.parseInt(text(form, key), 10);
    return Number.isFinite(value) ? value : fallback;
  }

  function numberValue(form: FormData, key: string, fallback: number): number {
    const value = Number.parseFloat(text(form, key));
    return Number.isFinite(value) ? value : fallback;
  }

  function formatBytes(value: number): string {
    if (!value) return 'Unknown';
    const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    let amount = value;
    let unit = 0;
    while (amount >= 1024 && unit < units.length - 1) {
      amount /= 1024;
      unit += 1;
    }
    return `${amount >= 10 ? amount.toFixed(0) : amount.toFixed(1)} ${units[unit]}`;
  }

  function percent(value: number): string {
    return `${Math.round(value)}%`;
  }

  function bounded(value: number): number {
    if (!Number.isFinite(value) || value < 0) return 0;
    if (value > 100) return 100;
    return value;
  }

  function gaugeSweep(value: number): string {
    return `${bounded(value) / 2}%`;
  }

  function indicatorLabel(value: string): string {
    if (value === 'critical') return 'Critical';
    if (value === 'warning') return 'Warning';
    if (value === 'healthy') return 'Healthy';
    return 'Unknown';
  }

  function displayState(value: string): DisplayState {
    if (
      value === 'healthy' ||
      value === 'warning' ||
      value === 'critical' ||
      value === 'unknown' ||
      value === 'stale' ||
      value === 'maintenance' ||
      value === 'disabled'
    ) {
      return value;
    }
    return 'unknown';
  }

  function nodePoint(index: number, total: number, count: number) {
    if (total === 1) return { x: 50, y: 50, r: 12 + Math.min(count, 12) };
    if (total === 2) return { x: index === 0 ? 30 : 70, y: 50, r: 9 + Math.min(count, 10) };
    const angle = (-90 + (360 / total) * index) * (Math.PI / 180);
    return {
      x: 50 + Math.cos(angle) * 32,
      y: 50 + Math.sin(angle) * 32,
      r: 7 + Math.min(count, 11),
    };
  }

  function closeBackdrop(event: MouseEvent) {
    if (event.target === event.currentTarget) modalOpen = false;
  }
</script>

<svelte:head><title>Hypervisor - Espial</title></svelte:head>

<header class="page-header hypervisor-header">
  <div>
    <h1>Hypervisor</h1>
    <p>Proxmox cluster health, utilization, and active guest distribution.</p>
  </div>
  {#if data.canManage}
    <button type="button" class="primary-action" onclick={() => (modalOpen = true)}>
      + Add Cluster
    </button>
  {/if}
</header>

{#if receipt}
  <div class="receipt" role="status">
    <strong>Cluster integration created.</strong>
    <span>Request ID: <code>{receipt.request_id}</code></span>
    {#if receipt.audit_url}
      <a class="text-link" href={receipt.audit_url}>Audit record</a>
    {/if}
  </div>
{/if}

{#if data.demo}
  <div class="demo-notice" role="status">
    Development fixture. Production shows real Proxmox data only after a cluster is added.
  </div>
{/if}

{#if data.problem}
  <section class="problem-panel" aria-labelledby="hypervisor-problem-title">
    <h2 id="hypervisor-problem-title">
      {data.problem.status === 403 ? 'Hypervisor access denied.' : 'Hypervisor data is unavailable.'}
    </h2>
    <p>{data.problem.message}</p>
    {#if data.problem.request_id}
      <p class="request-reference">Request ID: <code>{data.problem.request_id}</code></p>
    {/if}
  </section>
{:else if data.clusters?.items.length}
  <div class="cluster-stack">
    {#each data.clusters.items as cluster (cluster.id)}
      <article class="cluster-panel" aria-labelledby={`cluster-${cluster.id}`}>
        <div class="cluster-columns">
          <section class="cluster-list-column" aria-label={`${cluster.name} hosts`}>
            <h2>Hosts</h2>
            {#each cluster.hosts as host}
              <div class="host-row">
                <div class="row-label">
                  <code>{host.id}</code>
                  <span class={`indicator indicator-${host.swap_indicator}`} title={`Swap ${indicatorLabel(host.swap_indicator)} ${percent(host.swap_percent)}`} aria-label={`Swap ${indicatorLabel(host.swap_indicator)} ${percent(host.swap_percent)}`}></span>
                </div>
                <div class="dual-bar" aria-label={`${host.id} CPU ${percent(host.cpu_percent)}, RAM ${percent(host.ram_percent)}`}>
                  <span class="cpu-fill" style={`width:${bounded(host.cpu_percent)}%`}></span>
                  <span class="ram-fill" style={`width:${bounded(host.ram_percent)}%`}></span>
                </div>
                <small>{host.cpu_cores.toFixed(0)} cores / {formatBytes(host.ram_bytes)}</small>
              </div>
            {/each}
          </section>

          <section class="cluster-list-column storage-column" aria-label={`${cluster.name} storage`}>
            <h2>Storage</h2>
            {#each cluster.storage as item}
              <div class="storage-row">
                <div class="row-label">
                  <code>{item.id}</code>
                  <span class={`indicator indicator-${item.io_wait_indicator}`} title={`IO wait ${indicatorLabel(item.io_wait_indicator)} ${percent(item.io_wait_percent)}`} aria-label={`IO wait ${indicatorLabel(item.io_wait_indicator)} ${percent(item.io_wait_percent)}`}></span>
                </div>
                <div class="storage-bar" aria-label={`${item.id} storage ${percent(item.used_percent)} used`}>
                  <span style={`width:${bounded(item.used_percent)}%`}></span>
                </div>
                <small>{formatBytes(item.used_bytes)} used / {formatBytes(item.total_bytes)} max</small>
              </div>
            {/each}
          </section>

          <section class="cluster-gauges" aria-labelledby={`cluster-${cluster.id}`}>
            <div class="cluster-title">
              <h2 id={`cluster-${cluster.id}`}>{cluster.name}</h2>
              <StatusIndicator state={displayState(cluster.state)} compact />
            </div>
            <div class="gauge-set">
              <div class="gauge" style={`--value:${gaugeSweep(cluster.totals.cpu_percent)}`}>
                <span>{percent(cluster.totals.cpu_percent)}</span>
                <small>CPU total</small>
              </div>
              <div class="gauge" style={`--value:${gaugeSweep(cluster.totals.ram_percent)}`}>
                <span>{percent(cluster.totals.ram_percent)}</span>
                <small>RAM total</small>
              </div>
              <div class="gauge" style={`--value:${gaugeSweep(cluster.totals.storage_percent)}`}>
                <span>{percent(cluster.totals.storage_percent)}</span>
                <small>Storage total</small>
              </div>
            </div>
            <p class="cluster-time">
              {#if cluster.last_observed_at}
                Observed <Timestamp value={cluster.last_observed_at} />
              {:else}
                No observation received.
              {/if}
            </p>
          </section>

          <section class="load-graph" aria-label={`${cluster.name} active VM distribution`}>
            <svg viewBox="0 0 100 100" role="img" aria-label="Active VM distribution by node">
              {#if cluster.load_nodes.length > 2}
                <circle cx="50" cy="50" r="4" class="load-center" />
              {/if}
              {#each cluster.load_nodes as node, index}
                {@const point = nodePoint(index, cluster.load_nodes.length, node.active_vms)}
                {#if cluster.load_nodes.length > 2}
                  <line x1="50" y1="50" x2={point.x} y2={point.y} />
                {/if}
                <circle class:single-node={cluster.load_nodes.length === 1} cx={point.x} cy={point.y} r={point.r}>
                  <title>{node.id}: {node.active_vms} active VMs</title>
                </circle>
                <text x={point.x} y={point.y + point.r + 7}>{node.id}</text>
              {/each}
            </svg>
          </section>
        </div>
      </article>
    {/each}
  </div>
{:else}
  <section class="empty-state">
    <strong>No Proxmox clusters are configured.</strong>
    <span>Add a cluster with a read-only token before expecting Hypervisor data.</span>
  </section>
{/if}

{#if modalOpen}
  <div class="modal-backdrop" role="presentation" onclick={closeBackdrop}>
    <div class="modal" role="dialog" aria-modal="true" aria-labelledby="add-cluster-title">
      <div class="modal-heading">
        <h2 id="add-cluster-title">Add Proxmox cluster</h2>
        <button type="button" class="icon-button" aria-label="Close" onclick={() => (modalOpen = false)}>x</button>
      </div>
      <form class="cluster-form" onsubmit={createCluster}>
        <label>Cluster name <input name="display_name" required maxlength="128" /></label>
        <label>Base URL <input name="base_url" required placeholder="https://proxmox.example:8006" /></label>
        <label class="check-row">
          <input name="approve_target" type="checkbox" required />
          <span>Approve this Proxmox target for read-only collection</span>
        </label>
        <label>Deployment ID <input name="deployment_id" required pattern="[a-z][a-z0-9-]+" minlength="2" maxlength="63" /></label>
        <label>Token ID <input name="token_id" required /></label>
        <label>Token secret <input name="token_secret" type="password" required autocomplete="new-password" /></label>
        <label>Collection interval <input name="interval_seconds" type="number" min="1" max="86400" value="300" /></label>
        <label>Expected freshness <input name="expected_refresh_seconds" type="number" min="1" max="86400" value="300" /></label>
        <label>Request timeout <input name="request_timeout_seconds" type="number" min="1" max="60" value="10" /></label>
        <div class="threshold-grid">
          <label>Storage warning <input name="storage_warning_percent" type="number" min="0" max="100" value="80" /></label>
          <label>Storage critical <input name="storage_critical_percent" type="number" min="0" max="100" value="90" /></label>
          <label>Swap warning <input name="swap_warning_percent" type="number" min="0" max="100" value="10" /></label>
          <label>Swap critical <input name="swap_critical_percent" type="number" min="0" max="100" value="25" /></label>
          <label>IO wait warning <input name="io_wait_warning_percent" type="number" min="0" max="100" value="5" /></label>
          <label>IO wait critical <input name="io_wait_critical_percent" type="number" min="0" max="100" value="15" /></label>
        </div>
        <label>Expected-running guests <input name="expected_running_guests" placeholder="qemu:100,lxc:200" /></label>
        {#if formProblem}
          <p class="form-problem" role="alert">{formProblem}</p>
        {/if}
        <div class="modal-actions">
          <button type="button" class="secondary-action" onclick={() => (modalOpen = false)}>Cancel</button>
          <button type="submit" disabled={saving}>{saving ? 'Validating...' : 'Validate and add'}</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<style>
  .hypervisor-header {
    align-items: flex-start;
    gap: 1rem;
  }

  .primary-action,
  .cluster-form button[type='submit'] {
    border: 1px solid var(--color-accent, #00a9c9);
    background: var(--color-accent, #00a9c9);
    color: #031317;
    font-weight: 700;
    padding: 0.65rem 0.9rem;
  }

  .receipt,
  .demo-notice,
  .problem-panel,
  .empty-state {
    border: 1px solid rgba(151, 176, 190, 0.28);
    background: #101b24;
    padding: 1rem;
  }

  .cluster-stack {
    display: grid;
    gap: 1rem;
  }

  .cluster-panel {
    border: 1px solid rgba(151, 176, 190, 0.28);
    background: #101b24;
    padding: 1rem;
  }

  .cluster-panel:focus {
    outline: 2px solid var(--color-accent, #00a9c9);
    outline-offset: 3px;
  }

  .cluster-columns {
    display: grid;
    grid-template-columns: minmax(190px, 1fr) minmax(230px, 1.25fr) minmax(260px, 1.2fr) minmax(220px, 1fr);
    gap: 1rem;
    align-items: stretch;
  }

  .cluster-list-column,
  .cluster-gauges,
  .load-graph {
    border-left: 1px solid rgba(151, 176, 190, 0.22);
    padding-left: 0.9rem;
  }

  .cluster-list-column h2,
  .cluster-gauges h2 {
    margin: 0 0 0.7rem;
    font-size: 1rem;
  }

  .host-row,
  .storage-row {
    display: grid;
    gap: 0.25rem;
    margin-bottom: 0.7rem;
  }

  .row-label,
  .cluster-title {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 0.75rem;
  }

  .indicator {
    width: 0.72rem;
    height: 0.72rem;
    border-radius: 50%;
    background: #8a96a0;
    flex: 0 0 auto;
  }

  .indicator-healthy {
    background: #39b86f;
  }

  .indicator-warning {
    background: #e1ad33;
  }

  .indicator-critical {
    background: #e05a4f;
  }

  .dual-bar,
  .storage-bar {
    position: relative;
    height: 1.1rem;
    background: #071016;
    border: 1px solid rgba(151, 176, 190, 0.28);
    overflow: hidden;
  }

  .dual-bar span,
  .storage-bar span {
    position: absolute;
    inset-block: 0;
    left: 0;
  }

  .cpu-fill {
    background: rgba(0, 169, 201, 0.55);
  }

  .ram-fill {
    background: rgba(225, 173, 51, 0.55);
    top: 50%;
  }

  .storage-bar {
    height: 1.55rem;
  }

  .storage-bar span {
    background: rgba(0, 169, 201, 0.62);
  }

  .gauge-set {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0.75rem;
  }

  .gauge {
    display: grid;
    place-items: center;
    align-content: end;
    min-height: 7rem;
    background:
      radial-gradient(circle at 50% 80%, #101b24 0 46%, transparent 47%),
      conic-gradient(from 270deg, var(--color-accent, #00a9c9) var(--value), #243442 0 50%, transparent 0);
    clip-path: inset(0 0 45% 0);
  }

  .gauge span {
    font-size: 1.35rem;
    font-weight: 700;
  }

  .cluster-time {
    margin: 0.7rem 0 0;
    color: #bac6cf;
  }

  .load-graph svg {
    width: 100%;
    min-height: 13rem;
  }

  .load-graph line {
    stroke: rgba(151, 176, 190, 0.4);
  }

  .load-graph circle {
    fill: rgba(0, 169, 201, 0.72);
    stroke: #d7f8ff;
  }

  .load-graph .load-center {
    fill: #e1ad33;
  }

  .single-node {
    animation: pulse-node 1.6s ease-in-out infinite;
  }

  .load-graph text {
    fill: #d8e6ee;
    font-size: 0.42rem;
    text-anchor: middle;
  }

  .modal-backdrop {
    position: fixed;
    inset: 0;
    display: grid;
    place-items: center;
    background: rgba(0, 0, 0, 0.62);
    z-index: 50;
  }

  .modal {
    width: min(680px, calc(100vw - 2rem));
    max-height: calc(100vh - 2rem);
    overflow: auto;
    background: #101b24;
    border: 1px solid rgba(151, 176, 190, 0.42);
    padding: 1rem;
  }

  .modal-heading,
  .modal-actions {
    display: flex;
    justify-content: space-between;
    gap: 0.75rem;
    align-items: center;
  }

  .icon-button,
  .secondary-action {
    background: transparent;
    color: inherit;
    border: 1px solid rgba(151, 176, 190, 0.42);
    padding: 0.45rem 0.7rem;
  }

  .cluster-form,
  .cluster-form label {
    display: grid;
    gap: 0.4rem;
  }

  .cluster-form {
    gap: 0.75rem;
  }

  .cluster-form input {
    min-width: 0;
    background: #071016;
    border: 1px solid rgba(151, 176, 190, 0.36);
    color: inherit;
    padding: 0.55rem;
  }

  .cluster-form .check-row {
    display: flex;
    align-items: center;
    gap: 0.65rem;
  }

  .cluster-form .check-row input {
    min-width: 1rem;
    width: 1rem;
    height: 1rem;
    padding: 0;
  }

  .threshold-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0.75rem;
  }

  .form-problem {
    color: #ffd3ce;
  }

  @keyframes pulse-node {
    0%, 100% {
      opacity: 0.72;
    }
    50% {
      opacity: 1;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .single-node {
      animation: none;
    }
  }

  @media (max-width: 1100px) {
    .cluster-columns {
      grid-template-columns: 1fr 1fr;
    }
  }

  @media (max-width: 680px) {
    .cluster-columns,
    .threshold-grid {
      grid-template-columns: 1fr;
    }

    .cluster-list-column,
    .cluster-gauges,
    .load-graph {
      border-left: 0;
      padding-left: 0;
      border-top: 1px solid rgba(151, 176, 190, 0.22);
      padding-top: 0.9rem;
    }
  }
</style>
