<script lang="ts">
  import { invalidate } from '$app/navigation';
  import { tick } from 'svelte';
  import { readCookie } from '$lib/auth';
  import StatusIndicator from '$lib/components/StatusIndicator.svelte';
  import Timestamp from '$lib/components/Timestamp.svelte';
  import type { DisplayState } from '$lib/dashboard';
  import { ApiFailure, problemFrom, requestJSON } from '$lib/api/client';
  import {
    clusterFormFeedback,
    type ClusterFormFeedback,
  } from '$lib/proxmox-form';
  import type {
    AdministrativeMutationReceipt,
    ProxmoxClusterCreateRequest,
  } from '$lib/api/generated';

  let { data } = $props();
  let modalOpen = $state(false);
  let saving = $state(false);
  let formProblem = $state<ClusterFormFeedback | null>(null);
  let receipt = $state<AdministrativeMutationReceipt | null>(null);
  let addClusterButton = $state<HTMLButtonElement>();
  let firstClusterInput = $state<HTMLInputElement>();
  let clusterDialog = $state<HTMLDivElement>();

  async function createCluster(event: SubmitEvent) {
    event.preventDefault();
    const form = new FormData(event.currentTarget as HTMLFormElement);
    if (form.get('approve_target') !== 'on') {
      formProblem = {
        title: 'Target approval is required.',
        details: [
          'Approve this exact host and port before Espial attempts a connection.',
        ],
      };
      return;
    }
    const body: ProxmoxClusterCreateRequest = {
      display_name: text(form, 'display_name'),
      base_url: text(form, 'base_url'),
      approve_target: true,
      allow_insecure_http: form.get('allow_insecure_http') === 'on',
      trust_self_signed_certificate:
        form.get('trust_self_signed_certificate') === 'on',
      deployment_id: text(form, 'deployment_id'),
      token_id: text(form, 'token_id'),
      token_secret: text(form, 'token_secret'),
      interval_seconds: integer(form, 'interval_seconds', 300),
      expected_refresh_seconds: integer(form, 'expected_refresh_seconds', 300),
      request_timeout_seconds: integer(form, 'request_timeout_seconds', 10),
      storage_warning_percent: numberValue(form, 'storage_warning_percent', 80),
      storage_critical_percent: numberValue(
        form,
        'storage_critical_percent',
        90,
      ),
      swap_warning_percent: numberValue(form, 'swap_warning_percent', 10),
      swap_critical_percent: numberValue(form, 'swap_critical_percent', 25),
      io_wait_warning_percent: numberValue(form, 'io_wait_warning_percent', 5),
      io_wait_critical_percent: numberValue(
        form,
        'io_wait_critical_percent',
        15,
      ),
      expected_running_guests: text(form, 'expected_running_guests')
        .split(',')
        .map((value) => value.trim())
        .filter(Boolean),
    };
    saving = true;
    formProblem = null;
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
      const problem =
        error instanceof ApiFailure ? error.problem : problemFrom(error);
      formProblem = clusterFormFeedback(problem);
    } finally {
      saving = false;
    }
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
    if (total === 2)
      return { x: index === 0 ? 30 : 70, y: 50, r: 9 + Math.min(count, 10) };
    const angle = (-90 + (360 / total) * index) * (Math.PI / 180);
    return {
      x: 50 + Math.cos(angle) * 32,
      y: 50 + Math.sin(angle) * 32,
      r: 7 + Math.min(count, 11),
    };
  }

  function closeBackdrop(event: MouseEvent) {
    if (event.target === event.currentTarget) closeModal();
  }

  async function openModal() {
    formProblem = null;
    modalOpen = true;
    await tick();
    firstClusterInput?.focus();
  }

  async function closeModal() {
    if (saving) return;
    modalOpen = false;
    await tick();
    addClusterButton?.focus();
  }

  function handleWindowKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && modalOpen) {
      event.preventDefault();
      closeModal();
    }
  }

  function trapDialogFocus(event: KeyboardEvent) {
    if (event.key !== 'Tab') return;
    const controls = Array.from(
      clusterDialog?.querySelectorAll<HTMLElement>(
        'button:not(:disabled), input:not(:disabled)',
      ) ?? [],
    );
    if (controls.length === 0) return;
    const first = controls[0];
    const last = controls[controls.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

<svelte:head><title>Hypervisor - Espial</title></svelte:head>

<header class="page-header hypervisor-header">
  <div>
    <h1>Hypervisor</h1>
    <p>Proxmox cluster health, utilization, and active guest distribution.</p>
  </div>
  {#if data.canManage}
    <button
      bind:this={addClusterButton}
      type="button"
      class="primary-action"
      onclick={openModal}
    >
      Add cluster
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
    Development fixture. Production shows real Proxmox data only after a cluster
    is added.
  </div>
{/if}

{#if data.problem}
  <section class="problem-panel" aria-labelledby="hypervisor-problem-title">
    <h2 id="hypervisor-problem-title">
      {data.problem.status === 403
        ? 'Hypervisor access denied.'
        : 'Hypervisor data is unavailable.'}
    </h2>
    <p>{data.problem.message}</p>
    {#if data.problem.request_id}
      <p class="request-reference">
        Request ID: <code>{data.problem.request_id}</code>
      </p>
    {/if}
  </section>
{:else if data.clusters?.items.length}
  <div class="cluster-stack">
    {#each data.clusters.items as cluster (cluster.id)}
      <article class="cluster-panel" aria-labelledby={`cluster-${cluster.id}`}>
        <div class="cluster-columns">
          <section
            class="cluster-list-column"
            aria-label={`${cluster.name} hosts`}
          >
            <h2>Hosts</h2>
            {#each cluster.hosts as host}
              <div class="host-row">
                <div class="row-label">
                  <code>{host.id}</code>
                  <span
                    class={`indicator indicator-${host.swap_indicator}`}
                    title={`Swap ${indicatorLabel(host.swap_indicator)} ${percent(host.swap_percent)}`}
                    aria-label={`Swap ${indicatorLabel(host.swap_indicator)} ${percent(host.swap_percent)}`}
                  ></span>
                </div>
                <div
                  class="dual-bar"
                  aria-label={`${host.id} CPU ${percent(host.cpu_percent)}, RAM ${percent(host.ram_percent)}`}
                >
                  <span
                    class="cpu-fill"
                    style={`width:${bounded(host.cpu_percent)}%`}
                  ></span>
                  <span
                    class="ram-fill"
                    style={`width:${bounded(host.ram_percent)}%`}
                  ></span>
                </div>
                <small
                  >{host.cpu_cores.toFixed(0)} cores / {formatBytes(
                    host.ram_bytes,
                  )}</small
                >
              </div>
            {/each}
          </section>

          <section
            class="cluster-list-column storage-column"
            aria-label={`${cluster.name} storage`}
          >
            <h2>Storage</h2>
            {#each cluster.storage as item}
              <div class="storage-row">
                <div class="row-label">
                  <code>{item.id}</code>
                  <span
                    class={`indicator indicator-${item.io_wait_indicator}`}
                    title={`IO wait ${indicatorLabel(item.io_wait_indicator)} ${percent(item.io_wait_percent)}`}
                    aria-label={`IO wait ${indicatorLabel(item.io_wait_indicator)} ${percent(item.io_wait_percent)}`}
                  ></span>
                </div>
                <div
                  class="storage-bar"
                  aria-label={`${item.id} storage ${percent(item.used_percent)} used`}
                >
                  <span style={`width:${bounded(item.used_percent)}%`}></span>
                </div>
                <small
                  >{formatBytes(item.used_bytes)} used / {formatBytes(
                    item.total_bytes,
                  )} max</small
                >
              </div>
            {/each}
          </section>

          <section
            class="cluster-gauges"
            aria-labelledby={`cluster-${cluster.id}`}
          >
            <div class="cluster-title">
              <h2 id={`cluster-${cluster.id}`}>{cluster.name}</h2>
              <StatusIndicator state={displayState(cluster.state)} compact />
            </div>
            <div class="gauge-set">
              <div
                class="gauge"
                style={`--value:${gaugeSweep(cluster.totals.cpu_percent)}`}
              >
                <span>{percent(cluster.totals.cpu_percent)}</span>
                <small>CPU total</small>
              </div>
              <div
                class="gauge"
                style={`--value:${gaugeSweep(cluster.totals.ram_percent)}`}
              >
                <span>{percent(cluster.totals.ram_percent)}</span>
                <small>RAM total</small>
              </div>
              <div
                class="gauge"
                style={`--value:${gaugeSweep(cluster.totals.storage_percent)}`}
              >
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

          <section
            class="load-graph"
            aria-label={`${cluster.name} active VM distribution`}
          >
            <svg
              viewBox="0 0 100 100"
              role="img"
              aria-label="Active VM distribution by node"
            >
              {#if cluster.load_nodes.length > 2}
                <circle cx="50" cy="50" r="4" class="load-center" />
              {/if}
              {#each cluster.load_nodes as node, index}
                {@const point = nodePoint(
                  index,
                  cluster.load_nodes.length,
                  node.active_vms,
                )}
                {#if cluster.load_nodes.length > 2}
                  <line x1="50" y1="50" x2={point.x} y2={point.y} />
                {/if}
                <circle
                  class:single-node={cluster.load_nodes.length === 1}
                  cx={point.x}
                  cy={point.y}
                  r={point.r}
                >
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
    <span
      >Add a cluster with a read-only token before expecting Hypervisor data.</span
    >
  </section>
{/if}

{#if modalOpen}
  <div class="modal-backdrop" role="presentation" onclick={closeBackdrop}>
    <div
      bind:this={clusterDialog}
      class="modal"
      role="dialog"
      tabindex="-1"
      aria-modal="true"
      aria-labelledby="add-cluster-title"
      onkeydown={trapDialogFocus}
    >
      <div class="modal-heading">
        <div>
          <h2 id="add-cluster-title">Add Proxmox cluster</h2>
          <p id="add-cluster-description">
            Espial validates the read-only connection before saving it. The
            token secret is written to the configured secret store and is not
            returned.
          </p>
        </div>
        <button
          type="button"
          class="icon-button"
          aria-label="Close"
          onclick={closeModal}>Close</button
        >
      </div>
      <form
        class="cluster-form"
        aria-describedby="add-cluster-description"
        aria-busy={saving}
        onsubmit={createCluster}
      >
        <fieldset>
          <legend>Connection</legend>
          <div class="form-grid">
            <label class="form-field">
              <span>Cluster name</span>
              <input
                bind:this={firstClusterInput}
                name="display_name"
                required
                maxlength="128"
              />
              <small>The operator-facing name shown in Hypervisor.</small>
            </label>
            <label class="form-field">
              <span>Deployment ID</span>
              <input
                name="deployment_id"
                required
                pattern="[a-z][a-z0-9-]+"
                minlength="2"
                maxlength="63"
                placeholder="production-pve"
              />
              <small
                >Stable lowercase identity; this cannot be a display name.</small
              >
            </label>
            <label class="form-field form-field-wide">
              <span>Base URL</span>
              <input
                name="base_url"
                type="url"
                required
                placeholder="https://proxmox.example:8006"
              />
              <small
                >Origin only. Do not include an API path, query, or credentials.</small
              >
            </label>
          </div>
          <div class="security-options">
            <label class="check-row">
              <input name="approve_target" type="checkbox" required />
              <span>
                <strong>Approve this target</strong>
                <small
                  >Required. Allow read-only collection from the host and port above.</small
                >
              </span>
            </label>
            <label class="check-row">
              <input name="trust_self_signed_certificate" type="checkbox" />
              <span>
                <strong>Trust this cluster’s self-signed certificate</strong>
                <small
                  >Espial pins the certificate presented during setup and
                  rejects a later certificate change.</small
                >
              </span>
            </label>
            <label class="check-row">
              <input name="allow_insecure_http" type="checkbox" />
              <span>
                <strong>Allow insecure HTTP</strong>
                <small
                  >Not recommended. The API token will travel without TLS. Use
                  only for a temporary, isolated cluster.</small
                >
              </span>
            </label>
          </div>
        </fieldset>

        <fieldset>
          <legend>Read-only API token</legend>
          <div class="form-grid">
            <label class="form-field">
              <span>Token ID</span>
              <input
                name="token_id"
                required
                placeholder="espial@pve!monitoring"
                autocomplete="username"
              />
              <small>Include the user, realm, and token name.</small>
            </label>
            <label class="form-field">
              <span>Token secret</span>
              <input
                name="token_secret"
                type="password"
                required
                autocomplete="new-password"
              />
              <small>Accepted once and stored by secret reference.</small>
            </label>
          </div>
        </fieldset>

        <fieldset>
          <legend>Collection</legend>
          <div class="form-grid form-grid-three">
            <label class="form-field">
              <span>Collection interval</span>
              <div class="input-unit">
                <input
                  name="interval_seconds"
                  type="number"
                  min="1"
                  max="86400"
                  value="300"
                />
                <span>seconds</span>
              </div>
            </label>
            <label class="form-field">
              <span>Expected freshness</span>
              <div class="input-unit">
                <input
                  name="expected_refresh_seconds"
                  type="number"
                  min="1"
                  max="86400"
                  value="300"
                />
                <span>seconds</span>
              </div>
            </label>
            <label class="form-field">
              <span>Request timeout</span>
              <div class="input-unit">
                <input
                  name="request_timeout_seconds"
                  type="number"
                  min="1"
                  max="60"
                  value="10"
                />
                <span>seconds</span>
              </div>
            </label>
          </div>
        </fieldset>

        <fieldset>
          <legend>Warning and critical thresholds</legend>
          <p class="fieldset-description">
            Warning must be lower than critical for each percentage.
          </p>
          <div class="threshold-table">
            <span class="threshold-heading">Signal</span>
            <span class="threshold-heading">Warning</span>
            <span class="threshold-heading">Critical</span>
            {#each [['Storage used', 'storage', 80, 90], ['Swap used', 'swap', 10, 25], ['IO wait', 'io_wait', 5, 15]] as threshold}
              <span class="threshold-label">{threshold[0]}</span>
              <label>
                <span class="visually-hidden"
                  >{threshold[0]} warning percent</span
                >
                <div class="input-unit compact">
                  <input
                    name={`${threshold[1]}_warning_percent`}
                    type="number"
                    min="0"
                    max="100"
                    value={threshold[2]}
                  />
                  <span>%</span>
                </div>
              </label>
              <label>
                <span class="visually-hidden"
                  >{threshold[0]} critical percent</span
                >
                <div class="input-unit compact">
                  <input
                    name={`${threshold[1]}_critical_percent`}
                    type="number"
                    min="0"
                    max="100"
                    value={threshold[3]}
                  />
                  <span>%</span>
                </div>
              </label>
            {/each}
          </div>
        </fieldset>

        <fieldset>
          <legend>Guest expectations</legend>
          <label class="form-field">
            <span>Expected-running guests <small>Optional</small></span>
            <input
              name="expected_running_guests"
              placeholder="qemu:100, lxc:200"
            />
            <small
              >Comma-separated qemu:&lt;id&gt; or lxc:&lt;id&gt; values. Stopped
              guests in this list become critical.</small
            >
          </label>
        </fieldset>

        {#if formProblem}
          <section class="form-problem" role="alert" aria-live="assertive">
            <strong>{formProblem.title}</strong>
            <ul>
              {#each formProblem.details as detail}
                <li>{detail}</li>
              {/each}
            </ul>
            {#if formProblem.requestID}
              <small>Request ID: <code>{formProblem.requestID}</code></small>
            {/if}
          </section>
        {/if}
        <div class="modal-actions">
          <button type="button" class="secondary-action" onclick={closeModal}
            >Cancel</button
          >
          <button type="submit" disabled={saving}
            >{saving ? 'Validating...' : 'Validate and add'}</button
          >
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
    border: 1px solid var(--netdef-cyan);
    background: var(--netdef-cyan);
    color: var(--canvas);
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
    outline: 2px solid var(--netdef-cyan);
    outline-offset: 3px;
  }

  .cluster-columns {
    display: grid;
    grid-template-columns:
      minmax(190px, 1fr) minmax(230px, 1.25fr) minmax(260px, 1.2fr)
      minmax(220px, 1fr);
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
      conic-gradient(
        from 270deg,
        var(--color-accent, #00a9c9) var(--value),
        #243442 0 50%,
        transparent 0
      );
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
    background: rgba(0, 0, 0, 0.72);
    padding: 1rem;
    z-index: 50;
  }

  .modal {
    width: min(800px, 100%);
    max-height: calc(100dvh - 2rem);
    overflow: auto;
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
  }

  .modal-heading {
    position: sticky;
    top: 0;
    z-index: 1;
    display: flex;
    justify-content: space-between;
    gap: 1.5rem;
    align-items: flex-start;
    padding: 1.25rem 1.5rem 1rem;
    background: var(--surface-2);
    border-bottom: 1px solid var(--border);
  }

  .modal-heading h2 {
    margin-bottom: 0.35rem;
  }

  .modal-heading p {
    max-width: 62ch;
    margin: 0;
    color: var(--text-muted);
    line-height: 1.5;
  }

  .icon-button,
  .secondary-action {
    background: transparent;
    color: inherit;
    border: 1px solid var(--border-strong);
    padding: 0.55rem 0.75rem;
  }

  .cluster-form {
    display: grid;
    gap: 0;
  }

  .cluster-form fieldset {
    min-width: 0;
    margin: 0;
    padding: 1.25rem 1.5rem;
    border: 0;
    border-bottom: 1px solid var(--border);
  }

  .cluster-form legend {
    padding: 0;
    color: var(--text);
    font-size: 1rem;
    font-weight: 700;
  }

  .fieldset-description {
    margin: 0.4rem 0 0.9rem;
    color: var(--text-muted);
    line-height: 1.45;
  }

  .form-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1rem;
    margin-top: 0.85rem;
  }

  .form-grid-three {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .form-field {
    display: grid;
    gap: 0.4rem;
    min-width: 0;
    font-weight: 650;
  }

  .form-field-wide {
    grid-column: 1 / -1;
  }

  .form-field > small,
  .form-field > span small,
  .check-row small {
    color: var(--text-muted);
    font-weight: 400;
    line-height: 1.4;
  }

  .cluster-form input {
    min-width: 0;
    min-height: 2.65rem;
    background: var(--surface-1);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    color: inherit;
    padding: 0.65rem 0.7rem;
  }

  .cluster-form input::placeholder {
    color: #8295a5;
  }

  .security-options {
    display: grid;
    gap: 0.65rem;
    margin-top: 1rem;
  }

  .cluster-form .check-row {
    display: grid;
    grid-template-columns: auto 1fr;
    align-items: start;
    gap: 0.75rem;
    padding: 0.75rem;
    background: var(--surface-1);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    cursor: pointer;
  }

  .check-row > span {
    display: grid;
    gap: 0.2rem;
  }

  .cluster-form .check-row input {
    min-width: 1.15rem;
    min-height: 1.15rem;
    width: 1.15rem;
    height: 1.15rem;
    margin: 0.15rem 0 0;
    padding: 0;
    accent-color: var(--netdef-cyan);
  }

  .input-unit {
    display: flex;
    align-items: stretch;
    min-width: 0;
    background: var(--surface-1);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
  }

  .input-unit:focus-within {
    outline: 3px solid var(--netdef-cyan);
    outline-offset: 2px;
  }

  .input-unit input {
    width: 100%;
    border: 0;
    border-radius: 0;
  }

  .input-unit input:focus-visible {
    outline: 0;
  }

  .input-unit > span {
    display: grid;
    place-items: center;
    padding: 0 0.65rem;
    color: var(--text-muted);
    border-left: 1px solid var(--border);
    font-size: 0.86rem;
  }

  .threshold-table {
    display: grid;
    grid-template-columns: minmax(8rem, 1fr) repeat(2, minmax(7rem, 0.65fr));
    gap: 0.55rem 0.75rem;
    align-items: center;
  }

  .threshold-heading {
    padding-bottom: 0.3rem;
    color: var(--text-muted);
    font-size: 0.82rem;
    font-weight: 700;
    letter-spacing: 0.03em;
    text-transform: uppercase;
  }

  .threshold-label {
    font-weight: 650;
  }

  .input-unit.compact input {
    min-height: 2.35rem;
  }

  .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }

  .form-problem {
    margin: 1rem 1.5rem 0;
    padding: 0.9rem 1rem;
    color: var(--critical);
    background: var(--critical-surface);
    border: 1px solid #6c3034;
    border-left: 3px solid var(--critical);
  }

  .form-problem ul {
    margin: 0.5rem 0;
    padding-left: 1.2rem;
    color: var(--text);
  }

  .form-problem code {
    color: var(--text);
  }

  .modal-actions {
    position: sticky;
    bottom: 0;
    display: flex;
    justify-content: flex-end;
    gap: 0.75rem;
    align-items: center;
    padding: 1rem 1.5rem;
    background: var(--surface-2);
    border-top: 1px solid var(--border);
  }

  .cluster-form button:disabled {
    cursor: wait;
    opacity: 0.65;
  }

  @keyframes pulse-node {
    0%,
    100% {
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
    .form-grid,
    .form-grid-three {
      grid-template-columns: 1fr;
    }

    .modal-backdrop {
      place-items: stretch;
      padding: 0;
    }

    .modal {
      width: 100%;
      max-height: 100dvh;
      border: 0;
      border-radius: 0;
    }

    .modal-heading,
    .cluster-form fieldset,
    .modal-actions {
      padding-inline: 1rem;
    }

    .modal-heading {
      gap: 0.75rem;
    }

    .modal-heading p {
      font-size: 0.92rem;
    }

    .form-field-wide {
      grid-column: auto;
    }

    .threshold-table {
      grid-template-columns: minmax(7rem, 1fr) repeat(2, minmax(5rem, 0.7fr));
      gap: 0.5rem;
    }

    .threshold-heading {
      font-size: 0.72rem;
    }

    .form-problem {
      margin-inline: 1rem;
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
