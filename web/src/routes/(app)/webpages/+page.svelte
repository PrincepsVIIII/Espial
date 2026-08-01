<script lang="ts">
  import StatusIndicator from '$lib/components/StatusIndicator.svelte';
  import Timestamp from '$lib/components/Timestamp.svelte';
  let { data } = $props();
</script>

<svelte:head><title>Webpages · Espial</title></svelte:head>

<header class="page-header">
  <div>
    <p class="eyebrow">Authoritative DNS, TCP, TLS, and HTTP evidence</p>
    <h1>Webpages</h1>
    <p class="page-description">
      Availability reflects completed supervised checks. Response bodies and
      secret header values are never retained.
    </p>
  </div>
  {#if data.canManage}<a class="secondary-action" href="/webpages/monitors"
      >Manage monitors</a
    >{/if}
</header>
<nav class="section-navigation" aria-label="Webpage views">
  <a class="active" aria-current="page" href="/webpages">Availability</a>
  <a href="/webpages/certificates">Certificates</a>
  {#if data.canManage}<a href="/webpages/monitors">Monitors</a>{/if}
</nav>

{#if data.problem}
  <section class="problem-panel" aria-labelledby="webpages-problem-title">
    <p class="eyebrow">
      {data.problem.status === 403 ? 'Permission denied' : 'Core unavailable'}
    </p>
    <h2 id="webpages-problem-title">Website availability is unavailable.</h2>
    <p>{data.problem.message}</p>
    {#if data.problem.request_id}<p class="request-reference">
        Request ID: <code>{data.problem.request_id}</code>
      </p>{/if}
  </section>
{:else if data.webpages.length === 0}
  <section class="empty-state" aria-labelledby="webpages-empty-title">
    <strong id="webpages-empty-title">No website observations yet.</strong>
    <span>
      {data.canManage
        ? 'Create or inspect a monitor, then wait for its first bounded collection.'
        : 'An administrator must configure an approved website monitor before availability appears.'}
    </span>
    {#if data.canManage}<a class="text-link" href="/webpages/monitors"
        >Open monitor administration</a
      >{/if}
  </section>
{:else}
  <section class="admin-section" aria-labelledby="availability-title">
    <div class="operational-section-heading">
      <div>
        <p class="eyebrow">Newest evidence first</p>
        <h2 id="availability-title">Availability</h2>
      </div>
      <span class="section-count">{data.webpages.length} observed</span>
    </div>
    <div class="table-frame">
      <table class="resource-table">
        <thead
          ><tr
            ><th>Endpoint</th><th>State</th><th>HTTP</th><th>Total</th><th
              >Last check</th
            ><th>Incident</th></tr
          ></thead
        >
        <tbody>
          {#each data.webpages as webpage}
            <tr>
              <th
                ><a class="resource-name" href={`/webpages/${webpage.id}`}
                  >{webpage.display_name}</a
                ><small>{webpage.url}</small></th
              >
              <td
                ><StatusIndicator
                  state={webpage.state}
                  compact
                />{#if webpage.state === 'maintenance'}<small
                    >Raw: {webpage.raw_state}</small
                  >{/if}</td
              >
              <td>{webpage.stages.http_status ?? 'Not reported'}</td>
              <td
                >{webpage.observed_at
                  ? `${webpage.stages.total_ms} ms`
                  : 'Not reported'}</td
              >
              <td
                >{#if webpage.observed_at}<Timestamp
                    value={webpage.observed_at}
                  />{:else}Unknown{/if}</td
              >
              <td
                >{#if webpage.active_incident_id}<a
                    class="text-link"
                    href={`/alerts/${webpage.active_incident_id}`}
                    >View incident</a
                  >{:else}None active{/if}</td
              >
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if data.nextCursor}<a
        class="text-link"
        href={`/webpages?cursor=${encodeURIComponent(data.nextCursor)}`}
        >Next page</a
      >{/if}
  </section>
{/if}
