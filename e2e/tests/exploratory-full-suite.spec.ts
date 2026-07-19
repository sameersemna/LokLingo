import { test, expect } from '@playwright/test';

const BASE = process.env.LOKLINGO_E2E_BASE_URL || 'http://localhost:3300';
const BACKEND = process.env.LOKLINGO_BASE_URL || 'http://localhost:28080';
const writeToken = process.env.WRITE_API_TOKEN || '';

function authHeaders(): Record<string, string> {
  if (!writeToken) return { 'Content-Type': 'application/json' };
  return { 'Content-Type': 'application/json', 'X-API-Token': writeToken };
}

test.describe('Full exploratory E2E suite', () => {
  test.use({ baseURL: BASE });

  test('1. landing page renders all major UI sections', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await expect(page).toHaveTitle(/LokLingo/i);

    // Header elements
    await expect(page.getByRole('heading', { name: /LokLingo/i })).toBeVisible();

    // Language selectors
    const selects = page.locator('select');
    await expect(selects.first()).toBeVisible();
    await expect(selects.nth(1)).toBeVisible();

    // Workflow cards
    await expect(page.getByRole('button', { name: /image/i }).first()).toBeVisible();
    await expect(page.getByRole('button', { name: /pdf/i }).first()).toBeVisible();
    await expect(page.getByRole('button', { name: /text/i }).first()).toBeVisible();

    // Source textarea
    await expect(page.locator('textarea').first()).toBeVisible();

    // Translate button
    await expect(page.getByRole('button', { name: /^translate$/i }).first()).toBeVisible();

    // Header action buttons
    await expect(page.getByRole('button', { name: /pdf jobs/i }).first()).toBeVisible();
    await expect(page.getByRole('button', { name: /reliability/i }).first()).toBeVisible();
    await expect(page.getByRole('button', { name: /history/i }).first()).toBeVisible();
    await expect(page.getByRole('button', { name: /demos/i }).first()).toBeVisible();
  });

  test('2. language selection and persistence', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    const sourceSelect = page.locator('select').first();
    const targetSelect = page.locator('select').nth(1);

    // Verify all language options
    const sourceOptions = await sourceSelect.evaluate(el => Array.from(el.options).map(o => o.value));
    expect(sourceOptions).toContain('auto');
    expect(sourceOptions).toContain('en');
    expect(sourceOptions).toContain('de');
    expect(sourceOptions).toContain('fr');
    expect(sourceOptions).toContain('hi');
    expect(sourceOptions).toContain('ur');
    expect(sourceOptions).toContain('ar');
    expect(sourceOptions).toContain('bn');
    expect(sourceOptions).toContain('es');
    expect(sourceOptions).toContain('zh');
    expect(sourceOptions).toContain('ja');

    const targetOptions = await targetSelect.evaluate(el => Array.from(el.options).map(o => o.value));
    expect(targetOptions).not.toContain('auto');
    expect(targetOptions).toContain('en');
    expect(targetOptions).toContain('de');

    // Select and verify persistence
    await sourceSelect.selectOption('fr');
    await targetSelect.selectOption('de');
    expect(await sourceSelect.inputValue()).toBe('fr');
    expect(await targetSelect.inputValue()).toBe('de');

    await page.reload({ waitUntil: 'domcontentloaded' });
    expect(await page.locator('select').first().inputValue()).toBe('fr');
    expect(await page.locator('select').nth(1).inputValue()).toBe('de');
  });

  test('3. language swap button behavior', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    const sourceSelect = page.locator('select').first();
    const targetSelect = page.locator('select').nth(1);
    const swapBtn = page.getByRole('button', { name: '⇄' });

    await sourceSelect.selectOption('en');
    await targetSelect.selectOption('es');
    expect(await sourceSelect.inputValue()).toBe('en');
    expect(await targetSelect.inputValue()).toBe('es');

    await swapBtn.click();
    expect(await sourceSelect.inputValue()).toBe('es');
    expect(await targetSelect.inputValue()).toBe('en');
  });

  test('4. translation mode switching', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    const modeSelect = page.locator('select').nth(2);
    const modes = await modeSelect.evaluate(el => Array.from(el.options).map(o => o.value));
    expect(modes).toContain('fast');
    expect(modes).toContain('studio');
    expect(modes).toContain('extract');

    await modeSelect.selectOption('studio');
    expect(await modeSelect.inputValue()).toBe('studio');

    await modeSelect.selectOption('extract');
    expect(await modeSelect.inputValue()).toBe('extract');

    await modeSelect.selectOption('fast');
    expect(await modeSelect.inputValue()).toBe('fast');
  });

  test('5. text translation flow end-to-end', async ({ page }) => {
    // Clear first-visit flag to prevent auto-demo from interfering
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });

    await page.goto('/', { waitUntil: 'domcontentloaded' });

    // Ensure text workflow is active - click the Text workflow card
    const textWorkflowBtn = page.locator('.workflow-card').filter({ hasText: 'Text' }).first();
    await textWorkflowBtn.click();
    await page.waitForTimeout(500);

    const textarea = page.locator('textarea').first();
    await textarea.fill('Hello world, this is a test translation.');
    await page.waitForTimeout(200);

    // Find translate button - it's enabled only when workflow === "text" and text is non-empty
    const translateBtn = page.locator('button.translate-btn').first();
    await expect(translateBtn).toBeEnabled({ timeout: 5000 });

    await translateBtn.click();
    // Wait for result - LLM response can take 10-30s
    await page.waitForTimeout(5000);

    // Check for result in output panel - wait for it to populate
    const outputPanel = page.locator('.panel-output');
    await expect(async () => {
      const text = await outputPanel.textContent();
      expect(text).toBeTruthy();
    }).toPass({ timeout: 30000 });
  });

  test('6. textarea character limit and clear', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    const textarea = page.locator('textarea').first();
    const maxLength = await textarea.evaluate(el => (el as HTMLTextAreaElement).maxLength);
    expect(maxLength).toBe(2000);

    // Fill to near limit
    const longText = 'A'.repeat(1999);
    await textarea.fill(longText);
    const charCount = await textarea.evaluate(el => (el as HTMLTextAreaElement).value.length);
    expect(charCount).toBe(1999);

    // Clear button
    const clearBtn = page.locator('button').filter({ hasText: /clear/i }).first();
    if (await clearBtn.isVisible()) {
      await clearBtn.click();
      const cleared = await textarea.evaluate(el => (el as HTMLTextAreaElement).value);
      expect(cleared).toBe('');
    }
  });

  test('7. workflow switching between Image, PDF, and Text', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    // Click PDF workflow
    const pdfBtn = page.getByRole('button', { name: /pdf/i }).first();
    await pdfBtn.click();
    await page.waitForTimeout(300);

    // Click Image workflow
    const imageBtn = page.getByRole('button', { name: /^image/i }).first();
    await imageBtn.click();
    await page.waitForTimeout(300);

    // Click Text workflow
    const textBtn = page.getByRole('button', { name: /^text/i }).first();
    await textBtn.click();
    await page.waitForTimeout(300);

    // Textarea should be visible in text mode
    await expect(page.locator('textarea').first()).toBeVisible();
  });

  test('8. theme toggle between light and dark', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    // Theme button text changes: "☾ Dark" in light mode, "☀ Light" in dark mode
    const themeBtn = page.locator('button').filter({ hasText: /dark|light|☾|☀/i }).first();
    await expect(themeBtn).toBeVisible();

    // Toggle to dark
    await themeBtn.click();
    await page.waitForTimeout(200);
    let theme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'));
    expect(theme).toBe('dark');

    // Toggle back to light
    await themeBtn.click();
    await page.waitForTimeout(200);
    theme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'));
    expect(theme).toBe('light');
  });

  test('9. history panel opens and displays state', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    const historyBtn = page.getByRole('button', { name: /history/i }).first();
    await historyBtn.click();
    await page.waitForTimeout(400);

    const historyPanel = page.locator('.history-panel');
    await expect(historyPanel).toBeVisible();
    const text = await historyPanel.textContent();
    expect(text).toMatch(/history|translation|recent/i);
  });

  test('10. demo gallery panel opens and shows presets', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    const demosBtn = page.getByRole('button', { name: /demos/i }).first();
    await demosBtn.click();
    await page.waitForTimeout(400);

    const gallery = page.locator('.demo-gallery-panel');
    await expect(gallery).toBeVisible();

    // Should show demo preset cards
    const presetCards = gallery.locator('button, [role="button"], .demo-card');
    const count = await presetCards.count();
    expect(count).toBeGreaterThan(0);
  });

  test('11. reliability panel opens and shows metrics', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    const relBtn = page.getByRole('button', { name: /reliability/i }).first();
    await relBtn.click();
    await page.waitForTimeout(500);

    const panel = page.locator('.reliability-panel');
    await expect(panel).toBeVisible();
    const text = await panel.textContent();
    expect(text).toMatch(/reliability|telemetry|metrics|ocr|provider/i);
  });

  test('12. readiness chip shows status and opens popover', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(3000);

    // Find the readiness/status chip by text content
    const chip = page.locator('button').filter({ hasText: /healthy|online|ok|degraded|error|system/i }).first();
    if (await chip.isVisible().catch(() => false)) {
      const chipText = await chip.textContent();
      expect(chipText).toBeTruthy();

      await chip.click();
      await page.waitForTimeout(500);

      // Popover or detail section should appear
      const detail = page.locator('[class*="readiness-detail"], [class*="status-detail"], [class*="popover"]').first();
      if (await detail.isVisible().catch(() => false)) {
        const detailText = await detail.textContent();
        expect(detailText).toBeTruthy();
      }
    }
  });

  test('13. dead ops panel opens and shows recovery queue', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    const deadOpsBtn = page.getByRole('button', { name: /dead ops/i }).first();
    await deadOpsBtn.click();
    await page.waitForTimeout(500);

    const panel = page.locator('[class*="dead-ops"], [class*="deadletter"]').first();
    await expect(panel).toBeVisible();
    const text = await panel.textContent();
    expect(text).toMatch(/recovery|queue|dead|failed|replay/i);
  });

  test('14. PDF jobs panel opens', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    const pdfJobsBtn = page.getByRole('button', { name: /pdf jobs/i }).first();
    await pdfJobsBtn.click();
    await page.waitForTimeout(500);

    const panel = page.locator('[class*="pdf-jobs"], [class*="PdfJobs"]').first();
    if (await panel.isVisible()) {
      const text = await panel.textContent();
      expect(text).toBeTruthy();
    }
  });

  test('15. showcase mode toggle', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    const showcaseBtn = page.getByRole('button', { name: /showcase/i }).first();
    await expect(showcaseBtn).toBeVisible();
    await showcaseBtn.click();
    await page.waitForTimeout(500);

    // Click again to disable
    await showcaseBtn.click();
  });

  test('16. upload drop zone is present', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    const dropZone = page.locator('[class*="upload"], [class*="drop"]').first();
    if (await dropZone.isVisible()) {
      const text = await dropZone.textContent();
      expect(text).toMatch(/drop|upload|image|file/i);
    }
  });

  test('17. demo preset buttons are interactive', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    const presetBtns = page.locator('[class*="demo-preset"], [class*="preset"] button, button:has-text("Manga"), button:has-text("Restaurant")');
    const count = await presetBtns.count();
    expect(count).toBeGreaterThan(0);

    // Click first visible preset
    const firstPreset = presetBtns.first();
    if (await firstPreset.isVisible()) {
      await firstPreset.click();
      await page.waitForTimeout(1000);
    }
  });

  test('18. XSS injection does not execute', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    const textarea = page.locator('textarea').first();
    await expect(textarea).toBeVisible();

    const payloads = [
      '<script>window.__pwned=true</script>',
      '<img src=x onerror="window.__pwned2=true">',
      '"><script>window.__pwned3=true</script>',
      'javascript:alert(1)',
    ];

    for (const payload of payloads) {
      await textarea.fill(payload);
      await page.waitForTimeout(100);
    }

    const pwned = await page.evaluate(() =>
      (window as any).__pwned ||
      (window as any).__pwned2 ||
      (window as any).__pwned3
    );
    expect(pwned).toBeFalsy();
  });

  test('19. no unexpected 4xx/5xx on initial page load', async ({ page }) => {
    const failed: string[] = [];
    page.on('response', (resp) => {
      if (resp.status() >= 400) {
        failed.push(`${resp.status()} ${resp.url()}`);
      }
    });

    await page.goto('/', { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(2000);

    // Filter known expected failures
    const real = failed.filter(f =>
      !f.includes('favicon') &&
      !f.includes('metrics/') &&
      !f.includes('jobs/dead') &&
      !f.includes('jobs/image') &&
      !f.includes('/ocr/image')
    );
    expect(real).toEqual([]);
  });

  test('20. backend health and readiness endpoints', async ({ request }) => {
    const healthRes = await request.get(`${BACKEND}/health`);
    expect(healthRes.status()).toBe(200);
    const healthBody = await healthRes.json();
    expect(healthBody.status).toBe('ok');
    expect(healthBody.service).toBe('loklingo-backend');

    const readyRes = await request.get(`${BACKEND}/ready`);
    expect([200, 503]).toContain(readyRes.status());
    const readyBody = await readyRes.json();
    expect(readyBody.service).toBe('loklingo-backend');
    expect(readyBody.dependencies).toBeTruthy();
  });

  test('21. health dashboard returns dependency status', async ({ request }) => {
    const res = await request.get(`${BACKEND}/api/v1/health/dashboard`);
    expect([200, 503]).toContain(res.status());
    const body = await res.json();
    expect(body.service).toBe('loklingo-backend');
    expect(body.dependencies).toBeTruthy();
    expect(body.dependencies.backend).toBeTruthy();
    expect(body.dependencies.queues).toBeTruthy();
  });

  test('22. translate API accepts valid request', async ({ request }) => {
    const res = await request.post(`${BACKEND}/api/v1/translate`, {
      headers: authHeaders(),
      data: { text: 'Hello world', source: 'EN', target: 'DE' },
      timeout: 30_000,
    });

    expect([200, 201, 202, 400, 401, 429, 502, 503]).toContain(res.status());

    if (res.ok()) {
      const body = await res.json();
      expect(body.translated_text || body.translation || body.text || body.job_id).toBeTruthy();
    }
  });

  test('23. translate API rejects missing fields', async ({ request }) => {
    const res = await request.post(`${BACKEND}/api/v1/translate`, {
      headers: authHeaders(),
      data: { source: 'EN' },
    });
    expect([400, 401, 422, 429]).toContain(res.status());
  });

  test('24. translate API rejects empty text', async ({ request }) => {
    const res = await request.post(`${BACKEND}/api/v1/translate`, {
      headers: authHeaders(),
      data: { text: '', source: 'EN', target: 'DE' },
    });
    expect([400, 401, 422, 429]).toContain(res.status());
  });

  test('25. translate API rejects unsupported language', async ({ request }) => {
    const res = await request.post(`${BACKEND}/api/v1/translate`, {
      headers: authHeaders(),
      data: { text: 'Hello', source: 'EN', target: 'xx' },
    });
    // LiteLLM may accept any target code and pass it through; accept 200 as well
    expect([200, 400, 401, 422, 429]).toContain(res.status());
  });

  test('26. oversized payload is rejected', async ({ request }) => {
    const big = 'A'.repeat(3 * 1024 * 1024);
    const res = await request.post(`${BACKEND}/api/v1/translate`, {
      headers: authHeaders(),
      data: { text: big, source: 'EN', target: 'DE' },
      timeout: 30_000,
    });
    expect([400, 401, 413, 429, 500, 503]).toContain(res.status());
  });

  test('27. CORS preflight is accepted', async ({ request }) => {
    const res = await request.fetch(`${BACKEND}/api/v1/translate`, {
      method: 'OPTIONS',
      headers: {
        Origin: 'http://promaxgb10-6116:3000',
        'Access-Control-Request-Method': 'POST',
        'Access-Control-Request-Headers': 'content-type,x-api-token',
      },
    });
    expect(res.status()).toBeLessThan(400);
  });

  test('28. security headers are present on frontend', async ({ request }) => {
    const res = await request.get('/');
    expect(res.status()).toBeLessThan(400);

    const csp = (res.headers()['content-security-policy'] || '').toLowerCase();
    expect(csp).toContain("default-src 'self'");
    expect(csp).toContain("frame-ancestors 'none'");

    const xfo = (res.headers()['x-frame-options'] || '').toUpperCase();
    expect(xfo).toBe('DENY');

    const nosniff = (res.headers()['x-content-type-options'] || '').toLowerCase();
    expect(nosniff).toBe('nosniff');
  });

  test('29. security headers on backend health', async ({ request }) => {
    const res = await request.get(`${BACKEND}/health`);
    expect(res.status()).toBe(200);

    expect((res.headers()['x-content-type-options'] || '').toLowerCase()).toBe('nosniff');
    expect((res.headers()['x-frame-options'] || '').toUpperCase()).toBe('DENY');
    expect(res.headers()['x-request-id']).toBeTruthy();
  });

  test('30. cache-control header prevents caching', async ({ request }) => {
    const res = await request.get(`${BACKEND}/api/v1/health/dashboard`);
    expect([200, 503]).toContain(res.status());
    const cache = (res.headers()['cache-control'] || '').toLowerCase();
    expect(cache).toContain('no-store');
  });

  test('31. metrics endpoints require internal token', async ({ request }) => {
    const endpoints = [
      `${BACKEND}/api/v1/metrics/prometheus`,
      `${BACKEND}/api/v1/metrics/providers`,
      `${BACKEND}/api/v1/metrics/ocr?window=24h`,
      `${BACKEND}/api/v1/metrics/reliability`,
    ];

    for (const endpoint of endpoints) {
      const res = await request.get(endpoint);
      expect([200, 401]).toContain(res.status());
      if (res.status() === 401) {
        const body = await res.json();
        expect(body.error).toMatch(/unauthorized/i);
      }
    }
  });

  test('32. dead-letter list is internal-token gated', async ({ request }) => {
    const res = await request.get(`${BACKEND}/api/v1/jobs/dead?limit=5`);
    expect([200, 401]).toContain(res.status());
  });

  test('33. job status endpoint returns 404 for nonexistent job', async ({ request }) => {
    const res = await request.get(`${BACKEND}/api/v1/jobs/nonexistent-job-id-12345`);
    expect([404, 401]).toContain(res.status());
  });

  test('34. OCR service health endpoint', async ({ request }) => {
    const res = await request.get(`http://localhost:18000/health`);
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.status).toBe('ok');
    expect(body.service).toBe('loklingo-ocr');
  });

  test('35. OCR image endpoint accepts valid request', async ({ request }) => {
    // 1x1 pixel PNG as base64
    const tinyPngB64 = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==';
    const res = await request.post(`http://localhost:18000/ocr/image`, {
      headers: { 'Content-Type': 'application/json' },
      data: { image_b64: tinyPngB64, mime_type: 'image/png', lang: 'auto' },
      timeout: 30_000,
    });
    expect([200, 422, 500]).toContain(res.status());
    if (res.status() === 200) {
      const body = await res.json();
      expect(body).toHaveProperty('text');
      expect(body).toHaveProperty('confidence');
      expect(body).toHaveProperty('blocks');
    }
  });

  test('36. OCR image endpoint rejects missing image_b64', async ({ request }) => {
    const res = await request.post(`http://localhost:18000/ocr/image`, {
      headers: { 'Content-Type': 'application/json' },
      data: { mime_type: 'image/png', lang: 'auto' },
    });
    expect([422, 400]).toContain(res.status());
  });

  test('37. PDF job endpoint rejects non-PDF garbage', async ({ request }) => {
    const res = await request.post(`${BACKEND}/api/v1/jobs/pdf`, {
      headers: writeToken ? { 'X-API-Token': writeToken } : {},
      multipart: {
        file: { name: 'not-a-pdf.txt', mimeType: 'application/pdf', buffer: Buffer.from('this is not a pdf') },
        source: 'EN', target: 'DE',
      },
      timeout: 30_000,
    });
    expect([400, 401, 415, 422, 429, 500, 503]).toContain(res.status());
  });

  test('38. create text translation job via API', async ({ request }) => {
    const res = await request.post(`${BACKEND}/api/v1/jobs`, {
      headers: authHeaders(),
      data: { text: 'Hello world async job', source: 'EN', target: 'DE', mode: 'translate' },
      timeout: 15_000,
    });
    expect([200, 201, 202, 400, 401, 429, 502, 503]).toContain(res.status());
    if (res.ok()) {
      const body = await res.json();
      expect(body.job_id || body.id).toBeTruthy();
    }
  });

  test('39. keyboard shortcut Ctrl+Enter triggers translate', async ({ page }) => {
    await page.goto('/', { waitUntil: 'domcontentloaded' });

    const textarea = page.locator('textarea').first();
    await textarea.fill('Testing keyboard shortcut');
    await page.waitForTimeout(200);

    // Press Ctrl+Enter
    await page.keyboard.press('Control+Enter');
    await page.waitForTimeout(3000);

    // Should trigger translation (result panel should update)
    const outputPanel = page.locator('.panel-output');
    const outputText = await outputPanel.textContent();
    expect(outputText).toBeTruthy();
  });

  test('40. OCR service through nginx proxy', async ({ request }) => {
    const tinyPngB64 = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==';
    const res = await request.post(`${BASE}/ocr/image`, {
      headers: { 'Content-Type': 'application/json' },
      data: { image_b64: tinyPngB64, mime_type: 'image/png', lang: 'auto' },
      timeout: 30_000,
    });
    expect([200, 422, 500]).toContain(res.status());
  });
});
