<script lang="ts">
  import { invalidate } from '$app/navigation';
  import { administrativeMutation } from '$lib/administration';
  import { problemFrom } from '$lib/api/client';
  import Timestamp from '$lib/components/Timestamp.svelte';
  import type {
    AdministrativeMutationReceipt,
    NotificationDestinationReplacement,
    RedactedNotificationDestinationAPIView,
  } from '$lib/api/generated';

  let { data } = $props();
  let selected = $state<RedactedNotificationDestinationAPIView | null>(null);
  let pending = $state('');
  let problem = $state<ReturnType<typeof problemFrom> | null>(null);
  let receipt = $state<AdministrativeMutationReceipt | null>(null);
  let receiptMessage = $state('');

  const stateLabels: Record<string, string> = {
    queued: 'Queued',
    attempting: 'Attempting delivery',
    delivered: 'Delivered',
    retry_wait: 'Waiting to retry',
    failed: 'Failed',
    dead_letter: 'Dead letter',
    suppressed: 'Suppressed by silence',
  };

  function clearEditor() {
    selected = null;
    problem = null;
  }

  async function saveDestination(event: SubmitEvent) {
    event.preventDefault();
    pending = 'save';
    problem = null;
    receipt = null;
    const replacing = selected;
    const values = new FormData(event.currentTarget as HTMLFormElement);
    const body: NotificationDestinationReplacement = {
      display_name: String(values.get('display_name') ?? '').trim(),
      destination_type: 'mattermost',
      enabled: values.get('enabled') === 'on',
      endpoint_host: String(values.get('endpoint_host') ?? '').trim(),
      endpoint_port: Number(values.get('endpoint_port')),
      path_prefix: String(values.get('path_prefix') ?? '').trim(),
      secret_reference: String(values.get('secret_reference') ?? '').trim(),
    };
    try {
      receipt = await administrativeMutation(
        replacing
          ? `/api/v1/notification-destinations/${replacing.id}`
          : '/api/v1/notification-destinations',
        replacing ? 'PUT' : 'POST',
        body,
        replacing?.version,
      );
      receiptMessage = replacing
        ? 'Destination replacement committed.'
        : 'Destination created.';
      selected = null;
      await invalidate('espial:monitoring');
    } catch (error) {
      problem = problemFrom(error);
    } finally {
      pending = '';
    }
  }

  async function testDestination(
    destination: RedactedNotificationDestinationAPIView,
  ) {
    pending = `test-${destination.id}`;
    problem = null;
    receipt = null;
    try {
      receipt = await administrativeMutation(
        `/api/v1/notification-destinations/${destination.id}/test`,
        'POST',
        {},
        destination.version,
      );
      receiptMessage = 'Explicitly labeled test delivery queued.';
      await invalidate('espial:monitoring');
    } catch (error) {
      problem = problemFrom(error);
    } finally {
      pending = '';
    }
  }
</script>

<svelte:head><title>Alert notifications · Espial</title></svelte:head>

<header class="page-header">
  <div>
    <p class="eyebrow">Durable delivery evidence</p>
    <h1>Alert notifications</h1>
    <p class="page-description">
      Configure approved Mattermost destinations and inspect bounded delivery,
      retry, failure, and silence outcomes.
    </p>
  </div>
</header>
<nav class="section-navigation" aria-label="Alert views">
  <a href="/alerts">Active</a><a href="/alerts/history">History</a><a
    href="/alerts/rules">Rules</a
  ><a href="/alerts/suppressions">Suppressions</a><a
    class="active"
    aria-current="page"
    href="/alerts/notifications">Notifications</a
  >
</nav>

