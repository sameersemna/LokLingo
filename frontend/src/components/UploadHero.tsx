import {
  TRUST_SIGNALS,
  OCR_STAGGER_PRESETS,
  type OverlayStage,
  type UploadSelection,
} from "../constants"
import { type TextBlock } from "../api/ocr"

interface UploadHeroProps {
  workflow: string
  uploadSelection: UploadSelection | null
  uploadDragActive: boolean
  ocrLoading: boolean
  pdfLoading: boolean
  ocrStaggerMs: number
  overlayStage: OverlayStage
  showOCROverlay: boolean
  overlayBlocks: TextBlock[]
  overlayTexts: string[]
  overlayLabelSwapActive: boolean
  overlayDemoRunning: boolean
  overlayImageSize: { width: number; height: number } | null
  legendActiveStage: string
  uploadBackendState: string
  uploadBackendHint: string
  uploadBackendLabel: string
  uploadHeroSubtitle: string
  uploadEmptyTitle: string
  uploadEmptySupport: string
  uploadBrowseAccept: string
  onDragEnter: (e: React.DragEvent) => void
  onDragOver: (e: React.DragEvent) => void
  onDragLeave: (e: React.DragEvent) => void
  onUploadDrop: (e: React.DragEvent) => void
  onOverlayImageLoad: (e: React.SyntheticEvent<HTMLImageElement>) => void
  onOpenUploadPicker: (accept: string) => void
  onReplayDemo: () => void
  onStaggerChange: (ms: number) => void
  onStaggerPreset: (ms: number) => void
  formatFileSize: (bytes: number) => string
}

