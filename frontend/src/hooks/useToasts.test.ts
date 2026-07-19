import { describe, it, expect, beforeEach, vi } from "vitest"
import { renderHook, act } from "../test/renderHook"
import { useToasts } from "./useToasts"

describe("useToasts", () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  it("pushes toasts and removes them after the timeout", () => {
    const { result } = renderHook(() => useToasts())
    act(() => result.current.push("hi", "success"))
    expect(result.current.toasts).toHaveLength(1)
    expect(result.current.toasts[0].msg).toBe("hi")

    act(() => {
      vi.advanceTimersByTime(3500)
    })
    expect(result.current.toasts).toHaveLength(0)
  })

  it("assigns incrementing ids", () => {
    const { result } = renderHook(() => useToasts())
    act(() => result.current.push("a", "success"))
    act(() => result.current.push("b", "error"))
    const ids = result.current.toasts.map(t => t.id)
    expect(new Set(ids).size).toBe(2)
  })

  it("dismiss removes a specific toast", () => {
    const { result } = renderHook(() => useToasts())
    act(() => result.current.push("hello", "error"))
    const id = result.current.toasts[0].id
    act(() => result.current.dismiss(id))
    expect(result.current.toasts).toHaveLength(0)
  })
})
