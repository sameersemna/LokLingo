import { type ProductMode, DEMO_PRESETS } from "../constants"

interface ComparisonViewProps {
  ocrLoading: boolean
  imageProgressStage: string
  mode: ProductMode
  compareOriginalUrl: string | null
  compareOverlayUrl: string | null
  compareLayoutUrl: string | null
  compareRevealActive: boolean
  compareView: "side" | "slider" | "flash"
  sliderTarget: "overlay" | "layout"
  sliderPercent: number
  demoModeActive: boolean
  loading: boolean
  modalImageSrc: string | null
  modalTransform: string
  hasModalComparison: boolean
  comparisonModalOpen: boolean
  comparisonModalView: string
  comparisonModalFocus: string
  comparisonModalZoom: number
  comparisonModalPanning: boolean
  comparisonModalFlashToggle: boolean
  comparisonModalFlashTarget: "overlay" | "layout"
  comparisonModalSliderTarget: "overlay" | "layout"
  comparisonModalSliderPercent: number
  comparisonModalQuickToggle: boolean
  onDownload: (url: string, label: string) => void
  onOpenSliderModal: (focus: string) => void
  onSetCompareView: (view: "side" | "slider") => void
  onSetSliderTarget: (target: "overlay" | "layout") => void
  onSetSliderPercent: (percent: number) => void
  onOpenModal: (focus: string) => void
  onSetModalView: (view: string) => void
  onSetModalFocus: (focus: string) => void
  onAdjustZoom: (delta: number) => void
  onResetZoom: () => void
  onSetModalFlashToggle: () => void
  onSetModalSliderTarget: (target: string) => void
  onSetModalSliderPercent: (percent: number) => void
  onSetComparisonQuickToggle: (v: boolean) => void
  onSetCompareSectionRef: (el: HTMLElement | null) => void
  onWheelZoom: (e: React.WheelEvent<HTMLDivElement>) => void
  onPointerDown: (e: React.PointerEvent<HTMLDivElement>) => void
  onPointerMove: (e: React.PointerEvent<HTMLDivElement>) => void
  onPointerUp: (e: React.PointerEvent<HTMLDivElement>) => void
  onCloseModal: () => void
  onToggleDemoMode: () => void
  onOpenDemoPreset: (preset: typeof DEMO_PRESETS[number]) => void
  sectionRef: React.RefObject<HTMLElement | null>
  sliderDraggingRef: React.MutableRefObject<boolean>
}

function spotLightText(stage: string, mode: string, ocrLoading: boolean): string {
  if (!ocrLoading) return ""
  if (stage === "detecting") return "Detecting text regions..."
  if (stage === "layout") return "Understanding layout and reading order..."
  if (stage === "languages") return "Detecting language families and script direction..."
  if (stage === "translating") return "Translating content..."
  if (stage === "typography") return "Rebuilding typography and styling..."
  if (stage === "rendering") return "Rendering final image..."
  if (mode === "extract") return "Extracting text..."
  return "Preparing visual comparison — overlay and layout are processing..."
}

