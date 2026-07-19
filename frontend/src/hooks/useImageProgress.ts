import { useCallback, useEffect, useReducer, useRef } from "react"
import { IMAGE_PROGRESS_STAGES, type ProgressStage } from "../constants"

export type ImageProgressStatus = "idle" | "running" | "done" | "error"

interface State {
  status: ImageProgressStatus
  currentStage: ProgressStage | null
  completedStages: ProgressStage[]
  failedStage: ProgressStage | null
  regionIndex: number
  regionTotal: number
  chunkProgress: number | null
  phaseLabel: string | null
  reliabilityHint: string | null
  ocrConfidence: number | null
  backendActive: boolean
}

type Action =
  | { type: "start" }
  | { type: "advance"; stage: ProgressStage }
  | { type: "complete-stage"; stage: ProgressStage }
  | { type: "set-status"; status: ImageProgressStatus }
  | { type: "fail"; stage: ProgressStage }
  | { type: "reset" }
  | { type: "set-backend-active"; active: boolean }
  | { type: "set-region-total"; total: number }
  | { type: "set-region-index"; index: number }
  | { type: "set-chunk-progress"; progress: number | null }
  | { type: "set-phase-label"; label: string | null }
  | { type: "set-reliability-hint"; hint: string | null }
  | { type: "set-ocr-confidence"; confidence: number | null }
  | { type: "backend-progress"; currentStage: ProgressStage; regionIndex: number; regionTotal: number; phaseLabel: string | null; reliabilityHint: string | null; chunkProgress: number | null }

const INITIAL: State = {
  status: "idle",
  currentStage: null,
  completedStages: [],
  failedStage: null,
  regionIndex: 0,
  regionTotal: 0,
  chunkProgress: null,
  phaseLabel: null,
  reliabilityHint: null,
  ocrConfidence: null,
  backendActive: false,
}

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "start":
      return {
        ...INITIAL,
        status: "running",
        currentStage: IMAGE_PROGRESS_STAGES[0].key,
        completedStages: [],
      }
    case "advance":
      if (state.completedStages.includes(action.stage)) return state
      return {
        ...state,
        currentStage: action.stage,
        completedStages: state.completedStages.includes(action.stage)
          ? state.completedStages
          : [...state.completedStages, action.stage],
      }
    case "complete-stage":
      if (state.completedStages.includes(action.stage)) return state
      return { ...state, completedStages: [...state.completedStages, action.stage] }
    case "set-status":
      return { ...state, status: action.status }
    case "fail":
      return { ...state, status: "error", failedStage: action.stage }
    case "reset":
      return INITIAL
    case "set-backend-active":
      return { ...state, backendActive: action.active }
    case "set-region-total":
      return { ...state, regionTotal: action.total }
    case "set-region-index":
      return { ...state, regionIndex: action.index }
    case "set-chunk-progress":
      return { ...state, chunkProgress: action.progress }
    case "set-phase-label":
      return { ...state, phaseLabel: action.label }
    case "set-reliability-hint":
      return { ...state, reliabilityHint: action.hint }
    case "set-ocr-confidence":
      return { ...state, ocrConfidence: action.confidence }
    case "backend-progress":
      return {
        ...state,
        currentStage: action.currentStage,
        regionIndex: action.regionIndex,
        regionTotal: action.regionTotal,
        phaseLabel: action.phaseLabel,
        reliabilityHint: action.reliabilityHint,
        chunkProgress: action.chunkProgress,
        completedStages: state.completedStages.includes(action.currentStage)
          ? state.completedStages
          : [...state.completedStages, action.currentStage],
      }
    default:
      return state
  }
}

export interface ImageProgressApi {
  state: State
  start: () => void
  complete: () => void
  fail: (stage: ProgressStage) => void
  reset: () => void
  setBackendActive: (active: boolean) => void
  setRegionTotal: (total: number) => void
  setChunkProgress: (progress: number | null) => void
  setPhaseLabel: (label: string | null) => void
  setReliabilityHint: (hint: string | null) => void
  setOcrConfidence: (confidence: number | null) => void
  /** Apply a backend JobProgressUpdate as a single state transition. */
  applyBackendProgress: (currentStage: ProgressStage, regionIndex: number, regionTotal: number, phaseLabel: string | null, reliabilityHint: string | null, chunkProgress: number | null) => void
}

/**
 * Encapsulates the image translation progress state machine and the timer
 * management that drives the simulated/animated progress UI. The hook hides
 * the timer list ref and exposes a stable API the App can use.
 */
export function useImageProgress(): ImageProgressApi {
  const [state, dispatch] = useReducer(reducer, INITIAL)
  const timersRef = useRef<number[]>([])

  const clearTimers = useCallback(() => {
    for (const id of timersRef.current) {
      window.clearTimeout(id)
    }
    timersRef.current = []
  }, [])

  useEffect(() => {
    return () => clearTimers()
  }, [clearTimers])

  const start = useCallback(() => {
    clearTimers()
    dispatch({ type: "start" })
  }, [clearTimers])

  const complete = useCallback(() => {
    clearTimers()
    dispatch({ type: "complete-stage", stage: "rendering" })
    dispatch({ type: "set-status", status: "done" })
  }, [clearTimers])

  const fail = useCallback((stage: ProgressStage) => {
    clearTimers()
    dispatch({ type: "fail", stage })
  }, [clearTimers])

  const reset = useCallback(() => {
    clearTimers()
    dispatch({ type: "reset" })
  }, [clearTimers])

  const setBackendActive = useCallback((active: boolean) => {
    dispatch({ type: "set-backend-active", active })
  }, [])

  const setRegionTotal = useCallback((total: number) => {
    dispatch({ type: "set-region-total", total })
  }, [])

  const setChunkProgress = useCallback((progress: number | null) => {
    dispatch({ type: "set-chunk-progress", progress })
  }, [])

  const setPhaseLabel = useCallback((label: string | null) => {
    dispatch({ type: "set-phase-label", label })
  }, [])

  const setReliabilityHint = useCallback((hint: string | null) => {
    dispatch({ type: "set-reliability-hint", hint })
  }, [])

  const setOcrConfidence = useCallback((confidence: number | null) => {
    dispatch({ type: "set-ocr-confidence", confidence })
  }, [])

  const applyBackendProgress = useCallback(
    (
      currentStage: ProgressStage,
      regionIndex: number,
      regionTotal: number,
      phaseLabel: string | null,
      reliabilityHint: string | null,
      chunkProgress: number | null,
    ) => {
      dispatch({
        type: "backend-progress",
        currentStage,
        regionIndex,
        regionTotal,
        phaseLabel,
        reliabilityHint,
        chunkProgress,
      })
    },
    [],
  )

  return {
    state,
    start,
    complete,
    fail,
    reset,
    setBackendActive,
    setRegionTotal,
    setChunkProgress,
    setPhaseLabel,
    setReliabilityHint,
    setOcrConfidence,
    applyBackendProgress,
  }
}
