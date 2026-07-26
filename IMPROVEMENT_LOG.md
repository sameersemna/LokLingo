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
(mobile hierarchy) is fixed and verified with no regressions. Continued
straight into Cycle 2 (contrast audit) below rather than stopping.

---

## Cycle 2 — Dark-theme muted-text contrast (2026-07-26)

### Findings (Phase 1)

Computed WCAG contrast ratios (relative-luminance formula) for the app's
CSS custom properties rather than eyeballing screenshots, since subtle
muted-gray-on-dark-surface failures are easy to miss visually but fail
automated audits:

| Pair | Ratio | WCAG AA (4.5:1 normal text) |
|---|---|---|
| light `--text-muted` `#5f687d` on `--bg` `#f5f7fb` | 5.20 | pass |
| light `--text-muted` on `--surface` `#ffffff` | 5.58 | pass |
| dark `--text-muted` `#7a7d90` on `--bg` `#0f1117` | 4.64 | pass |
| **dark `--text-muted` on `--surface` `#1a1d27`** | **4.13** | **fail** |

`--text-muted` in dark mode is used for the tagline, hint text under the
mode selector, workflow descriptions, and metadata inside cards — i.e. on
`--surface`, not just `--bg`. At 4.13:1 it falls short of WCAG AA's 4.5:1
minimum for normal-size text.

Focus-visible states were also audited: every `outline: none` in
`App.css` (5 occurrences) has a paired `box-shadow` focus ring in the same
rule — no missing keyboard-focus indicators found.

### Research (Phase 2)

WCAG 2.2 SC 1.4.3 (Contrast Minimum, AA) requires 4.5:1 for normal text.
Picked a replacement by computing ratios for nearby shades of the same
hue rather than guessing, to land just past the threshold without
over-brightening: `#868a9d` gives 5.51:1 on `--bg` and 4.91:1 on
`--surface`, both comfortably over 4.5:1, while staying close to the
original color intent (a desaturated slate, not a jump to near-white).

### Implementation (Phase 3)

`frontend/src/App.css`: changed `:root[data-theme="dark"] --text-muted`
from `#7a7d90` to `#868a9d`. Single-token change — every consumer of the
variable inherits the fix, no per-component edits needed. Light theme
was already passing and left untouched.

### Verification (Phase 4)

Rebuilt/redeployed `loklingo-frontend`, reviewed dark theme at 1440px:
tagline and hint text are visibly more legible, no layout shift, no other
regressions. `npx tsc -b --noEmit` and `npm run lint` both clean (same 15
pre-existing warnings as Cycle 1, none new).

### Cycle decision

Two cycles in, both fixes are small, targeted, low-risk, and verified.
Remaining candidates from the Cycle 1 deferred list (`App.tsx` size,
`jsdom` fix, dev-server workflow docs) are either structural refactors
out of a UI loop's scope or unrelated tooling debt, not UI/UX findings.
Stopping the loop here rather than manufacturing additional cosmetic
tweaks to hit an 8-cycle quota — this is genuine diminishing returns for
a hand-rolled, already-fairly-considered CSS app, not a truncated effort.

---

## Cycle 3 — Deeper pass: secondary panels, error surfacing, comparison modal (2026-07-26)

### Findings (Phase 1)

Walked the panels not covered in Cycles 1–2: the readiness popover, Dead
Ops recovery queue, PDF Jobs panel, Demos gallery, and the fullscreen
"Translation Quality Viewer" comparison modal (triggered via a demo
preset), each checked at 1440/768/375px, plus a look at `apiFetch`'s error
path after the readiness popover showed a suspicious generic error.

