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
      page.getByRole('heading', { name: 'Load distribution' }),
    ).toBeVisible();
    await expect(page.locator('.load-radar')).toHaveCount(1);
    await expect(page.locator('.load-radar .radar-grid')).toHaveCount(5);
    await expect(page.locator('.load-radar .radar-active')).toHaveCount(3);
    await expect(page.locator('.radar-active').first()).toHaveCSS(
      'stroke',
      'rgb(212, 164, 231)',
    );
    await expect(page.locator('.node-count')).toHaveCount(0);
    await expect(page.locator('.console-summary')).toHaveText(
      '9 console-active VMs',
    );
    await expect(page.locator('.console-summary')).toHaveCSS(
      'text-align',
      'center',
    );
    await expect(
      page.locator('.cluster-title .status-indicator', { hasText: 'Warning' }),
    ).toBeVisible();
    await expect(page.getByText(/^Observed/)).toHaveCount(0);
    await expect(page.getByText(/Host Healthy · Swap/)).toHaveCount(0);
    await expect(page.getByText(/Storage (Healthy|Warning) · IO/)).toHaveCount(
      0,
    );
    await expect(page.locator('.node-health')).toHaveCount(0);
    await expect(
      page.getByRole('progressbar', { name: 'CPU total' }),
    ).toHaveAttribute('aria-valuenow', '57.6');
    await expect(
      page.getByRole('button', {
        name: /pve-compute-host-with-a-long-operator-facing-name, 40 idle VMs, 6 console-active VMs, health Critical/,
      }),
    ).toBeVisible();
    await expect(page.getByText('Host metrics table')).toHaveCount(0);
    await expect(page.getByText('Fresh observation')).toHaveCount(0);

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
      expect(hostTrack!.height).toBeLessThan(16);
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
      const gaugeBoxes = await page
        .locator('.semicircle-gauge')
        .evaluateAll((gauges) =>
          gauges.map((element) => element.getBoundingClientRect().toJSON()),
        );
      expect(Math.abs(gaugeBoxes[0].x - gaugeBoxes[1].x)).toBeLessThan(1);
      expect(gaugeBoxes[1].y).toBeGreaterThan(gaugeBoxes[0].y);
      expect(gaugeBoxes[2].y).toBeGreaterThan(gaugeBoxes[1].y);

      const overviewTop = (await page
        .locator('.resource-columns')
        .boundingBox())!.y;
      const loadBox = (await page.locator('.compute-map').boundingBox())!;
      const resourceBox = (await page
        .locator('.resource-columns')
        .boundingBox())!;
      const loadTop = loadBox.y;
      expect(Math.abs(overviewTop - loadTop)).toBeLessThan(2);
      expect(loadBox.width).toBeGreaterThan(resourceBox.width);

      const hostMarker = page.getByRole('button', {
        name: 'pve-001: Swap 2%',
      });
      await hostMarker.hover();
      await expect(hostMarker.locator('[role="tooltip"]')).toHaveCSS(
        'opacity',
        '1',
      );
      await expect(hostMarker.locator('[role="tooltip"]')).toContainText(
        'Swap 2%',
      );
      const hostBarTrack = page.locator('.bar-track').first();
      await hostBarTrack.hover();
      await expect(hostBarTrack.locator('[role="tooltip"]')).toContainText(
        'CPU 18% · RAM 24%',
      );
      await hostBarTrack.focus();
      await expect(hostBarTrack.locator('[role="tooltip"]')).toHaveCSS(
        'opacity',
        '1',
      );
      const storageMarker = page.getByRole('button', {
        name: 'shared-vmstore: I/O wait 7%',
      });
      await storageMarker.focus();
      await expect(storageMarker.locator('[role="tooltip"]')).toContainText(
        'I/O wait 7%',
      );
      const storageBarTrack = page.locator('.bar-track').nth(3);
      await storageBarTrack.hover();
      await expect(storageBarTrack.locator('[role="tooltip"]')).toContainText(
        '70% used',
      );

      const longHost = page.getByRole('button', {
        name: /pve-compute-host-with-a-long-operator-facing-name, 40 idle VMs, 6 console-active VMs, health Critical/,
      });
      await expect(longHost.locator('.node-name')).toHaveCSS('opacity', '1');
      await expect(longHost.locator('.node-load')).toHaveCSS('opacity', '0');
      await longHost.hover();
      await expect(longHost.locator('.node-name')).toHaveCSS('opacity', '0');
      await expect(longHost.locator('.node-load')).toHaveCSS('opacity', '1');
      const tooltip = longHost.locator('xpath=..').locator('[role="tooltip"]');
      await expect(tooltip).toContainText('40 idle VMs');
      await expect(tooltip).toContainText('6 console-active VMs');
      await expect(tooltip).toContainText('Swap 28%');
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

    await page.screenshot({
      path: testInfo.outputPath(`hypervisor-graphics-${viewport.name}.png`),
      fullPage: true,
      animations: 'disabled',
    });
  });
}

