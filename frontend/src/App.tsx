import { useCallback, useEffect, useRef, useState } from "react"
import { translate, translateImage, uploadPDF } from "./api/translate"
import { extractTextFromImage, type TextBlock } from "./api/ocr"
import { getOCRMetrics, getProviderMetrics, type MetricsWindow, type OCRMetricsResponse, type ProviderMetricsResponse } from "./api/metrics"
import { PdfJobsPanel } from "./PdfJobsPanel"
import { DeadLetterOpsPanel } from "./DeadLetterOpsPanel"
import { saveStoredJob } from "./pdfJobsStorage"

import { useTheme } from "./hooks/useTheme"
import { useToasts } from "./hooks/useToasts"
import { useLanguageSelection } from "./hooks/useLanguageSelection"
import { useOCRVisualization } from "./hooks/useOCRVisualization"
import { useHistory } from "./hooks/useHistory"
import { useComparisonModal } from "./hooks/useComparisonModal"
import { useReadiness } from "./hooks/useReadiness"
import { useAnimatedCount } from "./hooks/useAnimatedCount"
import { useExportActions } from "./hooks/useExportActions"
import { useDemoShowcase } from "./hooks/useDemoShowcase"
import { useKeyboardShortcuts } from "./hooks/useKeyboardShortcuts"
import { useImageProgress } from "./hooks/useImageProgress"
import { getErrorMessage } from "./utils/errors"
import {
  isRenderableOCRBlock,
  mapTranslatedLinesToBlocks,
  formatFileSize,
  hasRTLText,
  hasVerticalTypography,
  getPipelineHelpMessage,
} from "./utils/ocrPipeline"
import { MOTION } from "./motion"
import {
  LANGUAGES,
  PRODUCT_MODE_BACKEND_MAP,
  MAX_CHARS,
  DEMO_PRESETS,
  IMAGE_PROGRESS_STAGES,
  type ProductMode,
  type InputWorkflow,
  type UploadSelection,
  type ComparisonFocus,
  type OverlayStage,
} from "./constants"
import "./App.css"
import { Header, WorkflowSwitcher, UploadHero, ImageProgress, IntelligencePanel, PipelineWarning, DemoGallery, HistoryPanel, ExportActions, ComparisonView, ReliabilityTelemetry } from "./components"

