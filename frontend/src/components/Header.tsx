import { type DependencyStatus } from "../api/health"
import { type OCRMetricsResponse } from "../api/metrics"

interface HeaderProps {
  theme: string
  onToggleTheme: () => void
  showPdfJobs: boolean
  onTogglePdfJobs: () => void
  showReliability: boolean
  onToggleReliability: () => void
  showDeadOps: boolean
  onToggleDeadOps: () => void
  showHistory: boolean
  onToggleHistory: () => void
  showDemoGallery: boolean
  onToggleDemoGallery: () => void
  showcaseMode: boolean
  onToggleShowcase: () => void
  historyCount: number
  chipRef: React.Ref<HTMLDivElement>
  setShowStatusDetail: (open: boolean) => void
  showStatusDetail: boolean
  readinessLabel: string
  readinessState: string
  readiness: { dependencies: Record<string, DependencyStatus> } | null
  readinessMetricsLoading: boolean
  readinessMetricsError: string | null
  readinessMetrics: OCRMetricsResponse | null
  readinessPressureTrend: "up" | "down" | "flat"
  readinessPressureDelta: number
  readinessPressureLabel: string
  readinessPressureLevel: "normal" | "warn" | "critical"
  miniSparklinePoints: string | null
  onOpenReliability: () => void
}

