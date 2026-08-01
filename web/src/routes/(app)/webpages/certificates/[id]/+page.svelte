<script lang="ts">
  import StatusIndicator from '$lib/components/StatusIndicator.svelte';
  import Timestamp from '$lib/components/Timestamp.svelte';
  let { data } = $props();
  function validity(value: boolean | undefined): string {
    return value === undefined ? 'Not reported' : value ? 'Valid' : 'Invalid';
  }
</script>

<svelte:head
  ><title>{data.certificate?.endpoint ?? 'Certificate'} · Espial</title
  ></svelte:head
>
{#if data.problem}
  <section class="problem-panel" aria-labelledby="certificate-detail-problem">
    <h1 id="certificate-detail-problem">
      {data.problem.status === 404
        ? 'Certificate detail was not found.'
        : 'Certificate detail is unavailable.'}
    </h1>
    <p>{data.problem.message}</p>
    <a class="text-link" href="/webpages/certificates">Return to Certificates</a
    >
  </section>
{:else if data.certificate}
  <header class="page-header">
    <div>
      <h1>{data.certificate.endpoint}</h1>
      <p class="page-description">
        Source: {data.certificate.source} · Freshness: {data.certificate
          .freshness}
      </p>
    </div>
    <StatusIndicator state={data.certificate.state} />
  </header>
  <nav class="section-navigation" aria-label="Webpage views">
    <a href="/webpages">Availability</a><a
      class="active"
      aria-current="page"
      href="/webpages/certificates">Certificates</a
    >
  </nav>
  <section class="admin-section" aria-labelledby="certificate-status-title">
    <div class="operational-section-heading">
      <div>
        <h2 id="certificate-status-title">Validity and identity</h2>
      </div>
      {#if data.certificate.observed_at}<Timestamp
          value={data.certificate.observed_at}
        />{/if}
    </div>
    <dl class="operational-summary certificate-summary">
      <div>
        <dt>Status</dt>
        <dd>{data.certificate.certificate_state}</dd>
      </div>
      <div>
        <dt>Reason</dt>
        <dd>{data.certificate.reason}</dd>
      </div>
      <div>
        <dt>Expiry</dt>
        <dd>
          {#if data.certificate.not_after}<Timestamp
              value={data.certificate.not_after}
            />{:else}Unknown{/if}
        </dd>
      </div>
      <div>
        <dt>Remaining</dt>
        <dd>
          {data.certificate.days_remaining === undefined
            ? 'Unknown'
            : `${data.certificate.days_remaining} days`}
        </dd>
      </div>
      <div>
        <dt>Valid from</dt>
        <dd>
          {#if data.certificate.not_before}<Timestamp
              value={data.certificate.not_before}
            />{:else}Not reported{/if}
        </dd>
      </div>
      <div>
        <dt>Hostname</dt>
        <dd>{validity(data.certificate.hostname_valid)}</dd>
      </div>
      <div>
        <dt>Chain</dt>
        <dd>{validity(data.certificate.chain_valid)}</dd>
      </div>
      <div>
        <dt>Issuer</dt>
        <dd>{data.certificate.issuer || 'Not reported'}</dd>
      </div>
      <div>
        <dt>Subject</dt>
        <dd>{data.certificate.subject || 'Not reported'}</dd>
      </div>
      <div>
        <dt>SAN summary</dt>
        <dd>{data.certificate.san_summary || 'Not reported'}</dd>
      </div>
      <div>
        <dt>Serial</dt>
        <dd><code>{data.certificate.serial_number || 'Not reported'}</code></dd>
      </div>
      <div>
        <dt>SHA-256 fingerprint</dt>
        <dd>
          <code class="fingerprint"
            >{data.certificate.fingerprint_sha256 || 'Not reported'}</code
          >
        </dd>
      </div>
      <div>
        <dt>Replacement evidence</dt>
        <dd>
          {data.certificate.fingerprint_changed
            ? 'Fingerprint changed since prior observation'
            : 'No fingerprint change detected'}
        </dd>
      </div>
      <div>
        <dt>Issuer evidence</dt>
        <dd>
          {data.certificate.issuer_changed
            ? 'Issuer changed since prior observation'
            : 'No issuer change detected'}
        </dd>
      </div>
    </dl>
    {#if data.certificate.active_incident_id}<p class="form-note">
        This condition has an active incident. <a
          class="text-link"
          href={`/alerts/${data.certificate.active_incident_id}`}
          >Open incident</a
        >
      </p>{/if}
  </section>
{/if}

<style>
  .fingerprint {
    overflow-wrap: anywhere;
  }
  .certificate-summary dd {
    text-transform: none;
  }
</style>
