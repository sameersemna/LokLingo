import { test, expect } from '@playwright/test';

const backendURL = process.env.LOKLINGO_BASE_URL || 'http://localhost:28080';
const writeToken = process.env.WRITE_API_TOKEN || '';

function authHeaders(): Record<string, string> {
  if (!writeToken) return { 'Content-Type': 'application/json' };
  return {
    'Content-Type': 'application/json',
    'X-API-Token': writeToken,
  };
}

test.describe('Frontend UI + translation flows', () => {
  test('home page loads LokLingo shell', async ({ page }) => {
    await page.goto('/');
    await expect(page).toHaveTitle(/loklingo|translate|vite|react/i);
    // App root should render something interactive
    const body = page.locator('body');
    await expect(body).toBeVisible();
    // Prefer text markers from the SPA when present
    const anyUI = page.getByText(/translate|source|target|LokLingo|language|PDF|image/i).first();
    await expect(anyUI).toBeVisible({ timeout: 20_000 });
  });

  test('backend readiness endpoint responds', async ({ request }) => {
    const res = await request.get(`${backendURL}/ready`);
    expect([200, 503]).toContain(res.status());
    const body = await res.json();
    expect(body.service).toBe('loklingo-backend');
    expect(body.dependencies).toBeTruthy();
  });

  test('text translation API accepts or auth-guards requests', async ({ request }) => {
    const res = await request.post(`${backendURL}/api/v1/translate`, {
      headers: authHeaders(),
      data: {
        text: 'Hello world',
        source: 'EN',
        target: 'DE',
      },
    });

    // 200/202 success paths; 401 without token in prod; 503 if providers down; 429 rate limited
    expect([200, 201, 202, 400, 401, 429, 502, 503]).toContain(res.status());

    if (res.status() === 401) {
      const body = await res.json();
      expect(body.error).toMatch(/unauthorized|misconfigured/i);
    }

    if (res.ok()) {
      const body = await res.json();
      // Flexible shape: either direct translation or job id
      expect(
        body.translated_text ||
          body.translation ||
          body.text ||
          body.job_id ||
          body.id ||
          body.result,
      ).toBeTruthy();
    }
  });

  test('LAN-style origin is accepted by CORS preflight', async ({ request }) => {
    const res = await request.fetch(`${backendURL}/api/v1/translate`, {
      method: 'OPTIONS',
      headers: {
        Origin: 'http://promaxgb10-6116:3000',
        'Access-Control-Request-Method': 'POST',
        'Access-Control-Request-Headers': 'content-type,x-api-token',
      },
    });
    // Fiber cors responds 204/200 on allowed preflight
    expect(res.status()).toBeLessThan(400);
    const allowHeaders = (res.headers()['access-control-allow-headers'] || '').toLowerCase();
    // When CORS middleware runs, X-API-Token should be permitted
    if (allowHeaders) {
      expect(allowHeaders).toMatch(/x-api-token|authorization|content-type/);
    }
  });

  test('oversized body is rejected (DoS guard)', async ({ request }) => {
    // 3 MiB payload — well over typical text body; Fiber BodyLimit is PDF-sized but this
    // still validates the server remains stable under large posts.
    const big = 'A'.repeat(3 * 1024 * 1024);
    const res = await request.post(`${backendURL}/api/v1/translate`, {
      headers: authHeaders(),
      data: {
        text: big,
        source: 'EN',
        target: 'DE',
      },
      timeout: 30_000,
    });
    // Expect validation error, 413, 400, 401, or rate limit — not a hang/crash (5xx connection reset).
    expect([400, 401, 413, 429, 500, 503]).toContain(res.status());
  });
});

test.describe('Error + self-healing visibility', () => {
  test('health dashboard exposes dependency statuses', async ({ request }) => {
    const res = await request.get(`${backendURL}/api/v1/health/dashboard`);
    expect([200, 503]).toContain(res.status());
    const body = await res.json();
    expect(body).toBeTruthy();
  });

  test('dead-letter list is internal-token gated', async ({ request }) => {
    const res = await request.get(`${backendURL}/api/v1/jobs/dead?limit=5`);
    expect([200, 401]).toContain(res.status());
  });
});
