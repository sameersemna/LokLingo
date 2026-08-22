import { memo } from "react"

interface IntelligencePanelProps {
  regionCount: number
  languageCount: number
  languageCodes: string[]
  verticalCount: number
  rtlCount: number
  confidence: number
  status: "idle" | "running" | "done" | "error"
  langLabel: (code: string) => string
}

export const IntelligencePanel = memo(function IntelligencePanel({
  regionCount,
  languageCount,
  languageCodes,
  verticalCount,
  rtlCount,
  confidence,
  status,
  langLabel,
}: IntelligencePanelProps) {
  return (
    <section className="intelligence-panel" aria-live="polite">
      <div className="intelligence-header">
        <strong>Live intelligence</strong>
        <span>{status === "running" ? "Analyzing in real time" : "Analysis complete"}</span>
      </div>
      <div className="intelligence-grid">
        <article className="intelligence-card">
          <span className="intelligence-label">Text regions detected</span>
          <strong>{regionCount}</strong>
        </article>
        <article className="intelligence-card">
          <span className="intelligence-label">Languages</span>
          <strong>{languageCount}</strong>
          <div className="intelligence-tags">
            {languageCodes.length === 0 ? (
              <span className="intelligence-tag">Waiting…</span>
            ) : (
              languageCodes.map(code => (
                <span key={code} className="intelligence-tag">{langLabel(code)}</span>
              ))
            )}
          </div>
        </article>
        <article className="intelligence-card">
          <span className="intelligence-label">Vertical typography</span>
          <strong>{verticalCount ? "Detected" : "None"}</strong>
        </article>
        <article className="intelligence-card">
          <span className="intelligence-label">RTL text</span>
          <strong>{rtlCount ? "Detected" : "None"}</strong>
        </article>
        <article className="intelligence-card">
          <span className="intelligence-label">Confidence</span>
          <strong>{confidence}%</strong>
        </article>
      </div>
    </section>
  )
})