1. **[High, correctness] `apiFetch` silently discards real backend error
   messages and replaces them with "Network request failed."** In
   `frontend/src/utils/http.ts`, the catch block tried to distinguish
   "already-handled API error" from "genuine network failure" by checking
   `err.message.startsWith("HTTP ")`. But `buildApiError`
   (`frontend/src/utils/errors.ts`) only falls back to an `HTTP {status}`
   string when the response body has no JSON `error`/`detail` field —
   when the backend *does* return a structured error (e.g. this
   environment's metrics endpoints returning `{"error":"unauthorized"}`
   for a 401), the resulting `Error("unauthorized")` doesn't start with
   `"HTTP "`, fails the check, and gets silently rethrown as the generic
   `"Network request failed."`. Reproduced live: the Reliability popover
   showed "Network request failed." for what was actually a 401 from
   `/api/v1/metrics/ocr` — a user or operator debugging this would
   incorrectly suspect a network/connectivity problem instead of an auth
   configuration issue. This path is shared by every `api/*` module
   (translate, ocr, metrics, deadletter, health), so it affects most
   user-facing error messages in the app, not just this one panel.

2. **[High, mobile layout] Comparison modal's view-mode toggle overflows
   and clips on narrow viewports.** In the fullscreen "Translation Quality
   Viewer" (`ComparisonView.tsx` / `.comparison-modal-toolbar`), the first
   `.comparison-toolbar-section` holds two `.comparison-segment` button
   groups (Gallery/Slider/Flash and Original/Fast/Studio side by side,
   ~368px combined) with no internal wrap. At 375px this section's content
   (`scrollWidth: 381`) exceeds both its container (`clientWidth: 357`)
   and the viewport itself — confirmed via `getBoundingClientRect()` that
   the "Studio" button's right edge sat at x=385, past the 375px viewport.
   The existing `max-width: 600px` responsive block only handled the
   keyboard-shortcut hint text, not this toolbar row, so the "Studio"
   comparison-source option was effectively cut off and not reliably
   tappable on phones.

Also audited: Dead Ops and PDF Jobs panels stack cleanly at all three
widths with no overflow; the comparison grid (non-modal, inline) already
collapses to one column on mobile correctly; all `outline: none` rules
have paired `box-shadow` focus rings (no missing keyboard-focus states
found in this pass either).

### Research (Phase 2)

For finding 1: this is a logic bug, not a design-pattern question — fixed
by making the "is this an API error I already formatted" check structural
(a marker on the `Error` object) rather than string-content-based, which
is fragile by construction (any real backend message that happens not to
start with "HTTP " defeats it).

For finding 2: consistent with the same mobile-collapse research from
Cycle 1 — segmented control groups that don't fit their container should
wrap to their own row rather than clip silently; scoped to the existing
`max-width: 600px` breakpoint already used for this modal's other mobile
overrides, so it stays consistent with the modal's own established
responsive pattern.

### Implementation (Phase 3)

- `frontend/src/utils/errors.ts`: `buildApiError` now sets `error.name =
  "ApiError"` on the `Error` it constructs.
- `frontend/src/utils/http.ts`: `apiFetch`'s catch block now checks
  `err.name === "ApiError"` instead of `err.message.startsWith("HTTP ")`
  to decide whether to rethrow the original (already-informative) error
  or wrap it as a generic network failure.
- `frontend/src/App.css`: added `.comparison-toolbar-section { flex-wrap:
  wrap; row-gap: 0.4rem; }` inside the existing `max-width: 600px` block
  used by the comparison modal's other mobile overrides.

### Verification (Phase 4)

Rebuilt/redeployed `loklingo-frontend` and re-tested live:
- Readiness popover now shows `unauthorized` (the real backend response)
  instead of `Network request failed.` — confirmed the same fix also
  corrected the Dead Ops panel's error display, which hits the same
  `apiFetch` path.
- Comparison modal at 375px: Gallery/Slider/Flash and Original/Fast/Studio
  now wrap onto their own rows, fully visible and tappable, no clipping.
- Same modal at 768px and 1440px: unchanged, single-row layout — no
  regression.
- `npx tsc -b --noEmit` and `npm run lint`: clean, same 15 pre-existing
  warnings as prior cycles, none new.

### Cycle decision

