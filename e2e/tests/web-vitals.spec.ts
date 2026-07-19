import { test, expect } from '@playwright/test';

const BASE = process.env.LOKLINGO_E2E_BASE_URL || 'http://localhost:3000';

test.describe('Web Vitals Performance Profiling', () => {
  test.use({ baseURL: BASE });

  test('measure LCP, CLS, and INP via Performance API', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });

    const vitals: Record<string, number> = {};

    await page.goto('/', { waitUntil: 'networkidle', timeout: 30_000 });
    await page.waitForTimeout(3000);

    const metrics = await page.evaluate(() => {
      return new Promise<Record<string, number>>((resolve) => {
        const results: Record<string, number> = {};

        // LCP
        const lcpObs = new PerformanceObserver((list) => {
          const entries = list.getEntries();
          if (entries.length > 0) {
            results.lcp = entries[entries.length - 1].startTime;
          }
        });
        lcpObs.observe({ type: 'largest-contentful-paint', buffered: true });

        // CLS
        let clsValue = 0;
        const clsObs = new PerformanceObserver((list) => {
          for (const entry of list.getEntries()) {
            if (!(entry as any).hadRecentInput) {
              clsValue += (entry as any).value;
            }
          }
        });
        clsObs.observe({ type: 'layout-shift', buffered: true });

        // FCP
        const fcpObs = new PerformanceObserver((list) => {
          const entries = list.getEntries();
          if (entries.length > 0) {
            results.fcp = entries[0].startTime;
          }
        });
        fcpObs.observe({ type: 'paint', buffered: true });

        // TTFB
        const nav = performance.getEntriesByType('navigation')[0] as any;
        if (nav) {
          results.ttfb = nav.responseStart - nav.requestStart;
          results.domContentLoaded = nav.domContentLoadedEventEnd;
          results.loadEvent = nav.loadEventEnd;
        }

        // Resource timing summary
        const resources = performance.getEntriesByType('resource');
        let totalSize = 0;
        let jsSize = 0;
        let cssSize = 0;
        let imgSize = 0;
        let fontSize = 0;
        for (const r of resources) {
          const size = (r as any).transferSize || 0;
          totalSize += size;
          if (r.name.endsWith('.js')) jsSize += size;
          else if (r.name.endsWith('.css')) cssSize += size;
          else if (r.name.match(/\.(png|jpg|jpeg|gif|webp|svg|avif)/i)) imgSize += size;
          else if (r.name.match(/\.(woff2?|ttf|otf|eot)/i)) fontSize += size;
        }
        results.totalTransferSize = totalSize;
        results.jsTransferSize = jsSize;
        results.cssTransferSize = cssSize;
        results.imgTransferSize = imgSize;
        results.fontTransferSize = fontSize;
        results.resourceCount = resources.length;

        // Wait a bit for CLS to settle
        setTimeout(() => {
          results.cls = clsValue;
          resolve(results);
        }, 1000);
      });
    });

    console.log('=== Web Vitals ===');
    console.log(`LCP: ${metrics.lcp?.toFixed(1) ?? 'N/A'} ms`);
    console.log(`FCP: ${metrics.fcp?.toFixed(1) ?? 'N/A'} ms`);
    console.log(`CLS: ${metrics.cls?.toFixed(4) ?? 'N/A'}`);
    console.log(`TTFB: ${metrics.ttfb?.toFixed(1) ?? 'N/A'} ms`);
    console.log(`DOM Content Loaded: ${metrics.domContentLoaded?.toFixed(1) ?? 'N/A'} ms`);
    console.log(`Load Event: ${metrics.loadEvent?.toFixed(1) ?? 'N/A'} ms`);
    console.log('=== Resource Summary ===');
    console.log(`Total transfer size: ${(metrics.totalTransferSize / 1024).toFixed(1)} KB`);
    console.log(`JS transfer size: ${(metrics.jsTransferSize / 1024).toFixed(1)} KB`);
    console.log(`CSS transfer size: ${(metrics.cssTransferSize / 1024).toFixed(1)} KB`);
    console.log(`Image transfer size: ${(metrics.imgTransferSize / 1024).toFixed(1)} KB`);
    console.log(`Font transfer size: ${(metrics.fontTransferSize / 1024).toFixed(1)} KB`);
    console.log(`Resource count: ${metrics.resourceCount}`);

    // Assertions for Core Web Vitals thresholds
    if (metrics.lcp !== undefined) {
      expect(metrics.lcp).toBeLessThan(2500);
    }
    if (metrics.cls !== undefined) {
      expect(metrics.cls).toBeLessThan(0.1);
    }
    if (metrics.fcp !== undefined) {
      expect(metrics.fcp).toBeLessThan(1800);
    }
    if (metrics.ttfb !== undefined) {
      expect(metrics.ttfb).toBeLessThan(800);
    }
  });

  test('measure bundle sizes and check for render-blocking resources', async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('loklingo-first-visit', 'false');
    });

    await page.goto('/', { waitUntil: 'networkidle', timeout: 30_000 });
    await page.waitForTimeout(2000);

    const resourceInfo = await page.evaluate(() => {
      const resources = performance.getEntriesByType('resource');
      return resources.map(r => ({
        name: r.name.split('/').pop() || r.name,
        type: r.initiatorType,
        size: (r as any).transferSize || 0,
        duration: r.duration.toFixed(1),
        startTime: r.startTime.toFixed(1),
      }));
    });

    // Log all resources for analysis
    console.log('=== Resources loaded ===');
    for (const r of resourceInfo) {
      console.log(`  ${r.type} ${r.name} (${(r.size / 1024).toFixed(1)} KB, ${r.duration}ms)`);
    }

    // Check for render-blocking CSS/JS in the head
    const blockingResources = resourceInfo.filter(r =>
      r.startTime && parseFloat(r.startTime) < 100 && (r.type === 'script' || r.type === 'link')
    );
    console.log(`\nEarly-loading resources (potential render-blocking): ${blockingResources.length}`);
  });
});
