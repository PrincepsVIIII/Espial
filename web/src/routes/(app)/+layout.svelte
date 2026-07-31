<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { readCookie, type SessionResponse } from '$lib/auth';

  let { children } = $props();
  let session = $state<SessionResponse | null>(null);
  let loading = $state(true);

  onMount(async () => {
    const response = await fetch('/api/v1/auth/session');
    if (!response.ok) {
      await goto(
        `/login?returnTo=${encodeURIComponent(page.url.pathname + page.url.search)}`,
      );
      return;
    }
    session = await response.json();
    loading = false;
  });

  async function logout() {
    await fetch('/api/v1/auth/logout', {
      method: 'POST',
      headers: { 'X-CSRF-Token': readCookie('espial_csrf') },
    });
    await goto('/login');
  }
</script>

{#if loading}
  <main class="loading" aria-live="polite">Loading Espial…</main>
{:else if session}
  <div class="shell">
    <aside>
      <a class="wordmark" href="/overview"
        ><span class="brand-mark small">E</span> Espial</a
      >
      <nav aria-label="Primary">
        <a class:active={page.url.pathname === '/overview'} href="/overview"
          >Overview</a
        >
      </nav>
      <div class="account">
        <strong>{session.user.display_name}</strong>
        <span>{session.user.roles.join(', ')}</span>
        <button class="link-button" onclick={logout}>Sign out</button>
      </div>
    </aside>
    <main class="content">{@render children()}</main>
  </div>
{/if}