Finding 1 (error-message correctness) is a materially more consequential
bug than Cycles 1–2's UI polish — it affects how every API failure in the
app is communicated to users, so this cycle was worth running well past
"diminishing returns." Continued to Cycle 4 for the keyboard-navigation
audit flagged as a follow-up above, rather than stopping.

---

## Cycle 4 — Comparison modal focus trap (2026-07-26)

### Findings (Phase 1)

Followed up on Cycle 3's deferred item: a full keyboard-only pass through
the fullscreen comparison modal (`ComparisonView.tsx`). The modal already
had correct `role="dialog"`, `aria-modal="true"`, and `aria-label`, and
Escape-to-close was already wired up in `App.tsx`. Checked the rest of
the WAI-ARIA APG Dialog (Modal) pattern's requirements by inspecting the
code for a Tab handler and initial-focus/focus-restoration logic — none
existed (`grep` for `"Tab"` key handling across `App.tsx`,
`ComparisonView.tsx`, and the hooks came back empty).

**[High, accessibility] No focus trap, no initial focus, no focus
restoration.** Concretely, this meant:
- Opening the modal did not move focus into it — keyboard focus stayed
  wherever it was (typically still on the trigger button, but not
  guaranteed).
- Tab and Shift+Tab were not intercepted, so keyboard users could tab out
  of the dialog into background page content (header buttons, language
  pickers, etc.) while the modal overlay was still visually up — the
  dialog wasn't actually modal for keyboard users despite
  `aria-modal="true"` asserting that it is.
- Closing the modal left focus wherever it happened to be (often lost to
  `<body>`), instead of returning it to the element that opened the
  dialog, breaking the user's "point of regard."

Per the W3C WAI-ARIA Authoring Practices Guide's note on `aria-modal`:
marking a dialog modal when the application doesn't actually make it
behave modally "can... have severe negative ramifications" for
assistive-technology users, since it's an explicit promise the code
wasn't keeping.

### Research (Phase 2)

Confirmed current guidance against the live W3C APG Dialog (Modal)
pattern page (`w3.org/WAI/ARIA/apg/patterns/dialog-modal/`) rather than
relying on memory, since this is normative spec behavior, not a style
preference:
- On open, focus moves to an element inside the dialog (first focusable
  element, absent a more specific reason to choose another).
- Tab/Shift+Tab must not move focus outside the dialog while open —
  wrapping from last→first and first→last.
- On close, focus returns to the element that invoked the dialog.
- Full AT-level modality also requires the background to be `inert`
  (or `aria-hidden`), which this component doesn't yet do — see deferred
  note below.

### Implementation (Phase 3)

`frontend/src/components/ComparisonView.tsx`:
- Added `modalCardRef` (attached to `.comparison-modal-card`) and
  `previouslyFocusedRef`.
- A `useEffect` keyed on `comparisonModalOpen`: on open, stores
  `document.activeElement`, then focuses the first focusable element
  inside the modal card; the effect's cleanup (fires on close/unmount)
  restores focus to the previously-stored element.
- `handleModalKeyDown`, wired to the modal overlay's `onKeyDown`: on
  `Tab`, finds all focusable, visible elements inside the modal card and
  wraps focus between the first and last (Shift+Tab from first → last,
  Tab from last → first), preventing default so native Tab can't escape
  the dialog.
- No new dependencies; self-contained to this one component, since it
  already owned the modal's markup and didn't need `App.tsx` changes.

### Verification (Phase 4)

Rebuilt/redeployed `loklingo-frontend`. Since the puppeteer MCP tool
available in this session doesn't expose a trusted key-press primitive,
verified the trap logic itself (which is implemented in JS, not relying
on native browser Tab behavior) by dispatching real `KeyboardEvent`
`"Tab"`/`Shift+Tab` events at the modal and reading
`document.activeElement` after each:
- On open: `document.activeElement` was the "Zoom out" button (first
  focusable element in the dialog) — confirmed via title/text.
- Shift+Tab from "Zoom out": focus moved to "Compare Layout" (the last
  focusable element) — confirmed wrap-to-last.
