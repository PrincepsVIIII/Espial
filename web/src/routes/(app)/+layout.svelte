<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { readCookie, type SessionResponse } from '$lib/auth';

  let { children } = $props();
  let session = $state<SessionResponse | null>(null);
  let loading = $state(true);
  let loadError = $state('');
  let navOpen = $state(false);
  let accountOpen = $state(false);
  let navButton = $state<HTMLButtonElement>();
  let accountButton = $state<HTMLButtonElement>();

  const navItems = [
    { label: 'Dashboard', href: '/dashboard' },
    { label: 'Alerts', href: '/alerts' },
    { label: 'Datacenter', href: '/datacenter' },
    { label: 'Hypervisor', href: '/hypervisor' },
    { label: 'Webpages', href: '/webpages' },
  ];

  onMount(async () => {
    try {
      const response = await fetch('/api/v1/auth/session');
      if (response.status === 401 || response.status === 403) {
        await goto(
          `/login?returnTo=${encodeURIComponent(page.url.pathname + page.url.search)}`,
        );
        return;
      }
      if (!response.ok) {
        loadError =
          'Espial Core is unavailable. Session state could not be verified.';
        return;
      }
      session = await response.json();
    } catch {
      loadError =
        'Espial Core is unavailable. Session state could not be verified.';
    } finally {
      loading = false;
    }
  });

  function isActive(href: string): boolean {
    return (
      page.url.pathname === href || page.url.pathname.startsWith(`${href}/`)
    );
  }

  function closeNavigation() {
    navOpen = false;
  }

  function handleWindowKeydown(event: KeyboardEvent) {
    if (event.key !== 'Escape') return;
    if (accountOpen) {
      accountOpen = false;
      accountButton?.focus();
      return;
    }
    if (navOpen) {
      navOpen = false;
      navButton?.focus();
    }
  }

  async function logout() {
    await fetch('/api/v1/auth/logout', {
      method: 'POST',
      headers: { 'X-CSRF-Token': readCookie('espial_csrf') },
    });
    await goto('/');
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

{#if loading}
  <main class="system-state" aria-live="polite">
    <p class="eyebrow">Espial / UBNetDef Operations</p>
    <h1>Verifying session</h1>
    <p>Connecting to Espial Core…</p>
  </main>
{:else if loadError}
  <main class="system-state" aria-live="assertive">
    <p class="eyebrow">Core unavailable</p>
    <h1>Espial cannot verify this session.</h1>
    <p>{loadError}</p>
    <div class="system-actions">
      <button onclick={() => window.location.reload()}>Retry connection</button>
      <a class="text-link" href="/">Return to public page</a>
    </div>
  </main>
{:else if session}
  <div class="shell">
    <a class="skip-link" href="#main-content">Skip to content</a>
    <header class="app-header">
      <a class="wordmark" href="/dashboard" aria-label="Espial Dashboard">
        <span class="product-lockup">
          <span class="product-name">Espial</span>
          <span class="product-context">UBNetDef Operations</span>
        </span>
      </a>

      <button
        class="nav-toggle"
        type="button"
        bind:this={navButton}
        aria-controls="primary-navigation"
        aria-expanded={navOpen}
        onclick={() => (navOpen = !navOpen)}
      >
        <span aria-hidden="true">☰</span>
        Menu
      </button>

      <nav
        id="primary-navigation"
        class:open={navOpen}
        class="primary-nav"
        aria-label="Primary"
      >
        {#each navItems as item}
          <a
            class:active={isActive(item.href)}
            aria-current={isActive(item.href) ? 'page' : undefined}
            href={item.href}
            onclick={closeNavigation}>{item.label}</a
          >
        {/each}
        <div class="mobile-account">
          <span>
            <strong>{session.user.display_name}</strong>
            <small>{session.user.roles.join(', ')}</small>
          </span>
          <button class="link-button" type="button" onclick={logout}
            >Sign out</button
          >
        </div>
      </nav>

      <div class="app-meta">
        <span class="connection-status">
          <i aria-hidden="true"></i>
          Core connected
        </span>
        <div class="user-menu">
          <button
            class="user-menu-trigger"
            type="button"
            bind:this={accountButton}
            aria-label={`User menu for ${session.user.display_name}`}
            aria-controls="user-menu-panel"
            aria-expanded={accountOpen}
            onclick={() => (accountOpen = !accountOpen)}
          >
            <span>{session.user.display_name}</span>
            <span aria-hidden="true">▾</span>
          </button>
          {#if accountOpen}
            <div id="user-menu-panel" class="user-menu-panel">
              <strong>{session.user.display_name}</strong>
              <span>{session.user.roles.join(', ')}</span>
              <button class="link-button" type="button" onclick={logout}
                >Sign out</button
              >
            </div>
          {/if}
        </div>
      </div>
    </header>
    <main id="main-content" class="content">{@render children()}</main>
  </div>
{/if}