test('plot provides keyboard selection and host detail', async ({ page }) => {
  await page.goto('/hypervisor');
  await waitForHydration(page);

  const plotHost = page.getByRole('button', {
    name: /pve-002, 4 idle VMs, 2 console-active VMs, health Warning/,
  });
  await plotHost.focus();
  await expect(plotHost.locator('.node-load')).toHaveCSS('opacity', '1');
  const unselectedNodeWidth = await plotHost
    .locator('.node-disc')
    .evaluate((element) => element.getBoundingClientRect().width);
  await page.keyboard.press('Enter');
  await expect(page.getByRole('heading', { name: 'pve-002' })).toBeVisible();
  await expect(plotHost).toHaveAttribute('aria-pressed', 'true');
  await expect(plotHost.locator('.node-disc')).toHaveCSS(
    'outline-style',
    'none',
  );
  await expect
    .poll(async () =>
      plotHost
        .locator('.node-disc')
        .evaluate((element) => element.getBoundingClientRect().width),
    )
    .toBeGreaterThan(unselectedNodeWidth);
  await expect(page.locator('.node-detail')).toContainText(/CPU\s+31%/);
  await expect(page.locator('.node-detail')).toContainText(/RAM\s+35%/);
  await expect(page.locator('.node-detail')).toContainText(/Swap\s+14%/);
  await expect(page.getByText('pve-002 selected.')).toHaveCount(0);
  await expect(page.getByText(/Selected ·/)).toHaveCount(0);

  await page.keyboard.press('Escape');
  await expect(plotHost).toHaveAttribute('aria-pressed', 'false');
  await expect(page.getByText('Compute-node selection cleared.')).toBeVisible();

  const firstHost = page.getByRole('button', {
    name: /pve-001, 1 idle VM, 1 console-active VM, health Healthy/,
  });
  await expect(firstHost.locator('.node-health')).toHaveCount(0);
  await firstHost.focus();
  await page.keyboard.press('Space');
  await expect(firstHost).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('heading', { name: 'pve-001' })).toBeVisible();
  const firstHostBox = await firstHost.boundingBox();
  expect(firstHostBox).not.toBeNull();
  await firstHost.click({ position: { x: 2, y: firstHostBox!.height / 2 } });
  await expect(firstHost).toHaveAttribute('aria-pressed', 'false');
  await firstHost.focus();
  await page.keyboard.press('Space');
  await expect(firstHost).toHaveAttribute('aria-pressed', 'true');
  await page.locator('.compute-heading').click();
  await expect(firstHost).toHaveAttribute('aria-pressed', 'false');
  await expect(page.locator('.node-detail')).toHaveCount(0);
});

