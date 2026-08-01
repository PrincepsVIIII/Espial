<script lang="ts">
  import Timestamp from '$lib/components/Timestamp.svelte';

  let { data } = $props();
  let action = $state('');
  let result = $state('');
  let targetType = $state('');
  let correlationID = $state('');
  let appliedFilters = '';

  $effect(() => {
    const filters = JSON.stringify(data.filters);
    if (filters === appliedFilters) return;
    appliedFilters = filters;
    action = data.filters.action;
    result = data.filters.result;
    targetType = data.filters.target_type;
    correlationID = data.filters.correlation_id;
  });

  function summary(value: Record<string, unknown> | undefined): string {
    if (!value || Object.keys(value).length === 0) return '—';
    return JSON.stringify(value);
  }
</script>

<svelte:head><title>Audit · Espial</title></svelte:head>

<header class="page-header">
  <div>
    <h1>Audit</h1>
    <p class="page-description">
      Trace administrative and operational actions to an actor and request.
    </p>
  </div>
  {#if data.session?.user.permissions.includes('users:manage')}
    <a class="secondary-action" href="/audit/users">Manage users</a>
  {/if}
</header>

<nav class="section-navigation" aria-label="Audit sections">
  <a class="active" aria-current="page" href="/audit">History</a>
  {#if data.session?.user.permissions.includes('users:manage')}
    <a href="/audit/users">Users</a>
  {/if}
</nav>

<form class="audit-filters" method="GET" aria-label="Audit filters">
  <label>
    Action
    <input
      name="action"
      bind:value={action}
      maxlength="127"
      pattern="[a-z][a-z0-9_.-]*"
      placeholder="auth.user.updated"
    />
  </label>
  <label>
    Result
    <select name="result" bind:value={result}>
      <option value="">All results</option>
      <option value="succeeded">Succeeded</option>
      <option value="failed">Failed</option>
      <option value="denied">Denied</option>
    </select>
  </label>
  <label>
    Target type
    <input
      name="target_type"
      bind:value={targetType}
      maxlength="63"
      pattern="[a-z][a-z0-9_.-]*"
      placeholder="user"
    />
  </label>
  <label>
    Request ID
    <input
      name="correlation_id"
      bind:value={correlationID}
      maxlength="128"
      placeholder="Exact request ID"
    />
  </label>
  <div class="filter-actions">
    <button type="submit">Apply filters</button>
    <a href="/audit" class="text-link">Clear</a>
  </div>
</form>

{#if data.problem}
  <section class="problem-panel" aria-labelledby="audit-problem-title">
    <h2 id="audit-problem-title">
      {data.problem.status === 403
        ? 'Audit history is permission restricted.'
        : 'Audit history is unavailable.'}
    </h2>
    <p>{data.problem.message}</p>
    {#if data.problem.request_id}
      <p class="request-reference">
        Request ID: <code>{data.problem.request_id}</code>
      </p>
    {/if}
  </section>
{:else if data.audit && data.audit.items.length > 0}
  <p class="audit-range">
    Showing events from <Timestamp value={data.audit.from} /> through
    <Timestamp value={data.audit.to} />
  </p>
  <div class="table-frame" role="region" aria-label="Audit history table">
    <table class="resource-table audit-table">
      <thead>
        <tr>
          <th scope="col">When</th>
          <th scope="col">Actor</th>
          <th scope="col">Action</th>
          <th scope="col">Target</th>
          <th scope="col">Result</th>
          <th scope="col">Request and summary</th>
        </tr>
      </thead>
      <tbody>
        {#each data.audit.items as event}
          <tr>
            <td data-label="When"><Timestamp value={event.occurred_at} /></td>
            <td data-label="Actor">
              <strong>{event.actor_username ?? 'System'}</strong>
            </td>
            <td data-label="Action"><code>{event.action}</code></td>
            <td data-label="Target">
              <span>{event.target_type}</span>
              {#if event.target_id}<code>{event.target_id}</code>{/if}
            </td>
            <td data-label="Result">
              <span class={`audit-result result-${event.result}`}>
                {event.result}
              </span>
            </td>
            <td data-label="Request and summary" class="audit-summary">
              <code>{event.correlation_id}</code>
              {#if event.before_summary || event.after_summary}
                <details>
                  <summary>View redacted change summary</summary>
                  <dl>
                    <div>
                      <dt>Before</dt>
                      <dd><code>{summary(event.before_summary)}</code></dd>
                    </div>
                    <div>
                      <dt>After</dt>
                      <dd><code>{summary(event.after_summary)}</code></dd>
                    </div>
                  </dl>
                </details>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  {#if data.nextPageURL}
    <div class="pagination-actions">
      <span>50 events shown</span>
      <a class="next-page-link" href={data.nextPageURL}>Next page</a>
    </div>
  {/if}
{:else}
  <div class="empty-state">
    <strong>No matching audit events.</strong>
    <span>Change or clear the filters to broaden the time window.</span>
  </div>
{/if}
