<script lang="ts">
  import BrandLogo from '$lib/components/BrandLogo.svelte';
</script>

<svelte:head>
  <title>Espial · UBNetDef Infrastructure Operations</title>
  <meta
    name="description"
    content="Espial brings infrastructure health, operational context, and physical hardware location into one UBNetDef operator workspace."
  />
</svelte:head>

<div class="public-page">
  <header class="public-header">
    <a class="wordmark" href="/" aria-label="Espial public home">
      <BrandLogo />
      <span class="espial-product">Espial</span>
    </a>
    <a class="login-link" href="/login">Log in</a>
  </header>

  <main id="main-content" class="public-main">
    <section class="public-intro" aria-labelledby="public-title">
      <h1 id="public-title">
        Understand system health and locate the hardware behind it.
      </h1>
      <p>
        Espial is UBNetDef’s operations interface for monitoring infrastructure,
        understanding what needs attention, and moving from a system-level
        signal to its physical location.
      </p>
      <a class="text-link" href="/login"
        >Continue to authenticated access <span aria-hidden="true">→</span></a
      >
    </section>

    <section
      class="public-capability-section"
      aria-labelledby="capability-title"
    >
      <div class="section-heading">
        <h2 id="capability-title">
          One operational path from status to equipment.
        </h2>
      </div>
      <dl class="public-capabilities">
        <div>
          <dt>Monitor</dt>
          <dd>
            Bring current health, freshness, and integration coverage into one
            operator view.
          </dd>
        </div>
        <div>
          <dt>Understand</dt>
          <dd>
            Connect alerts to affected services and the resources that support
            them.
          </dd>
        </div>
        <div>
          <dt>Locate</dt>
          <dd>
            Move from a datacenter room to a rack, server, and individual drive.
          </dd>
        </div>
      </dl>
    </section>

    <section class="public-access" aria-labelledby="access-title">
      <div>
        <h2 id="access-title">
          Operational details stay behind authentication.
        </h2>
      </div>
      <p>
        This temporary public page contains no live status or environment
        details. Local access is audited while UBNetDef SSO is completed.
      </p>
    </section>
  </main>

  <footer class="public-footer">
    <span>Espial</span>
    <span>UBNetDef Infrastructure Operations</span>
  </footer>
</div>
