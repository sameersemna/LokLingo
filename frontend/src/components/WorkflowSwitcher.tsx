import { memo } from "react"
import { Image, FileText, Text, ArrowLeftRight, Check } from "lucide-react"
import { LANGUAGES, TARGET_LANGUAGES, WORKFLOWS, PRODUCT_MODES, type ProductMode, type InputWorkflow } from "../constants"

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
    <section className="translator workspace-controls" aria-label="Translation controls">
      <div className="workspace-heading">
        <div><span className="workspace-eyebrow">TRANSLATE</span><h2>{workflow === "image" ? "Image translation" : workflow === "pdf" ? "Document translation" : "Text translation"}</h2></div>
        <div className="workflow-tabs" role="group" aria-label="Input workflow">
          {WORKFLOWS.map(item => {
            const WorkflowIcon = item.value === "image" ? Image : item.value === "pdf" ? FileText : Text
            return <button key={item.value} type="button" className={`workflow-card${workflow === item.value ? " is-active" : ""}`} onClick={() => onWorkflowChange(item.value)} aria-pressed={workflow === item.value}><WorkflowIcon size={17} aria-hidden="true" /><span>{item.label}</span></button>
          })}
        </div>
      </div>
      <div className="lang-selectors">
        <div className="lang-select-wrap">
          <span className="field-caption">Source language</span>
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
            <span className="detected-badge"><Check size={13} aria-hidden="true" /> {langLabel(detectedLang)}</span>
          )}
        </div>

        <button
          className="swap-btn"
          title={source === "auto" ? "Cannot swap when source is Auto-detect" : "Swap languages"}
          disabled={source === "auto"}
          onClick={onSwap}
        >
          <ArrowLeftRight size={18} aria-hidden="true" />
        </button>

        <div className="lang-select-wrap">
        <span className="field-caption">Target language</span>
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
      </div>

      {workflow !== "text" && <div className="mode-row" role="group" aria-label="Translation mode">
        <span className="field-caption">Processing mode</span>
        <div className="mode-segments">
          {PRODUCT_MODES.filter(item => workflow === "image" || item.value !== "studio").map(item => <button key={item.value} type="button" className={`mode-option${mode === item.value ? " is-active" : ""}`} onClick={() => onModeChange(item.value)} aria-pressed={mode === item.value}>{item.value === "extract" ? "Extract" : item.label}</button>)}
        </div>
      </div>}

      <input
        type="file"
        accept="image/*,application/pdf"
        className="ocr-input"
        id="ocr-file"
        onChange={e => onFileChange(e.target.files?.[0] ?? null)}
      />

    </section>
  )
})
