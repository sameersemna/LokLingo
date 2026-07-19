import { useCallback, useEffect, useReducer, useRef } from "react"
import { getReadiness, getOCRMetrics, type ReadinessResponse, type OCRMetricsResponse } from "../api"
import { pressureLevel } from "../constants"

interface State {
  readiness: ReadinessResponse | null
  popoverOpen: boolean
  popoverLoading: boolean
  popoverError: string | null
  popoverMetrics: OCRMetricsResponse | null
  pressureDelta: number
  pressureTrend: "up" | "down" | "flat"
  pressureSeries: number[]
  pressureLevel: "normal" | "warn" | "critical"
}

type Action =
  | { type: "set-readiness"; readiness: ReadinessResponse | null }
  | { type: "set-popover-open"; open: boolean }
  | { type: "set-popover-loading"; loading: boolean }
  | { type: "set-popover-error"; error: string | null }
  | { type: "set-popover-metrics"; metrics: OCRMetricsResponse | null }
  | { type: "set-pressure"; delta: number; trend: "up" | "down" | "flat"; series: number[]; level: "normal" | "warn" | "critical" }

const INITIAL: State = {
  readiness: null,
  popoverOpen: false,
  popoverLoading: false,
  popoverError: null,
  popoverMetrics: null,
  pressureDelta: 0,
  pressureTrend: "flat",
  pressureSeries: [],
  pressureLevel: "normal",
}

const POLL_INTERVAL_MS = 30_000
const POPOVER_REFRESH_MS = 60_000

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "set-readiness":
      return { ...state, readiness: action.readiness }
    case "set-popover-open":
      return { ...state, popoverOpen: action.open }
    case "set-popover-loading":
      return { ...state, popoverLoading: action.loading }
    case "set-popover-error":
      return { ...state, popoverError: action.error }
    case "set-popover-metrics":
      return { ...state, popoverMetrics: action.metrics }
    case "set-pressure":
      return {
        ...state,
        pressureDelta: action.delta,
        pressureTrend: action.trend,
        pressureSeries: action.series,
        pressureLevel: action.level,
      }
    default:
      return state
  }
}

export interface ReadinessApi {
  state: State
  chipRef: React.MutableRefObject<HTMLDivElement | null>
  setPopoverOpen: (open: boolean) => void
}

function computePressure(metrics: OCRMetricsResponse): number {
  return (
    metrics.reliability.litellm.retry_attempts_total +
    metrics.reliability.litellm.circuit_opened_total +
    metrics.reliability.ocr.retry_attempts_total +
    metrics.reliability.ocr.response_rejected_total
  )
}

/**
 * Manages backend readiness polling (30s) and the status popover metrics
 * (60s) including the pressure delta/series/level derived from reliability
 * counters. Exposes a ref for the chip element so click-outside can detect
 * focus loss.
 */
export function useReadiness(): ReadinessApi {
  const [state, dispatch] = useReducer(reducer, INITIAL)
  const chipRef = useRef<HTMLDivElement | null>(null)
  const lastPressureRef = useRef<number | null>(null)

  // Readiness poll (30s)
  useEffect(() => {
    let cancelled = false
    const check = () =>
      getReadiness()
        .then(r => {
          if (!cancelled) dispatch({ type: "set-readiness", readiness: r })
        })
        .catch(() => {
          /* swallow: readiness chip will show last known state */
        })
    check()
    const interval = window.setInterval(check, POLL_INTERVAL_MS)
    return () => {
      cancelled = true
      window.clearInterval(interval)
    }
  }, [])

  // Popover metrics poll (60s, only while open)
  useEffect(() => {
    if (!state.popoverOpen) return
    let cancelled = false
    const fetchPopoverMetrics = async () => {
      dispatch({ type: "set-popover-loading", loading: true })
      try {
        const data = await getOCRMetrics("1h")
        if (cancelled) return
        dispatch({ type: "set-popover-metrics", metrics: data })
        dispatch({ type: "set-popover-error", error: null })

        const next = computePressure(data)
        const prev = lastPressureRef.current
        if (prev === null) {
          dispatch({
            type: "set-pressure",
            delta: 0,
            trend: "flat",
            series: [next],
            level: pressureLevel(next),
          })
        } else {
          const delta = next - prev
          dispatch({
            type: "set-pressure",
            delta,
            trend: delta > 0 ? "up" : delta < 0 ? "down" : "flat",
            series: [next],
            level: pressureLevel(next),
          })
        }
        lastPressureRef.current = next
      } catch (err) {
        if (cancelled) return
        dispatch({
          type: "set-popover-error",
          error: err instanceof Error ? err.message : "Failed to load reliability summary",
        })
      } finally {
        if (!cancelled) dispatch({ type: "set-popover-loading", loading: false })
      }
    }
    fetchPopoverMetrics()
    const interval = window.setInterval(fetchPopoverMetrics, POPOVER_REFRESH_MS)
    return () => {
      cancelled = true
      window.clearInterval(interval)
    }
  }, [state.popoverOpen])

  const setPopoverOpen = useCallback((open: boolean) => {
    dispatch({ type: "set-popover-open", open })
  }, [])

  return { state, chipRef, setPopoverOpen }
}
