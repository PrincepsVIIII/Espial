<script lang="ts">
  import { goto, invalidate } from '$app/navigation';
  import { page } from '$app/state';
  import { onDestroy, onMount } from 'svelte';
  import { readCookie } from '$lib/auth';
  import BrandLogo from '$lib/components/BrandLogo.svelte';
  import {
    liveConnection,
    startLiveInvalidations,
    type LiveStatus,
  } from '$lib/live';

  let { data, children } = $props();
  let navOpen = $state(false);
  let activeNav = $state<string | null>(null);
  let accountOpen = $state(false);
  let restoringNavFocus = false;
  let restoringAccountFocus = false;
  let navButton = $state<HTMLButtonElement>();
  let accountButton = $state<HTMLButtonElement>();
  let stopLive: (() => void) | null = null;

  const navItems = [
    {
      label: 'Dashboard',
      href: '/dashboard',
      children: [
        { label: 'Resources', href: '/dashboard#resources-title' },
        { label: 'Integrations', href: '/dashboard#integrations-title' },
      ],
    },
    {
      label: 'Alerts',
      href: '/alerts',
      children: [{ label: 'Planned workflow', href: '/alerts#planned-scope' }],
    },
    {
      label: 'Datacenter',
      href: '/datacenter',
      children: [
        { label: 'Planned drill-down', href: '/datacenter#planned-scope' },
      ],
    },
    {
      label: 'Hypervisor',
      href: '/hypervisor',
      children: [
        { label: 'Planned inventory', href: '/hypervisor#planned-scope' },
      ],
    },
    {
      label: 'Webpages',
      href: '/webpages',
      children: [{ label: 'Planned checks', href: '/webpages#planned-scope' }],
    },
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

  function closeNavDropdown(event: FocusEvent) {
    const group = event.currentTarget as HTMLElement;
    const next = event.relatedTarget as Node | null;
    if (!next || !group.contains(next)) activeNav = null;
  }

  function closeNavDropdownOnPointerLeave(event: MouseEvent) {
    const group = event.currentTarget as HTMLElement;
    if (!group.contains(document.activeElement)) activeNav = null;
  }

  function closeAccountDropdown(event: FocusEvent) {
    const menu = event.currentTarget as HTMLElement;
    const next = event.relatedTarget as Node | null;
    if (!next || !menu.contains(next)) accountOpen = false;
  }

  function closeAccountDropdownOnPointerLeave(event: MouseEvent) {
    const menu = event.currentTarget as HTMLElement;
    if (!menu.contains(document.activeElement)) accountOpen = false;
  }

  function handleWindowKeydown(event: KeyboardEvent) {
    if (event.key !== 'Escape') return;
    if (activeNav) {
      const label = activeNav;
      restoringNavFocus = true;
      activeNav = null;
      requestAnimationFrame(() => {
        document
          .querySelector<HTMLButtonElement>(`[data-nav-trigger="${label}"]`)
          ?.focus({ preventScroll: true });
        restoringNavFocus = false;
      });
      return;
    }
    if (accountOpen) {
      restoringAccountFocus = true;
      accountOpen = false;
      accountButton?.focus();
      restoringAccountFocus = false;
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
        <BrandLogo compact />
        <span class="espial-product">Espial</span>
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
        <div class="desktop-primary-nav">
          {#each navItems as item}
            <div
              class="nav-group"
              role="group"
              onmouseenter={() => (activeNav = item.label)}
              onmouseleave={closeNavDropdownOnPointerLeave}
              onfocusout={closeNavDropdown}
            >
              <button
                type="button"
                class="nav-label"
                class:active={isActive(item.href)}
                data-nav-trigger={item.label}
                aria-expanded={activeNav === item.label}
                aria-controls={`nav-panel-${item.label.toLowerCase()}`}
                onfocus={() => {
                  if (!restoringNavFocus) activeNav = item.label;
                }}
                onclick={() => (activeNav = item.label)}
              >
                {item.label}
              </button>
              {#if activeNav === item.label}
                <div
                  class="nav-dropdown"
                  id={`nav-panel-${item.label.toLowerCase()}`}
                >
                  <a
                    href={item.href}
                    aria-current={isActive(item.href) ? 'page' : undefined}
                  >
                    <strong>{item.label}</strong>
                    <span>Open section</span>
                  </a>
                  {#each item.children as child}
                    <a href={child.href}>{child.label}</a>
                  {/each}
                </div>
              {/if}
            </div>
          {/each}
        </div>
        <div class="mobile-primary-nav">
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
          {liveLabels[$liveConnection.status]}
        </span>
        <div
          class="user-menu"
          role="group"
          onmouseenter={() => (accountOpen = true)}
          onmouseleave={closeAccountDropdownOnPointerLeave}
          onfocusout={closeAccountDropdown}
        >
          <button
            class="user-menu-trigger"
            type="button"
            bind:this={accountButton}
            aria-label={`User menu for ${data.session.user.display_name}`}
            aria-controls="user-menu-panel"
            aria-expanded={accountOpen}
            onfocus={() => {
              if (!restoringAccountFocus) accountOpen = true;
            }}
            onclick={() => (accountOpen = true)}
          >
            <span>{data.session.user.display_name}</span>
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
