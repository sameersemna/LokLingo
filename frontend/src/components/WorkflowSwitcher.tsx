import { memo } from "react"
import { LANGUAGES, TARGET_LANGUAGES, WORKFLOWS, type ProductMode, type InputWorkflow } from "../constants"

interface WorkflowSwitcherProps {
  source: string
  target: string
  detectedLang: string
  mode: ProductMode
  workflow: InputWorkflow
  onSourceChange: (value: string) => void
  onTargetChange: (value: string) => void
  onSwap: () => void
  onModeChange: (value: ProductMode) => void
  onWorkflowChange: (value: InputWorkflow) => void
  langLabel: (code: string) => string
  onFileChange: (file: File | null) => void
}

export const WorkflowSwitcher = memo(function WorkflowSwitcher({
  source,
  target,
  detectedLang,
  mode,
  workflow,
  onSourceChange,
  onTargetChange,
  onSwap,
  onModeChange,
  onWorkflowChange,
  langLabel,
  onFileChange,
}: WorkflowSwitcherProps) {
  return (
    <main className="translator">
      <div className="lang-selectors">
        <div className="lang-select-wrap">
          <select
            aria-label="Source language"
            value={source}
            onChange={e => {
              onSourceChange(e.target.value)
            }}
          >
            {LANGUAGES.map(l => (
              <option key={l.code} value={l.code}>{l.label}</option>
            ))}
          </select>
          {detectedLang && (
            <span className="detected-badge">✓ {langLabel(detectedLang)}</span>
          )}
        </div>

        <button
          className="swap-btn"
          title={source === "auto" ? "Cannot swap when source is Auto-detect" : "Swap languages"}
          disabled={source === "auto"}
          onClick={onSwap}
        >
          ⇄
        </button>

        <select
          aria-label="Target language"
          value={target}
          onChange={e => onTargetChange(e.target.value)}
        >
          {TARGET_LANGUAGES.map(l => (
            <option key={l.code} value={l.code}>{l.label}</option>
          ))}
        </select>
      </div>

      <div className="mode-row">
        <label className="mode-label" htmlFor="translation-mode">Mode</label>
        <select
          id="translation-mode"
          className="mode-select"
          aria-label="Translation mode"
          value={mode}
          onChange={e => onModeChange(e.target.value as ProductMode)}
        >
          {["fast", "studio", "extract"].map(m => (
            <option key={m} value={m}>{m}</option>
          ))}
        </select>
      </div>

      <p className="mode-help" role="note" aria-live="polite">
        {mode === "extract"
          ? "OCR text extraction only — no translation"
          : mode === "studio"
            ? "Premium layout fidelity with dual rendering"
            : "Quick visual draft — fast translation overlay"}
      </p>

      <input
        type="file"
        accept="image/*,application/pdf"
        className="ocr-input"
        id="ocr-file"
        onChange={e => onFileChange(e.target.files?.[0] ?? null)}
      />

      <section className="workflow-switch-wrap" aria-label="Input workflow">
        <div className="workflow-switch-heading">Choose workflow: Image, PDF, or Text</div>
        <div className="workflow-switch">
          {WORKFLOWS.map(item => (
            <button
              key={item.value}
              type="button"
              className={`workflow-card${workflow === item.value ? " is-active" : ""}${item.value === "image" ? " is-primary" : ""}`}
              onClick={() => onWorkflowChange(item.value)}
              aria-pressed={workflow === item.value}
            >
              <span className="workflow-card-top">
                <span className="workflow-card-icon" aria-hidden="true">{item.icon}</span>
                <span className="workflow-card-title">{item.label}</span>
                {item.value === "image" && <span className="workflow-card-badge">Primary</span>}
              </span>
              <span className="workflow-card-detail">{item.detail}</span>
            </button>
          ))}
        </div>
      </section>
    </main>
  )
})
