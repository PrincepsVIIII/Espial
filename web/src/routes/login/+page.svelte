<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { safeReturnTo } from '$lib/auth';

  let username = '';
  let password = '';
  let submitting = false;
  let error = '';
  let capabilities = { local: true, sso: false };

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
  <section class="login-card" aria-labelledby="login-title">
    <div class="brand-mark">E</div>
    <p class="eyebrow">Espial control plane</p>
    <h1 id="login-title">Sign in</h1>
    <p class="muted">
      Use the temporary local administrator account while SSO integration is
      completed.
    </p>
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
    {#if capabilities.sso}<button class="secondary">Continue with SSO</button
      >{/if}
  </section>
</main>
