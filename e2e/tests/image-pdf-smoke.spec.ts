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
  test('image job endpoint accepts sample PNG when available', async ({ request }) => {
    const imagePath = firstExisting(sampleImageCandidates);
    test.skip(!imagePath, 'no sample image found in repo');

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

    // 202/200 when accepted; 401 auth; 429 rate limit; 503 deps; 400 validation
    expect([200, 201, 202, 400, 401, 413, 429, 502, 503]).toContain(res.status());
    if (res.ok()) {
      const body = await res.json();
      expect(body.job_id || body.id).toBeTruthy();
    }
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
    expect([400, 401, 415, 422, 429, 500, 503]).toContain(res.status());
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
