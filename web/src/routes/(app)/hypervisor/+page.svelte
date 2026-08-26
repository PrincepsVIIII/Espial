<script lang="ts">
  import { invalidate } from '$app/navigation';
  import { tick } from 'svelte';
  import { readCookie } from '$lib/auth';
  import HostLoadGraphic from '$lib/components/HostLoadGraphic.svelte';
  import MetricMeter from '$lib/components/MetricMeter.svelte';
  import StatusIndicator from '$lib/components/StatusIndicator.svelte';
  import Timestamp from '$lib/components/Timestamp.svelte';
  import { ApiFailure, problemFrom, requestJSON } from '$lib/api/client';
  import {
    displayState,
    formatBytes,
    formatPercentage,
    normalizePercentage,
    statusLabel,
  } from '$lib/graphics';
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

  function indicatorLabel(value: string): string {
    return statusLabel(value);
  }

  function meterWidth(value: number): number {
    return normalizePercentage(value) ?? 0;
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
        <header class="cluster-title">
          <div>
            <h2 id={`cluster-${cluster.id}`}>{cluster.name}</h2>
            <p>
              {#if cluster.last_observed_at}
                Observed <Timestamp value={cluster.last_observed_at} />
              {:else}
                No observation received.
              {/if}
            </p>
          </div>
          <StatusIndicator state={displayState(cluster.state)} compact />
        </header>

        <div class="cluster-overview">
          <section
            class="cluster-list-column"
            aria-label={`${cluster.name} hosts`}
          >
            <h3>Hosts</h3>
            {#each cluster.hosts as host}
              <div class="host-row">
                <div class="row-label">
                  <code>{host.id}</code>
                  <span
                    class={`indicator indicator-${host.swap_indicator}`}
                    title={`Swap ${indicatorLabel(host.swap_indicator)} ${formatPercentage(host.swap_percent)}`}
                    >Swap {indicatorLabel(host.swap_indicator)}</span
                  >
                </div>
                <div
                  class="dual-bar"
                  role="img"
                  aria-label={`${host.id} CPU ${formatPercentage(host.cpu_percent)}, RAM ${formatPercentage(host.ram_percent)}`}
                >
                  <span
                    class="cpu-fill"
                    style={`width:${meterWidth(host.cpu_percent)}%`}
                  ></span>
                  <span
                    class="ram-fill"
                    style={`width:${meterWidth(host.ram_percent)}%`}
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
            <h3>Storage</h3>
            {#each cluster.storage as item}
              <div class="storage-row">
                <div class="row-label">
                  <code>{item.id}</code>
                  <span
                    class={`indicator indicator-${item.io_wait_indicator}`}
                    title={`IO wait ${indicatorLabel(item.io_wait_indicator)} ${formatPercentage(item.io_wait_percent)}`}
                    >IO {indicatorLabel(item.io_wait_indicator)}</span
                  >
                </div>
                <div
                  class="storage-bar"
                  role="img"
                  aria-label={`${item.id} storage ${formatPercentage(item.used_percent)} used`}
                >
                  <span style={`width:${meterWidth(item.used_percent)}%`}
                  ></span>
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
            class="cluster-meters"
            aria-label={`${cluster.name} total utilization`}
          >
            <h3>Cluster utilization</h3>
            <div class="meter-set">
              <MetricMeter
                label="CPU total"
                value={cluster.totals.cpu_percent}
              />
              <MetricMeter
                label="RAM total"
                value={cluster.totals.ram_percent}
              />
              <MetricMeter
                label="Storage total"
                value={cluster.totals.storage_percent}
              />
            </div>
          </section>
        </div>

        <HostLoadGraphic
          id={`cluster-${cluster.id}-load`}
          clusterName={cluster.name}
          clusterState={cluster.state}
          freshness={cluster.freshness}
          lastObservedAt={cluster.last_observed_at}
          hosts={cluster.hosts}
          loadNodes={cluster.load_nodes}
        />
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
                <strong>Approve this exact target</strong>
                <small
                  >Required. Allow read-only collection from the host and port
                  above.</small
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
    display: grid;
    gap: 1rem;
  }

  .cluster-panel:focus {
    outline: 2px solid var(--netdef-cyan);
    outline-offset: 3px;
  }

  .cluster-title {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1rem;
    padding-bottom: 0.9rem;
    border-bottom: 1px solid var(--border);
  }

  .cluster-title h2,
  .cluster-title p {
    margin: 0;
  }

  .cluster-title p {
    margin-top: 0.35rem;
    color: var(--text-muted);
  }

  .cluster-overview {
    display: grid;
    grid-template-columns: minmax(190px, 1fr) minmax(230px, 1.2fr) minmax(
        250px,
        1fr
      );
    gap: 1rem;
    align-items: stretch;
  }

  .cluster-list-column,
  .cluster-meters {
    border-left: 1px solid rgba(151, 176, 190, 0.22);
    padding-left: 0.9rem;
  }

  .cluster-list-column h3,
  .cluster-meters h3 {
    margin: 0 0 0.7rem;
    font-size: 1rem;
  }

  .host-row,
  .storage-row {
    display: grid;
    gap: 0.25rem;
    margin-bottom: 0.7rem;
  }

  .row-label {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 0.75rem;
  }

  .indicator {
    display: inline-flex;
    gap: 0.35rem;
    align-items: center;
    color: var(--unknown);
    font-size: 0.72rem;
    font-weight: 700;
  }

  .indicator::before {
    width: 0.58rem;
    height: 0.58rem;
    border: 2px solid currentColor;
    border-radius: 50%;
    content: '';
    flex: 0 0 auto;
  }

  .indicator-healthy {
    color: var(--healthy);
  }

  .indicator-warning {
    color: var(--warning);
  }

  .indicator-warning::before {
    border-radius: 0;
    transform: rotate(45deg) scale(0.8);
  }

  .indicator-critical {
    color: var(--critical);
  }

  .indicator-critical::before {
    border-radius: 0;
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

  .meter-set {
    display: grid;
    gap: 0.9rem;
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

  @media (max-width: 1100px) {
    .cluster-overview {
      grid-template-columns: 1fr 1fr;
    }

    .cluster-meters {
      grid-column: 1 / -1;
    }

    .meter-set {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }

  @media (max-width: 680px) {
    .cluster-overview,
    .form-grid,
    .form-grid-three {
      grid-template-columns: 1fr;
    }

    .cluster-meters {
      grid-column: auto;
    }

    .meter-set {
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
    .cluster-meters {
      border-left: 0;
      padding-left: 0;
      border-top: 1px solid rgba(151, 176, 190, 0.22);
      padding-top: 0.9rem;
    }
  }
</style>
