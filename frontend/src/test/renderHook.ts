// Minimal renderHook helper for hook unit tests without @testing-library.
// Provides jsdom-friendly `act` integration via React's act().

import { act, createElement } from "react"
import { createRoot, type Root } from "react-dom/client"

// React 18+ requires IS_REACT_ACT_ENVIRONMENT to be set for act() to work
// in unit tests. We set it lazily on import.
;(globalThis as Record<string, unknown>).IS_REACT_ACT_ENVIRONMENT = true

export interface RenderHookResult<T> {
  result: { current: T }
  rerender: () => void
  unmount: () => void
}

export function renderHook<T>(callback: () => T): RenderHookResult<T> {
  const container = document.createElement("div")
  document.body.appendChild(container)
  const result: { current: T } = { current: undefined as T }
  const root: Root = createRoot(container)

  function Probe() {
    result.current = callback()
    return null
  }

  act(() => {
    root.render(createElement(Probe))
  })

  return {
    result,
    rerender: () => {
      act(() => {
        root.render(createElement(Probe))
      })
    },
    unmount: () => {
      act(() => {
        root.unmount()
      })
      document.body.removeChild(container)
    },
  }
}

export { act }
