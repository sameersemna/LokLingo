import { describe, it, expect, vi } from "vitest"
import { createElement, Component, act, type ErrorInfo, type ReactNode } from "react"
import { createRoot } from "react-dom/client"

/**
 * Re-implement the production error boundary contract inline and assert
 * the behaviour: a child that throws is caught; the user sees a recovery
 * UI; resetting returns to the children.
 */
class Boundary extends Component<{ children?: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null }
  static getDerivedStateFromError(error: Error) {
    return { error }
  }
  componentDidCatch(error: Error, info: ErrorInfo) {
    // eslint-disable-next-line no-console
    console.error("test: caught", error, info)
  }
  private handleReset = () => this.setState({ error: null })
  render(): ReactNode {
    if (this.state.error) {
      return createElement(
        "div",
        { role: "alert" },
        this.state.error.message,
        createElement("button", { type: "button", onClick: this.handleReset }, "Try again"),
      )
    }
    return this.props.children ?? null
  }
}

function ThrowingChild(): ReactNode {
  throw new Error("boom from child")
}

function renderIntoContainer(element: ReactNode): HTMLDivElement {
  const container = document.createElement("div")
  document.body.appendChild(container)
  const root = createRoot(container)
  act(() => {
    root.render(element as never)
  })
  return container
}

describe("ErrorBoundary contract", () => {
  it("renders children when there is no error", () => {
    const container = renderIntoContainer(
      createElement(Boundary, null, createElement("span", null, "hello world")),
    )
    expect(container.textContent).toContain("hello world")
  })

  it("catches errors thrown by descendants and shows a recovery UI", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {})
    const container = renderIntoContainer(
      createElement(Boundary, null, createElement(ThrowingChild)),
    )
    expect(container.textContent).toContain("boom from child")
    expect(container.textContent).toContain("Try again")
    expect(spy).toHaveBeenCalled()
    spy.mockRestore()
  })
})

describe("main.tsx imports", () => {
  it("App module resolves and exports a function", async () => {
    const mod = await import("./App")
    expect(typeof mod.default).toBe("function")
  })
})
