import { describe, it, expect, beforeEach } from "vitest"
import { renderHook, act } from "../test/renderHook"
import { useComparisonModal } from "./useComparisonModal"

describe("useComparisonModal", () => {
  beforeEach(() => {
    document.body.style.overflow = ""
  })

  it("opens and closes the modal", () => {
    const { result } = renderHook(() => useComparisonModal())
    act(() => result.current.open("overlay"))
    expect(result.current.state.open).toBe(true)
    expect(result.current.state.focus).toBe("overlay")
    act(() => result.current.close())
    expect(result.current.state.open).toBe(false)
  })

  it("clamps slider percent between 0 and 100", () => {
    const { result } = renderHook(() => useComparisonModal())
    act(() => result.current.setSliderPercent(150))
    expect(result.current.state.sliderPercent).toBe(100)
    act(() => result.current.setSliderPercent(-10))
    expect(result.current.state.sliderPercent).toBe(0)
  })

  it("clamps zoom between 0.6 and 3", () => {
    const { result } = renderHook(() => useComparisonModal())
    act(() => result.current.adjustZoom(10))
    expect(result.current.state.zoom).toBe(3)
    act(() => result.current.adjustZoom(-100))
    expect(result.current.state.zoom).toBe(0.6)
  })

  it("resets pan and panning when zoom returns to <= 1", () => {
    const { result } = renderHook(() => useComparisonModal())
    act(() => result.current.setPan({ x: 30, y: 30 }))
    act(() => result.current.setPanning(true))
    act(() => result.current.adjustZoom(-100))
    expect(result.current.state.pan).toEqual({ x: 0, y: 0 })
    expect(result.current.state.panning).toBe(false)
  })

  it("locks body scroll while modal is open and restores on close", () => {
    const { result } = renderHook(() => useComparisonModal())
    const before = document.body.style.overflow
    act(() => result.current.open("layout"))
    expect(document.body.style.overflow).toBe("hidden")
    act(() => result.current.close())
    expect(document.body.style.overflow).toBe(before)
  })
})
