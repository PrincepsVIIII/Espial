<script lang="ts">
  import { navigating } from '$app/state';
  import StatusIndicator from '$lib/components/StatusIndicator.svelte';
  import Timestamp from '$lib/components/Timestamp.svelte';
  import {
    countFor,
    dashboardState,
    resourceObservationTime,
    resourceStates,
    statusPresentation,
    totalResources,
    type ResourceState,
  } from '$lib/dashboard';
  import { markMonitoringRefresh } from '$lib/live';

  let { data } = $props();
  let stateFilter = $state<ResourceState | ''>('');
  let kindFilter = $state('');
  let integrationFilter = $state('');
  let staleFilter = $state<'true' | 'false' | ''>('');
  let appliedFilterKey = $state('');
  let lastMarkedGeneration = '';
  let total = $derived(totalResources(data.overview));
  let summaryState = $derived(dashboardState(data.overview));

  $effect(() => {
    const nextFilterKey = JSON.stringify(data.filters);
    if (nextFilterKey === appliedFilterKey) return;
    appliedFilterKey = nextFilterKey;
    stateFilter = data.filters.state;
    kindFilter = data.filters.kind;
    integrationFilter = data.filters.integration;
    staleFilter = data.filters.stale;
  });

  $effect(() => {
    const generated = data.overview?.generated_at;
    if (generated && generated !== lastMarkedGeneration) {
      lastMarkedGeneration = generated;
      markMonitoringRefresh(generated);
    }
  });
</script>

<svelte:head><title>Dashboard · Espial</title></svelte:head>

