import { memo } from "react"
import {
  type ComparisonFocus,
} from "../constants"

interface ExportActionsProps {
  result: string
  loading: boolean
  resultImageUrl: string | null
  resultImageLarge: boolean
  compareOriginalUrl: string | null
  onDownloadPNG: () => void
  onDownloadPDF: () => void
  onCopy: () => void
  onOpenCompare: (focus: ComparisonFocus) => void
}

export const ExportActions = memo(function ExportActions({
  result,
  loading,
  resultImageUrl,
  resultImageLarge,
  compareOriginalUrl,
  onDownloadPNG,
  onDownloadPDF,
  onCopy,
  onOpenCompare,
}: ExportActionsProps) {
  if (!result || loading) return null

  return (
    <div className="panel-footer panel-footer-export">
      {resultImageLarge && resultImageUrl && (
        <p className="text-mode-hint">Large preview detected; the output image is capped for smoother Studio mode rendering.</p>
      )}
      <span className="char-count">{result.length} chars</span>
      <div className="export-action-bar" role="group" aria-label="Export actions">
        <button
          className="icon-btn"
          title="Download PNG"
          disabled={!resultImageUrl}
          onClick={onDownloadPNG}
        >
          🖼 PNG
        </button>
        <button className="icon-btn" title="Download PDF" onClick={onDownloadPDF}>
          📄 PDF
        </button>
        <button className="icon-btn" title="Copy translation" onClick={onCopy}>
          ⎘ Copy text
        </button>
        <button
          className="icon-btn"
          title="Open comparison workspace"
          disabled={!(compareOriginalUrl || resultImageUrl)}
          onClick={() => onOpenCompare("layout")}
        >
          ⟲ Open compare
        </button>
      </div>
    </div>
  )
})
