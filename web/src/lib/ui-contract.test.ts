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
const alertHistoryPage = source('../routes/(app)/alerts/history/+page.svelte');
const alertDetailPage = source('../routes/(app)/alerts/[id]/+page.svelte');
const alertRulesPage = source('../routes/(app)/alerts/rules/+page.svelte');
const alertSuppressionsPage = source(
  '../routes/(app)/alerts/suppressions/+page.svelte',
);
const alertNotificationsPage = source(
  '../routes/(app)/alerts/notifications/+page.svelte',
);
const datacenterPage = source('../routes/(app)/datacenter/+page.svelte');
const hypervisorPage = source('../routes/(app)/hypervisor/+page.svelte');
const webpagesPage = source('../routes/(app)/webpages/+page.svelte');
const auditPage = source('../routes/(app)/audit/+page.svelte');
const usersPage = source('../routes/(app)/audit/users/+page.svelte');
const overviewRedirect = source('../routes/(app)/overview/+page.ts');
const stylesheet = source('../styles.css');
const brandLogo = source('../lib/components/BrandLogo.svelte');
const coreUserHandler = source('../../../core/internal/api/user_handlers.go');

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
  it('keeps the six primary routes in the required order', () => {
    const expected = [
      '/dashboard',
      '/alerts',
      '/datacenter',
      '/hypervisor',
      '/webpages',
      '/audit',
    ];
    const positions = expected.map((href) =>
      appShell.indexOf(`href: '${href}'`),
    );
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

  it('uses the supplied brand asset and minimalist floating navigation contract', () => {
    expect(appShell).toContain('BrandLogo compact');
    expect(brandLogo).toContain('data:image/png;base64');
    expect(stylesheet).toContain('.nav-link::after');
    expect(stylesheet).toContain('background: var(--hayes-white)');
    expect(stylesheet).toContain('.nav-dropdown');
    expect(stylesheet).toMatch(
      /\.app-header\s*\{[\s\S]*?position:\s*fixed;[\s\S]*?top:\s*1rem/,
    );
  });

  it('makes primary labels direct links and exposes only implemented children', () => {
    expect(appShell).toContain('class="nav-link"');
    expect(appShell).toContain('href={item.href}');
    expect(appShell).toContain("{ label: 'Users', href: '/audit/users' }");
    expect(appShell).toContain("{ label: 'History', href: '/alerts/history' }");
    expect(appShell).toContain("{ label: 'Rules', href: '/alerts/rules' }");
    expect(appShell).toContain(
      "{ label: 'Suppressions', href: '/alerts/suppressions' }",
    );
    expect(appShell).toContain(
      "{ label: 'Notifications', href: '/alerts/notifications' }",
    );
    expect(appShell).toContain("permissions.includes('incident_rules:manage')");
    expect(appShell).toContain("permissions.includes('suppressions:manage')");
    expect(appShell).toContain("'notification_destinations:manage',");
    expect(appShell).not.toMatch(
      /Planned workflow|Planned inventory|Open section/,
    );
    expect(appShell).toContain('aria-label={`${item.label} sections`}');
  });

  it('loads the session through SvelteKit and has a Core-unavailable state', () => {
    expect(appLoader).toContain("'/api/v1/auth/session'");
    expect(appLoader).toContain('redirect(303');
    expect(appShell).toContain('Core unavailable');
    expect(appShell).toContain('Retry connection');
  });

  it('keeps normal connection state out of navigation and reports interruptions', () => {
    expect(appShell).not.toContain("live: 'Live'");
    expect(appShell).not.toContain('connection-status');
    expect(appShell).toContain('Live updates interrupted');
    expect(appShell).toContain('Live updates are disconnected');
    expect(appShell).toContain("invalidate('espial:monitoring')");
  });

  it('keeps cyan separate from semantic dashboard state colors', () => {
    expect(stylesheet).toMatch(
      /\.summary-introduction\s*\{[\s\S]*?border-top:\s*2px solid var\(--netdef-cyan\)/,
    );
    const summaryRule = stylesheet.match(
      /\.monitoring-summary\s*\{[\s\S]*?\n\}/,
    )?.[0];
    expect(summaryRule).not.toContain('border-top');
    expect(stylesheet).toMatch(
      /\.state-counts > div\s*\{[\s\S]*?border-top:\s*3px solid currentColor/,
    );
  });
});

describe('administrator capability evidence', () => {
  it('provides discoverable Audit and Users pages rather than a documentation-only claim', () => {
    expect(auditPage).toContain('<h1>Audit</h1>');
    expect(auditPage).toContain('name="correlation_id"');
    expect(usersPage).toContain('<h1>Users</h1>');
    expect(usersPage).toContain('Create local user');
    expect(usersPage).toContain('Replace password');
  });

  it('exposes rule and suppression controls with receipts and honest semantics', () => {
    expect(alertRulesPage).toContain('<h1>Alert rules</h1>');
    expect(alertRulesPage).toContain('Explain rule precedence');
    expect(alertRulesPage).toContain('View matching audit record');
    expect(alertSuppressionsPage).toContain('<h1>Suppressions</h1>');
    expect(alertSuppressionsPage).toContain('Maintenance windows');
    expect(alertSuppressionsPage).toContain('Silencing never changes health');
    expect(alertSuppressionsPage).toContain('View matching audit record');
  });

  it('exposes redacted destination controls and delivery evidence only after the full path exists', () => {
    expect(alertNotificationsPage).toContain('<h1>Alert notifications</h1>');
    expect(alertNotificationsPage).toContain('Send labeled test');
    expect(alertNotificationsPage).toContain(
      'Endpoint and secret details are write-only',
    );
    expect(alertNotificationsPage).toContain('Waiting to retry');
    expect(alertNotificationsPage).toContain('View matching audit record');
    expect(alertDetailPage).toContain('Destination delivery evidence');
  });

  it('shows mutation proof and links it to an exact audit receipt', () => {
    expect(usersPage).toContain('Request ID:');
    expect(usersPage).toContain('View matching audit record');
    expect(usersPage).toContain('/audit?correlation_id=');
    expect(coreUserHandler).toContain('self_lockout');
    expect(coreUserHandler).toContain('last_administrator');
  });
});

describe('primary route skeletons', () => {
  it('makes Dashboard the canonical replacement for Overview', () => {
    expect(dashboardPage).toContain('<h1>Dashboard</h1>');
    expect(dashboardPage).toContain('Monitoring coverage');
    expect(dashboardPage).toContain('Authoritative resource health');
    expect(dashboardPage).toContain('Collection coverage');
    expect(dashboardPage).toContain('Active incidents');
    expect(dashboardPage).toContain('/alerts/${incident.id}');
    expect(overviewRedirect).toContain("redirect(308, '/dashboard')");
  });

  it('exposes authoritative read-only active, history, and detail incident views', () => {
    expect(alertsPage).toContain('<h1>Alerts</h1>');
    expect(alertsPage).toContain('history={false}');
    expect(alertHistoryPage).toContain('<h1>Alert history</h1>');
    expect(alertDetailPage).toContain('Immutable lifecycle record');
    expect(alertDetailPage).toContain('Read-only');
    expect(alertsPage).not.toContain('UnavailableSection');
  });

  it.each([
    ['Datacenter', datacenterPage],
    ['Hypervisor', hypervisorPage],
    ['Webpages', webpagesPage],
  ])('%s exposes an honest unavailable state', (name, page) => {
    expect(page).toContain(`title="${name}"`);
    expect(page).toMatch(/not implemented|not configured|no .* configured/i);
  });
});
