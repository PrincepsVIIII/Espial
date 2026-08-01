<script lang="ts">
  import StatusIndicator from '$lib/components/StatusIndicator.svelte';
  import Timestamp from '$lib/components/Timestamp.svelte';
  let { data } = $props();
  let statusFilter = $state('');
  let hostnameValid = $state('');
  let expiryDays = $state('');
  let appliedFilters = '';
  $effect(() => {
    const key = JSON.stringify(data.filters);
    if (key === appliedFilters) return;
    appliedFilters = key;
    statusFilter = data.filters.state;
    hostnameValid = data.filters.hostnameValid;
    expiryDays = data.filters.expiryDays;
  });
  const nextURL = $derived.by(() => {
    const query = new URLSearchParams();
    if (data.filters.state) query.set('state', data.filters.state);
    if (data.filters.hostnameValid)
      query.set('hostname_valid', data.filters.hostnameValid);
    if (data.filters.expiryDays)
      query.set('expiry_days', data.filters.expiryDays);
    if (data.nextCursor) query.set('cursor', data.nextCursor);
    return `/webpages/certificates?${query}`;
  });
  function validity(value: boolean | undefined): string {
    return value === undefined ? 'Not reported' : value ? 'Valid' : 'Invalid';
  }
</script>

<svelte:head><title>Certificates · Espial</title></svelte:head>
<header class="page-header">
  <div>
    <h1>Certificates</h1>
    <p class="page-description">
      Certificate status comes from supervised handshakes against approved
      endpoints and trust roots.
    </p>
  </div>
</header>
<nav class="section-navigation" aria-label="Webpage views">
  <a href="/webpages">Availability</a><a
    class="active"
    aria-current="page"
    href="/webpages/certificates">Certificates</a
  >{#if data.canManage}<a href="/webpages/monitors">Monitors</a>{/if}
</nav>

<form class="resource-filters" method="GET" aria-label="Certificate filters">
  <label
    >Status<select name="state" bind:value={statusFilter}
      ><option value="">All statuses</option><option value="healthy"
        >Healthy</option
      ><option value="warning">Warning</option><option value="critical"
        >Critical</option
      ><option value="unknown">Unknown</option></select
    ></label
  >
  <label
    >Hostname<select name="hostname_valid" bind:value={hostnameValid}
      ><option value="">Any result</option><option value="true">Valid</option
      ><option value="false">Invalid</option></select
    ></label
  >
  <label
    >Expires within days<input
      name="expiry_days"
      type="number"
      min="0"
      max="3650"
      bind:value={expiryDays}
      placeholder="Any date"
    /></label
  >
  <div class="filter-actions">
    <button type="submit">Apply filters</button><a
      class="text-link"
      href="/webpages/certificates">Clear</a
    >
  </div>
</form>

{#if data.problem}
  <section class="problem-panel" aria-labelledby="certificate-problem-title">
    <h2 id="certificate-problem-title">
      {data.problem.status === 403
        ? 'Certificate evidence is permission restricted.'
        : 'Certificate evidence is unavailable.'}
    </h2>
    <p>{data.problem.message}</p>
    {#if data.problem.request_id}<p class="request-reference">
        Request ID: <code>{data.problem.request_id}</code>
      </p>{/if}
  </section>
{:else if data.certificates.length === 0}
  <section class="empty-state" aria-labelledby="certificate-empty-title">
    <strong id="certificate-empty-title"
      >No certificate observations match.</strong
    ><span
      >HTTPS monitors appear after a supervised TLS handshake reports bounded
      certificate evidence.</span
    >
  </section>
{:else}
  <section class="admin-section" aria-labelledby="certificate-list-title">
    <div class="operational-section-heading">
      <div>
        <h2 id="certificate-list-title">Observed certificates</h2>
      </div>
      <span class="section-count">{data.certificates.length} shown</span>
    </div>
    <div
      class="table-frame"
      role="region"
      aria-label="Certificate observations"
    >
      <table class="resource-table">
        <thead
          ><tr
            ><th>Endpoint</th><th>Status</th><th>Expires</th><th>Issuer</th><th
              >Hostname</th
            ><th>Last check</th><th>Incident</th></tr
          ></thead
        ><tbody>
          {#each data.certificates as certificate (certificate.id)}<tr
              ><th
                ><a
                  class="resource-name"
                  href={`/webpages/certificates/${certificate.id}`}
                  >{certificate.endpoint}</a
                ><small>{certificate.source}</small></th
              ><td
                ><StatusIndicator
                  state={certificate.state}
                  compact
                />{#if certificate.freshness !== 'fresh'}<small
                    >Freshness: {certificate.freshness}</small
                  >{/if}</td
              ><td
                >{#if certificate.not_after}<Timestamp
                    value={certificate.not_after}
                  />
                  <small
                    >{certificate.days_remaining === undefined
                      ? 'Remaining unknown'
                      : `${certificate.days_remaining} days remaining`}</small
                  >{:else}Unknown{/if}</td
              ><td>{certificate.issuer || 'Not reported'}</td><td
                >{validity(certificate.hostname_valid)}</td
              ><td
                >{#if certificate.observed_at}<Timestamp
                    value={certificate.observed_at}
                  />{:else}Unknown{/if}</td
              ><td
                >{#if certificate.active_incident_id}<a
                    class="text-link"
                    href={`/alerts/${certificate.active_incident_id}`}
                    >View incident</a
                  >{:else}None active{/if}</td
              ></tr
            >{/each}
        </tbody>
      </table>
    </div>
    {#if data.nextCursor}<a class="text-link" href={nextURL}>Next page</a>{/if}
  </section>
{/if}
