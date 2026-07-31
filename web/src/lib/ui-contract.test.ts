import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

function source(path: string): string {
  return readFileSync(new URL(path, import.meta.url), 'utf8');
}

const publicPage = source('../routes/+page.svelte');
const appShell = source('../routes/(app)/+layout.svelte');
const appLoader = source('../routes/(app)/+layout.ts');
const dashboardPage = source('../routes/(app)/dashboard/+page.svelte');
const alertsPage = source('../routes/(app)/alerts/+page.svelte');
const datacenterPage = source('../routes/(app)/datacenter/+page.svelte');
const hypervisorPage = source('../routes/(app)/hypervisor/+page.svelte');
const webpagesPage = source('../routes/(app)/webpages/+page.svelte');
const overviewRedirect = source('../routes/(app)/overview/+page.ts');
const stylesheet = source('../styles.css');

describe('public entry contract', () => {
  it('identifies Espial and UBNetDef and keeps login at the public root', () => {
    expect(publicPage).toContain('Espial');
    expect(publicPage).toContain('UBNetDef Infrastructure Operations');
    expect(publicPage).toMatch(/class="login-link" href="\/login"/);
  });

  it('describes the product without embedding operational fixtures', () => {
    expect(publicPage).toContain('What Espial does');
    expect(publicPage).toMatch(
      /contains no live status or environment\s+details/,
    );
    expect(publicPage).not.toMatch(/node-\d|rack-\d|critical\s+\d/i);
  });
});

describe('authenticated shell contract', () => {
  it('keeps the five primary routes in the required order', () => {
    const expected = [
      "{ label: 'Dashboard', href: '/dashboard' }",
      "{ label: 'Alerts', href: '/alerts' }",
      "{ label: 'Datacenter', href: '/datacenter' }",
      "{ label: 'Hypervisor', href: '/hypervisor' }",
      "{ label: 'Webpages', href: '/webpages' }",
    ];
    const positions = expected.map((item) => appShell.indexOf(item));
    expect(positions.every((position) => position >= 0)).toBe(true);
    expect(positions).toEqual([...positions].sort((a, b) => a - b));
  });

  it('uses top navigation with a narrow-screen menu and no primary sidebar', () => {
    expect(appShell).toContain('class="app-header"');
    expect(appShell).toContain('aria-controls="primary-navigation"');
    expect(appShell).not.toMatch(/class="sidebar"|<aside/i);
    expect(stylesheet).toContain('.primary-nav.open');
    expect(stylesheet).not.toContain('.sidebar');
  });

  it('loads the session through SvelteKit and has a Core-unavailable state', () => {
    expect(appLoader).toContain("'/api/v1/auth/session'");
    expect(appLoader).toContain('redirect(303');
    expect(appShell).toContain('Core unavailable');
    expect(appShell).toContain('Retry connection');
  });

  it('shows all required live-connection states and uses invalidation refresh', () => {
    expect(appShell).toContain("live: 'Live'");
    expect(appShell).toContain("reconnecting: 'Reconnecting'");
    expect(appShell).toContain("disconnected: 'Disconnected'");
    expect(appShell).toContain("invalidate('espial:monitoring')");
  });
});

describe('primary route skeletons', () => {
  it('makes Dashboard the canonical replacement for Overview', () => {
    expect(dashboardPage).toContain('<h1>Dashboard</h1>');
    expect(dashboardPage).toContain('Monitoring coverage');
    expect(dashboardPage).toContain('Authoritative resource health');
    expect(dashboardPage).toContain('Collection coverage');
    expect(dashboardPage).not.toMatch(/incident count|active incidents/i);
    expect(overviewRedirect).toContain("redirect(308, '/dashboard')");
  });

  it.each([
    ['Alerts', alertsPage],
    ['Datacenter', datacenterPage],
    ['Hypervisor', hypervisorPage],
    ['Webpages', webpagesPage],
  ])('%s exposes an honest unavailable state', (name, page) => {
    expect(page).toContain(`title="${name}"`);
    expect(page).toMatch(/not implemented|not configured|no .* configured/i);
  });
});
