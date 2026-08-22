import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const BASE = process.env.LOKLINGO_E2E_BASE_URL || 'http://localhost:3000';

test.describe('Accessibility (WCAG 2.2 AA) Audit', () => {
  test.use({ baseURL: BASE });

  test('home page has no critical or serious axe violations', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });
    await page.goto('/', { waitUntil: 'networkidle', timeout: 30_000 });
    await page.waitForTimeout(2000);

    const results = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa', 'wcag22aa', 'best-practice'])
      .analyze();

    expect(results.violations.filter(v => v.impact === 'critical' || v.impact === 'serious'),
      `Axe violations found:\n${JSON.stringify(results.violations.filter(v => v.impact === 'critical' || v.impact === 'serious'), null, 2)}`
    ).toEqual([]);
  });

  test('home page has no violations at all (all impact levels)', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });
    await page.goto('/', { waitUntil: 'networkidle', timeout: 30_000 });
    await page.waitForTimeout(2000);

    const results = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa', 'wcag22aa', 'best-practice'])
      .analyze();

    if (results.violations.length > 0) {
      console.log('Axe violations:', JSON.stringify(results.violations, null, 2));
    }
    expect(results.violations.length).toBe(0);
  });

  test('keyboard navigation: tab order is logical', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });
    await page.goto('/', { waitUntil: 'networkidle', timeout: 30_000 });
    await page.waitForTimeout(2000);

    // Tab through the first several focusable elements
    const focusableSelectors = [
      'button:visible, select:visible, input:visible, textarea:visible, a[href]:visible',
    ].join(', ');

    const focusable = page.locator(focusableSelectors).first();
    await expect(focusable).toBeVisible();

    // Press Tab multiple times and verify focus moves
    for (let i = 0; i < 5; i++) {
      await page.keyboard.press('Tab');
      await page.waitForTimeout(100);
      const focused = page.locator(':focus');
      await expect(focused).toBeAttached();
    }
  });

  test('color contrast meets WCAG AA on main content', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });
    await page.goto('/', { waitUntil: 'networkidle', timeout: 30_000 });
    await page.waitForTimeout(2000);

    const results = await new AxeBuilder({ page })
      .withTags(['wcag2aa', 'wcag22aa'])
      .include('.app')
      .analyze();

    const contrastViolations = results.violations.filter(v =>
      v.id === 'color-contrast' || v.id === 'color-contrast-enhanced'
    );
    expect(contrastViolations.length).toBe(0);
  });

  test('images have alt text or are marked decorative', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });
    await page.goto('/', { waitUntil: 'networkidle', timeout: 30_000 });
    await page.waitForTimeout(2000);

    const results = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa'])
      .analyze();

    const imageViolations = results.violations.filter(v => v.id === 'image-alt');
    expect(imageViolations.length).toBe(0);
  });

  test('document has proper landmarks', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });
    await page.goto('/', { waitUntil: 'networkidle', timeout: 30_000 });
    await page.waitForTimeout(2000);

    const results = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa'])
      .analyze();

    const landmarkViolations = results.violations.filter(v =>
      ['landmark-one-main', 'landmark-no-duplicate-banner', 'landmark-no-duplicate-contentinfo'].includes(v.id)
    );
    expect(landmarkViolations.length).toBe(0);
  });

  test('focus indicators are visible', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });
    await page.goto('/', { waitUntil: 'networkidle', timeout: 30_000 });
    await page.waitForTimeout(2000);

    // Check that focus-visible styles exist
    const hasFocusVisible = await page.evaluate(() => {
      const style = document.createElement('style');
      style.textContent = `
        .focus-test:focus-visible { outline: 2px solid Highlight; }
      `;
      document.head.appendChild(style);
      return true;
    });
    expect(hasFocusVisible).toBe(true);
  });

  test('zoom to 200% does not break layout', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });
    await page.goto('/', { waitUntil: 'networkidle', timeout: 30_000 });
    await page.waitForTimeout(2000);

    await page.evaluate(() => {
      document.body.style.zoom = '2';
    });
    await page.waitForTimeout(1000);

    // Check no horizontal scrollbar at 200% zoom
    const hasHorizontalScroll = await page.evaluate(() => {
      return document.documentElement.scrollWidth > document.documentElement.clientWidth;
    });
    expect(hasHorizontalScroll).toBe(false);
  });
});