<header class="page-header dashboard-header">
  <div>
    <p class="eyebrow">UBNetDef infrastructure operations</p>
    <h1>Dashboard</h1>
    {#if data.overview}
      <p class="generated-at">
        Authoritative snapshot <Timestamp value={data.overview.generated_at} />
      </p>
    {/if}
  </div>
  <StatusIndicator state={summaryState} />
</header>

{#if navigating.to?.url.pathname === '/dashboard'}
  <div class="refresh-notice" aria-live="polite">
    Refreshing monitoring data…
  </div>
{/if}

{#if data.overview}
  <section class="monitoring-summary" aria-labelledby="summary-title">
    <div class="summary-introduction">
      <p class="eyebrow">Current resource state</p>
      <h2 id="summary-title">Monitoring coverage</h2>
      <p>
        {total}
        {total === 1 ? 'resource' : 'resources'} in the current snapshot.
      </p>
    </div>
    <dl class="state-counts">
      {#each resourceStates as state}
        {@const presentation = statusPresentation(state)}
        <div class={`count-${presentation.className}`}>
          <dt>
            {presentation.label}
          </dt>
          <dd>{countFor(data.overview, state)}</dd>
        </div>
      {/each}
    </dl>
  </section>
{:else}
  <section class="problem-panel" aria-labelledby="overview-problem-title">
    <p class="eyebrow">
      {data.problems.overview?.status === 403
        ? 'Permission denied'
        : 'Core unavailable'}
    </p>
    <h2 id="overview-problem-title">Monitoring summary is unavailable.</h2>
    <p>
      {data.problems.overview?.message ?? 'The overview could not be loaded.'}
    </p>
    {#if data.problems.overview?.request_id}
      <p class="request-reference">
        Request ID: <code>{data.problems.overview.request_id}</code>
      </p>
    {/if}
  </section>
{/if}

{#if data.overview}
  <section
    class="dashboard-incidents"
    aria-labelledby="dashboard-incidents-title"
  >
    <div class="operational-section-heading">
      <div>
        <p class="eyebrow">Current operational attention</p>
        <h2 id="dashboard-incidents-title">Active incidents</h2>
      </div>
      <a class="text-link" href="/alerts">View all active incidents</a>
    </div>
    <div class="incident-counts" aria-label="Active incident counts">
      <span
        ><strong
          >{data.overview.active_incident_counts?.find(
            (item) => item.severity === 'critical',
          )?.count ?? 0}</strong
        > critical</span
      >
      <span
        ><strong
          >{data.overview.active_incident_counts?.find(
            (item) => item.severity === 'warning',
          )?.count ?? 0}</strong
        > warning</span
      >
    </div>
    {#if data.overview.active_incidents?.length}
      <div class="dashboard-incident-list">
        {#each data.overview.active_incidents as incident (incident.id)}
          <a href={`/alerts/${incident.id}`}>
            <span class={`incident-severity severity-${incident.severity}`}
              >{incident.severity}</span
            >
            <span
              ><strong>{incident.title}</strong><small
                >{incident.resource_name} · {incident.integration_name}</small
              ></span
            >
            <Timestamp value={incident.updated_at} />
          </a>
        {/each}
      </div>
    {:else}
      <div class="empty-state compact">
        <strong>No active incidents.</strong><span
          >The automatic evaluator has not detected a currently active
          condition.</span
        >
      </div>
    {/if}
  </section>
{/if}

<section class="resource-section" aria-labelledby="resources-title">
  <div class="operational-section-heading">
    <div>
      <p class="eyebrow">Authoritative resource health</p>
      <h2 id="resources-title">Resources</h2>
    </div>
    <span class="section-count">{data.resources?.items.length ?? 0} shown</span>
  </div>

  <form class="resource-filters" method="GET" aria-label="Resource filters">
    <label>
      State
      <select name="state" bind:value={stateFilter}>
        <option value="">All states</option>
        {#each resourceStates as state}
          <option value={state}>{statusPresentation(state).label}</option>
        {/each}
      </select>
    </label>
    <label>
      Kind
      <input
        name="kind"
        bind:value={kindFilter}
        pattern="[a-z][a-z0-9_.-]*"
        maxlength="127"
        placeholder="All kinds"
      />
    </label>
    <label>
      Integration
      <select name="integration" bind:value={integrationFilter}>
        <option value="">All integrations</option>
        {#each data.integrations?.items ?? [] as integration}
          <option value={integration.id}>{integration.display_name}</option>
        {/each}
      </select>
    </label>
    <label>
      Freshness
      <select name="stale" bind:value={staleFilter}>
        <option value="">All freshness states</option>
        <option value="true">Stale only</option>
        <option value="false">Exclude stale</option>
      </select>
    </label>
    <div class="filter-actions">
      <button type="submit">Apply filters</button>
      <a href="/dashboard" class="text-link">Clear</a>
    </div>
  </form>

  {#if data.problems.resources}
    <div class="inline-problem" role="status">
      <strong>
        {data.problems.resources.status === 403
          ? 'Resource access denied'
          : 'Resource data unavailable'}
      </strong>
      <span>{data.problems.resources.message}</span>
      {#if data.problems.resources.request_id}
        <code>{data.problems.resources.request_id}</code>
      {/if}
    </div>
  {:else if data.resources && data.resources.items.length > 0}
    <div class="table-frame" role="region" aria-label="Resource health table">
      <table class="resource-table">
        <thead>
          <tr>
            <th scope="col">Resource</th>
            <th scope="col">State</th>
            <th scope="col">Integration</th>
            <th scope="col">Kind</th>
            <th scope="col">Observed</th>
            <th scope="col">Current reason</th>
          </tr>
        </thead>
        <tbody>
          {#each data.resources.items as resource (resource.id)}
            <tr>
              <th scope="row" data-label="Resource">
                <span class="resource-name">{resource.display_name}</span>
                <code>{resource.external_id}</code>
              </th>
              <td data-label="State">
                <StatusIndicator state={resource.health.state} compact />
              </td>
              <td data-label="Integration">{resource.integration_name}</td>
              <td data-label="Kind"><code>{resource.kind}</code></td>
              <td data-label="Observed">
                <Timestamp value={resourceObservationTime(resource)} />
              </td>
              <td data-label="Current reason" class="reason-cell">
                {resource.health.reason}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <nav class="pagination-actions" aria-label="Resource pages">
      {#if data.filters.cursor}
        <span>Use browser Back to return to the previous snapshot page.</span>
      {/if}
      {#if data.nextPageURL}
        <a class="next-page-link" href={data.nextPageURL}>Next page →</a>
      {/if}
    </nav>
  {:else}
    <div class="empty-state">
      <strong>
        {data.filters.state ||
        data.filters.kind ||
        data.filters.integration ||
        data.filters.stale
          ? 'No resources match these filters.'
          : 'No resources have been discovered.'}
      </strong>
      <span>
        {data.filters.state ||
        data.filters.kind ||
        data.filters.integration ||
        data.filters.stale
          ? 'Clear or adjust the filters to inspect the current inventory.'
          : 'Configure and run an integration before expecting resource health.'}
      </span>
    </div>
  {/if}
</section>

<section class="integration-section" aria-labelledby="integrations-title">
  <div class="operational-section-heading">
    <div>
      <p class="eyebrow">Collection coverage</p>
      <h2 id="integrations-title">Integrations</h2>
    </div>
    <span class="section-count"
      >{data.integrations?.items.length ?? 0} configured</span
    >
  </div>
  {#if data.problems.integrations}
    <div class="inline-problem" role="status">
      <strong>
        {data.problems.integrations.status === 403
          ? 'Integration access denied'
          : 'Integration data unavailable'}
      </strong>
      <span>{data.problems.integrations.message}</span>
      {#if data.problems.integrations.request_id}
        <code>{data.problems.integrations.request_id}</code>
      {/if}
    </div>
  {:else if data.integrations && data.integrations.items.length > 0}
    <div class="integration-list" role="list">
      {#each data.integrations.items as integration (integration.id)}
        <article class="integration-row" role="listitem">
          <div class="integration-identity">
            <strong>{integration.display_name}</strong>
            <code>{integration.adapter_id}</code>
          </div>
          <StatusIndicator state={integration.runtime_state} compact />
          <dl class="integration-measures">
            <div>
              <dt>Resources</dt>
              <dd>{integration.resource_count}</dd>
            </div>
            <div>
              <dt>Stale</dt>
              <dd>{integration.stale_count}</dd>
            </div>
            <div>
              <dt>Unknown</dt>
              <dd>{integration.unknown_count}</dd>
            </div>
          </dl>
          <div class="collection-state">
            {#if integration.last_collection}
              <span>{integration.last_collection.result}</span>
              <Timestamp value={integration.last_collection.completed_at} />
            {:else}
              <span>Never collected</span>
            {/if}
          </div>
        </article>
      {/each}
    </div>
    {#if data.integrations.next_cursor}
      <p class="truncation-note">
        More than 200 integrations exist. This Dashboard shows the first stable
        page; use the API for the complete collection.
      </p>
    {/if}
  {:else}
    <div class="empty-state">
      <strong>No integrations are configured.</strong>
      <span>
        Register the trusted sample adapter and create an integration before
        expecting monitoring coverage.
      </span>
    </div>
  {/if}
</section>
