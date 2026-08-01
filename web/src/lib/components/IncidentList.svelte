<script lang="ts">
  import Timestamp from '$lib/components/Timestamp.svelte';
  import type { ClientProblem } from '$lib/api/client';
  import {
    incidentStatusLabel,
    type IncidentFilters,
    type IncidentListResponse,
  } from '$lib/incidents';

  let {
    incidents,
    problem,
    filters,
    history,
    nextPageURL,
  }: {
    incidents: IncidentListResponse | null;
    problem: ClientProblem | null;
    filters: IncidentFilters;
    history: boolean;
    nextPageURL: string;
  } = $props();
</script>

<form class="incident-filters" method="GET" aria-label="Incident filters">
  <label>
    Severity
    <select name="severity" value={filters.severity}>
      <option value="">All severities</option>
      <option value="critical">Critical</option>
      <option value="warning">Warning</option>
    </select>
  </label>
  <label>
    Status
    <select name="status" value={filters.status}>
      <option value="">All {history ? 'historical' : 'active'} statuses</option>
      {#if history}
        <option value="recovered">Recovered</option>
        <option value="resolved">Resolved</option>
      {:else}
        <option value="open">Open</option>
        <option value="acknowledged">Acknowledged</option>
        <option value="investigating">Investigating</option>
      {/if}
    </select>
  </label>
  <div class="filter-actions">
    <button type="submit">Apply filters</button>
    <a class="text-link" href={history ? '/alerts/history' : '/alerts'}>Clear</a
    >
  </div>
</form>

{#if problem}
  <div class="inline-problem" role="status">
    <strong
      >{problem.status === 403
        ? 'Incident access denied'
        : 'Incident data unavailable'}</strong
    >
    <span>{problem.message}</span>
    {#if problem.request_id}<code>{problem.request_id}</code>{/if}
  </div>
{:else if incidents?.items.length}
  <div
    class="table-frame"
    role="region"
    aria-label={history ? 'Incident history table' : 'Active incidents table'}
  >
    <table class="resource-table incident-table">
      <thead>
        <tr>
          <th scope="col">Incident</th>
          <th scope="col">Severity</th>
          <th scope="col">Status</th>
          <th scope="col">Resource</th>
          <th scope="col">Integration</th>
          <th scope="col">Updated</th>
        </tr>
      </thead>
      <tbody>
        {#each incidents.items as incident (incident.id)}
          <tr>
            <th scope="row" data-label="Incident">
              <a class="incident-title" href={`/alerts/${incident.id}`}
                >{incident.title}</a
              >
              <span class="incident-summary">{incident.summary}</span>
            </th>
            <td data-label="Severity"
              ><span class={`incident-severity severity-${incident.severity}`}
                >{incident.severity}</span
              ></td
            >
            <td data-label="Status">{incidentStatusLabel(incident.status)}</td>
            <td data-label="Resource">{incident.resource_name}</td>
            <td data-label="Integration">{incident.integration_name}</td>
            <td data-label="Updated"
              ><Timestamp value={incident.updated_at} /></td
            >
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  <nav class="pagination-actions" aria-label="Incident pages">
    {#if filters.cursor}<span
        >Use browser Back to return to the previous snapshot page.</span
      >{/if}
    {#if nextPageURL}<a class="next-page-link" href={nextPageURL}>Next page →</a
      >{/if}
  </nav>
{:else}
  <div class="empty-state">
    <strong
      >{filters.severity || filters.status
        ? 'No incidents match these filters.'
        : history
          ? 'No incident history is available.'
          : 'No active incidents.'}</strong
    >
    <span
      >{filters.severity || filters.status
        ? 'Clear or adjust the filters.'
        : history
          ? 'Recovered incidents will appear here.'
          : 'Espial has not detected a currently active incident.'}</span
    >
  </div>
{/if}
