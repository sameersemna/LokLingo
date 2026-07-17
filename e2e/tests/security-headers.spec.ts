import { test, expect } from '@playwright/test';

const backendURL = process.env.LOKLINGO_BASE_URL || 'http://localhost:28080';

test.describe('Security headers and edge posture', () => {
  test('frontend serves CSP and frame denial headers', async ({ request }) => {
    const res = await request.get('/');
    expect(res.status()).toBeLessThan(400);

    const csp = res.headers()['content-security-policy'] || '';
    expect(csp.toLowerCase()).toContain("default-src 'self'");
    expect(csp.toLowerCase()).toContain("frame-ancestors 'none'");

    const xfo = res.headers()['x-frame-options'] || '';
    expect(xfo.toUpperCase()).toBe('DENY');

    const nosniff = res.headers()['x-content-type-options'] || '';
    expect(nosniff.toLowerCase()).toBe('nosniff');
  });

  test('backend health returns defensive security headers', async ({ request }) => {
    const res = await request.get(`${backendURL}/health`);
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.status).toBe('ok');

    expect((res.headers()['x-content-type-options'] || '').toLowerCase()).toBe('nosniff');
    expect((res.headers()['x-frame-options'] || '').toUpperCase()).toBe('DENY');
    expect(res.headers()['x-request-id']).toBeTruthy();
  });

  test('backend API responses are not cacheable', async ({ request }) => {
    const res = await request.get(`${backendURL}/api/v1/health/dashboard`);
    // dashboard may be 200 even when degraded dependencies exist
    expect([200, 503]).toContain(res.status());
    const cache = (res.headers()['cache-control'] || '').toLowerCase();
    expect(cache).toContain('no-store');
  });

  test('metrics endpoint requires internal token when configured', async ({ request }) => {
    const res = await request.get(`${backendURL}/api/v1/metrics/prometheus`);
    // Without token: 401 if INTERNAL_TOKEN set, else 200 in open dev.
    expect([200, 401]).toContain(res.status());
    if (res.status() === 401) {
      const body = await res.json();
      expect(body.error).toMatch(/unauthorized/i);
    }
  });
});