function App() {
  const [theme, setTheme] = useTheme()
  const { toasts, push: pushToast } = useToasts()
  const { handleCopy: copyText, handleDownload, handleDownloadPDF: downloadPDF } = useExportActions(pushToast)
  const { source: sourceLang, target: targetLang, setSource: setSourceLang, setTarget: setTargetLang } = useLanguageSelection()
  const { show: showOCROverlay, staggerMs: ocrStaggerMs, setStaggerMs: setOcrStaggerMs } = useOCRVisualization()
  const { history, add: addHistoryEntry, clear: clearHistory } = useHistory()
  const modal = useComparisonModal()
  /* eslint-disable react-hooks/refs */
  const {
    state: modalState,
    open: openModal,
    close: closeModal,
    setView: setModalView2,
    setSliderTarget: setModalSliderTarget2,
    setSliderPercent: setModalSliderPercent2,
    setFlashTarget: setModalFlashTarget2,
    toggleFlash: setModalFlashToggle2,
    setQuickToggle: setComparisonQuickToggle,
    setFocus: setModalFocus,
    adjustZoom: adjustComparisonModalZoom,
    resetZoom: resetComparisonModalZoom,
    setPan: setModalPan,
    setPanning: setModalPanning,
    panOriginRef: modalPanOriginRef,
  } = modal
  const comparisonModalOpen = modalState.open
  const comparisonModalView = modalState.view
  const comparisonModalSliderTarget = modalState.sliderTarget
  const comparisonModalSliderPercent = modalState.sliderPercent
  const comparisonModalFlashTarget = modalState.flashTarget
  const comparisonModalFlashToggle = modalState.flashToggle
  const comparisonModalFocus = modalState.focus
  const comparisonModalQuickToggle = modalState.quickToggle
  const comparisonModalZoom = modalState.zoom
  const comparisonModalPan = modalState.pan
  const comparisonModalPanning = modalState.panning
  const readinessApi = useReadiness()
  // Expose the readiness chip ref and popover toggle from the hook.
  const chipRef = readinessApi.chipRef
  const setShowStatusDetail = readinessApi.setPopoverOpen
  const readiness = readinessApi.state.readiness
  const showStatusDetail = readinessApi.state.popoverOpen
  const readinessMetrics = readinessApi.state.popoverMetrics
  const readinessMetricsLoading = readinessApi.state.popoverLoading
  const readinessMetricsError = readinessApi.state.popoverError
  const readinessPressureDelta = readinessApi.state.pressureDelta
  const readinessPressureTrend = readinessApi.state.pressureTrend
  const readinessPressureSeries = readinessApi.state.pressureSeries
  const readinessPressureLevel = readinessApi.state.pressureLevel
  const [sourceText, setSourceText] = useState("")
  const [mode, setMode] = useState<ProductMode>("fast")
  const [workflow, setWorkflow] = useState<InputWorkflow>("image")
  const [uploadSelection, setUploadSelection] = useState<UploadSelection | null>(null)
  const [uploadDragActive, setUploadDragActive] = useState(false)
  const [result, setResult] = useState("")
  const [resultKind, setResultKind] = useState<"translation" | "ocr">("translation")
  const [resultImageUrl, setResultImageUrl] = useState<string | null>(null)
  const [resultImageLarge, setResultImageLarge] = useState(false)
  const [compareOriginalUrl, setCompareOriginalUrl] = useState<string | null>(null)
  const [compareOverlayUrl, setCompareOverlayUrl] = useState<string | null>(null)
  const [compareLayoutUrl, setCompareLayoutUrl] = useState<string | null>(null)
  const [compareView, setCompareView] = useState<"side" | "slider" | "flash">("side")
  const [sliderTarget, setSliderTarget] = useState<"overlay" | "layout">("layout")
  const [sliderPercent, setSliderPercent] = useState(50)
  const [compareRevealActive, setCompareRevealActive] = useState(false)
  const [ocrConfidence, setOcrConfidence] = useState<number | null>(null)
  const [overlayBlocks, setOverlayBlocks] = useState<TextBlock[]>([])
  const [overlayTexts, setOverlayTexts] = useState<string[]>([])
  const [overlayStage, setOverlayStage] = useState<OverlayStage>("idle")
  const [overlayImageSize, setOverlayImageSize] = useState<{ width: number; height: number } | null>(null)
  const [overlayLabelSwapActive, setOverlayLabelSwapActive] = useState(false)
  const [overlayDemoRunning, setOverlayDemoRunning] = useState(false)
  const [overlayDemoPhase, setOverlayDemoPhase] = useState<"ocr" | "translation" | "rendering" | null>(null)
  const [detectedLang, setDetectedLang] = useState("")
  const [pipelineWarning, setPipelineWarning] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [ocrLoading, setOcrLoading] = useState(false)
  const [pdfLoading, setPdfLoading] = useState(false)
  const [showHistory, setShowHistory] = useState(false)
  const [showPdfJobs, setShowPdfJobs] = useState(false)
  const [showReliability, setShowReliability] = useState(false)
  const [showDemoGallery, setShowDemoGallery] = useState(false)
  const [showDeadOps, setShowDeadOps] = useState(false)
  const [pdfJobsVersion, setPdfJobsVersion] = useState(0)
  const [metricsWindow, setMetricsWindow] = useState<MetricsWindow>("24h")
  const [metrics, setMetrics] = useState<OCRMetricsResponse | null>(null)
  const [providerMetrics, setProviderMetrics] = useState<ProviderMetricsResponse | null>(null)
  const [metricsLoading, setMetricsLoading] = useState(false)
  const [metricsError, setMetricsError] = useState<string | null>(null)
  const [providerMetricsError, setProviderMetricsError] = useState<string | null>(null)
  const [metricsUpdatedAt, setMetricsUpdatedAt] = useState<number | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  const compareSectionRef = useRef<HTMLElement>(null)
  const sliderDraggingRef = useRef(false)
  const autoCompareKeyRef = useRef<string | null>(null)
  const overlaySwapTimerRef = useRef<number | null>(null)
  const overlayDemoTimersRef = useRef<number[]>([])
  const compareFlashTimerRef = useRef<number | null>(null)
  const modalFlashTimerRef = useRef<number | null>(null)
  const compareRevealKeyRef = useRef<string | null>(null)
  const abortControllerRef = useRef<AbortController | null>(null)

  useEffect(() => {
    return () => {
      if (compareOriginalUrl) URL.revokeObjectURL(compareOriginalUrl)
    }
  }, [compareOriginalUrl])

  useEffect(() => {
    let cancelled = false
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setResultImageLarge(false)
    if (!resultImageUrl) return
    const checkSize = async () => {
      try {
        const response = await fetch(resultImageUrl)
        const blob = await response.blob()
        if (cancelled) return
        setResultImageLarge(blob.size > 20 * 1024 * 1024)
      } catch {
        if (!cancelled) setResultImageLarge(false)
      }
    }
    void checkSize()
    return () => {
      cancelled = true
    }
  }, [resultImageUrl])

  useEffect(() => {
    return () => {
      if (uploadSelection?.previewUrl) {
        URL.revokeObjectURL(uploadSelection.previewUrl)
      }
    }
  }, [uploadSelection])

  useEffect(() => {
    if (ocrLoading) return
    if (!compareOriginalUrl || !compareOverlayUrl || !compareLayoutUrl) return
    compareSectionRef.current?.scrollIntoView({ behavior: "smooth", block: "center" })
  }, [ocrLoading, compareOriginalUrl, compareOverlayUrl, compareLayoutUrl])

  useEffect(() => {
    if (ocrLoading) return
    if (!compareOriginalUrl || !compareOverlayUrl || !compareLayoutUrl) return
    const key = `${compareOriginalUrl}|${compareOverlayUrl}|${compareLayoutUrl}`
    if (compareRevealKeyRef.current === key) return
    compareRevealKeyRef.current = key
    setCompareRevealActive(true)
    const timer = window.setTimeout(() => {
      setCompareRevealActive(false)
    }, 1050)
    return () => window.clearTimeout(timer)
  }, [ocrLoading, compareOriginalUrl, compareOverlayUrl, compareLayoutUrl])

  useEffect(() => {
    if (ocrLoading) return
    if (!compareOriginalUrl || !compareOverlayUrl || !compareLayoutUrl) return
    const key = `${compareOriginalUrl}|${compareOverlayUrl}|${compareLayoutUrl}`
    if (autoCompareKeyRef.current === key) return
    autoCompareKeyRef.current = key
    setModalFocus("layout")
    setModalSliderTarget2("layout")
    setModalFlashTarget2("layout")
  }, [ocrLoading, compareOriginalUrl, compareOverlayUrl, compareLayoutUrl])

  useEffect(() => {
    if (!showStatusDetail) return
    const handler = (e: MouseEvent) => {
      if (chipRef.current && !chipRef.current.contains(e.target as Node)) {
        setShowStatusDetail(false)
      }
    }
    document.addEventListener("mousedown", handler)
    return () => document.removeEventListener("mousedown", handler)
  }, [showStatusDetail, setShowStatusDetail])

  useEffect(() => {
    if (!showReliability) return

    let cancelled = false
    const fetchMetrics = async () => {
      setMetricsLoading(true)
      try {
        const [ocrResult, providerResult] = await Promise.allSettled([
          getOCRMetrics(metricsWindow),
          getProviderMetrics(),
        ])
        if (cancelled) return

        if (ocrResult.status === "fulfilled") {
          setMetrics(ocrResult.value)
          setMetricsError(null)
        } else {
          setMetricsError(ocrResult.reason instanceof Error ? ocrResult.reason.message : "Failed to load reliability metrics")
        }

        if (providerResult.status === "fulfilled") {
          setProviderMetrics(providerResult.value)
          setProviderMetricsError(null)
        } else {
          setProviderMetricsError(providerResult.reason instanceof Error ? providerResult.reason.message : "Failed to load provider metrics")
        }

        setMetricsUpdatedAt(Date.now())
      } catch (err) {
        if (cancelled) return
        setMetricsError(getErrorMessage(err, "Failed to load reliability metrics"))
      } finally {
        if (!cancelled) {
          setMetricsLoading(false)
        }
      }
    }

    fetchMetrics()
    const interval = setInterval(fetchMetrics, 30_000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [showReliability, metricsWindow])

  const handleTranslate = useCallback(async () => {
    if (!sourceText.trim()) return
    setLoading(true)
    setResult("")
    setResultKind("translation")
    setResultImageUrl(null)
    if (compareOriginalUrl) {
      URL.revokeObjectURL(compareOriginalUrl)
      setCompareOriginalUrl(null)
    }
    setCompareOverlayUrl(null)
    setCompareLayoutUrl(null)
    setDetectedLang("")
    try {
      const requestMode = workflow === "text" && mode === "extract"
        ? "overlay"
        : PRODUCT_MODE_BACKEND_MAP[mode]
      const res = await translate({ text: sourceText, source: sourceLang, target: targetLang, mode: requestMode })
      setResult(res.translated_text)
      setResultKind("translation")
      setResultImageUrl(res.image_url ?? null)
      // If auto-detect was used, reflect what the backend resolved it to
      if (sourceLang === "auto" && res.source && res.source !== "auto") {
        setDetectedLang(res.source)
      }
      addHistoryEntry({
        sourceText,
        sourceLang,
        targetLang,
        result: res.translated_text,
      })
    } catch (err) {
      pushToast(getErrorMessage(err, "Translation failed"), "error")
    } finally {
      setLoading(false)
    }
  }, [sourceText, sourceLang, targetLang, mode, workflow, pushToast, compareOriginalUrl, addHistoryEntry])

  // Body scroll lock is managed by useComparisonModal.

  const handleSwap = () => {
    if (sourceLang === "auto") return
    setSourceLang(targetLang)
    setTargetLang(sourceLang)
    setSourceText(result)
    setResult(sourceText)
    setResultKind("translation")
    setResultImageUrl(null)
    if (compareOriginalUrl) {
      URL.revokeObjectURL(compareOriginalUrl)
      setCompareOriginalUrl(null)
    }
    setCompareOverlayUrl(null)
    setCompareLayoutUrl(null)
    setDetectedLang("")
  }

  const openComparisonModal = useCallback((focus: ComparisonFocus) => {
    const hasComparison = Boolean(compareOriginalUrl && compareOverlayUrl && compareLayoutUrl)
    if (!hasComparison && !resultImageUrl) return
    setModalFocus(hasComparison ? focus : "layout")
    setModalSliderTarget2(focus === "overlay" ? "overlay" : "layout")
    setModalFlashTarget2(focus === "overlay" ? "overlay" : "layout")
    setModalSliderPercent2(50)
    setModalView2(hasComparison ? "slider" : "gallery")
    setComparisonQuickToggle(false)
    resetComparisonModalZoom()
    openModal()
  }, [compareOriginalUrl, compareOverlayUrl, compareLayoutUrl, resultImageUrl])

  const openComparisonSliderModal = useCallback((focus: ComparisonFocus = "layout") => {
    const hasComparison = Boolean(compareOriginalUrl && compareOverlayUrl && compareLayoutUrl)
    if (!hasComparison && !resultImageUrl) return
    setModalFocus(hasComparison ? focus : "layout")
    setModalSliderTarget2(focus === "overlay" ? "overlay" : "layout")
    setModalFlashTarget2(focus === "overlay" ? "overlay" : "layout")
    setModalSliderPercent2(50)
    setModalView2("slider")
    setComparisonQuickToggle(false)
    resetComparisonModalZoom()
    openModal()
  }, [compareOriginalUrl, compareOverlayUrl, compareLayoutUrl, resultImageUrl])

  const closeComparisonModal = () => {
    setModalPanning(false)
    setComparisonQuickToggle(false)
    closeModal()
  }

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

  const {
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
  } = useImageProgress({
    ocrLoading,
    overlayBlocksCount: overlayBlocks.length,
    compareOriginalUrl,
    compareOverlayUrl,
    compareLayoutUrl,
    resultImageUrl,
    setModalFocus,
    setModalSliderTarget: setModalSliderTarget2,
    setModalFlashTarget: setModalFlashTarget2,
    setModalSliderPercent: setModalSliderPercent2,
    setModalView: setModalView2,
    setComparisonQuickToggle,
    resetComparisonModalZoom,
    openModal,
  })

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

  useEffect(() => {
    return () => {
      if (compareFlashTimerRef.current !== null) {
        window.clearInterval(compareFlashTimerRef.current)
      }
      if (modalFlashTimerRef.current !== null) {
        window.clearInterval(modalFlashTimerRef.current)
      }
    }
  }, [])

  const hasModalComparison = Boolean(compareOriginalUrl && compareOverlayUrl && compareLayoutUrl)

  useKeyboardShortcuts({
    comparisonModalOpen,
    comparisonModalView,
    hasModalComparison,
    workflow,
    handleTranslate,
    runOverlayDemo,
    closeModal,
    closeComparisonModal,
    adjustComparisonModalZoom,
    resetComparisonModalZoom,
    setModalSliderPercent: setModalSliderPercent2,
    setModalView: setModalView2,
    setComparisonQuickToggle,
    setModalFocus,
  })

  const runImageFile = useCallback(async (file: File, src: string, tgt: string) => {
    setOcrLoading(true)
    setPipelineWarning(null)
    startImageProgress()
    resetOCRVisualization()
    setOcrConfidence(null)

    const controller = new AbortController()
    abortControllerRef.current = controller

    let ocrBlocks: TextBlock[] = []
    let translatedOverlayText = ""
    extractTextFromImage(file, src)
      .then(ocrRes => {
        setOcrConfidence(Number.isFinite(ocrRes.confidence) ? ocrRes.confidence * 100 : null)
        ocrBlocks = ocrRes.blocks.filter(isRenderableOCRBlock)
        setTranslationRegionTotal(ocrBlocks.length)
        if (ocrBlocks.length === 0) {
          return
        }

        if (translatedOverlayText.trim()) {
          revealTranslatedVisualization(ocrBlocks, translatedOverlayText)
          return
        }

        revealOCRVisualization(ocrBlocks)
      })
      .catch(() => {
        // Keep the main image pipeline running even if OCR visualization is unavailable.
      })

    try {
      const originalUrl = URL.createObjectURL(file)
      if (compareOriginalUrl) URL.revokeObjectURL(compareOriginalUrl)
      setCompareOriginalUrl(originalUrl)

      const requestMode = PRODUCT_MODE_BACKEND_MAP[mode]

      if (requestMode === "ocr_only") {
        const ocrRes = await translateImage(file, src, tgt, requestMode, handleImageJobProgress, controller.signal)
        translatedOverlayText = ocrRes.translated_text ?? ""
        if (ocrBlocks.length > 0 && translatedOverlayText.trim()) {
          revealTranslatedVisualization(ocrBlocks, translatedOverlayText)
        }

        setResult(ocrRes.translated_text)
        setResultKind("ocr")
        setResultImageUrl(ocrRes.image_url ?? null)
        setCompareOverlayUrl(null)
        setCompareLayoutUrl(null)
        if (src === "auto" && ocrRes.source && ocrRes.source !== "auto") {
          setDetectedLang(ocrRes.source)
        }
        setPipelineWarning(null)
        completeImageProgress()
        pushToast("OCR extracted", "success")
        return
      }

      const overlayRes = await translateImage(file, src, tgt, "overlay", handleImageJobProgress, controller.signal)
      const layoutRes = await translateImage(file, src, tgt, "layout", handleImageJobProgress, controller.signal)

      if (!overlayRes.image_url || !layoutRes.image_url) {
        throw new Error("Image comparison requires both overlay and layout outputs")
      }

      const activeRes = mode === "studio" ? layoutRes : overlayRes
      translatedOverlayText = activeRes.translated_text ?? ""
      if (ocrBlocks.length > 0 && translatedOverlayText.trim()) {
        revealTranslatedVisualization(ocrBlocks, translatedOverlayText)
      }

      setResult(activeRes.translated_text)
      setResultKind("translation")
      setResultImageUrl(activeRes.image_url ?? null)
      setCompareOverlayUrl(overlayRes.image_url)
      setCompareLayoutUrl(layoutRes.image_url)

      if (src === "auto") {
        const resolved = overlayRes.source && overlayRes.source !== "auto"
          ? overlayRes.source
          : layoutRes.source && layoutRes.source !== "auto"
          ? layoutRes.source
          : ""
        if (resolved) setDetectedLang(resolved)
      }
      setPipelineWarning(null)
      completeImageProgress()
      pushToast("Image translated", "success")
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") {
        setPipelineWarning("Translation cancelled.")
        failImageProgress()
        return
      }
      const friendlyWarning = getPipelineHelpMessage(err) ?? "Translation engine recovering. Please retry in a moment if processing does not complete."
      setPipelineWarning(friendlyWarning)
      failImageProgress()
      pushToast("Image translation paused. Retry to continue.", "error")
    } finally {
      setOcrLoading(false)
      if (abortControllerRef.current === controller) {
        abortControllerRef.current = null
      }
      if (fileRef.current) {
        fileRef.current.value = ""
        fileRef.current.accept = "image/*,application/pdf"
      }
    }
  }, [
    compareOriginalUrl,
    completeImageProgress,
    failImageProgress,
    mode,
    pushToast,
    resetOCRVisualization,
    revealOCRVisualization,
    revealTranslatedVisualization,
    handleImageJobProgress,
    startImageProgress,
    setTranslationRegionTotal,
  ])


  const effectiveModalFocus: ComparisonFocus = comparisonModalQuickToggle ? "original" : comparisonModalFocus
  const modalImageSrc =
    effectiveModalFocus === "original"
      ? compareOriginalUrl
      : effectiveModalFocus === "overlay"
      ? compareOverlayUrl
      : compareLayoutUrl ?? resultImageUrl
  const clampModalPan = (x: number, y: number, zoom: number) => {
    const maxOffset = Math.max(0, (zoom - 1) * 520)
    return {
      x: Math.max(-maxOffset, Math.min(maxOffset, x)),
      y: Math.max(-maxOffset, Math.min(maxOffset, y)),
    }
  }
  const modalTransform = `translate(${comparisonModalPan.x}px, ${comparisonModalPan.y}px) scale(${comparisonModalZoom})`

  const handleComparisonModalWheel = (event: React.WheelEvent<HTMLDivElement>) => {
    if (!comparisonModalOpen) return
    event.preventDefault()
    const delta = event.deltaY < 0 ? 0.12 : -0.12
    adjustComparisonModalZoom(delta)
  }

  const handleComparisonModalPointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    if (comparisonModalZoom <= 1) return
    event.preventDefault()
    event.currentTarget.setPointerCapture(event.pointerId)
    modalPanOriginRef.current = {
      pointerX: event.clientX,
      pointerY: event.clientY,
      panX: comparisonModalPan.x,
      panY: comparisonModalPan.y,
    }
    setModalPanning(true)
  }

  const handleComparisonModalPointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!modalPanOriginRef.current || !comparisonModalPanning) return
    const dx = event.clientX - modalPanOriginRef.current.pointerX
    const dy = event.clientY - modalPanOriginRef.current.pointerY
    setModalPan(clampModalPan(modalPanOriginRef.current.panX + dx, modalPanOriginRef.current.panY + dy, comparisonModalZoom))
  }

  const handleComparisonModalPointerUp = (event: React.PointerEvent<HTMLDivElement>) => {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    modalPanOriginRef.current = null
    setModalPanning(false)
  }

  const handleDemoPreset = useCallback(async (preset: typeof DEMO_PRESETS[number]) => {
    if (ocrLoading) return
    setSourceLang(preset.source)
    setTargetLang(preset.target)
    setDetectedLang("")
    setResult("")
    setResultKind("translation")
    setResultImageUrl(null)
    setCompareOverlayUrl(null)
    setCompareLayoutUrl(null)
    if (compareOriginalUrl) {
      URL.revokeObjectURL(compareOriginalUrl)
      setCompareOriginalUrl(null)
    }
    try {
      const res = await fetch(preset.src)
      const blob = await res.blob()
      const ext = preset.src.split(".").pop() ?? "png"
      const file = new File([blob], `demo-${preset.id}.${ext}`, { type: blob.type || `image/${ext}` })
      await runImageFile(file, preset.source, preset.target)
    } catch (err) {
      pushToast(getErrorMessage(err, "Demo preset failed"), "error")
    }
  }, [compareOriginalUrl, ocrLoading, pushToast, runImageFile])

  const { showcaseMode, toggleShowcase, demoModeActive, toggleDemoMode, stopDemoMode } = useDemoShowcase({
    runImageFile,
    handleDemoPreset,
    setWorkflow,
    setSourceLang,
    setTargetLang,
    setCompareView,
    setSliderTarget,
    ocrLoading,
    pdfLoading,
    loading,
  })

  const handleOCRFile = async (file: File) => {
    setPipelineWarning(null)
    if (demoModeActive) {
      stopDemoMode()
    }
    // PDF files → async translate_pdf job (enqueue + track in PDF Jobs panel)
    if (file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf')) {
      resetOCRVisualization()
      if (uploadSelection?.previewUrl) {
        URL.revokeObjectURL(uploadSelection.previewUrl)
      }
      setWorkflow("pdf")
      setUploadSelection({
        kind: "pdf",
        name: file.name,
        size: file.size,
      })
      setPdfLoading(true)
      try {
        const { job_id } = await uploadPDF(file, sourceLang, targetLang, PRODUCT_MODE_BACKEND_MAP[mode])
        saveStoredJob({
          job_id,
          filename: file.name,
          source: sourceLang,
          target: targetLang,
          submittedAt: Date.now(),
          mode,
        })
        // Force panel refresh by bumping key, then show it
        setPdfJobsVersion(v => v + 1)
        setShowPdfJobs(true)
        setShowHistory(false)
        pushToast('PDF uploaded — translating in background', 'success')
      } catch (err) {
        pushToast(getErrorMessage(err, "PDF upload failed"), "error")
      } finally {
        setPdfLoading(false)
        if (fileRef.current) {
          fileRef.current.value = ""
          fileRef.current.accept = "image/*,application/pdf"
        }
      }
      return
    }

    if (!file.type.startsWith("image/")) {
      pushToast("Please upload an image or PDF file", "error")
      if (fileRef.current) {
        fileRef.current.value = ""
        fileRef.current.accept = "image/*,application/pdf"
      }
      return
    }

    if (uploadSelection?.previewUrl) {
      URL.revokeObjectURL(uploadSelection.previewUrl)
    }
    setWorkflow("image")
    setUploadSelection({
      kind: "image",
      name: file.name,
      size: file.size,
      previewUrl: URL.createObjectURL(file),
    })

    await runImageFile(file, sourceLang, targetLang)
  }

  const openUploadPicker = (accept: string) => {
    if (!fileRef.current || ocrLoading || pdfLoading) return
    fileRef.current.accept = accept
    fileRef.current.click()
  }

  const handleUploadDrop = async (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault()
    event.stopPropagation()
    if (ocrLoading || pdfLoading) return
    setUploadDragActive(false)
    const dropped = event.dataTransfer.files?.[0]
    if (!dropped) return
    await handleOCRFile(dropped)
  }

  const langLabel = (code: string) =>
    LANGUAGES.find(l => l.code === code)?.label ?? code.toUpperCase()

  const charCount = sourceText.length
  const nearLimit = charCount > MAX_CHARS * 0.8
  const uploadBrowseAccept = workflow === "pdf" ? "application/pdf" : "image/*"
  const uploadHeroSubtitle = workflow === "pdf"
    ? "Drop a PDF to queue full-document translation in the background."
    : workflow === "text"
    ? "Text workflow is active. Image upload is still available as the primary path."
    : "Drop an image to translate instantly, or upload a PDF for async document translation."
  const uploadEmptyTitle = workflow === "pdf"
    ? "Drop a PDF to upload"
    : "Drop an image to upload"
  const uploadEmptySupport = workflow === "pdf"
    ? "PDF files are queued and tracked in PDF Jobs."
    : "Supports JPG, PNG, WEBP, and other image formats."
  const readinessState = readiness?.status ?? "checking"
  const readinessLabel =
    readinessState === "ok" ? "System healthy" :
    readinessState === "degraded" ? "System degraded" :
    "Checking system"
  const failedDependencyNames = readiness
    ? Object.entries(readiness.dependencies)
        .filter(([, dep]) => dep.status === "error")
        .map(([name]) => name)
    : []
  const uploadBackendState: "checking" | "online" | "degraded" | "offline" =
    readinessState === "checking"
      ? "checking"
      : failedDependencyNames.length === 0
      ? "online"
      : failedDependencyNames.length >= 2
      ? "offline"
      : "degraded"
  const uploadBackendLabel =
    uploadBackendState === "online"
      ? "Backend online"
      : uploadBackendState === "offline"
      ? "Backend unavailable"
      : uploadBackendState === "degraded"
      ? "Partial service"
      : "Checking backend"
  const uploadBackendHint =
    uploadBackendState === "checking"
      ? "Waiting for readiness check"
      : failedDependencyNames.length > 0
      ? `Issue: ${failedDependencyNames.slice(0, 2).join(", ")}${failedDependencyNames.length > 2 ? " +more" : ""}`
      : "All translation services responsive"
  const miniSparklinePoints = (() => {
    if (readinessPressureSeries.length === 0) {
      return ""
    }
    const width = 120
    const height = 28
    const minV = Math.min(...readinessPressureSeries)
    const maxV = Math.max(...readinessPressureSeries)
    const range = Math.max(1, maxV - minV)
    return readinessPressureSeries
      .map((v, i) => {
        const x = readinessPressureSeries.length === 1
          ? 0
          : (i * width) / (readinessPressureSeries.length - 1)
        const y = height - ((v - minV) / range) * height
        return `${x},${y}`
      })
      .join(" ")
  })()
  const readinessPressureLabel = readinessPressureLevel === "critical"
    ? "Critical"
    : readinessPressureLevel === "warn"
    ? "Warn"
    : "Normal"
  const intelligenceLanguageCodes = Array.from(new Set([
    detectedLang || (sourceLang !== "auto" ? sourceLang : ""),
    targetLang,
  ].filter(Boolean)))
  const verticalDetected = hasVerticalTypography(overlayBlocks)
  const rtlDetected = overlayBlocks.some(block => hasRTLText(block.text))
  const confidenceValue = Math.max(0, Math.min(100, ocrConfidence ?? 0))
  const animatedRegionCount = useAnimatedCount(overlayBlocks.length)
  const animatedLanguageCount = useAnimatedCount(intelligenceLanguageCodes.length)
  const animatedVerticalCount = useAnimatedCount(verticalDetected ? 1 : 0)
  const animatedRTLCount = useAnimatedCount(rtlDetected ? 1 : 0)
  const animatedConfidence = useAnimatedCount(Math.round(confidenceValue))
  const stageOrder = IMAGE_PROGRESS_STAGES.map(stage => stage.key)
  const activeStageIndex = imageProgressStage ? stageOrder.indexOf(imageProgressStage) : -1
  const stageProgressRatio = imageProgressStatus === "done"
    ? 1
    : imageProgressStatus === "running"
    ? Math.max(0, Math.min(1, (imageProgressCompleted.length + (activeStageIndex >= 0 ? 0.45 : 0)) / IMAGE_PROGRESS_STAGES.length))
    : imageProgressStatus === "error"
    ? Math.max(0, Math.min(1, imageProgressCompleted.length / IMAGE_PROGRESS_STAGES.length))
    : 0
  const translationProgressText = translationRegionTotal > 0
    ? `Translating region ${Math.max(1, Math.min(translationRegionIndex, translationRegionTotal))}/${translationRegionTotal}`
    : "Translating regions"
  const legendActiveStage: "ocr" | "translation" | "rendering" | null =
    overlayDemoRunning && overlayDemoPhase
      ? overlayDemoPhase
      : overlayStage === "translated"
      ? (ocrLoading ? "rendering" : null)
      : overlayStage === "ocr"
      ? "translation"
      : ocrLoading
      ? "ocr"
      : null

  return (
    <div className="app">
      <Header
        theme={theme}
        onToggleTheme={() => setTheme(t => t === "dark" ? "light" : "dark")}
        showPdfJobs={showPdfJobs}
        onTogglePdfJobs={() => { setShowPdfJobs(p => !p); setShowHistory(false) }}
        showReliability={showReliability}
        onToggleReliability={() => setShowReliability(v => !v)}
        showDeadOps={showDeadOps}
        onToggleDeadOps={() => setShowDeadOps(v => !v)}
        showHistory={showHistory}
        onToggleHistory={() => { setShowHistory(h => !h); setShowPdfJobs(false) }}
        showDemoGallery={showDemoGallery}
        onToggleDemoGallery={() => setShowDemoGallery(g => !g)}
        showcaseMode={showcaseMode}
        onToggleShowcase={toggleShowcase}
        historyCount={history.length}
        chipRef={chipRef}
        setShowStatusDetail={setShowStatusDetail}
        showStatusDetail={showStatusDetail}
        readinessLabel={readinessLabel}
        readinessState={readinessState}
        readiness={readiness}
        readinessMetricsLoading={readinessMetricsLoading}
        readinessMetricsError={readinessMetricsError}
        readinessMetrics={readinessMetrics}
        readinessPressureTrend={readinessPressureTrend}
        readinessPressureDelta={readinessPressureDelta}
        readinessPressureLabel={readinessPressureLabel}
        readinessPressureLevel={readinessPressureLevel}
        miniSparklinePoints={miniSparklinePoints}
        onOpenReliability={() => { setShowReliability(true); setShowStatusDetail(false) }}
      />

      {/* PDF Jobs panel */}
      {showPdfJobs && (
        <PdfJobsPanel key={pdfJobsVersion} onToast={pushToast} />
      )}

      <DemoGallery
        visible={showDemoGallery}
        onClose={() => setShowDemoGallery(false)}
        onSetWorkflow={w => setWorkflow(w)}
        onLoadDemo={preset => {
          setSourceLang(preset.source)
          setTargetLang(preset.target)
          fetch(preset.src)
            .then(res => res.blob())
            .then(blob => {
              const file = new File([blob], `demo-${preset.id}.png`, { type: blob.type })
              runImageFile(file, preset.source, preset.target)
            })
            .catch(err => pushToast(`Failed to load demo: ${err.message}`, "error"))
        }}
      />

      <ReliabilityTelemetry
        visible={showReliability}
        window={metricsWindow}
        onWindowChange={w => setMetricsWindow(w)}
        loading={metricsLoading}
        error={metricsError}
        metrics={metrics}
        providerMetrics={providerMetrics}
        providerError={providerMetricsError}
        updatedAt={metricsUpdatedAt}
      />

      {showDeadOps && (
        <DeadLetterOpsPanel onToast={pushToast} />
      )}

      <HistoryPanel
        history={history}
        onClearAll={() => clearHistory()}
        onRestore={h => {
          setSourceText(h.sourceText)
          setSourceLang(h.sourceLang)
          setTargetLang(h.targetLang)
          setResult(h.result)
          setResultKind("translation")
          setResultImageUrl(null)
          if (compareOriginalUrl) {
            URL.revokeObjectURL(compareOriginalUrl)
            setCompareOriginalUrl(null)
          }
          setCompareOverlayUrl(null)
          setCompareLayoutUrl(null)
        }}
        onClose={() => setShowHistory(false)}
        langLabel={langLabel}
      />

      <WorkflowSwitcher
        source={sourceLang}
        target={targetLang}
        detectedLang={detectedLang}
        mode={mode}
        workflow={workflow}
        onSourceChange={v => { setSourceLang(v); setDetectedLang("") }}
        onTargetChange={v => setTargetLang(v)}
        onSwap={handleSwap}
        onModeChange={v => setMode(v)}
        onWorkflowChange={v => setWorkflow(v)}
        langLabel={langLabel}
        onFileChange={f => { if (f) handleOCRFile(f) }}
      />

      <main className="translator">
        <UploadHero
          workflow={workflow}
          uploadSelection={uploadSelection}
          uploadDragActive={uploadDragActive}
          ocrLoading={ocrLoading}
          pdfLoading={pdfLoading}
          ocrStaggerMs={ocrStaggerMs}
          overlayStage={overlayStage}
          showOCROverlay={showOCROverlay}
          overlayBlocks={overlayBlocks}
          overlayTexts={overlayTexts}
          overlayLabelSwapActive={overlayLabelSwapActive}
          overlayDemoRunning={overlayDemoRunning}
          overlayImageSize={overlayImageSize}
          legendActiveStage={legendActiveStage ?? ""}
          uploadBackendState={uploadBackendState}
          uploadBackendHint={uploadBackendHint}
          uploadBackendLabel={uploadBackendLabel}
          uploadHeroSubtitle={uploadHeroSubtitle}
          uploadEmptyTitle={uploadEmptyTitle}
          uploadEmptySupport={uploadEmptySupport}
          uploadBrowseAccept={uploadBrowseAccept}
          onDragEnter={e => { e.preventDefault(); e.stopPropagation(); if (!ocrLoading && !pdfLoading) setUploadDragActive(true) }}
          onDragOver={e => { e.preventDefault(); e.stopPropagation(); if (!ocrLoading && !pdfLoading) setUploadDragActive(true) }}
          onDragLeave={e => { e.preventDefault(); e.stopPropagation(); setUploadDragActive(false) }}
          onUploadDrop={e => handleUploadDrop(e as unknown as React.DragEvent<HTMLDivElement>)}
          onOverlayImageLoad={event => {
            const img = event.currentTarget
            if (img.naturalWidth > 0 && img.naturalHeight > 0) {
              setOverlayImageSize({ width: img.naturalWidth, height: img.naturalHeight })
            }
          }}
          onOpenUploadPicker={accept => openUploadPicker(accept)}
          onReplayDemo={runOverlayDemo}
          onStaggerChange={ms => setOcrStaggerMs(ms)}
          onStaggerPreset={ms => setOcrStaggerMs(ms)}
          formatFileSize={formatFileSize}
        />

        <ImageProgress
          status={imageProgressStatus}
          stage={imageProgressStage}
          completedStages={imageProgressCompleted}
          failedStage={imageProgressFailedStage}
          reliabilityHint={reliabilityHint}
          stageProgressRatio={stageProgressRatio}
          livePhaseLabel={livePhaseLabel}
          liveChunkProgress={liveChunkProgress}
          translationProgressText={translationProgressText}
        />

        {(overlayBlocks.length > 0 || imageProgressStatus === "running") && (
          <IntelligencePanel
            regionCount={animatedRegionCount}
            languageCount={animatedLanguageCount}
            languageCodes={intelligenceLanguageCodes}
            verticalCount={animatedVerticalCount}
            rtlCount={animatedRTLCount}
            confidence={animatedConfidence}
            status={imageProgressStatus}
            langLabel={langLabel}
          />
        )}

        <PipelineWarning
          warning={pipelineWarning}
          onOpenReliability={() => { setShowReliability(true); setShowStatusDetail(false) }}
          onDismiss={() => setPipelineWarning(null)}
        />

        <ComparisonView
          ocrLoading={ocrLoading}
          imageProgressStage={imageProgressStage ?? ""}
          mode={mode}
          compareOriginalUrl={compareOriginalUrl}
          compareOverlayUrl={compareOverlayUrl}
          compareLayoutUrl={compareLayoutUrl}
          compareRevealActive={compareRevealActive}
          compareView={compareView}
          sliderTarget={sliderTarget}
          sliderPercent={sliderPercent}
          demoModeActive={demoModeActive}
          loading={loading}
          modalImageSrc={modalImageSrc}
          modalTransform={modalTransform}
          hasModalComparison={hasModalComparison}
          comparisonModalOpen={comparisonModalOpen}
          comparisonModalView={comparisonModalView}
          comparisonModalFocus={comparisonModalFocus}
          comparisonModalZoom={comparisonModalZoom}
          comparisonModalPanning={comparisonModalPanning}
          comparisonModalFlashToggle={comparisonModalFlashToggle}
          comparisonModalFlashTarget={comparisonModalFlashTarget}
          comparisonModalSliderTarget={comparisonModalSliderTarget}
          comparisonModalSliderPercent={comparisonModalSliderPercent}
          comparisonModalQuickToggle={comparisonModalQuickToggle}
          onDownload={handleDownload}
          onOpenSliderModal={focus => openComparisonSliderModal(focus as ComparisonFocus)}
          onSetCompareView={v => setCompareView(v)}
          onSetSliderTarget={v => setSliderTarget(v)}
          onSetSliderPercent={v => setSliderPercent(v)}
          onOpenModal={focus => openComparisonModal(focus as ComparisonFocus)}
          onSetModalView={v => setModalView2(v as "gallery" | "slider" | "flash")}
          onSetModalFocus={v => setModalFocus(v as ComparisonFocus)}
          onAdjustZoom={v => adjustComparisonModalZoom(v)}
          onResetZoom={resetComparisonModalZoom}
          onSetModalFlashToggle={setModalFlashToggle2}
          onSetModalSliderTarget={v => setModalSliderTarget2(v as "overlay" | "layout")}
          onSetModalSliderPercent={v => setModalSliderPercent2(v)}
          onSetComparisonQuickToggle={v => setComparisonQuickToggle(v)}
          onSetCompareSectionRef={el => { compareSectionRef.current = el }}
          onWheelZoom={handleComparisonModalWheel}
          onPointerDown={handleComparisonModalPointerDown}
          onPointerMove={handleComparisonModalPointerMove}
          onPointerUp={handleComparisonModalPointerUp}
          onCloseModal={closeComparisonModal}
          onToggleDemoMode={toggleDemoMode}
          onOpenDemoPreset={handleDemoPreset}
          sectionRef={compareSectionRef}
          sliderDraggingRef={sliderDraggingRef}
        />

        {workflow !== "text" && (
          <p className="text-mode-hint">Switch to Text workflow to translate typed text with the Translate button.</p>
        )}

        <div className={`panels panels-workflow-${workflow}`}>
          {/* Source panel */}
          <div className={`panel panel-source${workflow !== "text" ? " panel-muted" : ""}`}>
            <textarea
              aria-label="Source text"
              placeholder="Enter text to translate…"
              value={sourceText}
              maxLength={MAX_CHARS}
              onChange={e => {
                setSourceText(e.target.value)
                if (workflow !== "text") setWorkflow("text")
              }}
              onFocus={() => {
                if (workflow !== "text") setWorkflow("text")
              }}
              rows={8}
            />
            <div className="panel-footer">
              <span className={`char-count${nearLimit ? " near-limit" : ""}`}>
                {charCount}/{MAX_CHARS} · {sourceText.trim() ? sourceText.trim().split(/\s+/).length : 0}w
              </span>
              <div className="panel-footer-actions">
                {sourceText && (
                  <button
                    className="icon-btn"
                    title="Clear"
                    onClick={() => {
                      setSourceText("")
                      setResult("")
                      setResultKind("translation")
                      setResultImageUrl(null)
                      if (compareOriginalUrl) {
                        URL.revokeObjectURL(compareOriginalUrl)
                        setCompareOriginalUrl(null)
                      }
                      setCompareOverlayUrl(null)
                      setCompareLayoutUrl(null)
                      setDetectedLang("")
                    }}
                  >
                    ✕ Clear
                  </button>
                )}
              </div>
            </div>
          </div>

          {/* Output panel */}
          <div className={`panel panel-output${workflow === "text" ? " panel-priority" : ""}`}>
            <div className="output-body">
              {loading || pdfLoading ? (
                <span className="status">
                  <span className="spinner" aria-hidden="true" />
                  {pdfLoading ? 'Translating PDF…' : 'Translating…'}
                </span>
              ) : (
                <>
                  {result && (
                    <div className={`result-kind-badge result-kind-${resultKind}`}>
                      {resultKind === "ocr" ? "Extracted text" : "Translated text"}
                    </div>
                  )}
                  <div className="result-text">{result}</div>
                  {resultImageUrl && !(compareOverlayUrl && compareLayoutUrl) && (
                    <div className="result-image-wrap">
                      <img
                        className="result-image compare-clickable"
                        style={resultImageLarge ? { maxWidth: "min(100%, 960px)" } : undefined}
                        src={resultImageUrl}
                        alt="Translated output preview"
                        loading="lazy"
                        onClick={() => openComparisonModal("layout")}
                      />
                    </div>
                  )}
                  {resultImageLarge && resultImageUrl && (
                    <p className="text-mode-hint">Large preview detected; the output image is capped for smoother Studio mode rendering.</p>
                  )}
                </>
              )}
            </div>
            <ExportActions
              result={result}
              loading={loading}
              resultImageUrl={resultImageUrl}
              resultImageLarge={resultImageLarge}
              compareOriginalUrl={compareOriginalUrl}
              onDownloadPNG={() => resultImageUrl && handleDownload(resultImageUrl, "loklingo-output")}
              onDownloadPDF={() => downloadPDF(result, resultImageUrl)}
              onCopy={() => copyText(result)}
              onOpenCompare={focus => openComparisonModal(focus)}
            />
          </div>
        </div>

        <div className="actions">
          <span className="hint">
            {workflow === "text"
              ? "Ctrl+Enter to translate text"
              : "Image/PDF upload starts processing automatically"}
          </span>
          {ocrLoading && (
            <button
              type="button"
              className="icon-btn"
              onClick={() => abortControllerRef.current?.abort()}
            >
              Cancel
            </button>
          )}
          <button
            className="translate-btn"
            onClick={handleTranslate}
            disabled={loading || pdfLoading || ocrLoading || workflow !== "text" || !sourceText.trim()}
          >
            {loading
              ? <><span className="spinner spinner-sm spinner-white" aria-hidden="true" /> Translating…</>
              : pdfLoading
              ? <><span className="spinner spinner-sm spinner-white" aria-hidden="true" /> Translating PDF…</>
              : ocrLoading
              ? <><span className="spinner spinner-sm spinner-white" aria-hidden="true" /> Translating image…</>
              : "Translate"}
          </button>
        </div>
      </main>

      {/* Toast notifications */}
      <div className="toast-container" aria-live="polite">
        {toasts.map(t => (
          <div key={t.id} className={`toast toast-${t.type}`}>{t.msg}</div>
        ))}
      </div>


    </div>
  )
  /* eslint-enable react-hooks/refs */
}

export default App
