<script lang="ts">
  import { invalidate } from '$app/navigation';
  import { readCookie } from '$lib/auth';
  import {
    problemFrom,
    requestJSONWithMetadata,
    type ClientProblem,
  } from '$lib/api/client';
  import type { IncidentMutationReceipt } from '$lib/api/generated';
  import AlertNavigation from '$lib/components/AlertNavigation.svelte';
  import Timestamp from '$lib/components/Timestamp.svelte';
  import { incidentStatusLabel, timelineKindLabel } from '$lib/incidents';
  let { data } = $props();
  let pending = $state('');
  let actionProblem = $state<ClientProblem | null>(null);
  let receipt = $state<{
    message: string;
    requestID: string;
    auditURL?: string;
  } | null>(null);
  let ownerUserID = $state('');
  let note = $state('');
  let resolutionNote = $state('');
  let retryKeys = $state<Record<string, string>>({});
  const deliveryStateLabels: Record<string, string> = {
    queued: 'Queued',
    attempting: 'Attempting delivery',
    delivered: 'Delivered',
    retry_wait: 'Waiting to retry',
    failed: 'Failed',
    dead_letter: 'Dead letter',
    suppressed: 'Suppressed by silence',
  };

  function mutationHeaders(action: string) {
    const key = retryKeys[action] || newIdempotencyKey();
    retryKeys[action] = key;
    return {
      'Content-Type': 'application/json',
      'X-CSRF-Token': readCookie('espial_csrf'),
      'If-Match': data.etag ?? '',
      'Idempotency-Key': key,
    };
  }

  async function mutate(
    action: string,
    path: string,
    method: 'POST' | 'PUT',
    body: Record<string, string>,
    message: string,
  ) {
    if (!data.incident || !data.etag) return;
    pending = action;
    actionProblem = null;
    receipt = null;
    try {
      const result = await requestJSONWithMetadata<IncidentMutationReceipt>(
        fetch,
        `/api/v1/incidents/${data.incident.id}${path}`,
        {
          method,
          headers: mutationHeaders(action),
          body: JSON.stringify(body),
        },
      );
      receipt = {
        message,
        requestID: result.data.request_id,
        ...(result.data.audit_url ? { auditURL: result.data.audit_url } : {}),
      };
      retryKeys[action] = '';
      if (action === 'note') note = '';
      if (action === 'resolve') resolutionNote = '';
      await invalidate('espial:monitoring');
    } catch (error) {
      const problem = problemFrom(error);
      actionProblem =
        problem.status === 412
          ? {
              ...problem,
              message:
                'Another operator changed this incident. The current state has been reloaded; review it before submitting again.',
            }
          : problem;
      if ([400, 403, 409, 412].includes(problem.status)) retryKeys[action] = '';
      if (problem.status === 412) await invalidate('espial:monitoring');
    } finally {
      pending = '';
    }
  }

  function newIdempotencyKey(): string {
    if (globalThis.crypto?.randomUUID) return globalThis.crypto.randomUUID();
    return `incident-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
  }
</script>

<svelte:head
  ><title>{data.incident?.title ?? 'Incident'} · Espial</title></svelte:head
>

{#if data.problem}
  <section class="problem-panel" aria-labelledby="incident-problem-title">
    <h1 id="incident-problem-title">
      {data.problem.status === 404
        ? 'Incident was not found.'
        : data.problem.status === 403
          ? 'Incident detail is permission restricted.'
          : 'Incident detail is unavailable.'}
    </h1>
    <p>{data.problem.message}</p>
    {#if data.problem.request_id}<p class="request-reference">
        Request ID: <code>{data.problem.request_id}</code>
      </p>{/if}
    <a class="text-link" href="/alerts">Return to active alerts</a>
  </section>
{:else if data.incident}
  <header class="page-header incident-detail-header">
    <div>
      <h1>{data.incident.title}</h1>
      <p class="page-description">{data.incident.summary}</p>
    </div>
    <span class={`incident-severity severity-${data.incident.severity}`}
      >{data.incident.severity}</span
    >
  </header>
  <AlertNavigation permissions={data.session?.user.permissions ?? []} />

  {#if actionProblem}
    <div class="inline-problem" role="alert">
      <strong>
        {actionProblem.status === 412
          ? 'Incident changed'
          : actionProblem.status === 403
            ? 'Action denied'
            : actionProblem.status === 0 || actionProblem.status >= 500
              ? 'Core unavailable'
              : 'Action not completed'}
      </strong>
      <span>{actionProblem.message}</span>
      {#if actionProblem.request_id}<span>
          Request ID: <code>{actionProblem.request_id}</code>
        </span>{/if}
    </div>
  {/if}

  {#if receipt}
    <div class="mutation-receipt" role="status" aria-live="polite">
      <strong>{receipt.message}</strong>
      <span>Request ID: <code>{receipt.requestID}</code></span>
      {#if receipt.auditURL}
        <a href={receipt.auditURL}>View matching audit record</a>
      {/if}
    </div>
  {/if}

  {#if data.canOperate}
    <section
      class="incident-workflow"
      aria-labelledby="incident-workflow-title"
    >
      <div class="operational-section-heading">
        <div>
          <h2 id="incident-workflow-title">Workflow</h2>
        </div>
        <span class="section-count">Version {data.incident.version}</span>
      </div>

      <div class="incident-lifecycle-actions" aria-label="Lifecycle actions">
        {#if data.incident.status === 'open'}
          <button
            type="button"
            disabled={pending !== ''}
            onclick={() =>
              mutate(
                'acknowledge',
                '/acknowledge',
                'POST',
                {},
                'Incident acknowledged.',
              )}
            >{pending === 'acknowledge'
              ? 'Acknowledging…'
              : 'Acknowledge'}</button
          >
        {:else if data.incident.status === 'acknowledged'}
          <button
            type="button"
            disabled={pending !== ''}
            onclick={() =>
              mutate(
                'investigate',
                '/investigate',
                'POST',
                {},
                'Investigation started.',
              )}
            >{pending === 'investigate'
              ? 'Starting…'
              : 'Start investigating'}</button
          >
        {:else}
          <p class="form-note">
            {data.incident.status === 'recovered'
              ? 'Core reports recovery. Add a required resolution note to close this incident.'
              : data.incident.status === 'resolved'
                ? 'This incident is resolved. Its notes and timeline remain immutable evidence.'
                : 'Investigation is in progress.'}
          </p>
        {/if}
      </div>

      <div class="incident-action-grid">
        {#if data.incident.status !== 'resolved'}
          <form
            class="incident-action-form"
            onsubmit={(event) => {
              event.preventDefault();
              void mutate(
                'assign',
                '/owner',
                'PUT',
                { owner_user_id: ownerUserID },
                'Incident owner updated.',
              );
            }}
          >
            <h3>Assign owner</h3>
            <label>
              Enabled operator or administrator
              <select
                bind:value={ownerUserID}
                required
                disabled={pending !== ''}
              >
                <option value="" disabled>Select an assignee</option>
                {#each data.assignees as assignee}
                  <option value={assignee.id}>{assignee.display_name}</option>
                {/each}
              </select>
            </label>
            <button type="submit" disabled={pending !== '' || !ownerUserID}>
              {pending === 'assign' ? 'Assigning…' : 'Assign'}
            </button>
            {#if data.assigneeProblem}
              <p class="form-note">
                Eligible assignees are unavailable. Request ID:
                <code>{data.assigneeProblem.request_id ?? 'unavailable'}</code>
              </p>
            {:else if !data.assignees.length}
              <p class="form-note">No eligible assignees are available.</p>
            {/if}
          </form>
        {/if}

        <form
          class="incident-action-form"
          onsubmit={(event) => {
            event.preventDefault();
            void mutate(
              'note',
              '/notes',
              'POST',
              { note },
              'Operator note appended.',
            );
          }}
        >
          <h3>Append note</h3>
          <label>
            Plain text · {note.length}/2000
            <textarea
              bind:value={note}
              required
              maxlength="2000"
              rows="4"
              disabled={pending !== ''}></textarea>
          </label>
          <button type="submit" disabled={pending !== '' || !note.trim()}>
            {pending === 'note' ? 'Appending…' : 'Append note'}
          </button>
          <p class="form-note">Notes cannot be edited or deleted.</p>
        </form>

        {#if data.incident.status === 'recovered'}
          <form
            class="incident-action-form resolution-action"
            onsubmit={(event) => {
              event.preventDefault();
              void mutate(
                'resolve',
                '/resolve',
                'POST',
                { note: resolutionNote },
                'Recovered incident resolved.',
              );
            }}
          >
            <h3>Resolve recovered incident</h3>
            <label>
              Required resolution note · {resolutionNote.length}/2000
              <textarea
                bind:value={resolutionNote}
                required
                maxlength="2000"
                rows="4"
                disabled={pending !== ''}></textarea>
            </label>
            <button
              type="submit"
              disabled={pending !== '' || !resolutionNote.trim()}
              >{pending === 'resolve'
                ? 'Resolving…'
                : 'Resolve incident'}</button
            >
          </form>
        {/if}
      </div>
    </section>
  {:else}
    <section
      class="incident-readonly"
      aria-labelledby="incident-readonly-title"
    >
      <h2 id="incident-readonly-title">Read-only incident record</h2>
      <p>
        Your role can inspect current state and immutable evidence. Operator
        actions are not available.
      </p>
    </section>
  {/if}

  <section class="incident-facts" aria-labelledby="incident-facts-title">
    <h2 id="incident-facts-title">Current state</h2>
    <dl>
      <div>
        <dt>Status</dt>
        <dd>{incidentStatusLabel(data.incident.status)}</dd>
      </div>
      <div>
        <dt>Resource</dt>
        <dd>{data.incident.resource_name}</dd>
      </div>
      <div>
        <dt>Integration</dt>
        <dd>{data.incident.integration_name}</dd>
      </div>
      <div>
        <dt>Check</dt>
        <dd><code>{data.incident.check_type}</code></dd>
      </div>
      <div>
        <dt>Detected</dt>
        <dd><Timestamp value={data.incident.detected_at} /></dd>
      </div>
      <div>
        <dt>Latest signal</dt>
        <dd><Timestamp value={data.incident.latest_signal_at} /></dd>
      </div>
      <div>
        <dt>Owner</dt>
        <dd>{data.incident.owner_name || 'Unassigned'}</dd>
      </div>
      <div>
        <dt>Version</dt>
        <dd><code>{data.etag ?? `v${data.incident.version}`}</code></dd>
      </div>
    </dl>
  </section>

  <section class="admin-section" aria-labelledby="incident-delivery-title">
    <div class="operational-section-heading">
      <div>
        <h2 id="incident-delivery-title">Notifications</h2>
      </div>
      <span class="section-count">{data.deliveries.length} records</span>
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
                  ></th
                >
                <td>{delivery.event_kind.replaceAll('_', ' ')}</td>
                <td>{deliveryStateLabels[delivery.state] ?? delivery.state}</td>
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
        <strong>No delivery evidence for this incident.</strong>
        <span
          >No destination intent has been committed for its notification events.</span
        >
      </div>
    {/if}
  </section>

  <section class="incident-timeline" aria-labelledby="incident-timeline-title">
    <div class="operational-section-heading">
      <div>
        <h2 id="incident-timeline-title">Timeline</h2>
      </div>
      <span class="section-count"
        >{data.timeline?.items.length ?? 0} events</span
      >
    </div>
    {#if data.timeline?.items.length}
      <ol>
        {#each data.timeline.items as event (event.id)}
          <li>
            <div>
              <strong>{timelineKindLabel(event.kind)}</strong><Timestamp
                value={event.occurred_at}
              />
            </div>
            <p>{event.summary}</p>
            {#if event.actor_name}<small>Actor: {event.actor_name}</small>{/if}
            {#if event.subject_name}<small
                >Assigned owner: {event.subject_name}</small
              >{/if}
          </li>
        {/each}
      </ol>
      {#if data.timeline.next_cursor}<p class="truncation-note">
          This view shows the newest 100 events. Older events remain available
          through the API cursor.
        </p>{/if}
    {:else}<div class="empty-state">
        <strong>No timeline events are available.</strong><span
          >The incident exists, but Core returned no lifecycle entries.</span
        >
      </div>{/if}
  </section>
{/if}