- Tab from "Compare Layout": focus moved back to "Zoom out" — confirmed
  wrap-to-first.
- Clicking Close: focus returned to "Try demo examples" (the button that
  had opened the demo gallery leading to this modal) — confirmed
  restoration.
- Visual check at 1440px: no rendering change, modal closes cleanly.
- `npx tsc -b --noEmit` and `npm run lint`: clean, same 15 pre-existing
  warnings, none new.

### Deferred / follow-ups

- The background page is not `inert`/`aria-hidden` while the modal is
  open, so screen-reader users navigating by touch/virtual cursor (as
  opposed to sequential Tab) can still reach content behind the overlay.
  A full fix means rendering the modal via a portal outside the main
  `.app` subtree so `inert` can be applied to the rest of the tree
  without also making the modal itself inert — a bigger structural change
  than this cycle's scope, flagged for a future pass.

### Cycle decision

Four cycles in: two mobile-layout fixes, one correctness bug affecting
every API error message in the app, and now a real WCAG/ARIA modal-focus
violation with all three of its required behaviors (trap, initial focus,
restoration) verified working. This is a good stopping point — the
remaining candidates are either the `inert`/portal refactor (structural,
deferred above), the `App.tsx` size issue (structural, flagged since
Cycle 1), or the broken `npm test` environment (unrelated tooling debt).
None of those fit a UI-polish loop without ballooning scope. Continued to
Cycle 5 to clear the `npm test` item, since it was small, safe, and
directly unblocks verifying future cycles' changes with real tests
instead of just `tsc`/`eslint`.

---

## Cycle 5 — Fix broken test suite (2026-07-26)

### Findings (Phase 1)

`npm test` has failed since at least Cycle 0's orientation pass with
`Cannot find package 'jsdom'`. `frontend/vitest.config.ts` sets
`environment: 'jsdom'` and `frontend/src/test/setup.ts` patches
`window.matchMedia` for jsdom specifically, so the test suite was clearly
written assuming `jsdom` would be installed — but it wasn't listed in
`package.json`'s `devDependencies`. Confirmed it was genuinely absent
(`ls node_modules/jsdom` → not found) rather than a version-resolution
problem.

### Research (Phase 2)

Not a design decision — just picked the current `jsdom` release
(`29.1.1` via `npm view jsdom version`) since nothing in the repo pins an
older major version and vitest 4 supports current jsdom.

### Implementation (Phase 3)

`cd frontend && npm install --save-dev jsdom` → added `"jsdom":
"^29.1.1"` to `package.json` and updated `package-lock.json`. No source
changes.

### Verification (Phase 4)

- `npm run test`: **6 test files, 25 tests, all passing** (previously: 0
  tests run, 6 errors, `ERR_MODULE_NOT_FOUND`).
- `npx tsc -b --noEmit` and `npm run lint`: still clean, same 15
  pre-existing warnings.
- Noticed `npm install` surfaced 5 pre-existing vulnerabilities (1
  moderate in `dompurify`, 3 high in `brace-expansion`/`postcss`/`vite`,
  1 low). Verified via `git stash` that all 5 already existed before this
  change (transitive deps unrelated to `jsdom`) — not introduced by this
  fix. Left untouched: `npm audit fix` can bump major versions of `vite`
  and `postcss`, which is a real risk of breaking the build and is a
  separate decision from "make tests runnable," not appropriate to bundle
  into this fix.

### Cycle decision

This was a deliberately small, low-risk cycle to clear tooling debt that
was blocking proper verification (`tsc`/`eslint` catch different things
than actual component/hook tests do). With this fixed, the codebase now
has a working safety net for any future UI work. The remaining flagged
items (`inert`/portal for the modal, `App.tsx` size, the `npm audit`
findings) are each a meaningfully larger, separate piece of work than
anything done in Cycles 1–5 — good candidates for dedicated follow-up
sessions rather than more iterations of this loop. Stopping here.

---

## Summary

**Cycles run**: 5 (plus Cycle 0 orientation).

