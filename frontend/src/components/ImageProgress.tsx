import { IMAGE_PROGRESS_STAGES } from "../constants"

interface ImageProgressProps {
  status: "idle" | "running" | "done" | "error"
  stage: string | null
  completedStages: string[]
  failedStage: string | null
  reliabilityHint: string | null
  stageProgressRatio: number
  livePhaseLabel: string | null
  liveChunkProgress: number | null
  translationProgressText: string
}

export function ImageProgress({
  status,
  stage,
  completedStages,
  failedStage,
  reliabilityHint,
  stageProgressRatio,
  livePhaseLabel,
  liveChunkProgress,
  translationProgressText,
}: ImageProgressProps) {
  if (status === "idle") return null

  return (
    <section className={`image-progress image-progress-${status}`} aria-live="polite">
      <div className="image-progress-header">
        <strong>
          {status === "running"
            ? "Image pipeline in progress"
            : status === "done"
              ? "Image pipeline completed"
              : "Image pipeline failed"}
        </strong>
        <span className="image-progress-summary">
          {status === "running"
            ? `Active stage: ${IMAGE_PROGRESS_STAGES.find(s => s.key === stage)?.label ?? "Detecting text"}`
            : status === "done"
              ? "All stages finished"
              : "Job stopped before completion"}
        </span>
      </div>
      <div
        className="image-progress-meter"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(stageProgressRatio * 100)}
      >
        <div
          className="image-progress-meter-fill"
          style={{ transform: `scaleX(${stageProgressRatio})` }}
        />
      </div>
      <div className="image-progress-live-row">
        <span>{stage === "translating" ? translationProgressText : (livePhaseLabel ?? "Rendering typography")}</span>
        <span>{Math.round(((liveChunkProgress ?? stageProgressRatio) * 100))}%</span>
      </div>
      {reliabilityHint && (
        <div className="image-progress-reliability" role="status">{reliabilityHint}</div>
      )}
      <ol className="image-progress-stages">
        {IMAGE_PROGRESS_STAGES.map(s => {
          const done = completedStages.includes(s.key)
          const active = status === "running" && stage === s.key
          const failed = status === "error" && failedStage === s.key
          return (
            <li
              key={s.key}
              className={`image-progress-stage${done ? " is-done" : ""}${active ? " is-active" : ""}${failed ? " is-failed" : ""}`}
            >
              <span className="image-progress-dot" aria-hidden="true" />
              <span>{s.label}</span>
              {done && <span className="image-progress-state">Done</span>}
              {active && <span className="image-progress-state">Running</span>}
              {failed && <span className="image-progress-state">Failed</span>}
            </li>
          )
        })}
      </ol>
    </section>
  )
}
