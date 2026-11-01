import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { act, renderHook } from "../test/renderHook"
import { useImageProgress, type UseImageProgressParams } from "./useImageProgress"

function progressParams(): UseImageProgressParams {
  return {
    ocrLoading: true, overlayBlocksCount: 4,
    compareOriginalUrl: "/original.png", compareOverlayUrl: "/fast.png", compareLayoutUrl: "/studio.png", resultImageUrl: "/studio.png",
    setModalFocus: vi.fn(), setModalSliderTarget: vi.fn(), setModalFlashTarget: vi.fn(),
    setModalSliderPercent: vi.fn(), setModalView: vi.fn(), setComparisonQuickToggle: vi.fn(),
    resetComparisonModalZoom: vi.fn(), openModal: vi.fn(),
  }
}

describe("backend-driven image progress", () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it("does not invent stages, recovery hints or progress from elapsed time", async () => {
    const hook = renderHook(() => useImageProgress(progressParams()))
    try {
      act(() => hook.result.current.startImageProgress())
      await act(async () => { await vi.advanceTimersByTimeAsync(20_000) })
      expect(hook.result.current.imageProgressStage).toBe("detecting")
      expect(hook.result.current.imageProgressCompleted).toEqual([])
      expect(hook.result.current.liveChunkProgress).toBeNull()
      expect(hook.result.current.translationRegionIndex).toBe(0)
      expect(hook.result.current.reliabilityHint).toBeNull()
    } finally { hook.unmount() }
  })

  it("retains measured backend progress without a fictional region ticker", async () => {
    const hook = renderHook(() => useImageProgress(progressParams()))
    try {
      act(() => hook.result.current.startImageProgress())
      act(() => hook.result.current.handleImageJobProgress({ job_id: "test", status: "processing", stage: "translating", stage_progress: 0.25 }))
      await act(async () => { await vi.advanceTimersByTimeAsync(20_000) })
      expect(hook.result.current.liveChunkProgress).toBe(0.25)
      expect(hook.result.current.translationRegionIndex).toBe(0)
    } finally { hook.unmount() }
  })

  it("does not open an inspector automatically or dismiss an error on a timer", async () => {
    const params = progressParams()
    const hook = renderHook(() => useImageProgress(params))
    try {
      act(() => hook.result.current.completeImageProgress())
      await act(async () => { await vi.advanceTimersByTimeAsync(20_000) })
      expect(params.openModal).not.toHaveBeenCalled()
      act(() => hook.result.current.failImageProgress())
      await act(async () => { await vi.advanceTimersByTimeAsync(20_000) })
      expect(hook.result.current.imageProgressStatus).toBe("error")
      expect(hook.result.current.livePhaseLabel).toBe("Processing interrupted")
    } finally { hook.unmount() }
  })
})