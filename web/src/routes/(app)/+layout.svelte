<script lang="ts">
  import { goto, invalidate } from '$app/navigation';
  import { page } from '$app/state';
  import { onDestroy, onMount } from 'svelte';
  import { readCookie } from '$lib/auth';
  import BrandLogo from '$lib/components/BrandLogo.svelte';
  import { alertNavigationItems } from '$lib/navigation';
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

  const navItems = $derived([
    {
      label: 'Dashboard',
      href: '/dashboard',
      children: [],
    },
    {
      label: 'Alerts',
      href: '/alerts',
      children: alertNavigationItems(
        data.session?.user.permissions ?? [],
      ).slice(1),
    },
    {
      label: 'Datacenter',
      href: '/datacenter',
      children: [],
    },
    {
      label: 'Hypervisor',
      href: '/hypervisor',
      children: [],
    },
    {
      label: 'Webpages',
      href: '/webpages',
      children: [
        { label: 'Certificates', href: '/webpages/certificates' },
        ...(data.session?.user.permissions.includes('website_monitors:manage')
          ? [{ label: 'Monitors', href: '/webpages/monitors' }]
          : []),
      ],
    },
    ...(data.session?.user.permissions.includes('audit:read')
      ? [
          {
            label: 'Audit',
            href: '/audit',
            children: data.session?.user.permissions.includes('users:manage')
              ? [{ label: 'Users', href: '/audit/users' }]
              : [],
          },
        ]
      : []),
  ]);

  const interruptionLabels: Partial<Record<LiveStatus, string>> = {
    reconnecting: 'Live updates interrupted. Reconnecting…',
    disconnected: 'Live updates are disconnected. Displayed data may be stale.',
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
              onmouseenter={() => {
                if (item.children.length) activeNav = item.label;
              }}
              onmouseleave={closeNavDropdownOnPointerLeave}
              onfocusout={closeNavDropdown}
            >
              <a
                class="nav-link"
                class:active={isActive(item.href)}
                aria-current={isActive(item.href) ? 'page' : undefined}
                href={item.href}
                onfocus={() => {
                  if (!restoringNavFocus && item.children.length) {
                    activeNav = item.label;
                  }
                }}
              >
                {item.label}
              </a>
              {#if item.children.length}
                <button
                  type="button"
                  class="nav-child-toggle"
                  data-nav-trigger={item.label}
                  aria-label={`${item.label} sections`}
                  aria-expanded={activeNav === item.label}
                  aria-controls={`nav-panel-${item.label.toLowerCase()}`}
                  onclick={() => (activeNav = item.label)}
                >
                  <span aria-hidden="true">⌄</span>
                </button>
              {/if}
              {#if item.children.length && activeNav === item.label}
                <div
                  class="nav-dropdown"
                  id={`nav-panel-${item.label.toLowerCase()}`}
                >
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
            <div class="mobile-nav-group">
              <a
                class:active={isActive(item.href)}
                aria-current={isActive(item.href) ? 'page' : undefined}
                href={item.href}
                onclick={closeNavigation}>{item.label}</a
              >
              {#each item.children as child}
                <a
                  class="mobile-child-link"
                  class:active={isActive(child.href)}
                  aria-current={isActive(child.href) ? 'page' : undefined}
                  href={child.href}
                  onclick={closeNavigation}>{child.label}</a
                >
              {/each}
            </div>
          {/each}
          <div class="mobile-account">
            <span>
              <strong>{data.session.user.display_name}</strong>
              <small>{data.session.user.roles.join(', ')}</small>
            </span>
            <button class="link-button" type="button" onclick={logout}
              >Sign out</button
            >
          </div>
        </div>
      </nav>

      <div class="app-meta">
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
    <main id="main-content" class="content">
      {#if interruptionLabels[$liveConnection.status]}
        <div class="connection-notice" role="status" aria-live="polite">
          <span>{interruptionLabels[$liveConnection.status]}</span>
          {#if $liveConnection.last_refresh}
            <small>Last refresh: {$liveConnection.last_refresh}</small>
          {/if}
        </div>
      {/if}
      {@render children()}
    </main>
  </div>
{/if}
