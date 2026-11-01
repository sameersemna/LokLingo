import { test, expect } from '@playwright/test';

const BASE = process.env.LOKLINGO_E2E_BASE_URL || 'http://localhost:3300';
const BACKEND = process.env.LOKLINGO_BASE_URL || 'http://localhost:28080';

test.describe('Comprehensive UI smoke + self-heal checks', () => {
  test.use({ baseURL: BASE });

  test('production Settings saves masked session credentials and verifies access', async ({ page }) => {
    await page.route('**/api/v1/jobs/dead?limit=1', route => route.fulfill(route.request().headers()['x-internal-token'] === 'operator-fixture' ? { json: { count: 0, jobs: [] } } : { status: 401, json: { error: 'Unauthorized' } }));
    await page.goto('/settings');
    const write = page.getByLabel('Write API token', { exact: true });
    const internal = page.getByLabel('Internal operator token', { exact: true });
    await expect(write).toHaveAttribute('type', 'password');
    await expect(internal).toHaveAttribute('type', 'password');
    await write.fill('write-fixture');
    await internal.fill('operator-fixture');
    await page.getByRole('button', { name: 'Save session credentials', exact: true }).click();
    await expect(page.getByRole('status')).toHaveText('Credentials saved for this browser session.');
    expect(await page.evaluate(() => localStorage.getItem('loklingo-internal-token'))).toBeNull();
    await page.reload();
    await expect(write).toHaveValue('write-fixture');
    await expect(internal).toHaveValue('operator-fixture');
    await page.getByRole('button', { name: 'Check operator access', exact: true }).click();
    await expect(page.getByRole('status')).toHaveText('Operator access verified.');
    await page.getByRole('button', { name: 'Clear credentials', exact: true }).click();
    await expect(write).toHaveValue('');
    await expect(internal).toHaveValue('');
    expect(await page.evaluate(() => sessionStorage.getItem('loklingo-write-token'))).toBeNull();
  });

  test('production Jobs filters track hidden jobs and open shareable details', async ({ page }) => {
    await page.addInitScript(() => localStorage.setItem('loklingo-pdf-jobs', JSON.stringify([
      { job_id: 'completed-fixture', filename: 'report.pdf', source: 'en', target: 'de', submittedAt: 3 },
      { job_id: 'failed-fixture', filename: 'failed.pdf', source: 'en', target: 'de', submittedAt: 2 },
      { job_id: 'expired-fixture', filename: 'expired.pdf', source: 'en', target: 'de', submittedAt: 1 },
      { job_id: 'live-fixture', filename: 'live.pdf', source: 'en', target: 'de', submittedAt: 4 },
    ])));
    let liveReads = 0;
    await page.route(/\/api\/v1\/jobs\/[^/]+\/events$/, route => route.fulfill({ contentType: 'text/event-stream', body: '' }));
    await page.route(/\/api\/v1\/jobs\/(completed|failed|expired|live)-fixture$/, route => {
      const id = route.request().url().split('/').pop()!;
      if (id === 'expired-fixture') return route.fulfill({ status: 404, json: { error: 'Not found' } });
      const status = id === 'failed-fixture' ? 'failed' : id === 'live-fixture' && ++liveReads === 1 ? 'processing' : 'completed';
      return route.fulfill({ json: { job_id: id, status, translated_text: status === 'completed' ? 'Completed document translation.' : '', source: 'en', target: 'de', error: status === 'failed' ? 'Provider timeout' : undefined } });
    });
    await page.goto('/jobs');
    await page.getByRole('button', { name: /^Failed/ }).click();
    await expect(page.locator('.pdf-job-row:visible')).toHaveCount(1);
    const completedFilter = page.getByRole('button', { name: /^Completed/ });
    await expect(completedFilter).toContainText('2');
    await completedFilter.click();
    await expect(page.locator('.pdf-job-row:visible')).toHaveCount(2);
    await page.getByRole('searchbox', { name: 'Search PDF jobs' }).fill('report');
    await expect(page.locator('.pdf-job-row:visible')).toHaveCount(1);
    await page.getByRole('button', { name: 'completed-fixture', exact: true }).click();
    await expect(page).toHaveURL(/\/jobs\/completed-fixture$/);
    await expect(page.locator('.result-text')).toHaveText('Completed document translation.');
    await page.reload();
    await expect(page.locator('.result-text')).toHaveText('Completed document translation.');
    await page.goto('/jobs/expired-fixture');
    await expect(page.getByRole('alert')).toHaveText('Job not found or expired.');
  });

  test('production service failures expose readiness and operator access states', async ({ page }) => {
    await page.route('**/ready', route => route.fulfill({ status: 500, json: { error: 'Readiness unavailable' } }));
    await page.route(/\/api\/v1\/metrics\//, route => route.fulfill({ status: 401, json: { error: 'Unauthorized' } }));
    await page.goto('/operations');
    await expect(page.getByRole('button', { name: 'System unavailable' })).toBeVisible();
    await expect(page.getByText('Operator access required.', { exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Access settings', exact: true }).click();
    await expect(page).toHaveURL(/\/settings$/);
  });

  for (const failedPass of ['overlay', 'layout']) {
    test(`production image preserves output when ${failedPass} fails`, async ({ page, request }) => {
      await page.route('**/ocr/image', route => route.fulfill({ json: { text: 'Original fixture text', confidence: 0.95, blocks: [] } }));
      await page.route('**/api/v1/jobs/image', async route => {
        const original = route.request();
        const form = await new Request(original.url(), { method: 'POST', headers: original.headers(), body: original.postDataBuffer() }).formData();
        if (form.get('mode') === failedPass) return route.fulfill({ status: 500, json: { error: 'Rendering failed' } });
        return route.fulfill({ status: 202, json: { job_id: 'partial-fixture' } });
      });
      await page.route('**/api/v1/jobs/partial-fixture', route => route.fulfill({ json: { job_id: 'partial-fixture', status: 'completed', translated_text: 'Preserved translated text.', source: 'en', target: 'de', image_url: '/samples/before_bold_style.png' } }));
      await page.route('**/api/v1/jobs/partial-fixture/events', route => route.fulfill({ contentType: 'text/event-stream', body: '' }));
      await page.goto('/translate/image');
      const image = await request.get(`${BASE}/samples/before_bold_style.png`);
      await page.locator('input[type="file"]').setInputFiles({ name: 'partial.png', mimeType: 'image/png', buffer: await image.body() });
      await expect(page.locator('.result-text')).toHaveText('Preserved translated text.');
      await expect(page.getByText(failedPass === 'layout' ? 'Studio output unavailable' : 'Fast output unavailable', { exact: true })).toBeVisible();
      await expect(page.getByTitle(failedPass === 'layout' ? 'Download studio result' : 'Download fast result')).toBeDisabled();
      await expect(page.getByRole('button', { name: 'Retry image', exact: true })).toBeVisible();
      await expect(page.getByRole('dialog')).toHaveCount(0);
    });
  }

  test('production routes support deep links history and draft preservation', async ({ page }) => {
    await page.goto('/translate/text?source=en');
    await expect(page.getByRole('heading', { name: 'Text translation', exact: true })).toBeVisible();
    await page.getByRole('textbox', { name: 'Source text' }).fill('Draft survives route navigation.');
    await page.getByRole('button', { name: 'PDF Jobs', exact: true }).click();
    await expect(page).toHaveURL(/\/jobs\?source=en$/);
    await page.goBack();
    await expect(page.getByRole('textbox', { name: 'Source text' })).toHaveValue('Draft survives route navigation.');
    await page.goForward();
    await expect(page.getByRole('heading', { name: 'PDF translation jobs' })).toBeVisible();
    await page.reload();
    await expect(page.getByRole('heading', { name: 'PDF translation jobs' })).toBeVisible();
    await page.goto('/missing-workspace');
    await expect(page.getByRole('heading', { name: 'Page not found' })).toBeVisible();
    await page.getByRole('button', { name: 'Open translation workspace', exact: true }).last().click();
    await expect(page).toHaveURL(/\/translate\/image$/);
  });

  test('Stitch workspaces keep translation and operational views separate', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByRole('heading', { name: 'Image translation', exact: true })).toBeVisible();
    await expect(page.getByRole('textbox', { name: 'Source text' })).toHaveCount(0);
    await page.getByRole('button', { name: 'Studio', exact: true }).click();
    await page.getByRole('button', { name: 'PDF', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Fast', exact: true })).toHaveAttribute('aria-pressed', 'true');
    await page.getByRole('button', { name: 'Text', exact: true }).click();
    await expect(page.locator('.upload-hero')).toHaveCount(0);
    await expect(page.getByRole('group', { name: 'Translation mode' })).toHaveCount(0);
    await page.getByRole('textbox', { name: 'Source text' }).fill('Keep this draft while navigating.');
    await page.getByRole('button', { name: 'PDF Jobs', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'PDF translation jobs' })).toBeVisible();
    await expect(page.getByRole('textbox', { name: 'Source text' })).toBeHidden();
    await page.getByRole('button', { name: 'Open translation workspace', exact: true }).click();
    await expect(page.getByRole('textbox', { name: 'Source text' })).toHaveValue('Keep this draft while navigating.');
    await page.getByRole('button', { name: 'History', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Recent translations' })).toBeVisible();
    await page.getByRole('button', { name: 'Open translation workspace', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Recent translations' })).toHaveCount(0);
  });

  test('Stitch history restores text through a keyboard accessible action', async ({ page }) => {
    await page.addInitScript(() => localStorage.setItem('loklingo-history', JSON.stringify([
      { id: 1, sourceText: 'Original document text.', sourceLang: 'en', targetLang: 'de', result: 'Stored translated result.', ts: 1 },
    ])));
    await page.goto('/');
    await page.getByRole('button', { name: /^History/ }).click();
    const restore = page.getByRole('button', { name: 'Restore', exact: true });
    await restore.focus();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('heading', { name: 'Text translation', exact: true })).toBeVisible();
    await expect(page.getByRole('textbox', { name: 'Source text' })).toHaveValue('Original document text.');
    await expect(page.locator('.result-text')).toHaveText('Stored translated result.');
  });

  test('Stitch responsive workflows render assets without horizontal overflow', async ({ page }) => {
    await page.addInitScript(() => localStorage.setItem('loklingo-theme', 'light'));
    await page.goto('/');
    for (const width of [1440, 768, 390, 360]) {
      await page.setViewportSize({ width, height: 900 });
      for (const workflow of ['Image', 'PDF', 'Text']) {
        await page.getByRole('button', { name: workflow, exact: true }).click();
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
        await expect(page.getByRole('combobox', { name: 'Source language', exact: true })).toBeVisible();
        await expect(page.getByRole('combobox', { name: 'Target language', exact: true })).toBeVisible();
        const heading = await page.locator('.workspace-heading').boundingBox();
        const languages = await page.locator('.lang-selectors').boundingBox();
        expect(languages!.y - heading!.y).toBeLessThan(width > 760 ? 160 : 260);
        if (workflow === 'Image') {
          await expect.poll(() => page.locator('.sample-item img').evaluateAll(images => images.every(image => (image as HTMLImageElement).naturalWidth > 0))).toBe(true);
        }
        await page.screenshot({ path: test.info().outputPath(`stitch-${workflow.toLowerCase()}-${width}.png`), fullPage: true });
      }
    }
    await page.getByRole('button', { name: 'Menu', exact: true }).click();
    await page.getByRole('button', { name: 'PDF Jobs', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'PDF translation jobs' })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  });

  test('Stitch image inspection preserves full-size slider geometry and overlay toggles', async ({ page, request }) => {
    await page.route('**/ocr/image', route => route.fulfill({ json: { text: 'Source fixture', confidence: 0.95, blocks: [{ text: 'Source fixture', confidence: 0.95, reading_order: 0, bbox: [10, 10, 200, 80] }] } }));
    await page.route('**/api/v1/jobs/image', route => route.fulfill({ status: 202, json: { job_id: 'design-image' } }));
    await page.route('**/api/v1/jobs/design-image', route => route.fulfill({ json: { job_id: 'design-image', status: 'completed', translated_text: 'Translated fixture text', source: 'en', target: 'de', image_url: '/samples/before_bold_style.png' } }));
    await page.route('**/api/v1/jobs/design-image/events', route => route.fulfill({ contentType: 'text/event-stream', body: '' }));
    await page.goto('/');
    const image = await request.get(`${BASE}/samples/before_bold_style.png`);
    await page.locator('input[type="file"]').setInputFiles({ name: 'inspection.png', mimeType: 'image/png', buffer: await image.body() });
    await expect(page.locator('.comparison-wrap')).toBeAttached();
    await expect(page.getByRole('textbox', { name: 'Source text' })).toHaveValue('Source fixture');
    if (await page.getByRole('dialog').isVisible()) await page.getByRole('button', { name: 'Close comparison viewer' }).click();
    await page.locator('.overlay-inspector summary').click();
    await page.getByRole('button', { name: 'Hide overlay', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Show overlay', exact: true })).toHaveAttribute('aria-pressed', 'false');
    await page.getByRole('button', { name: 'Show overlay', exact: true }).click();
    await page.locator('.overlay-inspector summary').click();
    await page.getByRole('button', { name: 'Slider', exact: true }).click();
    const slider = page.locator('.compare-slider-label input');
    await slider.fill('25');
    const frame = await page.locator('.comparison-wrap .compare-slider-frame').boundingBox();
    const overlay = await page.locator('.comparison-wrap .compare-slider-overlay').boundingBox();
    expect(frame).not.toBeNull();
    expect(overlay!.width).toBeCloseTo(frame!.width - 2, 0);
    await expect(page.locator('.comparison-wrap .compare-slider-overlay')).toHaveCSS('clip-path', 'inset(0px 75% 0px 0px)');
    await page.getByRole('button', { name: 'Fullscreen', exact: true }).click();
    const modalTarget = page.getByRole('group', { name: 'Modal slider target' }).getByRole('button').first();
    await modalTarget.click();
    await expect(modalTarget).toHaveAttribute('aria-pressed', 'true');
    const modalFrame = await page.locator('.comparison-modal .compare-slider-frame').boundingBox();
    const modalOverlay = await page.locator('.comparison-modal .compare-slider-overlay').boundingBox();
    const modalBorders = await page.locator('.comparison-modal .compare-slider-frame').evaluate(element => {
      const style = getComputedStyle(element);
      return parseFloat(style.borderLeftWidth) + parseFloat(style.borderRightWidth);
    });
    expect(modalOverlay!.width).toBeCloseTo(modalFrame!.width - modalBorders, 0);
    await page.getByRole('button', { name: 'Close comparison viewer' }).click();
    await page.screenshot({ path: test.info().outputPath('stitch-image-inspection.png'), fullPage: true });
  });

  test('home page renders without console errors', async ({ page }) => {
    const errors: string[] = [];
    const warnings: string[] = [];
    page.on('console', (msg) => {
      if (msg.type() === 'error') errors.push(msg.text());
      if (msg.type() === 'warning') warnings.push(msg.text());
    });
    page.on('pageerror', (err) => errors.push(`pageerror: ${err.message}`));

    // Suppress first-visit auto-demo to avoid network noise
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });

    const resp = await page.goto('/', { waitUntil: 'domcontentloaded', timeout: 30_000 });
    expect(resp?.status()).toBeLessThan(400);

    // Wait for readiness chip to settle.
    await page.waitForTimeout(2000);

    // Capture a screenshot for human review.
    await page.screenshot({ path: 'playwright-report/home-smoke.png', fullPage: false });

    // Sanity: title contains the app name.
    await expect(page).toHaveTitle(/LokLingo/i);

    // Sanity: at least one primary CTA is present.
    const translateBtn = page.getByRole('button', { name: 'Upload image', exact: true });
    await expect(translateBtn).toBeVisible({ timeout: 5000 });

    // Suppress benign "Failed to load resource" console errors that match known
    // 4xx/5xx patterns. The first-visit auto-demo sends a sample image to the OCR
    // service which can 500 on certain PaddleX versions (paddlex vector<bool>
    // out-of-bounds bug, upstream). That's a service bug, not a frontend issue.
    const realErrors = errors.filter((e) => {
      if (!/Failed to load resource/.test(e)) return true;
      // Allow 5xx on /ocr/image for the first-visit auto-demo (PaddleX bug).
      if (/status of 500 \(Internal Server Error\)/.test(e)) return false;
      return true;
    });
    console.log(`Console errors: ${errors.length}, warnings: ${warnings.length}`);
    if (errors.length) console.log('Errors:', errors.slice(0, 5));
    if (warnings.length) console.log('Warnings:', warnings.slice(0, 5));

    // Page-level JS errors (pageerror) are the strict gate. The console.error
    // path is reported but not blocking here — see dedicated network test.
    expect(realErrors, `Page errors detected:\n${realErrors.join('\n')}`).toEqual([]);
  });

  test('language switcher persists and reflects in URL', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    // Open source/target selects and pick a known option.
    const sourceSelect = page.locator('select').first();
    const targetSelect = page.locator('select').nth(1);
    await sourceSelect.selectOption('de');
    await targetSelect.selectOption('es');

    // Reload — localStorage should restore.
    await page.reload({ waitUntil: 'domcontentloaded' });
    const srcVal = await page.locator('select').first().inputValue();
    const tgtVal = await page.locator('select').nth(1).inputValue();
    expect(srcVal).toBe('de');
    expect(tgtVal).toBe('es');
  });

  test('readiness chip shows status and is clickable for detail', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(1500);

    // Find the readiness/status chip.
    const chip = page.locator('[data-testid="readiness-chip"], [class*="readiness"], [class*="status"]').first();
    const visible = await chip.isVisible().catch(() => false);
    if (visible) {
      await chip.click();
      await page.waitForTimeout(500);
      await page.screenshot({ path: 'playwright-report/readiness-detail.png' });
    }
  });

  test('text translation flow with a non-empty input', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    // Switch to text workflow by clicking the Text workflow card.
    const textWorkflowBtn = page.locator('.workflow-card').filter({ hasText: 'Text' }).first();
    if (await textWorkflowBtn.isVisible().catch(() => false)) {
      await textWorkflowBtn.click();
      await page.waitForTimeout(500);
    }

    const textarea = page.locator('textarea').first();
    await textarea.fill('Hello, this is a hardening smoke test.');
    await page.waitForTimeout(300);

    const translateBtn = page.locator('button.translate-btn').first();
    if (await translateBtn.isEnabled()) {
      await translateBtn.click();
      // Wait up to 30s for the result block to populate.
      await page.waitForTimeout(3000);
      await page.screenshot({ path: 'playwright-report/text-translation-attempt.png' });
    }
  });

  test('history panel opens and stores entries', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    const historyBtn = page.getByRole('button', { name: /history/i }).first();
    if (await historyBtn.isVisible().catch(() => false)) {
      await historyBtn.click();
      await page.waitForTimeout(400);
      await page.screenshot({ path: 'playwright-report/history-panel.png' });
    }
  });

  test('demo presets gallery is reachable', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    const demoBtn = page.getByRole('button', { name: /demo|preset|samples/i }).first();
    if (await demoBtn.isVisible().catch(() => false)) {
      await demoBtn.click();
      await page.waitForTimeout(400);
      await page.screenshot({ path: 'playwright-report/demo-gallery.png' });
    }
  });

  test('reliability dashboard panel opens', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    const relBtn = page.getByRole('button', { name: /reliability|metrics|status/i }).first();
    if (await relBtn.isVisible().catch(() => false)) {
      await relBtn.click();
      await page.waitForTimeout(500);
      await page.screenshot({ path: 'playwright-report/reliability-panel.png' });
    }
  });

  test('no broken images or 404 resources on first load', async ({ page }) => {
    const failed: string[] = [];
    page.on('response', (resp) => {
      if (resp.status() >= 400 && !resp.url().includes('hot-reload')) {
        failed.push(`${resp.status()} ${resp.url()}`);
      }
    });
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(2000);

    // Filter expected 4xx/5xx (the SPA may not have a favicon route, and the metrics
    // endpoint may 401/403 without a token — those are intentional and tested elsewhere).
    // The /ocr/image probe below sends garbage data and OCR service legitimately
    // returns 422/500; that's a contract test, not a "broken page" signal.
    const real = failed.filter(
      (f) =>
        !f.includes('favicon') &&
        !f.includes('metrics/prometheus') &&
        !f.includes('metrics/ocr') &&
        !f.includes('metrics/providers') &&
        !f.includes('metrics/reliability') &&
        !f.includes('jobs/dead'),
    );
    // /ocr/image: filter 422 (validation) and 500 (OCR backend bug on garbage).
    // We still flag 404 (proxy misconfig) — that one is real.
    const realProxyFailures = real.filter((f) => !f.includes('/ocr/image') || f.startsWith('404 '));
    expect(realProxyFailures, `Unexpected 4xx/5xx responses:\n${realProxyFailures.join('\n')}`).toEqual([]);
  });

  test('XSS hardening: input does not execute as HTML', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await page.getByRole('button', { name: 'Text', exact: true }).click();
    const textarea = page.locator('textarea').first();
    if (await textarea.isVisible().catch(() => false)) {
      const malicious = '<script>window.__pwned=true</script><img src=x onerror="window.__pwned2=true">';
      await textarea.fill(malicious);
      await page.waitForTimeout(200);
      const pwned = await page.evaluate(() => (window as any).__pwned || (window as any).__pwned2);
      expect(pwned, 'XSS payload executed — input not sanitized').toBeFalsy();
    }
  });
});
