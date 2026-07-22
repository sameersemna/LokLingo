interface PipelineWarningProps {
  warning: string | null
  onOpenReliability: () => void
  onDismiss: () => void
}

export function PipelineWarning({ warning, onOpenReliability, onDismiss }: PipelineWarningProps) {
  if (!warning) return null
  return (
    <section className="status-banner" role="status" aria-live="polite">
      <span className="status-banner-icon" aria-hidden="true">⚠</span>
      <span>{warning}</span>
      <button type="button" className="icon-btn" onClick={onOpenReliability}>
        Open reliability
      </button>
      <button type="button" className="icon-btn" onClick={onDismiss}>
        Dismiss
      </button>
    </section>
  )
}
