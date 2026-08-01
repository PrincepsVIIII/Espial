<script lang="ts">
  import { invalidate } from '$app/navigation';
  import { administrativeMutation } from '$lib/administration';
  import { problemFrom } from '$lib/api/client';
  import Timestamp from '$lib/components/Timestamp.svelte';
  import type {
    AdministrativeMutationReceipt,
    RedactedWebsiteMonitorAPIView,
    WebsiteMonitorReplacement,
  } from '$lib/api/generated';
  let { data } = $props();
  let selected = $state<RedactedWebsiteMonitorAPIView | null>(null);
  let pending = $state('');
  let problem = $state<ReturnType<typeof problemFrom> | null>(null);
  let receipt = $state<AdministrativeMutationReceipt | null>(null);
  let receiptMessage = $state('');

  function clearEditor() {
    selected = null;
    problem = null;
  }
  async function save(event: SubmitEvent) {
    event.preventDefault();
    pending = 'save';
    problem = null;
    receipt = null;
    const replacing = selected;
    const values = new FormData(event.currentTarget as HTMLFormElement);
    const headerName = String(values.get('header_name') ?? '').trim();
    const headerReference = String(values.get('header_reference') ?? '').trim();
    const body: WebsiteMonitorReplacement = {
      display_name: String(values.get('display_name') ?? '').trim(),
      enabled: values.get('enabled') === 'on',
      url: String(values.get('url') ?? '').trim(),
      interval_seconds: Number(values.get('interval_seconds')),
      timeout_ms: Number(values.get('timeout_ms')),
      warning_latency_ms: Number(values.get('warning_latency_ms')),
      allowed_statuses: String(values.get('allowed_statuses') ?? '')
        .split(',')
        .map((item) => Number(item.trim()))
        .filter((item) => Number.isInteger(item)),
      content_match: String(values.get('content_match') ?? ''),
      follow_redirects: values.get('follow_redirects') === 'on',
      max_redirects: Number(values.get('max_redirects')),
      secret_headers:
        headerName && headerReference
          ? [{ name: headerName, secret_reference: headerReference }]
          : [],
    };
    try {
      receipt = await administrativeMutation(
        replacing
          ? `/api/v1/website-monitors/${replacing.id}`
          : '/api/v1/website-monitors',
        replacing ? 'PUT' : 'POST',
        body,
        replacing?.version,
      );
      receiptMessage = replacing
        ? 'Monitor replacement committed.'
        : 'Website monitor created.';
      selected = null;
      await invalidate('espial:monitoring');
    } catch (error) {
      problem = problemFrom(error);
    } finally {
      pending = '';
    }
  }
  async function check(monitor: RedactedWebsiteMonitorAPIView) {
    pending = `check-${monitor.id}`;
    problem = null;
    receipt = null;
    try {
      receipt = await administrativeMutation(
        `/api/v1/website-monitors/${monitor.id}/check`,
        'POST',
        {},
        monitor.version,
      );
      receiptMessage = 'Manual check accepted by the bounded scheduler.';
      await invalidate('espial:monitoring');
    } catch (error) {
      problem = problemFrom(error);
    } finally {
      pending = '';
    }
  }
</script>

<svelte:head><title>Website monitors · Espial</title></svelte:head>
<header class="page-header">
  <div>
    <p class="eyebrow">Approved supervised collection</p>
    <h1>Website monitors</h1>
    <p class="page-description">
      Create exact endpoints within Core's host, address, and port allowlists.
      Protected headers are write-only secret references.
    </p>
  </div>
</header>
<nav class="section-navigation" aria-label="Webpage views">
  <a href="/webpages">Availability</a><a
    class="active"
    aria-current="page"
    href="/webpages/monitors">Monitors</a
  >
