import { Component, type ErrorInfo, type ReactNode, StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'

interface BoundaryState {
  error: Error | null
}

interface BoundaryProps {
  children: ReactNode
}

/**
 * Top-level React error boundary. Per the React 19 docs, a single
 * uncaught error in any descendant component (render, lifecycle,
 * constructor) would otherwise blank the entire SPA. The boundary
 * catches it, logs it, and offers the user a way to recover without
 * losing the page shell.
 */
class ErrorBoundary extends Component<BoundaryProps, BoundaryState> {
  state: BoundaryState = { error: null }

  static getDerivedStateFromError(error: Error): BoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // eslint-disable-next-line no-console
    console.error('LokLingo: uncaught error in component tree', error, info)
  }

  private handleReset = (): void => {
    this.setState({ error: null })
  }

  private handleReload = (): void => {
    window.location.reload()
  }

  render(): ReactNode {
    if (this.state.error) {
      return (
        <div role="alert" className="error-boundary">
          <h1>Something went wrong</h1>
          <p className="error-boundary-message">
            The translation UI hit an unexpected error. Your in-progress work
            may not be recoverable, but you can try again without reloading
            the page, or reload to start fresh.
          </p>
          {import.meta.env.DEV && this.state.error.message ? (
            <pre className="error-boundary-stack">{this.state.error.message}</pre>
          ) : null}
          <div className="error-boundary-actions">
            <button type="button" className="btn" onClick={this.handleReset}>
              Try again
            </button>
            <button type="button" className="btn btn-secondary" onClick={this.handleReload}>
              Reload page
            </button>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </StrictMode>,
)
