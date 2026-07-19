import { describe, it, expect, beforeEach } from "vitest"
import { renderHook, act } from "../test/renderHook"
import { useLanguageSelection } from "./useLanguageSelection"

describe("useLanguageSelection", () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it("defaults to auto/en→de on first use", () => {
    const { result } = renderHook(() => useLanguageSelection())
    expect(result.current.source).toBe("auto")
    expect(result.current.target).toBe("de")
  })

  it("persists language changes", () => {
    const { result } = renderHook(() => useLanguageSelection())
    act(() => result.current.setSource("fr"))
    act(() => result.current.setTarget("ja"))
    expect(JSON.parse(localStorage.getItem("loklingo-langs") ?? "{}")).toEqual({
      source: "fr",
      target: "ja",
    })
  })

  it("swap refuses to swap when source is auto", () => {
    const { result } = renderHook(() => useLanguageSelection())
    const ok = result.current.swap()
    expect(ok).toBe(false)
  })

  it("swap exchanges source and target when source is concrete", () => {
    const { result } = renderHook(() => useLanguageSelection())
    act(() => result.current.setSource("fr"))
    act(() => result.current.setTarget("ja"))
    act(() => result.current.swap())
    expect(result.current.source).toBe("ja")
    expect(result.current.target).toBe("fr")
  })
})
