import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.LOKLINGO_E2E_BASE_URL || 'http://localhost:3000';
const backendURL = process.env.LOKLINGO_BASE_URL || 'http://localhost:28080';

export default defineConfig({
  testDir: './tests',
  timeout: 90_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: [
    ['list'],
    ['html', { open: 'never', outputFolder: 'playwright-report' }],
  ],
  use: {
    baseURL,
    headless: true,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    extraHTTPHeaders: {
      // Optional write token for authenticated UI/API flows when set.
      ...(process.env.WRITE_API_TOKEN
        ? { 'X-API-Token': process.env.WRITE_API_TOKEN }
        : {}),
    },
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  metadata: {
    backendURL,
  },
});
