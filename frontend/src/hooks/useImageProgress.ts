import { useCallback, useEffect, useRef, useState } from "react"
import type { JobProgressUpdate } from "../api/translate"
import { IMAGE_PROGRESS_STAGES, type ComparisonFocus, type ProgressStage, type LiveProgressDetail } from "../constants"
import type { ModalSliderTarget, ModalView } from "./useComparisonModal"
import { MOTION } from "../motion"
import {
  mapBackendImageStage,
  mapBackendStageNarrative,
  mapBackendReliabilityHint,
} from "../utils/ocrPipeline"

export interface UseImageProgressParams {
  ocrLoading: boolean
  overlayBlocksCount: number
  compareOriginalUrl: string | null
  compareOverlayUrl: string | null
  compareLayoutUrl: string | null
  resultImageUrl: string | null
  setModalFocus: (focus: ComparisonFocus) => void
  setModalSliderTarget: (target: ModalSliderTarget) => void
  setModalFlashTarget: (target: ModalSliderTarget) => void
  setModalSliderPercent: (percent: number) => void
  setModalView: (view: ModalView) => void
  setComparisonQuickToggle: (on: boolean) => void
  resetComparisonModalZoom: () => void
  openModal: (focus?: ComparisonFocus) => void
}

export interface UseImageProgressApi {
  imageProgressStage: ProgressStage | null
  imageProgressCompleted: ProgressStage[]
  imageProgressFailedStage: ProgressStage | null
  imageProgressStatus: "idle" | "running" | "done" | "error"
  translationRegionIndex: number
  translationRegionTotal: number
  liveChunkProgress: number | null
  livePhaseLabel: string | null
  reliabilityHint: string | null
  setTranslationRegionTotal: (total: number) => void
  startImageProgress: () => void
  completeImageProgress: () => void
  failImageProgress: () => void
  handleImageJobProgress: (job: JobProgressUpdate) => void
}

/**
 * The staged image-translation progress state machine: detecting → layout
 * → languages → translating → typography → rendering, plus the live
 * backend job-progress event handling and the reliability hints shown
 * while a request is recovering/retrying. Owns its own timers; exposes
 * lifecycle functions for runImageFile (in App.tsx) to drive.
 */
