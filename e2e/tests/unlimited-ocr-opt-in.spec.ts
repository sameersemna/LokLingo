import { test, expect } from '@playwright/test';

const backendURL = process.env.LOKLINGO_BASE_URL || 'http://localhost:28080';
const writeToken = process.env.WRITE_API_TOKEN || '';

test.describe('Unlimited OCR opt-in', () => {
  test.skip(process.env.LOKLINGO_E2E_UOCR !== '1', 'requires the ocr-vlm profile and OCR_UNLIMITED_ENABLED=true');

  test('PDF job accepts ocr_engine=unlimited and reports the engine', async ({ request, browser }) => {
    test.setTimeout(240_000);
    const fixturePage = await browser.newPage();
    let pdf: Buffer;
    try {
      await fixturePage.setContent('<html lang="en"><body><p>Hello unlimited OCR.</p></body></html>');
      pdf = await fixturePage.pdf();
    } finally {
      await fixturePage.close();
    }

    const headers: Record<string, string> = {};
    if (writeToken) headers['X-API-Token'] = writeToken;
    const submitted = await request.post(`${backendURL}/api/v1/jobs/pdf`, {
      headers,
      multipart: {
        file: { name: 'e2e-unlimited-ocr.pdf', mimeType: 'application/pdf', buffer: pdf },
        source: 'en',
        target: 'de',
        ocr_engine: 'unlimited',
        force_ocr: 'true',
      },
    });
    expect(submitted.status(), await submitted.text()).toBe(202);
    const { job_id: jobId } = await submitted.json();
    expect(jobId).toBeTruthy();

    let job: { status?: string; error?: string; ocr_engine?: string; translated_text?: string } = {};
    await expect.poll(async () => {
      const status = await request.get(`${backendURL}/api/v1/jobs/${jobId}`);
      expect(status.status()).toBe(200);
      job = await status.json();
      return job.status;
    }, { timeout: 210_000, intervals: [2_000, 4_000] }).toMatch(/^(completed|failed)$/);

    expect(job.status, job.error).toBe('completed');
    expect(job.ocr_engine).toBe('unlimited');
    expect(job.translated_text?.trim()).toBeTruthy();
  });
});
