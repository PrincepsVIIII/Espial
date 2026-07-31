import { defineConfig, devices } from '@playwright/test';

const webPort = 4174;
const corePort = 18081;

export default defineConfig({
  testDir: './tests/browser',
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? [['html', { open: 'never' }], ['list']] : 'list',
  use: {
    baseURL: `http://127.0.0.1:${webPort}`,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: [
    {
      command: `node tests/mock-core.mjs ${corePort}`,
      port: corePort,
      reuseExistingServer: !process.env.CI,
      timeout: 30_000,
    },
    {
      command: `npm run dev -- --host 127.0.0.1 --port ${webPort}`,
      port: webPort,
      env: { ESPIAL_CORE_URL: `http://127.0.0.1:${corePort}` },
      reuseExistingServer: !process.env.CI,
      timeout: 60_000,
    },
  ],
});
