import { useCallback, useEffect, useRef, useState } from "react"
import type { TextBlock } from "../api/ocr"
import type { OverlayStage } from "../constants"
import { MOTION } from "../motion"
import { mapTranslatedLinesToBlocks } from "../utils/ocrPipeline"

export interface UseOverlayVisualizationApi {
  overlayBlocks: TextBlock[]
  overlayTexts: string[]
  overlayStage: OverlayStage
  overlayImageSize: { width: number; height: number } | null
  overlayLabelSwapActive: boolean
  overlayDemoRunning: boolean
  overlayDemoPhase: "ocr" | "translation" | "rendering" | null
  setOverlayImageSize: (size: { width: number; height: number } | null) => void
  resetOCRVisualization: () => void
  revealOCRVisualization: (blocks: TextBlock[]) => void
  revealTranslatedVisualization: (blocks: TextBlock[], translatedText: string) => void
  runOverlayDemo: () => void
}

/**
 * The OCR text-region overlay shown while/after an image is processed:
 * the detected blocks, their (optionally translated) labels, the
 * OCR->translated label-swap animation, and the replayable "demo" that
 * cycles through OCR -> translation -> rendering phases for show-and-tell.
 */
export function useOverlayVisualization(ocrLoading: boolean): UseOverlayVisualizationApi {
  const [overlayBlocks, setOverlayBlocks] = useState<TextBlock[]>([])
  const [overlayTexts, setOverlayTexts] = useState<string[]>([])
  const [overlayStage, setOverlayStage] = useState<OverlayStage>("idle")
  const [overlayImageSize, setOverlayImageSize] = useState<{ width: number; height: number } | null>(null)
  const [overlayLabelSwapActive, setOverlayLabelSwapActive] = useState(false)
  const [overlayDemoRunning, setOverlayDemoRunning] = useState(false)
  const [overlayDemoPhase, setOverlayDemoPhase] = useState<"ocr" | "translation" | "rendering" | null>(null)

  const overlaySwapTimerRef = useRef<number | null>(null)
  const overlayDemoTimersRef = useRef<number[]>([])

  const clearOverlaySwapTimer = useCallback(() => {
    if (overlaySwapTimerRef.current !== null) {
      window.clearTimeout(overlaySwapTimerRef.current)
      overlaySwapTimerRef.current = null
    }
  }, [])

  const clearOverlayDemoTimers = useCallback(() => {
    overlayDemoTimersRef.current.forEach(timer => window.clearTimeout(timer))
    overlayDemoTimersRef.current = []
  }, [])

  const resetOCRVisualization = useCallback(() => {
    clearOverlayDemoTimers()
    clearOverlaySwapTimer()
    setOverlayBlocks([])
    setOverlayTexts([])
    setOverlayStage("idle")
    setOverlayImageSize(null)
    setOverlayLabelSwapActive(false)
    setOverlayDemoRunning(false)
    setOverlayDemoPhase(null)
  }, [clearOverlayDemoTimers, clearOverlaySwapTimer])

  const revealOCRVisualization = useCallback((blocks: TextBlock[]) => {
    clearOverlayDemoTimers()
    clearOverlaySwapTimer()
    setOverlayBlocks(blocks)
    setOverlayTexts([])
    setOverlayStage(blocks.length > 0 ? "ocr" : "idle")
    setOverlayLabelSwapActive(false)
    setOverlayDemoRunning(false)
    setOverlayDemoPhase(null)
  }, [clearOverlayDemoTimers, clearOverlaySwapTimer])

  const revealTranslatedVisualization = useCallback((blocks: TextBlock[], translatedText: string) => {
    clearOverlayDemoTimers()
    clearOverlaySwapTimer()
    setOverlayBlocks(blocks)
    setOverlayTexts(mapTranslatedLinesToBlocks(blocks, translatedText))
    setOverlayStage(blocks.length > 0 ? "translated" : "idle")
    if (blocks.length > 0) {
      setOverlayLabelSwapActive(true)
      overlaySwapTimerRef.current = window.setTimeout(() => {
        setOverlayLabelSwapActive(false)
        overlaySwapTimerRef.current = null
      }, MOTION.overlaySwapLabelMs)
    }
    setOverlayDemoRunning(false)
    setOverlayDemoPhase(null)
  }, [clearOverlayDemoTimers, clearOverlaySwapTimer])

  const runOverlayDemo = useCallback(() => {
    if (ocrLoading || overlayBlocks.length === 0 || overlayStage === "idle") {
      return
    }

    clearOverlayDemoTimers()
    clearOverlaySwapTimer()

    const hasTranslatedLabels = overlayTexts.some(text => text.trim() !== "")

    setOverlayDemoRunning(true)
    setOverlayDemoPhase("ocr")
    setOverlayStage("ocr")
    setOverlayLabelSwapActive(false)

    const toTranslation = window.setTimeout(() => {
      setOverlayDemoPhase("translation")
      if (hasTranslatedLabels) {
        setOverlayStage("translated")
        setOverlayLabelSwapActive(true)
      }
    }, MOTION.overlayDemo.toTranslationMs)

    const clearSwap = window.setTimeout(() => {
      setOverlayLabelSwapActive(false)
    }, MOTION.overlayDemo.clearSwapMs)

    const toRendering = window.setTimeout(() => {
      setOverlayDemoPhase("rendering")
    }, MOTION.overlayDemo.toRenderingMs)

    const settle = window.setTimeout(() => {
      setOverlayDemoRunning(false)
      setOverlayDemoPhase(null)
      setOverlayLabelSwapActive(false)
      setOverlayStage(hasTranslatedLabels ? "translated" : "ocr")
    }, MOTION.overlayDemo.settleMs)

    overlayDemoTimersRef.current = [toTranslation, clearSwap, toRendering, settle]
  }, [clearOverlayDemoTimers, clearOverlaySwapTimer, ocrLoading, overlayBlocks, overlayStage, overlayTexts])

  useEffect(() => {
    return () => {
      clearOverlaySwapTimer()
    }
  }, [clearOverlaySwapTimer])

  useEffect(() => {
    return () => {
      clearOverlayDemoTimers()
    }
  }, [clearOverlayDemoTimers])

  return {
    overlayBlocks,
    overlayTexts,
    overlayStage,
    overlayImageSize,
    overlayLabelSwapActive,
    overlayDemoRunning,
    overlayDemoPhase,
    setOverlayImageSize,
    resetOCRVisualization,
    revealOCRVisualization,
    revealTranslatedVisualization,
    runOverlayDemo,
  }
}
