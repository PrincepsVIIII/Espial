import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';

const mockCore = 'http://127.0.0.1:18081';

test.beforeEach(async ({ request }) => {
  await request.get(`${mockCore}/__test/control?reset=true`);
});

for (const viewport of [
  { name: 'large', width: 1440, height: 900 },
  { name: 'laptop', width: 1280, height: 800 },
  { name: 'narrow', width: 500, height: 900 },
  { name: 'zoom-200', width: 640, height: 800 },
]) {
  test(`Hypervisor graphics remain complete at ${viewport.name} viewport`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize(viewport);
    await page.goto('/hypervisor');
    await waitForHydration(page);

    await expect(
      page.getByRole('heading', { name: 'Active VMs by host' }),
    ).toBeVisible();
    await expect(
      page.getByRole('progressbar', { name: 'CPU total' }),
    ).toHaveAttribute('aria-valuenow', '57.6');
    await expect(
      page.getByRole('button', {
        name: /pve-compute-host-with-a-long-operator-facing-name, 40 active VMs, health Critical/,
      }),
    ).toBeVisible();
    await expect(
      page.getByRole('table', {
        name: 'Host metrics equivalent to the active VM plot',
      }),
    ).toBeVisible();

    const overflow = await page.evaluate(
      () =>
        document.documentElement.scrollWidth -
        document.documentElement.clientWidth,
    );
    expect(overflow).toBeLessThanOrEqual(1);

    const results = await new AxeBuilder({ page }).analyze();
    expect(
      results.violations.filter((violation) =>
        ['serious', 'critical'].includes(violation.impact ?? ''),
      ),
    ).toEqual([]);

    await page.screenshot({
      path: testInfo.outputPath(`hypervisor-graphics-${viewport.name}.png`),
      fullPage: true,
      animations: 'disabled',
    });
  });
}

test('plot and table provide linked keyboard selection', async ({ page }) => {
  await page.goto('/hypervisor');
  await waitForHydration(page);

  const plotHost = page.getByRole('button', {
    name: /pve-002, 4 active VMs, health Warning/,
  });
  await plotHost.focus();
  await expect(page.getByRole('heading', { name: 'pve-002' })).toBeVisible();
  await page.keyboard.press('Enter');
  await expect(plotHost).toHaveAttribute('aria-pressed', 'true');
  await expect(
    page.locator('tbody tr.selected').filter({ hasText: 'pve-002' }),
  ).toBeVisible();

  await page.keyboard.press('Escape');
  await expect(plotHost).toHaveAttribute('aria-pressed', 'false');
  await expect(page.getByText('Host selection cleared.')).toBeVisible();

  const tableHost = page.getByRole('button', { name: 'pve-001', exact: true });
  await tableHost.focus();
  await page.keyboard.press('Space');
  await expect(tableHost).toHaveAttribute('aria-pressed', 'true');
  await expect(
    page.getByRole('button', {
      name: /pve-001, 1 active VM, health Healthy/,
    }),
  ).toHaveAttribute('aria-pressed', 'true');
});

test('touch selects a host without topology controls', async ({ browser }) => {
  const context = await browser.newContext({
    hasTouch: true,
    viewport: { width: 500, height: 900 },
  });
  const page = await context.newPage();
  await page.goto('/hypervisor');
  await waitForHydration(page);
  const host = page.getByRole('button', {
    name: /pve-002, 4 active VMs, health Warning/,
  });
  await host.tap();
  await expect(host).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('heading', { name: 'pve-002' })).toBeVisible();
  await expect(
    page.getByRole('button', { name: /zoom|layout|drag/i }),
  ).toHaveCount(0);
  await context.close();
});

test('selection survives live authoritative data replacement', async ({
  page,
  request,
}) => {
  await request.get(`${mockCore}/__test/control?events=hypervisor-update`);
  await page.goto('/hypervisor');
  await waitForHydration(page);
  const initial = page.getByRole('button', {
    name: /pve-002, 4 active VMs, health Warning/,
  });
  await initial.click();
  await expect(initial).toHaveAttribute('aria-pressed', 'true');

  await expect
    .poll(async () => {
      const response = await request.get(`${mockCore}/__test/state`);
      return (await response.json()).hypervisorRevision;
    })
    .toBe(1);
  const updated = page.getByRole('button', {
    name: /pve-002, 5 active VMs, health Warning/,
  });
  await expect(updated).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByText('5 active VMs')).toBeVisible();
});

for (const scenario of [
  { mode: 'zero', rows: 3 },
  { mode: 'one', rows: 1 },
  { mode: 'large', rows: 128 },
]) {
  test(`${scenario.mode} host dataset renders every host`, async ({
    page,
    request,
  }) => {
    await request.get(`${mockCore}/__test/control?hypervisor=${scenario.mode}`);
    await page.goto('/hypervisor');
    await waitForHydration(page);
    await expect(page.locator('.plot-row')).toHaveCount(scenario.rows);
    await expect(page.locator('tbody tr')).toHaveCount(scenario.rows);
    if (scenario.mode === 'zero') {
      await expect(page.locator('.vm-count')).toHaveText(['0', '0', '0']);
    }
    if (scenario.mode === 'large') {
      await expect(page.locator('.plot-rows')).toHaveCSS('overflow-y', 'auto');
    }
  });
}

test('stale, disconnected, empty, and Core-error states stay explicit', async ({
  page,
  request,
}) => {
  await request.get(`${mockCore}/__test/control?hypervisor=stale`);
  await page.goto('/hypervisor');
  await waitForHydration(page);
  await expect(page.getByText('Stale observation')).toBeVisible();

  await request.get(`${mockCore}/__test/control?reset=true`);
  await request.get(`${mockCore}/__test/control?events=disconnect-once`);
  await page.goto('/hypervisor');
  await waitForHydration(page);
  await expect(
    page.getByText(/Live updates (interrupted|are disconnected)/),
  ).toBeVisible();

  await request.get(`${mockCore}/__test/control?reset=true`);
  await request.get(`${mockCore}/__test/control?hypervisor=empty`);
  await page.goto('/hypervisor?fixture=off');
  await waitForHydration(page);
  await expect(
    page.getByText('No Proxmox clusters are configured.'),
  ).toBeVisible();

  await request.get(`${mockCore}/__test/control?reset=true`);
  await request.get(`${mockCore}/__test/control?api=unavailable`);
  await page.goto('/hypervisor');
  await waitForHydration(page);
  await expect(
    page.getByRole('heading', { name: 'Hypervisor data is unavailable.' }),
  ).toBeVisible();
  await expect(page.getByText('browser-test-request')).toBeVisible();
});

test('reduced motion removes graphic transitions and continuous animation', async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/hypervisor');
  await waitForHydration(page);
  const transition = await page
    .locator('.marker')
    .first()
    .evaluate((element) => getComputedStyle(element).transitionDuration);
  expect(Number.parseFloat(transition)).toBeLessThanOrEqual(0.001);
  const animations = await page.evaluate(
    () =>
      document
        .getAnimations()
        .filter((animation) => animation.playState === 'running').length,
  );
  expect(animations).toBe(0);
});

async function waitForHydration(page: import('@playwright/test').Page) {
  await expect(page.locator('.shell')).toBeVisible({ timeout: 5_000 });
  await page.waitForFunction(() => document.readyState === 'complete');
}
