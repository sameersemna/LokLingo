import { test, expect } from '@playwright/test';

const BASE = process.env.LOKLINGO_E2E_BASE_URL || 'http://localhost:3300';

test.describe('Visual regression snapshots', () => {
  test.use({ baseURL: BASE });

  test('home page hero layout', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });
    await page.goto('/', { waitUntil: 'networkidle' });
    await page.waitForTimeout(2000);
    await expect(page).toHaveScreenshot('home-hero.png', {
      fullPage: false,
      mask: [page.locator('[class*="readiness"], [class*="status-chip"]')],
    });
  });

  test('demo gallery panel open', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });
    await page.goto('/', { waitUntil: 'networkidle' });
    await page.waitForTimeout(2000);

    const demosBtn = page.getByRole('button', { name: /demos/i }).first();
    if (await demosBtn.isVisible().catch(() => false)) {
      await demosBtn.click();
      await page.waitForTimeout(500);
    }

    await expect(page).toHaveScreenshot('demo-gallery.png', { fullPage: false });
  });

  test('reliability panel open', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });
    await page.goto('/', { waitUntil: 'networkidle' });
    await page.waitForTimeout(2000);

    const relBtn = page.getByRole('button', { name: /reliability/i }).first();
    if (await relBtn.isVisible().catch(() => false)) {
      await relBtn.click();
      await page.waitForTimeout(500);
    }

    await expect(page).toHaveScreenshot('reliability-panel.png', { fullPage: false });
  });

  test('mobile menu at 375x812', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });
    await page.goto('/', { waitUntil: 'networkidle' });
    await page.waitForTimeout(2000);

    const menuButton = page.locator('button').filter({ hasText: /menu|hamburger|☰/i }).first();
    if (await menuButton.isVisible().catch(() => false)) {
      await menuButton.click();
      await page.waitForTimeout(500);
    }

    await expect(page).toHaveScreenshot('mobile-menu.png', { fullPage: false });
  });
});
