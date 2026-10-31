import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { renderHook, act } from "../test/renderHook"
import { FIRST_VISIT_KEY } from "../constants"
import { MOTION } from "../motion"
import { useDemoShowcase, type UseDemoShowcaseParams } from "./useDemoShowcase"

function demoParams(): UseDemoShowcaseParams {
  return {
    runImageFile: vi.fn(async () => {}),
    handleDemoPreset: vi.fn(async () => {}),
    setWorkflow: vi.fn(),
    setSourceLang: vi.fn(),
    setTargetLang: vi.fn(),
    setCompareView: vi.fn(),
    setSliderTarget: vi.fn(),
    ocrLoading: false,
    pdfLoading: false,
    loading: false,
  }
}

describe("useDemoShowcase", () => {
  beforeEach(() => {
    vi.useFakeTimers()
    localStorage.clear()
    localStorage.setItem(FIRST_VISIT_KEY, "true")
    vi.stubGlobal("fetch", vi.fn(async () => new Response("image", { headers: { "Content-Type": "image/png" } })))
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it("uses the latest callback without restarting the showcase timer", async () => {
    const params = demoParams()
    const initialRun = params.runImageFile
    const hook = renderHook(() => useDemoShowcase(params))
    try {
      act(() => hook.result.current.toggleShowcase())
      await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
      expect(initialRun).toHaveBeenCalledTimes(1)
      const latestRun = vi.fn(async () => {})
      params.runImageFile = latestRun
      hook.rerender()
      await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
      expect(latestRun).not.toHaveBeenCalled()
      await act(async () => { await vi.advanceTimersByTimeAsync(MOTION.imageProgress.successSettleMs + 6000) })
      expect(latestRun).toHaveBeenCalledTimes(1)
    } finally {
      hook.unmount()
    }
  })

  it("does not schedule another demo when stopped during processing", async () => {
    const params = demoParams()
    let finish: () => void = () => {}
    params.runImageFile = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
    const hook = renderHook(() => useDemoShowcase(params))
    try {
      act(() => hook.result.current.toggleShowcase())
      await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
      expect(params.runImageFile).toHaveBeenCalledTimes(1)
      act(() => hook.result.current.toggleShowcase())
      await act(async () => { finish() })
      await act(async () => { await vi.advanceTimersByTimeAsync(20_000) })
      expect(params.runImageFile).toHaveBeenCalledTimes(1)
    } finally {
      hook.unmount()
    }
  })

  it("does not start a demo or change workflow on first visit", async () => {
    localStorage.removeItem(FIRST_VISIT_KEY)
    const params = demoParams()
    const hook = renderHook(() => useDemoShowcase(params))
    try {
      await act(async () => { await vi.advanceTimersByTimeAsync(10_000) })
      expect(fetch).not.toHaveBeenCalled()
      expect(params.setWorkflow).not.toHaveBeenCalled()
      expect(params.runImageFile).not.toHaveBeenCalled()
    } finally {
      hook.unmount()
    }
  })
})