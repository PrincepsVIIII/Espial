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
      page.getByRole('heading', { name: 'Compute nodes' }),
    ).toBeVisible();
    await expect(
      page.getByRole('progressbar', { name: 'CPU total' }),
    ).toHaveAttribute('aria-valuenow', '57.6');
    await expect(
      page.getByRole('button', {
        name: /pve-compute-host-with-a-long-operator-facing-name, 40 VMs online, health Critical/,
      }),
    ).toBeVisible();
    await page.getByText('Host metrics table').click();
    const table = page.getByRole('table', {
      name: 'Host metrics equivalent to the compute-node graphic',
    });
    await expect(table).toBeVisible();
    await expect(
      table.getByRole('row', {
        name: /pve-compute-host-with-a-long-operator-facing-name 40 44% 46% 28% Critical/,
      }),
    ).toBeVisible();

    if (viewport.name === 'large') {
      const clusterPanel = page.locator('.cluster-panel').first();
      await expect(clusterPanel).toHaveCSS(
        'background-color',
        'rgba(0, 0, 0, 0)',
      );
      await expect(clusterPanel).toHaveCSS('border-left-width', '0px');

      const resourceTracks = page.locator('.bar-track');
      const hostTrack = await resourceTracks.nth(0).boundingBox();
      const storageTrack = await resourceTracks.nth(3).boundingBox();
      expect(hostTrack).not.toBeNull();
      expect(storageTrack).not.toBeNull();
      expect(Math.abs(hostTrack!.width - storageTrack!.width)).toBeLessThan(1);
      const marker = await page.locator('.bar-marker').first().boundingBox();
      expect(marker).not.toBeNull();
      expect(marker!.x).toBeGreaterThanOrEqual(hostTrack!.x + hostTrack!.width);
      expect(
        Math.abs(
          marker!.y +
            marker!.height / 2 -
            (hostTrack!.y + hostTrack!.height / 2),
        ),
      ).toBeLessThan(1);
      await expect(page.locator('.reading').last()).toHaveCSS(
        'text-align',
        'right',
      );

      const gauge = page.locator('.semicircle-gauge svg').first();
      expect((await gauge.boundingBox())!.width).toBeGreaterThan(150);
      await expect(page.locator('.gauge-value')).toHaveCount(3);

      const longHost = page.getByRole('button', {
        name: /pve-compute-host-with-a-long-operator-facing-name, 40 VMs online, health Critical/,
      });
      await longHost.hover();
      const tooltip = longHost.locator('xpath=..').locator('[role="tooltip"]');
      await expect(tooltip).toContainText('40 VMs online');
      await expect(tooltip).toHaveCSS('opacity', '1');
    }

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

    await page.mouse.move(0, 0);
    if (
      await page
        .locator('.host-data')
        .evaluate((element) => element.hasAttribute('open'))
    ) {
      await page.getByText('Host metrics table').click();
    }

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
    name: /pve-002, 4 VMs online, health Warning/,
  });
  await plotHost.focus();
  await expect(page.getByRole('heading', { name: 'pve-002' })).toBeVisible();
  await page.keyboard.press('Enter');
  await expect(plotHost).toHaveAttribute('aria-pressed', 'true');
  await page.getByText('Host metrics table').click();
  await expect(
    page.locator('tbody tr.selected').filter({ hasText: 'pve-002' }),
  ).toBeVisible();

  await page.keyboard.press('Escape');
  await expect(plotHost).toHaveAttribute('aria-pressed', 'false');
  await expect(page.getByText('Compute-node selection cleared.')).toBeVisible();

  const tableHost = page.getByRole('button', { name: 'pve-001', exact: true });
  await tableHost.focus();
  await page.keyboard.press('Space');
  await expect(tableHost).toHaveAttribute('aria-pressed', 'true');
  await expect(
    page.getByRole('button', {
      name: /pve-001, 1 VM online, health Healthy/,
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
    name: /pve-002, 4 VMs online, health Warning/,
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
    name: /pve-002, 4 VMs online, health Warning/,
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
    name: /pve-002, 5 VMs online, health Warning/,
  });
  await expect(updated).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByText('Selected · 5 VMs online')).toBeVisible();
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
    await expect(page.locator('.node-button')).toHaveCount(scenario.rows);
    await expect(page.locator('tbody tr')).toHaveCount(scenario.rows);
    if (scenario.mode === 'zero') {
      await expect(page.locator('.node-disc strong')).toHaveText([
        '0',
        '0',
        '0',
      ]);
    }
    if (scenario.mode === 'large') {
      const plot = page.locator('.node-field');
      await expect(plot).toHaveCSS('overflow-y', 'auto');
      await page.locator('.node-button').last().focus();
      expect(
        await plot.evaluate((element) => element.scrollTop),
      ).toBeGreaterThan(0);
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
    .locator('.node-disc')
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