export function ComparisonView({
  ocrLoading,
  imageProgressStage,
  mode,
  compareOriginalUrl,
  compareOverlayUrl,
  compareLayoutUrl,
  compareRevealActive,
  compareView,
  sliderTarget,
  sliderPercent,
  demoModeActive,
  loading,
  modalImageSrc,
  modalTransform,
  hasModalComparison,
  comparisonModalOpen,
  comparisonModalView,
  comparisonModalFocus,
  comparisonModalZoom,
  comparisonModalPanning,
  comparisonModalFlashToggle,
  comparisonModalFlashTarget,
  comparisonModalSliderTarget,
  comparisonModalSliderPercent,
  comparisonModalQuickToggle,
  onDownload,
  onOpenSliderModal,
  onSetCompareView,
  onSetSliderTarget,
  onSetSliderPercent,
  onOpenModal,
  onSetModalView,
  onSetModalFocus,
  onAdjustZoom,
  onResetZoom,
  onSetModalFlashToggle,
  onSetModalSliderTarget,
  onSetModalSliderPercent,
  onSetComparisonQuickToggle,
  onSetCompareSectionRef,
  onWheelZoom,
  onPointerDown,
  onPointerMove,
  onPointerUp,
  onCloseModal,
  onToggleDemoMode,
  onOpenDemoPreset,
  sliderDraggingRef,
}: ComparisonViewProps) {
  const inProgress = ocrLoading && compareOriginalUrl
  const resultsReady = !ocrLoading && compareOriginalUrl && compareOverlayUrl && compareLayoutUrl

  return (
    <>
      {/* Comparison spotlight */}
      {inProgress && (
        <div className="comparison-loading comparison-loading-spotlight">
          <span className="spinner" aria-hidden="true" />
          {spotLightText(imageProgressStage, mode, ocrLoading)}
        </div>
      )}

      {resultsReady && (
        <section
          ref={onSetCompareSectionRef}
          className={`comparison-wrap comparison-wrap--full comparison-hero${compareRevealActive ? " comparison-reveal" : ""}`}
          aria-label="Image comparison"
        >
          <div className="comparison-intro">
            <div className="comparison-kicker">Comparison first</div>
            <h3>Original, Fast, Studio</h3>
            <p>See the source, the quick pass, and the premium layout result side by side.</p>
          </div>
          <div className="comparison-controls">
            <div className="comparison-segment" role="group" aria-label="Comparison layout">
              <button
                type="button"
                className={`icon-btn${compareView === "side" ? " is-active" : ""}`}
                onClick={() => onSetCompareView("side")}
              >
                Grid
              </button>
              <button
                type="button"
                className={`icon-btn${compareView === "slider" ? " is-active" : ""}`}
                onClick={() => onSetCompareView("slider")}
              >
                Slider
              </button>
              <button
                type="button"
                className="icon-btn"
                onClick={() => onOpenSliderModal("layout")}
              >
                Fullscreen
              </button>
            </div>
            {compareView === "slider" && (
              <div className="comparison-segment" role="group" aria-label="Slider target">
                <button
                  type="button"
                  className={`icon-btn${sliderTarget === "overlay" ? " is-active" : ""}`}
                  onClick={() => onSetSliderTarget("overlay")}
                >
                  Fast
                </button>
                <button
                  type="button"
                  className={`icon-btn${sliderTarget === "layout" ? " is-active" : ""}`}
                  onClick={() => onSetSliderTarget("layout")}
                >
                  Studio
                </button>
              </div>
            )}
          </div>

          {compareView === "side" ? (
            <div className="comparison-grid">
              <figure className="compare-card">
                <figcaption>Original</figcaption>
                <img
                  src={compareOriginalUrl ?? ""}
                  alt="Original image"
                  loading="lazy"
                  className="compare-clickable"
                  onClick={() => onOpenModal("original")}
                />
              </figure>
              <figure className="compare-card">
                <div className="compare-card-header">
                  <figcaption>Fast</figcaption>
                  <button
                    type="button"
                    className="icon-btn compare-dl-btn"
                    title="Download fast result"
                    onClick={() => compareOverlayUrl && onDownload(compareOverlayUrl, "overlay-translation")}
                  >
                    ⬇ Download
                  </button>
                </div>
                <img
                  src={compareOverlayUrl ?? ""}
                  alt="Fast translation result"
                  loading="lazy"
                  className="compare-clickable"
                  onClick={() => onOpenModal("overlay")}
                />
              </figure>
              <figure className="compare-card">
                <div className="compare-card-header">
                  <figcaption>Studio</figcaption>
                  <button
                    type="button"
                    className="icon-btn compare-dl-btn"
                    title="Download studio result"
                    onClick={() => compareLayoutUrl && onDownload(compareLayoutUrl, "layout-translation")}
                  >
                    ⬇ Download
                  </button>
                </div>
                <img
                  src={compareLayoutUrl ?? ""}
                  alt="Studio translation result"
                  loading="lazy"
                  className="compare-clickable"
                  onClick={() => onOpenModal("layout")}
                />
              </figure>
            </div>
          ) : (
            <div className={`compare-slider-wrap${compareView === "flash" ? " compare-slider-wrap--compact" : ""}`}>
              <div
                className="compare-slider-frame"
                aria-live="polite"
                onMouseMove={(e) => {
                  if (!sliderDraggingRef.current) return
                  const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
                  const x = e.clientX - rect.left
                  const percent = Math.max(0, Math.min(100, (x / rect.width) * 100))
                  onSetSliderPercent(percent)
                }}
                onMouseDown={(e) => {
                  if (e.button !== 0) return
                  sliderDraggingRef.current = true
                  const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
                  const x = e.clientX - rect.left
                  onSetSliderPercent(Math.max(0, Math.min(100, (x / rect.width) * 100)))
                }}
                onMouseUp={() => {
                  sliderDraggingRef.current = false
                }}
                onMouseLeave={() => {
                  sliderDraggingRef.current = false
                }}
              >
                <img
                  className="compare-slider-base"
                  src={compareOriginalUrl ?? ""}
                  alt="Original image"
                  loading="lazy"
                />
                <div
                  className="compare-slider-overlay"
                  style={{ width: `${sliderPercent}%` }}
                  aria-hidden="true"
                >
                  <img
                    src={compareView === "flash" ? (compareOriginalUrl ?? "") : (sliderTarget === "overlay" ? (compareOverlayUrl ?? "") : (compareLayoutUrl ?? ""))}
                    alt=""
                    loading="lazy"
                  />
                </div>
                <div className="compare-slider-handle" style={{ left: `${sliderPercent}%` }} aria-hidden="true" />
              </div>
              {compareView === "slider" && (
                <>
                  <label className="compare-slider-label">
                    Original vs {sliderTarget === "overlay" ? "Fast" : "Studio"} — drag to reveal
                    <input
                      type="range"
                      min={0}
                      max={100}
                      value={sliderPercent}
                      onChange={e => onSetSliderPercent(Number(e.target.value))}
                    />
                  </label>
                  <div className="compare-dl-row">
                    <button
                      type="button"
                      className="icon-btn"
                      onClick={() => compareOverlayUrl && onDownload(compareOverlayUrl, "overlay-translation")}
                    >
                      ⬇ Download Fast
                    </button>
                    <button
                      type="button"
                      className="icon-btn"
                      onClick={() => compareLayoutUrl && onDownload(compareLayoutUrl, "layout-translation")}
                    >
                      ⬇ Download Studio
                    </button>
                  </div>
                </>
              )}
            </div>
          )}
        </section>
      )}

      {/* Comparison modal */}
      {comparisonModalOpen && modalImageSrc && (
        <div
          className="comparison-modal"
          role="dialog"
          aria-modal="true"
          aria-label="Translation comparison viewer"
          onClick={onCloseModal}
        >
          <div className="comparison-modal-card" onClick={e => e.stopPropagation()}>
            <div className="comparison-modal-header">
              <strong>Translation Quality Viewer</strong>
              <div className="comparison-modal-header-actions">
                <button type="button" className="icon-btn" onClick={() => onAdjustZoom(-0.2)} title="Zoom out">
                  −
                </button>
                <span className="comparison-modal-zoom-label">{Math.round(comparisonModalZoom * 100)}%</span>
                <button type="button" className="icon-btn" onClick={() => onAdjustZoom(0.2)} title="Zoom in">
                  +
                </button>
                <button type="button" className="icon-btn" onClick={onResetZoom} title="Reset zoom">
                  Reset
                </button>
                <button type="button" className="icon-btn" onClick={onCloseModal} aria-label="Close comparison viewer">
                  ✕
                </button>
              </div>
            </div>

            {hasModalComparison && (
              <div className="comparison-modal-toolbar">
                <div className="comparison-toolbar-section">
                  <div className="comparison-segment" role="group" aria-label="Modal comparison mode">
                    <button
                      type="button"
                      className={`icon-btn${comparisonModalView === "gallery" ? " is-active" : ""}`}
                      onClick={() => onSetModalView("gallery")}
                      title="Gallery view (G)"
                    >
                      Gallery
                    </button>
                    <button
                      type="button"
                      className={`icon-btn${comparisonModalView === "slider" ? " is-active" : ""}`}
                      onClick={() => onSetModalView("slider")}
                      title="Slider view (S)"
                    >
                      Slider
                    </button>
                    <button
                      type="button"
                      className={`icon-btn${comparisonModalView === "flash" ? " is-active" : ""}`}
                      onClick={() => {
                        onSetModalFlashToggle()
                        onSetModalView("flash")
                      }}
                      title="Flash toggle (F)"
                    >
                      Flash
                    </button>
                  </div>
                  <div className="comparison-segment" role="group" aria-label="Focused image">
                    <button
                      type="button"
                      className={`icon-btn${comparisonModalFocus === "original" ? " is-active" : ""}`}
                      onClick={() => onSetModalFocus("original")}
                      title="Original image (O)"
                    >
                      Original
                    </button>
                    <button
                      type="button"
                      className={`icon-btn${comparisonModalFocus === "overlay" ? " is-active" : ""}`}
                      onClick={() => onSetModalFocus("overlay")}
                      title="Overlay/Fast result (V)"
                    >
                      Fast
                    </button>
                    <button
                      type="button"
                      className={`icon-btn${comparisonModalFocus === "layout" ? " is-active" : ""}`}
                      onClick={() => onSetModalFocus("layout")}
                      title="Studio/Layout result (L)"
                    >
                      Studio
                    </button>
                  </div>
                </div>

                <div className="comparison-toolbar-section">
                  <button
                    type="button"
                    className={`icon-btn${comparisonModalQuickToggle ? " is-active" : ""}`}
                    onMouseDown={() => onSetComparisonQuickToggle(true)}
                    onMouseUp={() => onSetComparisonQuickToggle(false)}
                    onMouseLeave={() => onSetComparisonQuickToggle(false)}
                    onTouchStart={() => onSetComparisonQuickToggle(true)}
                    onTouchEnd={() => onSetComparisonQuickToggle(false)}
                    title="Press and hold for before/after toggle"
                  >
                    Before / After
                  </button>
                  <button
                    type="button"
                    className="icon-btn"
                    onClick={() => {
                      if (modalImageSrc) onDownload(modalImageSrc, "comparison-result")
                    }}
                    title="Download this image"
                  >
                    ⬇ Download
                  </button>
                  <button
                    type="button"
                    className="icon-btn"
                    onClick={() => {
                      const card = document.querySelector(".comparison-modal-card") as HTMLElement | null
                      card?.requestFullscreen?.()
                    }}
                    title="Fullscreen view"
                  >
                    ⛶ Fullscreen
                  </button>
                </div>

                <span className="comparison-shortcuts-hint" aria-hidden="true">
                  <span className="hint-label">Keyboard:</span> G/S/F (modes) | O/V/L (views) | +/- (zoom) | ESC (close)
                </span>
              </div>
            )}

            {hasModalComparison && comparisonModalView === "slider" ? (
              <div className="comparison-modal-body">
                <div
                  className={`comparison-modal-stage${comparisonModalZoom > 1 ? " is-pannable" : ""}${comparisonModalPanning ? " is-panning" : ""}`}
                  onWheel={onWheelZoom}
                  onPointerDown={onPointerDown}
                  onPointerMove={onPointerMove}
                  onPointerUp={onPointerUp}
                  onPointerLeave={onPointerUp}
                >
                  <div
                    className="compare-slider-frame comparison-modal-slider-frame"
                    aria-live="polite"
                   onMouseMove={(e) => {
                     if (!sliderDraggingRef.current) return
                     const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
                     const x = e.clientX - rect.left
                     const percent = Math.max(0, Math.min(100, (x / rect.width) * 100))
                     onSetModalSliderPercent(percent)
                   }}
                   onMouseDown={(e) => {
                     if (e.button !== 0) return
                     sliderDraggingRef.current = true
                     const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
                     const x = e.clientX - rect.left
                     onSetModalSliderPercent(Math.max(0, Math.min(100, (x / rect.width) * 100)))
                   }}
                   onMouseUp={() => {
                     sliderDraggingRef.current = false
                   }}
                   onMouseLeave={() => {
                     sliderDraggingRef.current = false
                   }}
                   onTouchMove={(e) => {
                     if (!sliderDraggingRef.current) return
                     const touch = e.touches[0]
                     if (!touch) return
                     const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
                     const x = touch.clientX - rect.left
                     onSetModalSliderPercent(Math.max(0, Math.min(100, (x / rect.width) * 100)))
                   }}
                   onTouchStart={(e) => {
                     const touch = e.touches[0]
                     if (!touch) return
                     sliderDraggingRef.current = true
                     const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
                     const x = touch.clientX - rect.left
                     onSetModalSliderPercent(Math.max(0, Math.min(100, (x / rect.width) * 100)))
                   }}
                   onTouchEnd={() => {
                     sliderDraggingRef.current = false
                   }}
                  >
                    <img
                      className="compare-slider-base"
                      src={compareOriginalUrl ?? ""}
                      alt="Original image"
                      loading="lazy"
                      style={{ transform: modalTransform, transformOrigin: "center" }}
                    />
                    <div
                      className="compare-slider-overlay"
                      style={{ width: `${comparisonModalSliderPercent}%` }}
                      aria-hidden="true"
                    >
                      <img
                        src={comparisonModalSliderTarget === "overlay" ? (compareOverlayUrl ?? "") : (compareLayoutUrl ?? "")}
                        alt=""
                        loading="lazy"
                        style={{ transform: modalTransform, transformOrigin: "center" }}
                      />
                    </div>
                    <div className="compare-slider-handle" style={{ left: `${comparisonModalSliderPercent}%` }} aria-hidden="true" />
                  </div>
                </div>
                <label className="compare-slider-label">
                  Original vs {comparisonModalSliderTarget === "overlay" ? "overlay" : "layout"} — drag to reveal
                  <input
                    type="range"
                    min={0}
                    max={100}
                    value={comparisonModalSliderPercent}
                    onChange={e => onSetModalSliderPercent(Number(e.target.value))}
                  />
                </label>
                <div className="comparison-segment" role="group" aria-label="Modal slider target">
                  <button
                    type="button"
                    className={`icon-btn${comparisonModalSliderTarget === "overlay" ? " is-active" : ""}`}
                    onClick={() => onSetModalSliderTarget("overlay")}
                  >
                    Compare Overlay
                  </button>
                  <button
                    type="button"
                    className={`icon-btn${comparisonModalSliderTarget === "layout" ? " is-active" : ""}`}
                    onClick={() => onSetModalSliderTarget("layout")}
                  >
                    Compare Layout
                  </button>
                </div>
              </div>
            ) : hasModalComparison && comparisonModalView === "flash" ? (
              <div className="comparison-modal-body">
                <div
                  className={`comparison-modal-stage${comparisonModalZoom > 1 ? " is-pannable" : ""}${comparisonModalPanning ? " is-panning" : ""}`}
                  onWheel={onWheelZoom}
                  onPointerDown={onPointerDown}
                  onPointerMove={onPointerMove}
                  onPointerUp={onPointerUp}
                  onPointerLeave={onPointerUp}
                >
                  <div className="compare-flash-frame comparison-modal-flash-frame">
                    <img
                      className="compare-flash-image compare-flash-base"
                      src={compareOriginalUrl ?? ""}
                      alt="Original image"
                      style={{ transform: modalTransform, transformOrigin: "center" }}
                    />
                    <img
                      className={`compare-flash-image compare-flash-top${comparisonModalFlashToggle ? " is-visible" : ""}`}
                       src={comparisonModalFlashTarget === "overlay" ? (compareOverlayUrl ?? "") : (compareLayoutUrl ?? "")}
                      alt="Flash comparison"
                      style={{ transform: modalTransform, transformOrigin: "center" }}
                    />
                    <div className="compare-flash-badge">
                      {comparisonModalFlashToggle ? `Showing ${comparisonModalFlashTarget}` : "Showing original"}
                    </div>
                  </div>
                </div>
                <div className="comparison-segment" role="group" aria-label="Modal flash target">
                  <button
                    type="button"
                    className={`icon-btn${comparisonModalFlashTarget === "overlay" ? " is-active" : ""}`}
                    onClick={() => onSetModalSliderTarget("overlay")}
                  >
                    Flash Overlay
                  </button>
                  <button
                    type="button"
                    className={`icon-btn${comparisonModalFlashTarget === "layout" ? " is-active" : ""}`}
                    onClick={() => onSetModalSliderTarget("layout")}
                  >
                    Flash Layout
                  </button>
                </div>
              </div>
            ) : (
              <div className="comparison-modal-body">
                <div
                  className={`comparison-modal-stage${comparisonModalZoom > 1 ? " is-pannable" : ""}${comparisonModalPanning ? " is-panning" : ""}`}
                  onWheel={onWheelZoom}
                  onPointerDown={onPointerDown}
                  onPointerMove={onPointerMove}
                  onPointerUp={onPointerUp}
                  onPointerLeave={onPointerUp}
                >
                  <img
                    src={modalImageSrc}
                    alt="Comparison preview"
                    className="comparison-modal-image"
                    style={{ transform: modalTransform }}
                  />
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Demo presets */}
      {!ocrLoading && (
        <div className="demo-presets" role="group" aria-label="Demo presets">
          <span className="demo-presets-label">Sample images:</span>
          <button
            type="button"
            className={`icon-btn demo-mode-btn${demoModeActive ? " is-active" : ""}`}
            onClick={onToggleDemoMode}
            title="Cycle sample images automatically"
          >
            {demoModeActive ? "⏹ Stop demo" : "▶ Demo mode"}
          </button>
          {DEMO_PRESETS.map(preset => (
            <button
              key={preset.id}
              type="button"
              className="demo-preset-btn"
              disabled={loading || demoModeActive}
              onClick={() => onOpenDemoPreset(preset)}
              title={`${preset.label} — ${preset.source === "auto" ? "auto" : preset.source} → ${preset.target}`}
            >
              <span className="demo-preset-thumb-wrap">
                <img className="demo-preset-thumb" src={preset.src} alt="" />
              </span>
            </button>
          ))}
        </div>
      )}
    </>
  )
}
