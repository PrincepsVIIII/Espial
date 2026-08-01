import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

const mockCore = 'http://127.0.0.1:18081';

test.beforeEach(async ({ request }) => {
  await request.get(`${mockCore}/__test/control?reset=true`);
});

test('public page is factual, accessible, and keeps sign-in visible', async ({
  page,
}, testInfo) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1 })).toContainText(
    'Understand system health',
  );
  await expect(page.getByRole('link', { name: 'Log in' })).toBeVisible();
  await expect(page.getByText('Core switch 1')).toHaveCount(0);
  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath('public.png'),
    fullPage: true,
    animations: 'disabled',
  });
});

test('login carries the same UBNetDef brand shell', async ({
  page,
}, testInfo) => {
  await page.goto('/login');
  await expect(page.getByRole('img', { name: 'UBNetDef' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath('login.png'),
    fullPage: true,
    animations: 'disabled',
  });
});

for (const viewport of [
  { name: 'large', width: 1440, height: 900 },
  { name: 'laptop', width: 1280, height: 800 },
  { name: 'narrow', width: 500, height: 900 },
]) {
  test(`Dashboard preserves critical information at ${viewport.name} viewport`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize(viewport);
    await page.goto('/dashboard');
    await waitForHydration(page);
    await expect(
      page.getByRole('heading', { name: 'Monitoring coverage' }),
    ).toBeVisible();
    await expect(page.getByText('Core switch 1')).toBeVisible();
    await expect(page.getByText('Archive node 3')).toBeVisible();
    await expect(
      page.locator('.integration-identity strong', {
        hasText: 'Sample infrastructure',
      }),
    ).toBeVisible();
    await expect(page.getByLabel('Stale').first()).toBeVisible();
    if (viewport.name === 'narrow') {
      await page.getByRole('button', { name: 'Menu' }).click();
      const navigation = page.getByRole('navigation', { name: 'Primary' });
      await expect(navigation.getByRole('link')).toHaveText([
        'Dashboard',
        'Alerts',
        'History',
        'Rules',
        'Suppressions',
        'Notifications',
        'Datacenter',
        'Hypervisor',
        'Webpages',
        'Audit',
        'Users',
      ]);
      await expect(
        navigation.getByRole('button', { name: 'Sign out' }),
      ).toBeVisible();
    }
    await page.screenshot({
      path: testInfo.outputPath(`dashboard-${viewport.name}.png`),
      fullPage: true,
      animations: 'disabled',
    });
  });
}

for (const viewport of [
  { name: 'large', width: 1440, height: 900 },
  { name: 'laptop', width: 1280, height: 800 },
  { name: 'narrow', width: 500, height: 900 },
]) {
  test(`Alerts preserves incident evidence at ${viewport.name} viewport`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize(viewport);
    await page.goto('/alerts');
    await waitForHydration(page);
    await expect(page.getByRole('heading', { name: 'Alerts' })).toBeVisible();
    await expect(page.getByText('Compute node 2: availability')).toBeVisible();
    await expect(page.getByText('Host unreachable.')).toBeVisible();
    await page
      .getByRole('link', { name: 'Compute node 2: availability' })
      .click();
    await expect(
      page.getByRole('heading', { name: 'Current state' }),
    ).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Timeline' })).toBeVisible();
    await expect(
      page.getByText('Incident detected: host unreachable'),
    ).toBeVisible();
    const results = await new AxeBuilder({ page }).analyze();
    expect(
      results.violations.filter((violation) =>
        ['serious', 'critical'].includes(violation.impact ?? ''),
      ),
    ).toEqual([]);
    await page.screenshot({
      path: testInfo.outputPath(`incident-${viewport.name}.png`),
      fullPage: true,
      animations: 'disabled',
    });
  });
}