export function UploadHero({
  workflow,
  uploadSelection,
  uploadDragActive,
  ocrLoading,
  pdfLoading,
  ocrStaggerMs,
  overlayStage,
  showOCROverlay,
  overlayBlocks,
  overlayTexts,
  overlayImageSize,
  overlayLabelSwapActive,
  overlayDemoRunning,
  legendActiveStage,
  uploadBackendState,
  uploadBackendHint,
  uploadBackendLabel,
  uploadHeroSubtitle,
  uploadEmptyTitle,
  uploadEmptySupport,
  uploadBrowseAccept,
  onDragEnter,
  onDragOver,
  onDragLeave,
  onUploadDrop,
  onOverlayImageLoad,
  onOpenUploadPicker,
  onReplayDemo,
  onStaggerChange,
  onStaggerPreset,
  formatFileSize,
}: UploadHeroProps) {
  const busy = ocrLoading || pdfLoading

  return (
    <section
      className={`upload-hero${uploadDragActive ? " is-drag-active" : ""}${workflow === "image" ? " is-image-primary" : ""}${workflow === "pdf" ? " is-pdf-focus" : ""}${workflow === "text" ? " is-text-focus" : ""}`}
      onDragEnter={onDragEnter}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onUploadDrop}
      aria-label="Upload image or PDF"
    >
      <div className="upload-hero-head">
        <h2>Visual localization</h2>
        <div className="upload-hero-head-right">
          <span className="upload-hero-pill">Local by default</span>
          <button
            type="button"
            className={`upload-backend-chip upload-backend-${uploadBackendState}`}
            onClick={() => {
              /* showReliability handled by parent toggle */
            }}
            title={uploadBackendHint}
          >
            <span className="upload-backend-dot" aria-hidden="true" />
            <span>{uploadBackendLabel}</span>
          </button>
        </div>
      </div>
      <p className="upload-hero-subtitle">{uploadHeroSubtitle}</p>

      <div className="upload-trust-row" aria-label="Trust indicators">
        {TRUST_SIGNALS.map(signal => (
          <span key={signal} className="upload-trust-pill">{signal}</span>
        ))}
      </div>

      {uploadSelection?.kind === "image" && uploadSelection.previewUrl ? (
        <div className="upload-preview upload-preview-image">
          <div className="upload-preview-image-stage">
            <div className="ocr-pipeline-legend" aria-label="Visual pipeline legend">
              <span className={`ocr-pipeline-step${overlayStage !== "idle" ? " is-done" : ""}${legendActiveStage === "ocr" ? " is-active" : ""}`}>Scan</span>
              <span className={`ocr-pipeline-step${overlayStage === "translated" ? " is-done" : ""}${legendActiveStage === "translation" ? " is-active" : ""}`}>Translate</span>
              <span className={`ocr-pipeline-step${!ocrLoading && overlayStage === "translated" ? " is-done" : ""}${legendActiveStage === "rendering" ? " is-active" : ""}`}>Render</span>
            </div>
            <img
              src={uploadSelection.previewUrl}
              alt="Selected upload preview"
              onLoad={onOverlayImageLoad}
            />
            {showOCROverlay && overlayImageSize && overlayBlocks.length > 0 && overlayStage !== "idle" && (
              <div className={`ocr-visualization-overlay ocr-visualization-${overlayStage}${overlayLabelSwapActive ? " is-switching" : ""}`} aria-hidden="true">
                {overlayBlocks.map((block, idx) => {
                  const [bx1, by1, bx2, by2] = block.bbox
                  const left = (Math.min(bx1, bx2) / overlayImageSize.width) * 100
                  const top = (Math.min(by1, by2) / overlayImageSize.height) * 100
                  const width = (Math.abs(bx2 - bx1) / overlayImageSize.width) * 100
                  const height = (Math.abs(by2 - by1) / overlayImageSize.height) * 100
                  const label = overlayStage === "translated" ? (overlayTexts[idx] || block.text) : block.text
                  return (
                    <div
                      key={`${idx}-${block.bbox.join("-")}`}
                      className={`ocr-visualization-box${overlayStage === "translated" ? " is-translated" : ""}`}
                      style={{ left: `${left}%`, top: `${top}%`, width: `${width}%`, height: `${height}%` }}
                    >
                      <span
                        className={`ocr-visualization-label${overlayStage === "translated" ? " is-translated" : ""}`}
                      >
                        {label}
                      </span>
                    </div>
                  )
                })}
              </div>
            )}
            {showOCROverlay && overlayBlocks.length > 0 && overlayStage !== "idle" && (
              <div className={`ocr-visualization-badge ocr-visualization-badge-${overlayStage}`}>
                {overlayStage === "ocr" ? `Detected ${overlayBlocks.length} regions` : `Translated ${overlayBlocks.length} regions`}
              </div>
            )}
          </div>
          <div className="upload-preview-meta">
            <strong>{uploadSelection.name}</strong>
            <span>{formatFileSize(uploadSelection.size)} · Image</span>
            {overlayStage === "ocr" && (
              <span className="upload-preview-stage-note">Animating scan regions…</span>
            )}
            {overlayStage === "translated" && (
              <span className="upload-preview-stage-note">Regions now show translated text.</span>
            )}
            {!showOCROverlay && overlayStage !== "idle" && (
              <span className="upload-preview-stage-note">Visual overlay hidden.</span>
            )}
          </div>
        </div>
      ) : uploadSelection?.kind === "pdf" ? (
        <div className="upload-preview upload-preview-pdf">
          <div className="upload-preview-file-icon" aria-hidden="true">PDF</div>
          <div className="upload-preview-meta">
            <strong>{uploadSelection.name}</strong>
            <span>{formatFileSize(uploadSelection.size)} · queued for background translation</span>
          </div>
        </div>
      ) : (
        <div className="upload-preview upload-preview-empty-panel">
          <button
            type="button"
            className={`upload-preview upload-preview-empty upload-drop-target${uploadDragActive ? " is-drag-active" : ""}`}
            disabled={busy}
            onClick={() => onOpenUploadPicker(uploadBrowseAccept)}
          >
            <strong>{uploadDragActive ? "Release to upload" : uploadEmptyTitle}</strong>
            <span>{uploadEmptySupport}</span>
          </button>
          <div className="upload-example-grid" aria-label="Example files">
            {/* Demo preset icons rendered by parent DemoGallery */}
          </div>
        </div>
      )}

      <div className="upload-hero-actions">
        <button
          type="button"
          className="upload-primary-btn"
          disabled={busy}
          onClick={() => onOpenUploadPicker("image/*")}
        >
          {ocrLoading ? "Translating image..." : "Upload image"}
        </button>
        <button
          type="button"
          className="icon-btn"
          disabled={busy}
          onClick={() => onOpenUploadPicker("application/pdf")}
        >
          {pdfLoading ? "Uploading PDF..." : "Upload PDF"}
        </button>
        <button
          type="button"
          className="icon-btn"
          onClick={() => {} /* workflow switch handled by parent */}
        >
          Use text input
        </button>
        <button
          type="button"
          className={`icon-btn${showOCROverlay ? " is-active" : ""}`}
          onClick={() => {}}
          aria-pressed={showOCROverlay}
          title="Show or hide visual overlay"
        >
          {showOCROverlay ? "Hide overlay" : "Show overlay"}
        </button>
        <label className="ocr-stagger-control">
          Region stagger
          <input
            type="range"
            min={20}
            max={220}
            step={10}
            value={ocrStaggerMs}
            onChange={event => onStaggerChange(Number(event.target.value))}
          />
          <span>{ocrStaggerMs} ms</span>
        </label>
        <div className="ocr-stagger-presets" role="group" aria-label="Region stagger presets">
          {OCR_STAGGER_PRESETS.map(preset => (
            <button
              key={preset.id}
              type="button"
              className={`icon-btn${ocrStaggerMs === preset.ms ? " is-active" : ""}`}
              onClick={() => onStaggerPreset(preset.ms)}
            >
              {preset.label}
            </button>
          ))}
        </div>
        <button
          type="button"
          className={`icon-btn ocr-demo-btn${overlayDemoRunning ? " is-active" : ""}`}
          onClick={onReplayDemo}
          disabled={busy || overlayStage === "idle" || overlayBlocks.length === 0}
          title="Replay visual pipeline stages on the current preview"
        >
          {overlayDemoRunning ? "Replaying demo..." : "Replay demo"}
        </button>
      </div>
    </section>
  )
}