test('switches load distribution to storage main-disk attribution', async ({
  page,
}) => {
  await page.goto('/hypervisor');
  await waitForHydration(page);

  const viewSwitch = page.getByRole('switch', {
    name: 'Show storage load distribution',
  });
  await expect(viewSwitch).toHaveAttribute('aria-checked', 'false');
  await viewSwitch.focus();
  await page.keyboard.press('Space');
  await expect(viewSwitch).toHaveAttribute('aria-checked', 'true');
  await expect(
    page.getByText(/storage backing each running guest's main disk/i),
  ).toBeVisible();
  await expect(
    page.getByText(/EFI, ISO, and auxiliary volumes are excluded/),
  ).toBeVisible();
  await expect(page.locator('.load-radar .radar-active')).toHaveCount(2);

  const storage = page.getByRole('button', {
    name: /shared-vmstore, 8 idle VMs, 2 console-active VMs, health Warning/,
  });
  await storage.click();
  await expect(
    page.getByRole('heading', { name: 'shared-vmstore' }),
  ).toBeVisible();
  await expect(page.locator('.node-detail')).toContainText(/Used\s+70%/);
  await expect(page.locator('.node-detail')).toContainText(/I\/O wait\s+7%/);
});

test('one console-active VM renders one lavender active spoke', async ({
  page,
  request,
}) => {
  await request.get(`${mockCore}/__test/control?hypervisor=console-one`);
  await page.goto('/hypervisor');
  await waitForHydration(page);

  await expect(page.locator('.load-radar .radar-active')).toHaveCount(1);
  await expect(page.locator('.radar-active')).toHaveCSS(
    'stroke-dasharray',
    'none',
  );
  await expect(page.locator('.console-summary')).toHaveText(
    '1 console-active VM',
  );
});

test('storage attribution remains explicit when guest disk reads are unavailable', async ({
  page,
  request,
}) => {
  await request.get(
    `${mockCore}/__test/control?hypervisor=storage-unavailable`,
  );
  await page.goto('/hypervisor');
  await waitForHydration(page);
  await page
    .getByRole('switch', { name: 'Show storage load distribution' })
    .click();
  await expect(page.locator('.unavailable-storage')).toBeVisible();
  await expect(page.locator('.load-radar')).toHaveCount(0);
});

test('administrator opens the audited cluster configuration editor', async ({
  page,
}) => {
  await page.goto('/hypervisor');
  await waitForHydration(page);
  await page
    .getByRole('button', { name: 'Edit settings for Production Proxmox' })
    .click();
  await expect(
    page.getByRole('heading', { name: 'Edit Proxmox research cluster' }),
  ).toBeVisible();
  await expect(page.getByLabel('Cluster name')).toHaveValue(
    'Proxmox research cluster',
  );
  await expect(page.getByLabel('Token secret')).toHaveValue('');
  await expect(
    page.getByText('Leave empty to retain the stored secret.'),
  ).toBeVisible();
  await page.getByRole('button', { name: 'Validate and save' }).click();
  await expect(page.getByText('Cluster configuration saved.')).toBeVisible();
  await expect(
    page.getByRole('link', { name: 'Audit record' }),
  ).toHaveAttribute(
    'href',
    '/audit?correlation_id=proxmox-configuration-request-1',
  );
});

test('touch selects a host without topology controls', async ({ browser }) => {
  const context = await browser.newContext({
    hasTouch: true,
    viewport: { width: 500, height: 900 },
  });
  const page = await context.newPage();
  await page.goto('/hypervisor');
  await waitForHydration(page);
  const marker = page.getByRole('button', {
    name: 'pve-001: Swap 2%',
  });
  await marker.tap();
  await expect(marker).toHaveAttribute('aria-expanded', 'true');
  await expect(marker.locator('[role="tooltip"]')).toHaveCSS('opacity', '1');
  const track = page.getByRole('button', {
    name: 'pve-001: CPU 18% · RAM 24%',
  });
  await track.tap();
  await expect(track).toHaveAttribute('aria-expanded', 'true');
  await expect(track.locator('[role="tooltip"]')).toHaveCSS('opacity', '1');
  const host = page.getByRole('button', {
    name: /pve-002, 4 idle VMs, 2 console-active VMs, health Warning/,
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
    name: /pve-002, 4 idle VMs, 2 console-active VMs, health Warning/,
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
    name: /pve-002, 5 idle VMs, 2 console-active VMs, health Warning/,
  });
  await expect(updated).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('.node-detail p')).toHaveText(
    '5 idle VMs · 2 console-active VMs',
  );
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
    await expect(page.locator('table')).toHaveCount(0);
    await expect(page.locator('.load-radar')).toHaveCount(
      scenario.mode === 'zero' ? 1 : 0,
    );
    if (scenario.mode === 'one') {
      await expect(page.locator('.compact-node-field')).toHaveCount(1);
      await expect(page.locator('.node-count')).toHaveCount(0);
      await expect(page.locator('.console-summary')).toHaveText(
        '1 console-active VM',
      );
    }
    if (scenario.mode === 'zero') {
      await expect(page.locator('.node-load strong')).toHaveText([
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
  await expect(page.getByText('Stale observation')).toHaveCount(0);
  await expect(
    page.locator('.cluster-title').getByText('Stale', { exact: true }),
  ).toBeVisible();

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
