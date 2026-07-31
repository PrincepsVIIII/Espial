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
        'Datacenter',
        'Hypervisor',
        'Webpages',
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
    name: 'User menu for NOC Operator',
  });
  await trigger.click();
  await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('button', { name: 'Sign out' })).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test('primary navigation dropdown opens on hover and closes with Escape', async ({
  page,
}, testInfo) => {
  await page.goto('/dashboard');
  await waitForHydration(page);
  const trigger = page.getByRole('button', { name: 'Dashboard' });
  await trigger.hover();
  await expect(page.getByRole('link', { name: 'Resources' })).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath('dashboard-dropdown.png'),
    animations: 'disabled',
  });
  await page.getByRole('link', { name: 'Resources' }).focus();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('link', { name: 'Resources' })).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test('SSE reconnects, refreshes REST, and respects reduced motion', async ({
  page,
  request,
}) => {
  await request.get(`${mockCore}/__test/control?events=disconnect-once`);
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/dashboard');
  const liveStatus = page.locator('.connection-status');
  await expect(liveStatus).toHaveText('Reconnecting');
  await expect(liveStatus).toHaveText('Live', {
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
  await expect(page.locator('.connection-status')).toHaveText('Live', {
    timeout: 5_000,
  });
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
  await expect(page.locator('.connection-status')).toHaveText('Live', {
    timeout: 5_000,
  });
}
