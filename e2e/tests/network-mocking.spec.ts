import { test, expect } from '@playwright/test';

const BASE = process.env.LOKLINGO_E2E_BASE_URL || 'http://localhost:3300';

test.describe('Network resilience: API mocking edge cases', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });
  });

  async function fillAndTranslate(page: import('@playwright/test').Page) {
    const textWorkflowBtn = page.locator('.workflow-card').filter({ hasText: 'Text' }).first();
    if (await textWorkflowBtn.isVisible().catch(() => false)) {
      await textWorkflowBtn.click();
      await page.waitForTimeout(300);
    }

    const textarea = page.locator('textarea').first();
    await textarea.fill('Hello world');
    await page.waitForTimeout(200);

    const translateBtn = page.locator('button.translate-btn').first();
    if (await translateBtn.isEnabled({ timeout: 5000 }).catch(() => false)) {
      await translateBtn.click();
    }
  }

  test('500 Server Error on /api/v1/jobs shows graceful error toast', async ({ page }) => {
    await page.route('**/api/v1/jobs', (route) => {
      if (route.request().method() === 'POST') {
        route.fulfill({
          status: 500,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'Internal server error' }),
        });
      } else {
        route.continue();
      }
    });

    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await fillAndTranslate(page);
    await page.waitForTimeout(2000);

    const toast = page.locator('.toast-container .toast').first();
    await expect(toast).toBeVisible({ timeout: 5000 });
    const toastText = await toast.textContent().catch(() => '');
    expect(toastText.toLowerCase()).toContain('error');
  });

  test('429 rate limit on /api/v1/jobs shows error feedback', async ({ page }) => {
    await page.route('**/api/v1/jobs', (route) => {
      if (route.request().method() === 'POST') {
        route.fulfill({
          status: 429,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'Rate limit exceeded', retry_after: 30 }),
        });
      } else {
        route.continue();
      }
    });

    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await fillAndTranslate(page);
    await page.waitForTimeout(2000);

    const toast = page.locator('.toast-container .toast').first();
    await expect(toast).toBeVisible({ timeout: 5000 });
    const toastText = await toast.textContent().catch(() => '');
    expect(toastText.toLowerCase()).toMatch(/error|rate limit|429/);
  });

  test('delayed 10s response does not crash the UI', async ({ page }) => {
    await page.route('**/api/v1/jobs', async (route) => {
      if (route.request().method() === 'POST') {
        await new Promise((r) => setTimeout(r, 10_000));
        route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ job_id: 'mock-job-001' }),
        });
      } else {
        route.continue();
      }
    });

    await page.route('**/api/v1/jobs/mock-job-001', (route) => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ status: 'completed', translated_text: 'Hallo Welt' }),
      });
    });

    const pageErrors: string[] = [];
    page.on('pageerror', (err) => pageErrors.push(err.message));

    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await fillAndTranslate(page);
    await page.waitForTimeout(3000);

    expect(pageErrors.length).toBe(0);
  });

  test('invalid JSON response does not crash the UI', async ({ page }) => {
    await page.route('**/api/v1/jobs', (route) => {
      if (route.request().method() === 'POST') {
        route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: 'this is not valid json at all',
        });
      } else {
        route.continue();
      }
    });

    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await fillAndTranslate(page);
    await page.waitForTimeout(2000);

    const toast = page.locator('.toast-container .toast').first();
    await expect(toast).toBeVisible({ timeout: 5000 });
    const toastText = await toast.textContent().catch(() => '');
    expect(toastText).toBeTruthy();
  });

  test('network failure (aborted) does not crash the UI', async ({ page }) => {
    await page.route('**/api/v1/jobs', (route) => {
      if (route.request().method() === 'POST') {
        route.abort('connectionrefused');
      } else {
        route.continue();
      }
    });

    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await fillAndTranslate(page);
    await page.waitForTimeout(2000);

    const toast = page.locator('.toast-container .toast').first();
    await expect(toast).toBeVisible({ timeout: 5000 });
    const toastText = await toast.textContent().catch(() => '');
    expect(toastText).toBeTruthy();
  });
});
