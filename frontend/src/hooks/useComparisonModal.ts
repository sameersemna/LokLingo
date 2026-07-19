import { useCallback, useEffect, useReducer, useRef } from "react"
import type { ComparisonFocus } from "../constants"
import { MOTION } from "../motion"

export type ComparisonView = "side" | "slider" | "flash"
export type ModalView = "gallery" | "slider" | "flash"
export type ModalSliderTarget = "overlay" | "layout"

interface State {
  open: boolean
  focus: ComparisonFocus
  view: ModalView
  sliderTarget: ModalSliderTarget
  sliderPercent: number
  flashTarget: ModalSliderTarget
  flashToggle: boolean
  quickToggle: boolean
  zoom: number
  pan: { x: number; y: number }
  panning: boolean
}

type Action =
  | { type: "open"; focus: ComparisonFocus }
  | { type: "close" }
  | { type: "set-view"; view: ModalView }
  | { type: "set-slider-target"; target: ModalSliderTarget }
  | { type: "set-slider-percent"; percent: number }
  | { type: "set-flash-target"; target: ModalSliderTarget }
  | { type: "flip-flash" }
  | { type: "set-quick-toggle"; on: boolean }
  | { type: "zoom"; delta: number }
  | { type: "reset-zoom" }
  | { type: "pan"; pan: { x: number; y: number } }
  | { type: "panning"; on: boolean }
  | { type: "set-focus"; focus: ComparisonFocus }

const INITIAL: State = {
  open: false,
  focus: "layout",
  view: "gallery",
  sliderTarget: "layout",
  sliderPercent: 50,
  flashTarget: "layout",
  flashToggle: false,
  quickToggle: false,
  zoom: 1,
  pan: { x: 0, y: 0 },
  panning: false,
}

const ZOOM_MIN = 0.6
const ZOOM_MAX = 3
const SLIDER_STEP = 4

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value))
}

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "open":
      return { ...state, open: true, focus: action.focus }
    case "close":
      return { ...state, open: false, quickToggle: false }
    case "set-view":
      return { ...state, view: action.view }
    case "set-slider-target":
      return { ...state, sliderTarget: action.target }
    case "set-slider-percent":
      return { ...state, sliderPercent: clamp(action.percent, 0, 100) }
    case "set-flash-target":
      return { ...state, flashTarget: action.target }
    case "flip-flash":
      return { ...state, flashToggle: !state.flashToggle }
    case "set-quick-toggle":
      return { ...state, quickToggle: action.on }
    case "zoom": {
      const next = clamp(Number((state.zoom + action.delta).toFixed(2)), ZOOM_MIN, ZOOM_MAX)
      if (next <= 1) {
        return { ...state, zoom: next, pan: { x: 0, y: 0 }, panning: false }
      }
      return { ...state, zoom: next }
    }
    case "reset-zoom":
      return { ...state, zoom: 1, pan: { x: 0, y: 0 }, panning: false }
    case "pan":
      return { ...state, pan: action.pan }
    case "panning":
      return { ...state, panning: action.on }
    case "set-focus":
      return { ...state, focus: action.focus }
    default:
      return state
  }
}

export interface ComparisonModalApi {
  state: State
  open: (focus?: ComparisonFocus) => void
  close: () => void
  setView: (view: ModalView) => void
  setSliderTarget: (target: ModalSliderTarget) => void
  setSliderPercent: (next: number) => void
  bumpSlider: (delta: number) => void
  setFlashTarget: (target: ModalSliderTarget) => void
  toggleFlash: () => void
  setQuickToggle: (on: boolean) => void
  setFocus: (focus: ComparisonFocus) => void
  setZoom: (zoom: number) => void
  adjustZoom: (delta: number) => void
  resetZoom: () => void
  setPan: (pan: { x: number; y: number }) => void
  setPanning: (on: boolean) => void
  panOriginRef: React.MutableRefObject<{ pointerX: number; pointerY: number; panX: number; panY: number } | null>
}

/**
 * Encapsulates the comparison modal state machine: focus, view mode, slider
 * position, flash animation, zoom, and panning. The hook also drives the
 * flash interval and the body scroll lock when the modal is open.
 */
export function useComparisonModal(): ComparisonModalApi {
  const [state, dispatch] = useReducer(reducer, INITIAL)
  const flashTimerRef = useRef<number | null>(null)
  const panOriginRef = useRef<{ pointerX: number; pointerY: number; panX: number; panY: number } | null>(null)

  // Flash animation: when in flash view, toggle the flash target every interval.
  useEffect(() => {
    if (!state.open || state.view !== "flash") {
      if (flashTimerRef.current !== null) {
        window.clearInterval(flashTimerRef.current)
        flashTimerRef.current = null
      }
      return
    }
    flashTimerRef.current = window.setInterval(() => {
      dispatch({ type: "flip-flash" })
    }, MOTION.modalFlashIntervalMs)
    return () => {
      if (flashTimerRef.current !== null) {
        window.clearInterval(flashTimerRef.current)
        flashTimerRef.current = null
      }
    }
  }, [state.open, state.view])

  // Body scroll lock when modal is open.
  useEffect(() => {
    if (!state.open) return
    const previous = document.body.style.overflow
    document.body.style.overflow = "hidden"
    return () => {
      document.body.style.overflow = previous
    }
  }, [state.open])

  const open = useCallback((focus: ComparisonFocus = "layout") => {
    dispatch({ type: "open", focus })
  }, [])
  const close = useCallback(() => dispatch({ type: "close" }), [])
  const setView = useCallback((view: ModalView) => dispatch({ type: "set-view", view }), [])
  const setSliderTarget = useCallback(
    (target: ModalSliderTarget) => dispatch({ type: "set-slider-target", target }),
    [],
  )
  const setSliderPercent = useCallback(
    (percent: number) => dispatch({ type: "set-slider-percent", percent }),
    [],
  )
  const bumpSlider = useCallback(
    (delta: number) => dispatch({ type: "set-slider-percent", percent: 50 + delta * SLIDER_STEP / 2 }),
    [],
  )
  const setFlashTarget = useCallback(
    (target: ModalSliderTarget) => dispatch({ type: "set-flash-target", target }),
    [],
  )
  const toggleFlash = useCallback(() => dispatch({ type: "flip-flash" }), [])
  const setQuickToggle = useCallback((on: boolean) => dispatch({ type: "set-quick-toggle", on }), [])
  const setFocus = useCallback((focus: ComparisonFocus) => dispatch({ type: "set-focus", focus }), [])
  const setZoom = useCallback((_zoom: number) => dispatch({ type: "reset-zoom" }), [])
  const adjustZoom = useCallback((delta: number) => dispatch({ type: "zoom", delta }), [])
  const resetZoom = useCallback(() => dispatch({ type: "reset-zoom" }), [])
  const setPan = useCallback((pan: { x: number; y: number }) => dispatch({ type: "pan", pan }), [])
  const setPanning = useCallback((on: boolean) => dispatch({ type: "panning", on }), [])

  return {
    state,
    open,
    close,
    setView,
    setSliderTarget,
    setSliderPercent,
    bumpSlider,
    setFlashTarget,
    toggleFlash,
    setQuickToggle,
    setFocus,
    setZoom,
    adjustZoom,
    resetZoom,
    setPan,
    setPanning,
    panOriginRef,
  }
}
