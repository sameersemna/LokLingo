# LokLingo UI Improvement Log

## Cycle 0 — Orientation (2026-07-26)

**App**: LokLingo, a self-hosted image/PDF/text translation tool. Single-page
React 19 + Vite 8 + TypeScript app (`frontend/src`), Go Fiber backend, Python
OCR service, all served via Docker Compose.

**Primary user flow**: land on the page → pick source/target language →
choose a workflow (Image / PDF / Text) → drop an image or paste text → watch
a staged progress UI (OCR detection → layout → language → translation →
typography → render) → compare original vs. translated output in a
slider/flash/gallery modal → export (copy / download / PDF).

**Structure**: `App.tsx` (1985 lines — well over the project's own 500-line
component guideline in `.agentrules`; flagged as an architectural follow-up,
out of scope for this UI loop) orchestrates ~14 presentational components in
`components/` plus several hooks (`useTheme`, `useToasts`,
`useLanguageSelection`, `useComparisonModal`, `useReadiness`, etc.). Styling
is a single hand-written `App.css` (3.6k lines, CSS custom properties for
theme, no CSS framework).

**Dev/test surface**: Frontend is served from a built Docker image
(`loklingo-frontend`, nginx), not a live Vite dev server — verifying changes
requires `docker compose build/up --no-deps loklingo-frontend`. Host port is
driven by `.env`'s `FRONTEND_HOST_PORT` (currently `13000`; the container
had drifted to a stale manual mapping of `3300` before this session's
rebuild corrected it back to match `.env`). `npm test` is currently broken
in this checkout (missing `jsdom` devDependency, pre-existing, unrelated to
this loop — noted as a gap, not fixed here).

**Candidate goals for this loop**: mobile responsiveness/hierarchy,
accessibility (touch targets, contrast, focus states), and interaction
consistency. Visual polish is already quite good (gradient branding,
consistent card/pill vocabulary, dark/light theme parity).

**Incident note**: An early `docker compose up -d loklingo-frontend` pulled
in a dependency recreate that dropped `loklingo-ollama`'s host port
mapping (`11435:11434`). Caught and restored immediately via the
`docker-compose.ollama-host.yml` overlay with `OLLAMA_HOST_PORT=11435`.
Lesson applied for the rest of the loop: always pass `--no-deps` when
touching a single compose service.

---

## Cycle 1 — Mobile header hierarchy (2026-07-26)

### Findings (Phase 1, observed at 375×812 / 768×1024 / 1440×900)

1. **[High] Mobile: 7 secondary utility buttons dominate above the fold.**
   `Header.tsx` renders PDF Jobs / Reliability / Dead Ops / History / Demos /
   Showcase / Dark-Light as a flat button row. At ≤600px this collapses into
   a 2-column grid (`App.css` `.header-actions` in the `max-width: 600px`
   block) that pushes the tagline, health chip, language pickers, mode
   selector, and the entire core translate workflow below the fold. A
   first-time mobile visitor has to scroll past a wall of buttons before
   reaching the product's primary action.
   Observed via headless Chromium at 375×812 (screenshot reviewed in-session;
   the MCP screenshot tool used returns images inline rather than writing
   files to disk, so no `screenshots/` artifact was persisted this cycle).

2. **[Medium] Touch targets below recommended minimum.** `.header-actions
   .icon-btn` / `.theme-btn` used `min-height: 34px` (desktop) / `38px`
   (≤600px) / `36px` (≤400px) — under the 44–48px minimum recommended by
   current mobile nav guidance (Nielsen Norman / WCAG 2.5.5 AAA / Material
   Design touch target guidance).

### Research (Phase 2)

Searched current (2026) guidance on mobile nav collapse patterns. Consensus
across sources (developerux.com "Primary Navigation Best Practices" 2026-07,
phone-simulator.com "Mobile Navigation Patterns 2026", Pravin Kumar's 2026
Webflow nav writeup):
- Mobile should show ≤3–5 primary items directly; everything else belongs in
  a collapsed "More"/overflow menu, not a flat wall of buttons.
- Labeled toggle buttons ("Menu") are preferred over icon-only affordances —
  cited: 64% of users prefer labeled controls for primary actions.
- Minimum tap target: 44–48px CSS pixels.
- Use `aria-expanded` on the toggle, keep the collapsed content in the DOM
  (not conditionally unmounted) so it stays accessible when open.

None of LokLingo's 7 header actions are needed for first-run comprehension
(they're power-user/ops tools: PDF job tracking, reliability telemetry,
dead-letter ops, history, demos, showcase, theme) — all are safe to move
behind a single mobile toggle without hurting task completion.

### Implementation (Phase 3)

- `frontend/src/components/Header.tsx`: added local `mobileNavOpen` state
  and a `Menu ▼/▲` toggle button (visible only ≤600px via CSS), wrapping the
  existing `.header-actions` in `aria-expanded`/`aria-controls`. Clicking any
  action inside the menu closes it (bubn-phase `onClick` on the container).
  All 7 actions remain in the DOM and reachable by keyboard/screen reader —
  only their visibility is toggled.
- `frontend/src/App.css`: `.header-actions` now `display: none` by default
  under `max-width: 600px`, `display: grid` when `.is-open`. Bumped
  `min-height` on mobile icon/theme buttons from 38px/36px to 44px at both
  the 600px and 400px breakpoints. Desktop/tablet (>600px) layout is
  untouched — the toggle button stays `display: none` there.

### Verification (Phase 4)

Rebuilt and redeployed `loklingo-frontend` (`docker compose build` +
`up -d --no-deps`), re-screenshotted (all reviewed in-session; see note
above on why no files were persisted to `screenshots/`):
- Mobile (375px) collapsed: title + tagline + health chip + full translate
  workflow now visible without scrolling.
- Mobile (375px) expanded: tapping "Menu" reveals all 7 actions in a
  44px-tall 2-column grid, closes on selection.
- Tablet (768px) and desktop (1440px): unchanged, buttons still shown
  inline as before — confirmed no regression.
- `npx tsc -b --noEmit`: clean. `npm run lint`: 0 errors, 15 pre-existing
  warnings (unrelated to this change, all in `App.tsx`/`main.tsx`).
- `npm test`: fails to run in this checkout due to a pre-existing missing
  `jsdom` devDependency — unrelated to this change, not fixed here (flagged
  as a follow-up below).

### Deferred / follow-ups

- `App.tsx` at 1985 lines violates the project's own 500-line component
  guideline (`.agentrules`). A real fix (extracting the translate/OCR
  orchestration into hooks) is a structural change well beyond a UI loop —
  flagging for a dedicated refactor session.
- `npm test` is broken in this checkout (`Cannot find package 'jsdom'`) —
  worth a one-line `npm i -D jsdom` fix in a follow-up, done outside this
  UI-focused loop to avoid scope creep.
- Contrast/focus-state audit of the readiness popover and comparison modal
  not yet done — good candidate for Cycle 2.
- Frontend is only reachable via the built Docker image; no fast dev-server
  loop exists for this kind of iteration. Consider documenting a
  `vite dev`-based workflow for local UI work in `guide/`.

### Cycle decision

Diminishing-returns check: the highest-severity, highest-confidence finding
(mobile hierarchy) is fixed and verified with no regressions. Remaining
candidates (contrast audit, focus states, `jsdom` fix, `App.tsx` size) are
lower urgency or out of this loop's blast radius. Stopping here for this
session rather than manufacturing lower-value findings to fill an iteration
quota — happy to continue with Cycle 2 (accessibility/contrast pass) on
request.