export function Header({
  theme,
  onToggleTheme,
  showPdfJobs,
  onTogglePdfJobs,
  showReliability,
  onToggleReliability,
  showDeadOps,
  onToggleDeadOps,
  showHistory,
  onToggleHistory,
  showDemoGallery,
  onToggleDemoGallery,
  showcaseMode,
  onToggleShowcase,
  historyCount,
  chipRef,
  setShowStatusDetail,
  showStatusDetail,
  readinessLabel,
  readinessState,
  readiness,
  readinessMetricsLoading,
  readinessMetricsError,
  readinessMetrics,
  readinessPressureTrend,
  readinessPressureDelta,
  readinessPressureLabel,
  readinessPressureLevel,
  miniSparklinePoints,
  onOpenReliability,
}: HeaderProps) {
  return (
    <header className="app-header">
      <div className="header-row">
        <h1>LokLingo</h1>
        <div className="header-actions">
          <button
            className={`icon-btn${showPdfJobs ? " is-active" : ""}`}
            type="button"
            onClick={onTogglePdfJobs}
            aria-pressed={showPdfJobs}
            title="PDF translation jobs"
          >
            📄 PDF Jobs
          </button>
          <button
            className={`icon-btn${showReliability ? " is-active" : ""}`}
            type="button"
            onClick={onToggleReliability}
            aria-pressed={showReliability}
            title="Reliability telemetry"
          >
            📊 Reliability
          </button>
          <button
            className={`icon-btn${showDeadOps ? " is-active" : ""}`}
            type="button"
            onClick={onToggleDeadOps}
            aria-pressed={showDeadOps}
            title="Dead-letter operations"
          >
            🛠 Dead Ops
          </button>
          <button
            className={`icon-btn${showHistory ? " is-active" : ""}`}
            type="button"
            onClick={onToggleHistory}
            aria-pressed={showHistory}
            title="Translation history"
          >
            ⏱ History {historyCount > 0 && <span className="badge">{historyCount}</span>}
          </button>
          <button
            className={`icon-btn${showDemoGallery ? " is-active" : ""}`}
            type="button"
            onClick={onToggleDemoGallery}
            aria-pressed={showDemoGallery}
            title="Try demo examples"
          >
            🎨 Demos
          </button>
          <button
            className="icon-btn"
            type="button"
            onClick={onToggleShowcase}
            aria-pressed={showcaseMode}
            title="Auto-play demo showcase"
          >
            ▶ Showcase
          </button>
          <button
            className="theme-btn"
            type="button"
            aria-label="Toggle color scheme"
            aria-pressed={theme === "dark"}
            onClick={onToggleTheme}
          >
            {theme === "dark" ? "☀ Light" : "☾ Dark"}
          </button>
        </div>
      </div>
      <p className="tagline">Self-hosted translation &mdash; no limits</p>
      <div className="readiness-chip-wrap" ref={chipRef}>
        <button
          type="button"
          className={`readiness-chip readiness-${readinessState}${showStatusDetail ? " is-open" : ""}`}
          onClick={() => setShowStatusDetail(!showStatusDetail)}
          aria-expanded={showStatusDetail}
          aria-haspopup="true"
        >
          {readinessLabel} <span className="chip-caret">{showStatusDetail ? "▲" : "▼"}</span>
        </button>
        {showStatusDetail && (
          <div className="readiness-popover" role="status">
            <div className="readiness-popover-title">Dependency status</div>
            {readiness === null ? (
              <p className="readiness-popover-checking">Fetching…</p>
            ) : (
              <>
                <ul className="readiness-dep-list">
                  {Object.entries(readiness.dependencies)
                    .sort(([a], [b]) => a.localeCompare(b))
                    .map(([name, dep]) => (
                      <li key={name} className={`readiness-dep-item dep-${dep.status}`}>
                        <span className="dep-dot" />
                        <span className="dep-name">{name}</span>
                        {dep.status === "error" && dep.error && (
                          <span className="dep-error" title={dep.error}>
                            {dep.error.length > 60 ? dep.error.slice(0, 57) + "…" : dep.error}
                          </span>
                        )}
                      </li>
                    ))}
                </ul>

                <div className="readiness-mini-reliability">
                  <div className="mini-rel-header">
                    <div className="mini-rel-title">Reliability (1h)</div>
                    {readinessMetrics && (
                      <span
                        className={`mini-rel-trend mini-rel-trend-${readinessPressureTrend} mini-rel-level-${readinessPressureLevel}`}
                      >
                        {readinessPressureTrend === "up"
                          ? "▲"
                          : readinessPressureTrend === "down"
                            ? "▼"
                            : "■"}
                        {readinessPressureDelta === 0
                          ? "0"
                          : readinessPressureDelta > 0
                            ? `+${readinessPressureDelta}`
                            : `${readinessPressureDelta}`}{" "}
                        {readinessPressureLabel}
                      </span>
                    )}
                  </div>
                  {readinessMetricsLoading && <p className="mini-rel-state">Loading...</p>}
                  {readinessMetricsError && (
                    <p className="mini-rel-state mini-rel-state-error">{readinessMetricsError}</p>
                  )}
                  {readinessMetrics && !readinessMetricsLoading && !readinessMetricsError && (
                    <>
                      {miniSparklinePoints && (
                        <div
                          className={`mini-rel-sparkline-wrap mini-rel-level-${readinessPressureLevel}`}
                          aria-hidden="true"
                        >
                          <svg
                            className={`mini-rel-sparkline mini-rel-level-${readinessPressureLevel}`}
                            viewBox="0 0 120 28"
                            preserveAspectRatio="none"
                          >
                            <polyline points={miniSparklinePoints} />
                          </svg>
                        </div>
                      )}
                      <div className="mini-rel-grid">
                        <div className="mini-rel-item">
                          <span>LiteLLM retries</span>
                          <strong>{readinessMetrics.reliability.litellm.retry_attempts_total}</strong>
                        </div>
                        <div className="mini-rel-item">
                          <span>LiteLLM circuit opens</span>
                          <strong>{readinessMetrics.reliability.litellm.circuit_opened_total}</strong>
                        </div>
                        <div className="mini-rel-item">
                          <span>OCR retries</span>
                          <strong>{readinessMetrics.reliability.ocr.retry_attempts_total}</strong>
                        </div>
                        <div className="mini-rel-item">
                          <span>OCR response rejects</span>
                          <strong>{readinessMetrics.reliability.ocr.response_rejected_total}</strong>
                        </div>
                      </div>
                      <button
                        type="button"
                        className="mini-rel-open-btn"
                        onClick={() => {
                          onOpenReliability()
                          setShowStatusDetail(false)
                        }}
                      >
                        Open full Reliability panel
                      </button>
                    </>
                  )}
                </div>
              </>
            )}
          </div>
        )}
      </div>
    </header>
  )
}