{#if problem ?? data.problem}
  {@const currentProblem = problem ?? data.problem}
  <div class="inline-problem" role="alert">
    <strong>Notification operation unavailable</strong>
    <span>{currentProblem?.message}</span>
    {#if currentProblem?.request_id}<code>{currentProblem.request_id}</code
      >{/if}
  </div>
{/if}
{#if receipt}
  <div class="mutation-receipt" role="status" aria-live="polite">
    <strong>{receiptMessage}</strong>
    <span>Request ID: <code>{receipt.request_id}</code></span>
    {#if receipt.audit_url}<a href={receipt.audit_url}
        >View matching audit record</a
      >{/if}
  </div>
{/if}

{#if !data.problem}
  <div class="notification-admin-layout">
    <section class="admin-section" aria-labelledby="destination-title">
      <div class="operational-section-heading">
        <div>
          <p class="eyebrow">Redacted configuration</p>
          <h2 id="destination-title">Mattermost destinations</h2>
        </div>
        <span class="section-count">{data.destinations.length} configured</span>
      </div>
      {#if data.destinations.length}
        <div class="table-frame">
          <table class="resource-table">
            <thead
              ><tr
                ><th>Destination</th><th>State</th><th>Updated</th><th
                  >Actions</th
                ></tr
              ></thead
            >
            <tbody>
              {#each data.destinations as destination}
                <tr>
                  <th
                    ><span class="resource-name"
                      >{destination.display_name}</span
                    ><small>Mattermost · version {destination.version}</small
                    ></th
                  >
                  <td>{destination.enabled ? 'Enabled' : 'Disabled'}</td>
                  <td><Timestamp value={destination.updated_at} /></td>
                  <td class="destination-actions">
                    <button
                      class="table-action"
                      type="button"
                      onclick={() => (selected = destination)}>Replace</button
                    >
                    <button
                      class="table-action"
                      type="button"
                      disabled={pending !== ''}
                      onclick={() => testDestination(destination)}
                    >
                      {pending === `test-${destination.id}`
                        ? 'Queueing…'
                        : 'Send labeled test'}
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {:else}
        <div class="empty-state">
          <strong>No destinations configured.</strong><span
            >Create one only after its host, resolved CIDRs, port, and secret
            file are approved in Core configuration.</span
          >
        </div>
      {/if}
    </section>

    <section class="admin-section" aria-labelledby="destination-editor-title">
      <div class="editor-heading">
        <div>
          <p class="eyebrow">
            {selected ? 'Full replacement' : 'New destination'}
          </p>
          <h2 id="destination-editor-title">
            {selected?.display_name ?? 'Create destination'}
          </h2>
        </div>
        {#if selected}<button
            class="text-button"
            type="button"
            onclick={clearEditor}>Create instead</button
          >{/if}
      </div>
      {#key selected?.id ?? 'new'}
        <form class="admin-form" onsubmit={saveDestination}>
          <label
            >Display name<input
              name="display_name"
              required
              maxlength="128"
              value={selected?.display_name ?? ''}
            /></label
          >
          <label class="checkbox-field"
            ><input
              name="enabled"
              type="checkbox"
              checked={selected?.enabled ?? true}
            /> Enabled</label
          >
          <label
            >Approved HTTPS host<input
              name="endpoint_host"
              required
              maxlength="253"
              autocomplete="off"
            /></label
          >
          <label
            >Approved port<input
              name="endpoint_port"
              type="number"
              min="1"
              max="65535"
              required
              value="443"
            /></label
          >
          <label
            >Webhook path prefix<input
              name="path_prefix"
              required
              maxlength="256"
              value="/hooks"
              autocomplete="off"
            /></label
          >
          <label
            >Secret reference<input
              name="secret_reference"
              required
              maxlength="128"
              autocomplete="off"
            /></label
          >
          <p class="form-note">
            Endpoint and secret details are write-only. Replacing a destination
            requires entering all protected values again. Webhook tokens must
            exist in Core's mounted secret directory.
          </p>
          <button type="submit" disabled={pending !== ''}
            >{pending === 'save'
              ? 'Committing…'
              : selected
                ? 'Replace destination'
                : 'Create destination'}</button
          >
        </form>
      {/key}
    </section>
  </div>

  <section
    class="admin-section delivery-section"
    aria-labelledby="delivery-title"
  >
    <div class="operational-section-heading">
      <div>
        <p class="eyebrow">Newest first · safe failure reasons</p>
        <h2 id="delivery-title">Delivery history</h2>
      </div>
      <span class="section-count">{data.deliveries.length} shown</span>
    </div>
    {#if data.deliveries.length}
      <div class="table-frame">
        <table class="resource-table">
          <thead
            ><tr
              ><th>Destination</th><th>Event</th><th>Status</th><th>Attempts</th
              ><th>Last activity</th><th>Safe reason</th></tr
            ></thead
          >
          <tbody>
            {#each data.deliveries as delivery}
              <tr>
                <th
                  ><span class="resource-name">{delivery.destination_name}</span
                  >{#if delivery.test}<small>Explicit test</small>{/if}</th
                >
                <td>{delivery.event_kind.replaceAll('_', ' ')}</td>
                <td>{stateLabels[delivery.state] ?? delivery.state}</td>
                <td>{delivery.attempt_count} of 6</td>
                <td
                  ><Timestamp
                    value={delivery.last_attempt_at ??
                      delivery.event_occurred_at}
                  /></td
                >
                <td><code>{delivery.last_error_code ?? '—'}</code></td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <div class="empty-state">
        <strong>No delivery evidence yet.</strong><span
          >Initial, severity-change, recurrence, recovery, and explicit test
          intents appear here after Core commits them.</span
        >
      </div>
    {/if}
  </section>
{/if}

<style>
  .notification-admin-layout {
    display: grid;
    grid-template-columns: minmax(0, 1.45fr) minmax(18rem, 0.75fr);
    gap: 1rem;
    align-items: start;
  }
  .delivery-section {
    margin-top: 1rem;
  }
  .destination-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 0.45rem;
  }
  @media (max-width: 900px) {
    .notification-admin-layout {
      grid-template-columns: 1fr;
    }
  }
</style>