test('operator action returns a visible receipt and exactly one audit record', async ({
  page,
  request,
}) => {
  await page.goto(`/alerts/${'80000000-0000-4000-8000-000000000021'}`);
  await waitForHydration(page);
  await expect(page.getByRole('heading', { name: 'Workflow' })).toBeVisible();
  await page.getByRole('button', { name: 'Acknowledge' }).click();
  const receipt = page.locator('.mutation-receipt');
  await expect(receipt).toContainText('Incident acknowledged.');
  await expect(receipt).toContainText('Request ID:');
  await expect(
    receipt.getByRole('link', { name: 'View matching audit record' }),
  ).toBeVisible();
  await expect(
    page.getByText('Acknowledged', { exact: true }).first(),
  ).toBeVisible();
  await expect
    .poll(async () => {
      const response = await request.get(`${mockCore}/__test/state`);
      return (await response.json()).incidentAuditCount;
    })
    .toBe(1);
});

test('viewer sees read-only evidence without operator controls', async ({
  page,
  request,
}) => {
  await request.get(`${mockCore}/__test/control?session=viewer`);
  await page.goto(`/alerts/${'80000000-0000-4000-8000-000000000021'}`);
  await waitForHydration(page);
  await expect(
    page.getByRole('heading', { name: 'Read-only incident record' }),
  ).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Workflow' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Acknowledge' })).toHaveCount(
    0,
  );
});

test('stale operator action reloads and asks for review', async ({
  page,
  request,
}) => {
  await request.get(`${mockCore}/__test/control?api=conflict-once`);
  await page.goto(`/alerts/${'80000000-0000-4000-8000-000000000021'}`);
  await waitForHydration(page);
  await page.getByRole('button', { name: 'Acknowledge' }).click();
  await expect(page.getByRole('alert')).toContainText(
    'The current state has been reloaded; review it before submitting again.',
  );
  const state = await (await request.get(`${mockCore}/__test/state`)).json();
  expect(state.incidentAuditCount).toBe(0);
});

test('filters are URL-backed and keep stale distinct from unknown', async ({
  page,
}) => {
  await page.goto('/dashboard');
  await waitForHydration(page);
  await page.locator('select[name="state"]').selectOption('stale');
  await page.locator('input[name="kind"]').fill('server');
  await page.getByRole('button', { name: 'Apply filters' }).click();
  await expect(page).toHaveURL(/state=stale/);
  await expect(page).toHaveURL(/kind=server/);
  await expect(page.getByText('Archive node 3')).toBeVisible();
  await expect(page.getByText('Unknown node 4')).toHaveCount(0);
});