export function useImageProgress({
  ocrLoading,
  overlayBlocksCount,
}: UseImageProgressParams): UseImageProgressApi {
  const [imageProgressStage, setImageProgressStage] = useState<ProgressStage | null>(null)
  const [imageProgressCompleted, setImageProgressCompleted] = useState<ProgressStage[]>([])
  const [imageProgressFailedStage, setImageProgressFailedStage] = useState<ProgressStage | null>(null)
  const [imageProgressStatus, setImageProgressStatus] = useState<"idle" | "running" | "done" | "error">("idle")
  const [translationRegionIndex, setTranslationRegionIndex] = useState(0)
  const [translationRegionTotal, setTranslationRegionTotal] = useState(0)
  const [liveChunkProgress, setLiveChunkProgress] = useState<number | null>(null)
  const [livePhaseLabel, setLivePhaseLabel] = useState<string | null>(null)
  const [reliabilityHint, setReliabilityHint] = useState<string | null>(null)

  const imageProgressTimersRef = useRef<number[]>([])
  const backendImageProgressActiveRef = useRef(false)

  const clearImageProgressTimers = useCallback(() => {
    imageProgressTimersRef.current.forEach(timer => window.clearTimeout(timer))
    imageProgressTimersRef.current = []
  }, [])

  const startImageProgress = useCallback(() => {
    backendImageProgressActiveRef.current = false
    clearImageProgressTimers()
    setTranslationRegionIndex(0)
    setTranslationRegionTotal(0)
    setLiveChunkProgress(null)
    setLivePhaseLabel(null)
    setReliabilityHint(null)
    setImageProgressStatus("running")
    setImageProgressFailedStage(null)
    setImageProgressStage("detecting")
    setImageProgressCompleted([])

  }, [clearImageProgressTimers])

  const completeImageProgress = useCallback(() => {
    clearImageProgressTimers()
    setImageProgressStatus("done")
    setImageProgressFailedStage(null)
    setImageProgressCompleted(["detecting", "layout", "languages", "translating", "typography", "rendering"])
    setImageProgressStage(null)
    setLiveChunkProgress(1)
    setLivePhaseLabel("Complete")
    setReliabilityHint(null)

    const settle = window.setTimeout(() => {
      setImageProgressStatus("idle")
      setImageProgressFailedStage(null)
      setImageProgressCompleted([])
      setImageProgressStage(null)
      setTranslationRegionIndex(0)
      setTranslationRegionTotal(0)
      setLiveChunkProgress(null)
      setLivePhaseLabel(null)
    }, MOTION.imageProgress.successSettleMs)

    imageProgressTimersRef.current = [settle]
  }, [clearImageProgressTimers])

  const failImageProgress = useCallback(() => {
    clearImageProgressTimers()
    setImageProgressStatus("error")
    setImageProgressFailedStage(imageProgressStage)
    setLivePhaseLabel("Processing interrupted")
    setReliabilityHint(null)
  }, [clearImageProgressTimers, imageProgressStage])

  const handleImageJobProgress = useCallback((job: JobProgressUpdate) => {
    if (!backendImageProgressActiveRef.current) {
      backendImageProgressActiveRef.current = true
      clearImageProgressTimers()
    }

    const mappedStage = mapBackendImageStage(job.stage)
    const progressValue = typeof job.stage_progress === "number"
      ? Math.max(0, Math.min(1, job.stage_progress))
      : null

    if (mappedStage) {
      setImageProgressStage(mappedStage)
      const stageIndex = IMAGE_PROGRESS_STAGES.findIndex(stage => stage.key === mappedStage)
      if (stageIndex > 0) {
        setImageProgressCompleted(IMAGE_PROGRESS_STAGES.slice(0, stageIndex).map(stage => stage.key))
      }
    }

    if (progressValue !== null) {
      setLiveChunkProgress(progressValue)
    }

    const backendNarrative = mapBackendStageNarrative(job)
    if (backendNarrative) {
      setLivePhaseLabel(backendNarrative)
    } else if (job.stage_message?.trim()) {
      setLivePhaseLabel(job.stage_message.trim())
    }

    const reliabilityMessage = mapBackendReliabilityHint(job)
    if (reliabilityMessage) {
      setReliabilityHint(reliabilityMessage)
    } else if (job.status === "processing") {
      setReliabilityHint(null)
    }
  }, [clearImageProgressTimers])

  useEffect(() => {
    return () => {
      clearImageProgressTimers()
    }
  }, [clearImageProgressTimers])

  useEffect(() => {
    if (!ocrLoading) return
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setTranslationRegionTotal(overlayBlocksCount)
  }, [ocrLoading, overlayBlocksCount])

  useEffect(() => {
    const handleLiveProgress = (event: Event) => {
      const customEvent = event as CustomEvent<LiveProgressDetail>
      const detail = customEvent.detail
      if (!detail) return
      if (typeof detail.region === "number") {
        setTranslationRegionIndex(Math.max(0, Math.floor(detail.region)))
      }
      if (typeof detail.totalRegions === "number") {
        setTranslationRegionTotal(Math.max(0, Math.floor(detail.totalRegions)))
      }
      if (typeof detail.chunkProgress === "number") {
        setLiveChunkProgress(Math.max(0, Math.min(1, detail.chunkProgress)))
      }
      if (typeof detail.phaseLabel === "string" && detail.phaseLabel.trim()) {
        setLivePhaseLabel(detail.phaseLabel.trim())
      }
      if (detail.stage) {
        setImageProgressStage(detail.stage)
      }
    }

    window.addEventListener("loklingo:image-progress", handleLiveProgress as EventListener)
    return () => {
      window.removeEventListener("loklingo:image-progress", handleLiveProgress as EventListener)
    }
  }, [])

  return {
    imageProgressStage,
    imageProgressCompleted,
    imageProgressFailedStage,
    imageProgressStatus,
    translationRegionIndex,
    translationRegionTotal,
    liveChunkProgress,
    livePhaseLabel,
    reliabilityHint,
    setTranslationRegionTotal,
    startImageProgress,
    completeImageProgress,
    failImageProgress,
    handleImageJobProgress,
  }
}
