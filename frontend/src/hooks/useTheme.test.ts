import { describe, it, expect, beforeEach } from "vitest"
import { renderHook, act } from "../test/renderHook"
import { useTheme } from "./useTheme"

describe("useTheme", () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute("data-theme")
  })

  it("returns light as default when no preference is set and matchMedia reports light", () => {
    const { result } = renderHook(() => useTheme())
    expect(result.current[0]).toBe("light")
  })

  it("persists the theme to localStorage and the data-theme attribute", () => {
    const { result } = renderHook(() => useTheme())
    act(() => result.current[1]("dark"))
    expect(result.current[0]).toBe("dark")
    expect(localStorage.getItem("loklingo-theme")).toBe("dark")
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark")
  })

  it("toggle switches between light and dark", () => {
    const { result } = renderHook(() => useTheme())
    act(() => result.current[2]())
    expect(result.current[0]).toBe("dark")
    act(() => result.current[2]())
    expect(result.current[0]).toBe("light")
  })
})