test('user dropdown supports keyboard Escape and returns focus', async ({
  page,
}) => {
  await page.goto('/dashboard');
  await waitForHydration(page);
  const trigger = page.getByRole('button', {
    name: 'User menu for NOC Administrator',
  });
  await trigger.click();
  await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('button', { name: 'Sign out' })).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test('primary navigation links directly and its real child menu closes with Escape', async ({
  page,
}, testInfo) => {
  await page.goto('/dashboard');
  await waitForHydration(page);
  await expect(
    page.getByRole('link', { name: 'Dashboard', exact: true }),
  ).toHaveAttribute('href', '/dashboard');
  const trigger = page.getByRole('button', { name: 'Audit sections' });
  await trigger.hover();
  await expect(page.getByRole('link', { name: 'Users' })).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath('audit-dropdown.png'),
    animations: 'disabled',
  });
  await page.getByRole('link', { name: 'Users' }).focus();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('link', { name: 'Users' })).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test('administrator alert controls are discoverable and viewer navigation hides them', async ({
  page,
  request,
}) => {
  await page.goto('/dashboard');
  await waitForHydration(page);
  const alertTrigger = page.getByRole('button', { name: 'Alerts sections' });
  await alertTrigger.click();
  await expect(
    page.getByRole('link', { name: 'Rules', exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole('link', { name: 'Suppressions', exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole('link', { name: 'Notifications', exact: true }),
  ).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(alertTrigger).toBeFocused();

  await request.get(`${mockCore}/__test/control?session=viewer`);
  await page.goto('/alerts');
  await waitForHydration(page);
  await page.getByRole('button', { name: 'Alerts sections' }).click();
  await expect(
    page.getByRole('link', { name: 'Rules', exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole('link', { name: 'Suppressions', exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole('link', { name: 'Notifications', exact: true }),
  ).toHaveCount(0);

  await page.goto('/alerts/rules');
  await waitForHydration(page);
  await expect(page.getByRole('alert')).toContainText(
    'You do not have permission to manage incident rules.',
  );
  await expect(page.getByRole('button', { name: 'Create rule' })).toHaveCount(
    0,
  );

  await page.goto('/alerts/notifications');
  await waitForHydration(page);
  await expect(page.getByRole('alert')).toContainText(
    'You do not have permission to manage notification destinations.',
  );
});

for (const viewport of [
  { name: 'large', width: 1440, height: 900 },
  { name: 'laptop', width: 1280, height: 800 },
  { name: 'narrow', width: 500, height: 900 },
]) {
  test(`rule and suppression administration remains usable at ${viewport.name} viewport`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize(viewport);
    await page.goto('/alerts/rules');
    await waitForHydration(page);
    await expect(
      page.getByRole('heading', { name: 'Alert rules' }),
    ).toBeVisible();
    await expect(page.getByText('Default resource health')).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Explain rule precedence' }),
    ).toBeVisible();

    await page.goto('/alerts/suppressions');
    await waitForHydration(page);
    await expect(
      page.getByRole('heading', { name: 'Suppressions' }),
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: 'Maintenance windows' }),
    ).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Silences' })).toBeVisible();
    const results = await new AxeBuilder({ page }).analyze();
    expect(
      results.violations.filter((violation) =>
        ['serious', 'critical'].includes(violation.impact ?? ''),
      ),
    ).toEqual([]);
    await page.screenshot({
      path: testInfo.outputPath(`suppressions-${viewport.name}.png`),
      fullPage: true,
      animations: 'disabled',
    });
  });
}

for (const viewport of [
  { name: 'large', width: 1440, height: 900 },
  { name: 'laptop', width: 1280, height: 800 },
  { name: 'narrow', width: 500, height: 900 },
]) {
  test(`notification administration and evidence remain usable at ${viewport.name} viewport`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize(viewport);
    await page.goto('/alerts/notifications');
    await waitForHydration(page);
    await expect(
      page.getByRole('heading', { name: 'Alert notifications' }),
    ).toBeVisible();
    await expect(page.getByText('Operations Mattermost').first()).toBeVisible();
    await expect(page.getByText('Waiting to retry')).toBeVisible();
    await page.getByRole('button', { name: 'Send labeled test' }).click();
    await expect(page.getByRole('status')).toContainText(
      'Explicitly labeled test delivery queued.',
    );
    await expect(
      page.getByRole('link', { name: 'View matching audit record' }),
    ).toBeVisible();
    const results = await new AxeBuilder({ page }).analyze();
    expect(
      results.violations.filter((violation) =>
        ['serious', 'critical'].includes(violation.impact ?? ''),
      ),
    ).toEqual([]);
    await page.screenshot({
      path: testInfo.outputPath(`notifications-${viewport.name}.png`),
      fullPage: true,
      animations: 'disabled',
    });
  });
}

test('SSE reconnects, refreshes REST, and respects reduced motion', async ({
  page,
  request,
}) => {
  await request.get(`${mockCore}/__test/control?events=disconnect-once`);
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/dashboard');
  const interruption = page.locator('.connection-notice');
  await expect(interruption).toContainText('Reconnecting');
  await expect(interruption).toHaveCount(0, {
    timeout: 5_000,
  });
  await expect
    .poll(async () => {
      const response = await request.get(`${mockCore}/__test/state`);
      return (await response.json()).monitoringReads;
    })
    .toBeGreaterThanOrEqual(6);
  const reducedDuration = await page
    .locator('.app-header')
    .evaluate((element) => getComputedStyle(element).animationDuration);
  expect(Number.parseFloat(reducedDuration)).toBeLessThanOrEqual(0.00001);
});

test('resync triggers a full authoritative refresh', async ({
  page,
  request,
}) => {
  await request.get(`${mockCore}/__test/control?events=resync`);
  await page.goto('/dashboard');
  await expect
    .poll(async () => {
      const response = await request.get(`${mockCore}/__test/state`);
      const state = await response.json();
      return state.streamConnections;
    })
    .toBeGreaterThanOrEqual(2);
  await expect
    .poll(async () => {
      const response = await request.get(`${mockCore}/__test/state`);
      return (await response.json()).monitoringReads;
    })
    .toBeGreaterThanOrEqual(6);
});

test('floating navigation stays fixed while dashboard content scrolls', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 500 });
  await page.goto('/dashboard');
  await waitForHydration(page);
  const header = page.locator('.app-header');
  const before = await header.boundingBox();
  expect(
    await header.evaluate((element) => getComputedStyle(element).position),
  ).toBe('fixed');
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  const after = await header.boundingBox();
  expect(after?.y).toBeCloseTo(before?.y ?? 0, 0);
});

