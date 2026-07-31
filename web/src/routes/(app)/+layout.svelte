<script lang="ts">
  import { goto, invalidate } from '$app/navigation';
  import { page } from '$app/state';
  import { onDestroy, onMount } from 'svelte';
  import { readCookie } from '$lib/auth';
  import {
    liveConnection,
    startLiveInvalidations,
    type LiveStatus,
  } from '$lib/live';

  let { data, children } = $props();
  let navOpen = $state(false);
  let accountOpen = $state(false);
  let navButton = $state<HTMLButtonElement>();
  let accountButton = $state<HTMLButtonElement>();
  let stopLive: (() => void) | null = null;

  const navItems = [
    { label: 'Dashboard', href: '/dashboard' },
    { label: 'Alerts', href: '/alerts' },
    { label: 'Datacenter', href: '/datacenter' },
    { label: 'Hypervisor', href: '/hypervisor' },
    { label: 'Webpages', href: '/webpages' },
  ];

  const liveLabels: Record<LiveStatus, string> = {
    live: 'Live',
    reconnecting: 'Reconnecting',
    disconnected: 'Disconnected',
  };

  onMount(() => {
    if (!data.session) return;
    stopLive = startLiveInvalidations({
      onInvalidate: () => invalidate('espial:monitoring'),
      onUnauthorized: () =>
        goto(
          `/login?returnTo=${encodeURIComponent(page.url.pathname + page.url.search)}`,
        ),
    });
  });

  onDestroy(() => stopLive?.());

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
    stopLive?.();
    try {
      await fetch('/api/v1/auth/logout', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'X-CSRF-Token': readCookie('espial_csrf') },
      });
    } finally {
      await goto('/');
    }
  }
</script>

<svelte:window onkeydown={handleWindowKeydown} />

{#if data.loadError}
  <main class="system-state" aria-live="assertive">
    <p class="eyebrow">Core unavailable</p>
    <h1>Espial cannot verify this session.</h1>
    <p>{data.loadError}</p>
    <div class="system-actions">
      <button onclick={() => window.location.reload()}>Retry connection</button>
      <a class="text-link" href="/">Return to public page</a>
    </div>
  </main>
{:else if data.session}
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
            <strong>{data.session.user.display_name}</strong>
            <small>{data.session.user.roles.join(', ')}</small>
            <small class={`live-text live-${$liveConnection.status}`}>
              {liveLabels[$liveConnection.status]}
            </small>
          </span>
          <button class="link-button" type="button" onclick={logout}
            >Sign out</button
          >
        </div>
      </nav>

      <div class="app-meta">
        <span
          class={`connection-status live-${$liveConnection.status}`}
          aria-live="polite"
          title={$liveConnection.last_refresh
            ? `Last successful refresh: ${$liveConnection.last_refresh}`
            : 'Waiting for the first monitoring refresh'}
        >
          <i aria-hidden="true"></i>
          {liveLabels[$liveConnection.status]}
        </span>
        <div class="user-menu">
          <button
            class="user-menu-trigger"
            type="button"
            bind:this={accountButton}
            aria-label={`User menu for ${data.session.user.display_name}`}
            aria-controls="user-menu-panel"
            aria-expanded={accountOpen}
            onclick={() => (accountOpen = !accountOpen)}
          >
            <span>{data.session.user.display_name}</span>
            <span aria-hidden="true">▾</span>
          </button>
          {#if accountOpen}
            <div id="user-menu-panel" class="user-menu-panel">
              <strong>{data.session.user.display_name}</strong>
              <span>{data.session.user.roles.join(', ')}</span>
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
