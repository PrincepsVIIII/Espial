<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { safeReturnTo } from '$lib/auth';
  import BrandLogo from '$lib/components/BrandLogo.svelte';

  let username = $state('');
  let password = $state('');
  let submitting = $state(false);
  let error = $state('');
  let capabilities = $state({ local: true, sso: false });
  let notice = $derived(
    page.url.searchParams.get('notice') === 'password_changed'
      ? 'Password changed and active sessions revoked. Sign in again to continue.'
      : '',
  );
  let receiptRequestID = $derived(
    /^[A-Za-z0-9._:-]{1,128}$/.test(
      page.url.searchParams.get('request_id') ?? '',
    )
      ? (page.url.searchParams.get('request_id') ?? '')
      : '',
  );

  onMount(async () => {
    const response = await fetch('/api/v1/auth/capabilities');
    if (response.ok) capabilities = await response.json();
  });

  async function login(event: SubmitEvent) {
    event.preventDefault();
    submitting = true;
    error = '';
    try {
      const response = await fetch('/api/v1/auth/local/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      });
      if (!response.ok) {
        const body = await response.json().catch(() => null);
        error = body?.error?.message ?? 'Sign-in is temporarily unavailable.';
        return;
      }
      await goto(safeReturnTo(page.url.searchParams.get('returnTo')));
    } finally {
      password = '';
      submitting = false;
    }
  }
</script>

<main class="login-page">
  <div class="login-frame">
    <header class="login-header">
      <div class="wordmark" aria-label="Espial by UBNetDef">
        <BrandLogo />
        <span class="espial-product">Espial</span>
      </div>
      <a class="public-home-link" href="/">Public overview</a>
    </header>
    <div class="login-grid">
      <section class="login-context" aria-labelledby="login-context-title">
        <div>
          <p class="eyebrow">Protected operations workspace</p>
          <h1 id="login-context-title">
            Infrastructure monitoring for UBNetDef.
          </h1>
          <p class="muted">
            Authenticated access to resource health, integration state, and
            source freshness as the Phase 1 monitoring slices come online.
          </p>
        </div>
        <p class="auth-note">Local access is temporary and fully audited</p>
      </section>
      <section class="login-card" aria-labelledby="login-title">
        <p class="eyebrow">Local authentication</p>
        <h2 id="login-title">Sign in</h2>
        <p class="muted">
          Use the administrator account created during bootstrap. UBNetDef SSO
          will replace this primary flow when its integration is ready.
        </p>
        {#if notice}
          <div class="login-notice" role="status">
            <strong>{notice}</strong>
            {#if receiptRequestID}
              <span>Request ID: <code>{receiptRequestID}</code></span>
            {/if}
          </div>
        {/if}
        {#if capabilities.local}
          <form onsubmit={login}>
            <label
              >Username<input
                bind:value={username}
                autocomplete="username"
                required
              /></label
            >
            <label
              >Password<input
                type="password"
                bind:value={password}
                autocomplete="current-password"
                required
              /></label
            >
            {#if error}<p class="error" role="alert">{error}</p>{/if}
            <button disabled={submitting}
              >{submitting ? 'Signing in…' : 'Sign in'}</button
            >
          </form>
        {/if}
        {#if capabilities.sso}<button class="secondary"
            >Continue with UBNetDef SSO</button
          >{/if}
      </section>
    </div>
  </div>
</main>
