# TanStack Query adoption — considered, deferred

Status: **considered, deferred (2026-07-19)**

## Context

The 2026-07-19 hardening assessment recommended "Consider TanStack Query
to replace ~400 lines of fetch/caching plumbing in App.tsx".

After the Phase 1 hook refactor (commit 395fee3 + earlier), the volume of
fetch plumbing in App.tsx dropped substantially:

- Theme, toasts, language, OCR visualization controls, history: all
  moved to dedicated hooks with localStorage persistence.
- Comparison modal and readiness: moved to reducer-based hooks.
- Translation / OCR / job polling: still inline in App.tsx, but
  encapsulated in `src/api/translate.ts`.

The remaining `fetch` calls fall into three categories:

1. **One-shot mutations** (POST /translate, POST /jobs/*) — short
   latency, no caching benefit, TanStack Query's
   `useMutation` would not reduce code.
2. **Polling for job state** (GET /jobs/:id) — handled by
   `pollJob` with a 600ms interval. TanStack Query's
   `useQuery({ refetchInterval })` could replace this and add
   window-focus refetching, but the existing code is ~30 lines.
3. **SSE subscriptions** (`subscribeJobEvents`) — TanStack Query does
   not natively understand SSE; you would still need a custom hook.

## Why we are not adopting TanStack Query right now

- **Marginal code reduction.** Net delta after migration is
  estimated at -50 to -100 lines of fetch plumbing, offset by +20
  lines of `QueryClient` setup and provider wiring. The current
  `src/api/*.ts` modules are deliberately framework-agnostic and
  trivially testable.
- **Bundle cost.** `@tanstack/react-query` is ~13KB gzip. The
  frontend bundle is currently 292KB / 86KB gzip; +5% is a
  noticeable regression for a self-hosted LAN tool that already
  loads the entire app on first paint.
- **No cache-sharing need.** The app has no second screen, no
  history view that needs stale-while-revalidate, and the only
  repeated fetch is the 30s readiness poll (already handled by
  `useReadiness`).
- **SSE is not solved by TanStack Query.** Replacing
  `subscribeJobEvents` would require keeping a hand-rolled hook
  anyway, defeating the purpose of the migration.

## When to revisit

- **Two or more panels need to share the same fetched data.** A
  likely near-term trigger: a new "metrics dashboard" panel that
  needs to share the same `/api/v1/metrics/ocr` data with the
  existing reliability panel. Today they are separate fetches with
  their own loading states.
- **Optimistic updates for retries / new-translation-pick.** The
  current code re-fetches after the user picks a history entry.
  TanStack Query's `setQueryData` would make this instant.
- **Offline support.** TanStack Query has a built-in offline cache
  via persisters. If we add a PWA manifest, this becomes a
  meaningful win.

## Tracking

- Subscribe to https://github.com/TanStack/query/releases for
  notable changes (SSE support, React 19 improvements, bundle
  size reductions).
- Re-evaluate after the next major frontend refactor (e.g. when
  the `App.tsx` is split into per-workflow sub-components and a
  router is introduced).
