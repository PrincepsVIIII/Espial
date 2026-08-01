<script lang="ts">
  import StatusIndicator from '$lib/components/StatusIndicator.svelte';
  import Timestamp from '$lib/components/Timestamp.svelte';
  let { data } = $props();
  type StageName = 'dns' | 'tcp' | 'tls' | 'http' | 'body';
  const stageLabels: Record<string, string> = {
    dns: 'DNS',
    tcp: 'TCP',
    tls: 'TLS',
    http: 'HTTP headers',
    body: 'Response body',
  };
  function stageEvidence(stage: StageName): string {
    if (!data.webpage) return 'Not reported';
    if (stage === 'body') return `${data.webpage.stages.body_bytes} bytes`;
    return `${data.webpage.stages[`${stage}_ms`]} ms`;
  }
</script>

<svelte:head
  ><title>{data.webpage?.display_name ?? 'Webpage'} · Espial</title
  ></svelte:head
>

{#if data.problem}
  <section class="problem-panel" aria-labelledby="webpage-problem-title">
    <p class="eyebrow">
      {data.problem.status === 404
        ? 'Not found'
        : data.problem.status === 403
          ? 'Permission denied'
          : 'Core unavailable'}
    </p>
    <h1 id="webpage-problem-title">Webpage detail is unavailable.</h1>
    <p>{data.problem.message}</p>
    <a class="text-link" href="/webpages">Return to Webpages</a>
  </section>
{:else if data.webpage}
  <header class="page-header">
    <div>
      <p class="eyebrow">Availability detail</p>
      <h1>{data.webpage.display_name}</h1>
      <p class="page-description"><code>{data.webpage.url}</code></p>
    </div>
    <StatusIndicator state={data.webpage.state} />
  </header>
  <nav class="section-navigation" aria-label="Webpage views">
    <a href="/webpages">Availability</a>
  </nav>
  <section class="admin-section" aria-labelledby="current-evidence-title">
    <div class="operational-section-heading">
      <div>
        <p class="eyebrow">Core read model</p>
        <h2 id="current-evidence-title">Current evidence</h2>
      </div>
      {#if data.webpage.observed_at}<Timestamp
          value={data.webpage.observed_at}
        />{/if}
    </div>
    <dl class="operational-summary webpage-summary">
      <div>
        <dt>Effective state</dt>
        <dd>{data.webpage.state}</dd>
      </div>
      <div>
        <dt>Raw state</dt>
        <dd>{data.webpage.raw_state}</dd>
      </div>
      <div>
        <dt>Safe reason</dt>
        <dd class="detail">{data.webpage.reason}</dd>
      </div>
      <div>
        <dt>Reason code</dt>
        <dd><code>{data.webpage.reason_code ?? 'Not reported'}</code></dd>
      </div>
      <div>
        <dt>HTTP status</dt>
        <dd>{data.webpage.stages.http_status ?? 'Not reported'}</dd>
      </div>
      <div>
        <dt>Total elapsed</dt>
        <dd>
          {data.webpage.observed_at
            ? `${data.webpage.stages.total_ms} ms`
            : 'Not reported'}
        </dd>
      </div>
      <div>
        <dt>First observed</dt>
        <dd><Timestamp value={data.webpage.first_seen_at} /></dd>
      </div>
      <div>
        <dt>Last resource evidence</dt>
        <dd><Timestamp value={data.webpage.last_seen_at} /></dd>
      </div>
    </dl>
  </section>
  <section
    class="admin-section webpage-stage-section"
    aria-labelledby="stage-title"
  >
    <div class="operational-section-heading">
      <div>
        <p class="eyebrow">Partial failures preserve completed stages</p>
        <h2 id="stage-title">Check stages</h2>
      </div>
    </div>
    <div class="table-frame">
      <table class="resource-table">
        <thead><tr><th>Stage</th><th>Outcome</th><th>Elapsed</th></tr></thead
        ><tbody>
          {#each ['dns', 'tcp', 'tls', 'http', 'body'] as stage (stage)}
            <tr
              ><th>{stageLabels[stage]}</th><td
                >{data.webpage.stages.completed.includes(stage as StageName)
                  ? 'Completed'
                  : 'Not completed'}</td
              ><td>{stageEvidence(stage as StageName)}</td></tr
            >
          {/each}
        </tbody>
      </table>
    </div>
    {#if data.webpage.active_incident_id}<p class="form-note">
        An active incident is linked to this availability check. <a
          class="text-link"
          href={`/alerts/${data.webpage.active_incident_id}`}>Open incident</a
        >
      </p>{/if}
  </section>
{/if}

<style>
  .webpage-stage-section {
    margin-top: 1rem;
  }
  .webpage-summary code {
    overflow-wrap: anywhere;
  }
</style>
