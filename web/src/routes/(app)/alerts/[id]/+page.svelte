<script lang="ts">
  import Timestamp from '$lib/components/Timestamp.svelte';
  import { incidentStatusLabel, timelineKindLabel } from '$lib/incidents';
  let { data } = $props();
</script>

<svelte:head
  ><title>{data.incident?.title ?? 'Incident'} · Espial</title></svelte:head
>

{#if data.problem}
  <section class="problem-panel" aria-labelledby="incident-problem-title">
    <p class="eyebrow">
      {data.problem.status === 404
        ? 'Not found'
        : data.problem.status === 403
          ? 'Permission denied'
          : 'Core unavailable'}
    </p>
    <h1 id="incident-problem-title">Incident detail is unavailable.</h1>
    <p>{data.problem.message}</p>
    {#if data.problem.request_id}<p class="request-reference">
        Request ID: <code>{data.problem.request_id}</code>
      </p>{/if}
    <a class="text-link" href="/alerts">Return to active alerts</a>
  </section>
{:else if data.incident}
  <header class="page-header incident-detail-header">
    <div>
      <p class="eyebrow">Incident detail · Read-only</p>
      <h1>{data.incident.title}</h1>
      <p class="page-description">{data.incident.summary}</p>
    </div>
    <span class={`incident-severity severity-${data.incident.severity}`}
      >{data.incident.severity}</span
    >
  </header>
  <nav class="section-navigation" aria-label="Alert views">
    <a href="/alerts">Active</a><a href="/alerts/history">History</a>
  </nav>

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

  <section class="incident-timeline" aria-labelledby="incident-timeline-title">
    <div class="operational-section-heading">
      <div>
        <p class="eyebrow">Immutable lifecycle record</p>
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