</nav>
{#if problem ?? data.problem}{@const currentProblem = problem ?? data.problem}
  <div class="inline-problem" role="alert">
    <strong
      >{currentProblem?.status === 403
        ? 'Monitor administration denied'
        : 'Monitor operation unavailable'}</strong
    ><span>{currentProblem?.message}</span>{#if currentProblem?.request_id}<code
        >{currentProblem.request_id}</code
      >{/if}
  </div>{/if}
{#if receipt}<div class="mutation-receipt" role="status" aria-live="polite">
    <strong>{receiptMessage}</strong><span
      >Request ID: <code>{receipt.request_id}</code></span
    >{#if receipt.audit_url}<a href={receipt.audit_url}
        >View matching audit record</a
      >{/if}
  </div>{/if}
{#if !data.problem}<div class="website-admin-layout">
    <section class="admin-section" aria-labelledby="monitor-list-title">
      <div class="operational-section-heading">
        <div>
          <p class="eyebrow">Redacted configuration</p>
          <h2 id="monitor-list-title">Configured monitors</h2>
        </div>
        <span class="section-count">{data.monitors.length} configured</span>
      </div>
      {#if data.monitors.length}<div class="table-frame">
          <table class="resource-table">
            <thead
              ><tr
                ><th>Monitor</th><th>Runtime</th><th>Schedule</th><th
                  >Updated</th
                ><th>Actions</th></tr
              ></thead
            ><tbody
              >{#each data.monitors as monitor}<tr
                  ><th
                    ><span class="resource-name">{monitor.display_name}</span
                    ><small>{monitor.url}</small></th
                  ><td>{monitor.runtime_state.replaceAll('_', ' ')}</td><td
                    >Every {monitor.interval_seconds}s</td
                  ><td><Timestamp value={monitor.updated_at} /></td><td
                    class="monitor-actions"
                    ><button
                      class="table-action"
                      type="button"
                      onclick={() => (selected = monitor)}>Replace</button
                    ><button
                      class="table-action"
                      type="button"
                      disabled={pending !== '' || !monitor.enabled}
                      onclick={() => check(monitor)}
                      >{pending === `check-${monitor.id}`
                        ? 'Requesting…'
                        : 'Check now'}</button
                    ></td
                  ></tr
                >{/each}</tbody
            >
          </table>
        </div>
        {#if data.nextCursor}<a
            class="text-link"
            href={`/webpages/monitors?cursor=${encodeURIComponent(data.nextCursor)}`}
            >Next page</a
          >{/if}{:else}<div class="empty-state">
          <strong>No website monitors configured.</strong><span
            >The network allowlist and adapter executable must be configured in
            Core before a monitor can be accepted.</span
          >
        </div>{/if}
    </section>
    <section class="admin-section" aria-labelledby="monitor-editor-title">
      <div class="editor-heading">
        <div>
          <p class="eyebrow">{selected ? 'Full replacement' : 'New monitor'}</p>
          <h2 id="monitor-editor-title">
            {selected?.display_name ?? 'Create monitor'}
          </h2>
        </div>
        {#if selected}<button
            class="text-button"
            type="button"
            onclick={clearEditor}>Create instead</button
          >{/if}
      </div>
      {#key selected?.id ?? 'new'}<form class="admin-form" onsubmit={save}>
          <label
            >Display name<input
              name="display_name"
              required
              maxlength="128"
              value={selected?.display_name ?? ''}
            /></label
          ><label class="checkbox-field"
            ><input
              name="enabled"
              type="checkbox"
              checked={selected?.enabled ?? true}
            /> Enabled</label
          >
          <label
            >Exact URL<input
              name="url"
              type="url"
              required
              maxlength="2048"
              value={selected?.url ?? 'https://'}
              autocomplete="off"
            /></label
          >
          <div class="compact-fields">
            <label
              >Interval seconds<input
                name="interval_seconds"
                type="number"
                min="1"
                max="86400"
                required
                value={selected?.interval_seconds ?? 60}
              /></label
            ><label
              >Timeout ms<input
                name="timeout_ms"
                type="number"
                min="100"
                max="60000"
                required
                value={selected?.timeout_ms ?? 5000}
              /></label
            >
          </div>
          <label
            >Warning latency ms<input
              name="warning_latency_ms"
              type="number"
              min="0"
              max="59999"
              required
              value={selected?.warning_latency_ms ?? 1000}
            /></label
          ><label
            >Allowed HTTP statuses<input
              name="allowed_statuses"
              required
              value={selected?.allowed_statuses.join(', ') ?? '200'}
            /></label
          ><label
            >Exact content to find <span class="optional"
              >optional, bounded</span
            ><input
              name="content_match"
              maxlength="4096"
              autocomplete="off"
            /></label
          >
          <label class="checkbox-field"
            ><input
              name="follow_redirects"
              type="checkbox"
              checked={selected?.follow_redirects ?? false}
            /> Follow approved redirects</label
          ><label
            >Maximum redirects<input
              name="max_redirects"
              type="number"
              min="0"
              max="5"
              required
              value={selected?.max_redirects ?? 0}
            /></label
          >
          <label
            >Secret header name <span class="optional">optional</span><input
              name="header_name"
              maxlength="64"
              autocomplete="off"
            /></label
          ><label
            >Secret reference <span class="optional">optional</span><input
              name="header_reference"
              maxlength="128"
              autocomplete="off"
            /></label
          >
          <p class="form-note">
            Content match values and secret references are write-only. A
            replacement requires re-entering them. URL user information and
            secret-like query keys are rejected.
          </p>
          <button type="submit" disabled={pending !== ''}
            >{pending === 'save'
              ? 'Committing…'
              : selected
                ? 'Replace monitor'
                : 'Create monitor'}</button
          >
        </form>{/key}
    </section>
  </div>{/if}

<style>
  .website-admin-layout {
    display: grid;
    grid-template-columns: minmax(0, 1.45fr) minmax(19rem, 0.75fr);
    gap: 1rem;
    align-items: start;
  }
  .monitor-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.45rem;
  }
  .compact-fields {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.65rem;
  }
  @media (max-width: 900px) {
    .website-admin-layout,
    .compact-fields {
      grid-template-columns: 1fr;
    }
  }
</style>
