# LokLingo Transformation Plan

## What Needs to Change

The real problems are simple: **oversized files** that are hard to maintain, **dead code** left behind from prior refactorings, and **CSS duplication**. No new dependencies or architectural shifts are needed.

### Frontend Issues
- `App.tsx` — 3,287 lines. Contains UI for header, workflow switcher, upload hero, OCR visualization, image progress, comparison modal, demo presets, history panel, PDF jobs panel, reliability telemetry, dead-letter ops, and export actions. Should be decomposed into scoped components.
- `App.css` — 4,254 lines with class definitions repeated in multiple places (`.upload-preview`, `.reliability-panel`, `.header-actions .icon-btn`, `.dead-ops-*`). Needs dedup.
- `useImageProgress.ts` — 226 lines of dead code identified in prior refactor but never deleted. No imports exist today.
- No routing is needed. The app is a single-page tool; App.tsx just needs to be **decomposed into components imported by a thin shell**, not routed.

### Backend Issues
- `internal/services/image_service.go` — 2,450 lines covering font loading/layout/overlay rendering/image encoding in one file. Should split into sub-packages under `image/`.
- `jobs/worker.go` — 1,847 lines handling text, PDF, and image job lifecycles plus checkpointing/dead-letter logic. The types should stay together but can be split into focused helpers without creating import cycles.

## Implementation Steps

### Step 1 — Remove dead code
- Delete `src/hooks/useImageProgress.ts` (0 remaining imports). 
- Confirm frontend builds cleanly after removal.

### Step 2 — Deduplicate App.css
- Locate and remove repeated block definitions: `.upload-preview`, `.reliability-panel`, `.header-actions .icon-btn`, `.dead-ops-*`, `.upload-backend-chip/.dot/online/degraded/offline/checking`.
- Keep the **later, more complete** definition in each duplicated pair.
- Verify no visual regression by running frontend locally.

### Step 3 — Extract front-end feature components from App.tsx
Extract each into its own file under `src/components/`. Use existing patterns: double-quoted imports, shared utils, constants from `constants.ts`, error handling via `getErrorMessage()`. No new dependencies.

1. **`components/Header.tsx`** — h1, header-actions (PDF Jobs / Reliability / Dead Ops / History / Demos / Showcase buttons), theme toggle, readiness chip + popover with mini-reliability metrics.
2. **`components/WorkflowSwitcher.tsx`** — workflow cards (Image / PDF / Text), language selectors + swap button, mode selector with help text.
3. **`components/UploadHero.tsx`** — upload drag-drop surface, preview states (image + OCR overlay visualization, PDF placeholder, empty panel with examples), trust signals, backend status chip.
4. **`components/ImageProgress.tsx`** — pipeline progress timeline, stage list, translation region ticker, reliability hint display, intelligence panel (region count, languages, vertical/RTL detection, confidence).
5. **`components/ComparisonView.tsx`** — side-by-side grid, slider control with compare target toggle, flash view. Non-modal inline comparison below upload hero.
6. **`components/ComparisonModal.tsx`** — fullscreen modal with zoom/pan, mode tabs (gallery/slider/flash), focus selectors, before/after toggle, download, fullscreen. Replaces the large `comparisonModalOpen && ...` block in App.tsx.
7. **`components/DemoGallery.tsx`** — demo preset card grid + inline demo thumbnail strip at bottom of main view.
8. **`components/ExportActions.tsx`** — PNG / PDF / Copy / Open Compare buttons, char count footer on output panel.
9. **`components/IntelligencePanel.tsx`** — animated region/language/confidence counters (currently mixed into App's inline logic).

After extraction, `App.tsx` becomes a thin orchestrator (~300-400 lines) that manages top-level state (loading flags, result text/url, workflow/mode selection, upload selection) and mounts the extracted components. The comparison modal state can be managed with `useComparisonModal` hook as-is; no store needed.

### Step 4 — Decompose backend image_service.go
Split into sub-package `internal/services/image/`:

1. **`font.go`** — embedded fonts, face caching (`overlayFontData`, `overlayFaceCache`, `overlayAscentCache`), font loading logic.
2. **`layout.go`** — bbox overlap analysis, reading-order sort, vertical CJK detection, dominant color / shadow direction estimation per region.
3. **`overlay.go`** — text fitting loop, font-size selection, background patching with feather and blur, shadow rendering, image compositing.
4. **`render.go`** — image encoding/decoding helpers, output format conversion, DPI handling.

The old `image_service.go` becomes a thin wrapper or is removed entirely after imports are updated in worker.go and handlers. All existing tests should continue to pass with only import path changes.

### Step 5 — Add focused backend tests
- Unit tests for the extracted image sub-packages covering: font face caching, bbox overlap computation, vertical text detection threshold, background patch blur radius.
- Verify `go test ./...` still passes.

### Step 6 — Final verification
```bash
# Frontend
cd frontend && npm run build && npm test
# Backend  
cd backend && go build ./... && go test ./...
```

## What We're NOT Doing
- No Zustand, Redux, or new state libraries. React hooks and existing `useComparisonModal` are sufficient.
- No command palette (Cmd+K), sidebar, or global search. The current flat layout with header action buttons is fine for this tool's scope.
- No CSS modules or Tailwind. Global `App.css` works; the fix is dedup, not architecture change.
- No routing changes. This remains a single-page React app.