test('user changes return a receipt linked to the exact audit evidence', async ({
  page,
}, testInfo) => {
  await page.goto('/audit/users');
  await waitForHydration(page);
  await expect(page.getByRole('heading', { name: 'Users' })).toBeVisible();
  const usersAccessibility = await new AxeBuilder({ page }).analyze();
  expect(
    usersAccessibility.violations.filter((violation) =>
      ['serious', 'critical'].includes(violation.impact ?? ''),
    ),
  ).toEqual([]);
  await page.screenshot({
    path: testInfo.outputPath('users.png'),
    fullPage: true,
    animations: 'disabled',
  });
  await page.getByLabel('Username').fill('new-viewer');
  await page.getByLabel('Display name').fill('New Viewer');
  await page.getByLabel('Email').fill('viewer@example.test');
  await page.getByLabel('Role').selectOption('viewer');
  await page
    .getByLabel('Password', { exact: true })
    .fill('A browser viewer password 90210');
  await page
    .getByLabel('Confirm password')
    .fill('A browser viewer password 90210');
  await page.getByRole('button', { name: 'Create user' }).click();
  const receiptLink = page.getByRole('link', {
    name: 'View matching audit record',
  });
  await expect(page.getByText('Created new-viewer.')).toBeVisible();
  await expect(receiptLink).toBeVisible();
  await receiptLink.click();
  await expect(page).toHaveURL(/\/audit\?correlation_id=/);
  await expect(page.getByText('auth.local.user.created')).toBeVisible();
  await page.getByText('View redacted change summary').click();
  await expect(page.getByText('new-viewer')).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath('audit-receipt.png'),
    fullPage: true,
    animations: 'disabled',
  });
});

test('permission and Core failures retain shell context without stale rows', async ({
  page,
  request,
}) => {
  await request.get(`${mockCore}/__test/control?api=forbidden`);
  await page.goto('/dashboard');
  await expect(page.getByText('Permission denied')).toBeVisible();
  await expect(page.getByText('Resource access denied')).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Primary' })).toBeVisible();

  await request.get(`${mockCore}/__test/control?api=unavailable`);
  await page.goto('/dashboard?state=critical');
  await expect(page.getByText('Resource data unavailable')).toBeVisible();
  await expect(page.getByText('Core switch 1')).toHaveCount(0);
  await expect(page.getByRole('navigation', { name: 'Primary' })).toBeVisible();
});

test('unauthenticated protected routes return safely to login', async ({
  page,
  request,
}) => {
  await request.get(`${mockCore}/__test/control?session=unauthorized`);
  await page.goto('/dashboard');
  await expect(page).toHaveURL(/\/login\?returnTo=/);
});

test('Dashboard has no serious automated accessibility violations', async ({
  page,
}) => {
  await page.goto('/dashboard');
  const results = await new AxeBuilder({ page }).analyze();
  expect(
    results.violations.filter((violation) =>
      ['serious', 'critical'].includes(violation.impact ?? ''),
    ),
  ).toEqual([]);
});

async function waitForHydration(page: import('@playwright/test').Page) {
  await expect(page.locator('.shell')).toBeVisible({ timeout: 5_000 });
  await page.waitForFunction(() => document.readyState === 'complete');
}
