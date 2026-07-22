import { DEMO_PRESETS } from "../constants"

interface DemoGalleryProps {
  visible: boolean
  onClose: () => void
  onSetWorkflow: (w: "image" | "pdf" | "text") => void
  onLoadDemo: (preset: typeof DEMO_PRESETS[number]) => void
}

export function DemoGallery({ visible, onClose, onSetWorkflow, onLoadDemo }: DemoGalleryProps) {
  if (!visible) return null

  return (
    <section className="demo-gallery-panel" aria-label="Demo gallery - try examples">
      <div className="demo-gallery-header">
        <h2>Try Examples</h2>
        <button type="button" className="icon-btn" onClick={onClose} aria-label="Close gallery">
          ✕
        </button>
      </div>
      <div className="demo-gallery-grid">
        {DEMO_PRESETS.map(preset => (
          <button
            key={preset.id}
            className="demo-card"
            onClick={() => {
              onSetWorkflow("image")
              onLoadDemo(preset)
              onClose()
            }}
            title={`Load demo: ${preset.label}`}
          >
            <div className="demo-card-emoji">{preset.emoji}</div>
            <div className="demo-card-label">{preset.label}</div>
            <div className="demo-card-category">{preset.category}</div>
            <div className="demo-card-image-wrapper">
              <img src={preset.src} alt={preset.label} loading="lazy" className="demo-card-image" />
            </div>
          </button>
        ))}
      </div>
    </section>
  )
}
