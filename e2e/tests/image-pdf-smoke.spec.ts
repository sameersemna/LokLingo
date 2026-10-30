import { test, expect } from '@playwright/test';
import path from 'node:path';
import fs from 'node:fs';

const backendURL = process.env.LOKLINGO_BASE_URL || 'http://localhost:28080';
const writeToken = process.env.WRITE_API_TOKEN || '';

function authHeaders(multipart = false): Record<string, string> {
  const h: Record<string, string> = {};
  if (writeToken) h['X-API-Token'] = writeToken;
  if (!multipart) h['Content-Type'] = 'application/json';
  return h;
}

// Prefer committed sample assets when present.
const sampleImageCandidates = [
  path.resolve(__dirname, '../../frontend/public/samples/before_bold_style.png'),
  path.resolve(__dirname, '../../frontend/dist/samples/before_bold_style.png'),
  path.resolve(__dirname, '../../guide/render_samples/before_bold_style.png'),
];

function firstExisting(paths: string[]): string | null {
  for (const p of paths) {
    if (fs.existsSync(p)) return p;
  }
  return null;
}

test.describe('Image and PDF API smoke', () => {
  test('image job completes with translated text and a downloadable image', async ({ request }) => {
    test.setTimeout(180_000);
    const imagePath = firstExisting(sampleImageCandidates);
    expect(imagePath, 'a committed sample image is required').not.toBeNull();

    const res = await request.post(`${backendURL}/api/v1/jobs/image`, {
      headers: authHeaders(true),
      multipart: {
        file: {
          name: path.basename(imagePath!),
          mimeType: 'image/png',
          buffer: fs.readFileSync(imagePath!),
        },
        source: 'EN',
        target: 'DE',
      },
      timeout: 60_000,
    });

    expect(res.status(), await res.text()).toBe(202);
    const { job_id: jobId } = await res.json();
    expect(jobId).toBeTruthy();
    let job: { status?: string; error?: string; translated_text?: string; image_url?: string } = {};
    await expect.poll(async () => {
      const status = await request.get(`${backendURL}/api/v1/jobs/${jobId}`);
      expect(status.status()).toBe(200);
      job = await status.json();
      return job.status;
    }, { timeout: 150_000, intervals: [1_000, 2_000] }).toMatch(/^(completed|failed)$/);
    expect(job.status, job.error).toBe('completed');
    expect(job.translated_text?.trim()).toBeTruthy();
    expect(job.image_url).toBe(`/api/v1/jobs/${jobId}/output`);
    const output = await request.get(`${backendURL}${job.image_url}`);
    expect(output.status()).toBe(200);
    expect(output.headers()['content-type']).toMatch(/^image\//);
    expect((await output.body()).length).toBeGreaterThan(100);
  });

  test('PDF job endpoint rejects non-PDF garbage', async ({ request }) => {
    const res = await request.post(`${backendURL}/api/v1/jobs/pdf`, {
      headers: authHeaders(true),
      multipart: {
        file: {
          name: 'not-a-pdf.txt',
          mimeType: 'application/pdf',
          buffer: Buffer.from('this is not a pdf'),
        },
        source: 'EN',
        target: 'DE',
      },
      timeout: 30_000,
    });
    expect(res.status(), await res.text()).toBe(400);
  });

  test('valid PDF uploads through the browser and displays its completed translation', async ({ browser, page, request }) => {
    test.setTimeout(180_000);
    const fixturePage = await browser.newPage();
    let pdf: Buffer;
    try {
      await fixturePage.setContent('<html lang="en"><body><p>Hello world. This is a document for translation.</p></body></html>');
      pdf = await fixturePage.pdf();
    } finally {
      await fixturePage.close();
    }
    await page.goto('/');
    await page.getByRole('combobox', { name: 'Source language', exact: true }).selectOption('en');
    await page.getByRole('combobox', { name: 'Target language', exact: true }).selectOption('de');
    const submitted = page.waitForResponse(response =>
      response.url().endsWith('/api/v1/jobs/pdf') && response.request().method() === 'POST');
    await page.locator('input[type="file"]').setInputFiles({
      name: 'e2e-translation.pdf',
      mimeType: 'application/pdf',
      buffer: pdf,
    });
    const response = await submitted;
    expect(response.status(), await response.text()).toBe(202);
    const { job_id: jobId } = await response.json();
    expect(jobId).toBeTruthy();
    let job: { status?: string; error?: string; translated_text?: string; processed_pages?: number } = {};
    await expect.poll(async () => {
      const status = await request.get(`${backendURL}/api/v1/jobs/${jobId}`);
      expect(status.status()).toBe(200);
      job = await status.json();
      return job.status;
    }, { timeout: 150_000, intervals: [1_000, 2_000] }).toMatch(/^(completed|failed)$/);
    expect(job.status, job.error).toBe('completed');
    expect(job.processed_pages).toBe(1);
    expect(job.translated_text?.trim()).toBeTruthy();
    const row = page.locator('.pdf-job-row').filter({ hasText: 'e2e-translation.pdf' });
    await expect(row.locator('.pdf-job-badge')).toHaveText('Done', { timeout: 15_000 });
    await row.getByTitle('Show / hide translation', { exact: true }).click();
    await expect(row).toContainText(job.translated_text!);
  });

  test('scanned PDF completes through OCR and persists analytics', async ({ browser, request }) => {
    test.skip(process.env.LOKLINGO_E2E_ANALYTICS !== '1', 'requires opt-in Postgres analytics');
    test.setTimeout(210_000);
    const metricsURL = `${backendURL}/api/v1/metrics/ocr?window=1h`;
    const internalHeaders = process.env.INTERNAL_TOKEN
      ? { 'X-Internal-Token': process.env.INTERNAL_TOKEN }
      : {};
    const before = await request.get(metricsURL, { headers: internalHeaders });
    expect(before.status(), await before.text()).toBe(200);
    const baseline = await before.json();
    expect(baseline.succeeded_total).toBe((baseline.outcomes.succeeded ?? 0) + (baseline.outcomes.cached ?? 0));
    const fixturePage = await browser.newPage({ viewport: { width: 900, height: 260 } });
    let pdf: Buffer;
    try {
      await fixturePage.setContent('<html lang="en"><body style="margin:32px;font-size:32px;color:black;background:white">Hello world. This scanned document verifies OCR analytics.</body></html>');
      const image = await fixturePage.screenshot();
      await fixturePage.setContent(`<html><body><img style="width:700px" src="data:image/png;base64,${image.toString('base64')}"></body></html>`);
      pdf = await fixturePage.pdf();
    } finally {
      await fixturePage.close();
    }
    const ocrStarted = Date.now();
    const extracted = await request.post('/ocr/pdf', {
      multipart: {
        file: { name: 'e2e-ocr-quality.pdf', mimeType: 'application/pdf', buffer: pdf },
        lang: 'en',
        dpi: '200',
      },
      timeout: 150_000,
    });
    expect(extracted.status(), await extracted.text()).toBe(200);
    const source = await extracted.json();
    expect(source.text.replace(/\s+/g, ' ').trim()).toBe('Hello world. This scanned document verifies OCR analytics.');
    console.log(JSON.stringify({ ocr_fixture_request_ms: Date.now() - ocrStarted }));
    const submitted = await request.post(`${backendURL}/api/v1/jobs/pdf`, {
      headers: authHeaders(true),
      multipart: {
        file: { name: 'e2e-ocr-analytics.pdf', mimeType: 'application/pdf', buffer: pdf },
        source: 'en',
        target: 'de',
        lang: 'en',
      },
    });
    expect(submitted.status(), await submitted.text()).toBe(202);
    const { job_id: jobId } = await submitted.json();
    expect(jobId).toBeTruthy();
    console.log(JSON.stringify({ ocr_analytics_job: jobId }));
    let job: { status?: string; error?: string; processing_method?: string; translated_text?: string } = {};
    await expect.poll(async () => {
      const status = await request.get(`${backendURL}/api/v1/jobs/${jobId}`);
      expect(status.status()).toBe(200);
      job = await status.json();
      return job.status;
    }, { timeout: 150_000, intervals: [1_000, 2_000] }).toMatch(/^(completed|failed)$/);
    expect(job.status, job.error).toBe('completed');
    expect(job.processing_method).toBe('ocr');
    expect(job.translated_text?.trim()).toBeTruthy();
    await expect.poll(async () => {
      const response = await request.get(metricsURL, { headers: internalHeaders });
      expect(response.status(), await response.text()).toBe(200);
      const metrics = await response.json();
      const successful = (metrics.outcomes.succeeded ?? 0) + (metrics.outcomes.cached ?? 0);
      const previousSuccessful = (baseline.outcomes.succeeded ?? 0) + (baseline.outcomes.cached ?? 0);
      expect(metrics.succeeded_total).toBe(successful);
      expect(metrics.success_rate_pct).toBeCloseTo(successful * 100 / metrics.events_total, 5);
      return metrics.events_total > baseline.events_total && successful > previousSuccessful;
    }, { timeout: 15_000, intervals: [500, 1_000] }).toBe(true);
  });

  test('frontend image/PDF UI controls are present when SPA loads', async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');
    // Soft UI presence checks — labels evolve but tabs/buttons usually mention these.
    const markers = [
      page.getByText(/image|photo|upload|ocr/i).first(),
      page.getByText(/pdf|document/i).first(),
      page.getByText(/translate/i).first(),
    ];
    let visible = 0;
    for (const m of markers) {
      if (await m.isVisible().catch(() => false)) visible += 1;
    }
    expect(visible).toBeGreaterThan(0);
  });
});
