# Stitch Frontend Design

## Design Source

The frontend adapts Stitch project `16776319798782383658`:

- Image inspection: `7d453629be804caf890e70d4224a0c06`
- Text translation: `daf04411bf5448d0a1de16b73701d9c1`
- PDF jobs: `a4e5b025b9564f4c80910a58270b484a`
- Operations and recovery: `219cc5e12c27474bba60db8dba4c15ef`

The design is implemented as React components, not embedded generated HTML.
It retains the existing API layer, translation hooks, asynchronous job tracking,
and protected operator requests.

## Design System

- Cool neutral surfaces with teal primary actions and distinct semantic states.
- Self-hosted IBM Plex Sans, with existing bundled multilingual script fonts.
- Named Lucide imports for lightweight, accessible interface controls.
- Compact navigation and workflow-specific translation surfaces.
- Responsive layouts checked at 1440, 768, 390, and 360 pixels.
- Read-only OCR source inspection, selectable translated text, and image comparison.
- Keyboard-accessible history restoration and labeled mobile navigation.
- Indeterminate PDF progress when measured progress is unavailable.
- Samples and Showcase start only on explicit user request; first visits remain idle.

The generated screens contain illustrative statistics and unsupported features.
Translation memory, glossaries, automatic text translation, model selectors,
hardware claims, invented version numbers, and account controls were not imported.
Self-hosting does not imply every configured translation provider runs locally.

## Local Preview

Run `npm run dev --prefix frontend -- --port 13001 --strictPort` to use an
alternate preview port without replacing an existing deployment. The Vite
configuration forwards `/ready` and `/api` to `VITE_BACKEND_PROXY_TARGET`, and
`/ocr` to `VITE_OCR_PROXY_TARGET`. Defaults remain local ports 28080 and 18000.

## Verification

Run the frontend production build, unit tests, and lint before deployment.
The existing comprehensive UI suite includes four Stitch-specific regressions
covering view separation, history restoration, responsive assets and toolbar
geometry, and image slider clipping and overlay controls.

Use the Compose-served frontend for the complete E2E suite: security-header
assertions cover nginx, not the Vite development server. Refresh the four
existing visual baselines only when accepting an intentional design change.
The isolated production runtime gate remains `node scripts/validate-prod.mjs`;
it tests a mock provider with temporary credentials and isolated queues, not
production-provider quality or load capacity.

Frontend-only deployment uses the `loklingo-frontend` Compose service with
`--no-deps`; backend, OCR, and persistent data do not need recreation.

## Verified Results

- Frontend production build, 28 unit tests, and lint passed.
- Deployed Chromium suite: 88 passed, one opt-in Postgres analytics test skipped.
- Four existing local visual baselines regenerated and passed on a subsequent run.
- Isolated production runtime: authorization/hardening assertions and four E2E flows passed.
- LAN frontend at `http://promaxgb10-6116:13000/` returned HTTP 200.
- Dependency audit reported 21 findings (15 moderate, five high, one critical).
	Neither Lucide nor the new font package was flagged. Broad upgrades remain separate.