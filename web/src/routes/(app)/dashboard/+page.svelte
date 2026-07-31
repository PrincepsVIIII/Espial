<svelte:head><title>Dashboard · Espial</title></svelte:head>

<header class="page-header">
  <div>
    <p class="eyebrow">UBNetDef infrastructure operations</p>
    <h1>Dashboard</h1>
  </div>
  <span class="state-label state-unknown"
    ><i aria-hidden="true"></i>Monitoring pending</span
  >
</header>

<section
  class="state-panel dashboard-foundation"
  aria-labelledby="foundation-title"
>
  <div class="state-panel-summary">
    <p class="eyebrow">Phase 1 foundation</p>
    <h2 id="foundation-title">The protected workspace is active.</h2>
    <p>
      Authentication and the session boundary are implemented. Health counts,
      integration coverage, alerts, and resource state will appear only after
      Core exposes authoritative monitoring data.
    </p>
  </div>
  <dl class="operational-summary">
    <div>
      <dt>Authentication</dt>
      <dd>
        <span class="state-mark state-healthy" aria-hidden="true"
        ></span>Available
      </dd>
      <dd class="detail">Audited local fallback</dd>
    </div>
    <div>
      <dt>Session boundary</dt>
      <dd>
        <span class="state-mark state-healthy" aria-hidden="true"
        ></span>Available
      </dd>
      <dd class="detail">Verified through Espial Core</dd>
    </div>
    <div>
      <dt>Monitoring data</dt>
      <dd>
        <span class="state-mark state-unknown" aria-hidden="true"></span>Not
        available
      </dd>
      <dd class="detail">No resource state is displayed yet</dd>
    </div>
  </dl>
</section>

<section class="next-sections" aria-labelledby="next-sections-title">
  <div class="section-heading">
    <p class="eyebrow">Primary workspace</p>
    <h2 id="next-sections-title">Operational sections</h2>
  </div>
  <nav aria-label="Operational sections">
    <a href="/alerts"
      ><span>Alerts</span><small>Alert data not available</small></a
    >
    <a href="/datacenter"
      ><span>Datacenter</span><small>Inventory not configured</small></a
    >
    <a href="/hypervisor"
      ><span>Hypervisor</span><small>Integration not configured</small></a
    >
    <a href="/webpages"
      ><span>Webpages</span><small>Checks not configured</small></a
    >
  </nav>
</section>