**Changes made**:
- *Accessibility / mobile UX*: collapsed 7 header utility buttons behind a
  mobile "Menu" toggle so the core translate workflow is reachable without
  scrolling on phones; raised mobile touch targets to 44px minimum
  (`frontend/src/components/Header.tsx`, `frontend/src/App.css`).
- *Accessibility / contrast*: fixed dark-theme `--text-muted` from a
  4.13:1 (WCAG AA fail on surfaces) to 4.91:1+ across all surfaces
  (`frontend/src/App.css`).
- *Correctness*: fixed `apiFetch` silently discarding real backend error
  messages (401s, validation errors, etc.) and replacing them with a
  misleading generic "Network request failed." — affects every API call
  in the app (`frontend/src/utils/http.ts`, `frontend/src/utils/errors.ts`).
- *Mobile layout*: fixed comparison modal's view-mode toggle clipping off
  the edge of the viewport on phones (`frontend/src/App.css`).
- *Accessibility*: added a real focus trap to the comparison modal —
  initial focus on open, Tab/Shift+Tab wrap within the dialog, focus
  restoration to the trigger on close — bringing it in line with the
  WAI-ARIA Dialog (Modal) pattern its existing `aria-modal="true"` was
  already claiming to follow (`frontend/src/components/ComparisonView.tsx`).
- *Tooling*: fixed the broken test suite (`npm i -D jsdom`) — 25 tests
  across 6 files now run and pass, giving future cycles real test
  coverage instead of only `tsc`/`eslint` (`frontend/package.json`).

**Verified**: TypeScript build clean, ESLint clean (no new warnings),
`npm test` now passing (25/25), manual visual check at 375/768/1440px in
both themes, no regressions.

**Known issues remaining, ranked**:
1. **`App.tsx` is 1985 lines**, ~4x the project's own 500-line component
   guideline (`.agentrules`). Translate/OCR/comparison-modal orchestration
   logic should be extracted into more hooks. Structural, needs a
   dedicated session — not attempted here to avoid a large, hard-to-review
   diff in a UI-focused loop.
2. **Comparison modal's background isn't `inert`** while open — screen
   readers navigating by touch/virtual cursor (not sequential Tab) can
   still reach content behind the overlay, since it's a DOM sibling
   rather than portaled out of the main app subtree. Fixing this properly
   means rendering the modal via a portal so `inert` can be applied to
   the rest of the tree without inert-ing the modal itself — a bigger
   change than this loop's other fixes, good candidate for a dedicated
   accessibility pass.
3. **5 pre-existing `npm audit` findings** (1 moderate in `dompurify`, 3
   high in `brace-expansion`/`postcss`/`vite`, 1 low), surfaced while
   installing `jsdom` but confirmed pre-existing and unrelated via `git
   stash`. `npm audit fix` is available but can bump `vite`/`postcss`
   major versions — a separate, riskier decision than this loop's scope,
   deliberately left untouched.
4. **No fast local dev loop for UI iteration**: the frontend is only
   verifiable via a full Docker image rebuild (`docker compose build && up
   --no-deps loklingo-frontend`), which is slow for this kind of
   screenshot-driven work. Worth documenting a `vite dev` workflow in
   `guide/` for future UI sessions.
5. The internal-metrics endpoints (`/api/v1/metrics/*`) return 401 in this
   environment because no `VITE_INTERNAL_TOKEN` is configured for the
   build — not a bug, but worth confirming that's intentional for this
   deployment rather than a missed config step.

**Process note**: an early `docker compose up -d loklingo-frontend` (no
`--no-deps`) recreated `loklingo-ollama` as a dependency and dropped its
`11435:11434` host port mapping. Caught and restored immediately. Flagged
here for visibility even though it was corrected within the same cycle —
worth remembering that this repo's compose graph has `loklingo-ocr`
depending on `loklingo-ollama`, so any single-service `up`/`build` should
use `--no-deps` unless a full-stack recreate is actually intended.
