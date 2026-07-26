import { useCallback, useEffect, useRef, useState } from "react"
import { translate, translateImage, uploadPDF, type JobProgressUpdate } from "./api/translate"
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
import { getErrorMessage } from "./utils/errors"
import {
  isRenderableOCRBlock,
  mapTranslatedLinesToBlocks,
  formatFileSize,
  hasRTLText,
  hasVerticalTypography,
  getPipelineHelpMessage,
  mapBackendImageStage,
  mapBackendStageNarrative,
  mapBackendReliabilityHint,
} from "./utils/ocrPipeline"
import { MOTION } from "./motion"
import {
  LANGUAGES,
  PRODUCT_MODE_BACKEND_MAP,
  MAX_CHARS,
  FIRST_VISIT_KEY,
  DEMO_PRESETS,
  IMAGE_PROGRESS_STAGES,
  type ProductMode,
  type InputWorkflow,
  type UploadSelection,
  type ComparisonFocus,
  type ProgressStage,
  type OverlayStage,
  type LiveProgressDetail,
} from "./constants"
import "./App.css"
import { Header, WorkflowSwitcher, UploadHero, ImageProgress, IntelligencePanel, PipelineWarning, DemoGallery, HistoryPanel, ExportActions, ComparisonView, ReliabilityTelemetry } from "./components"

function App() {
  const [theme, setTheme] = useTheme()
  const { toasts, push: pushToast } = useToasts()
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
  const [imageProgressStage, setImageProgressStage] = useState<ProgressStage | null>(null)
  const [imageProgressCompleted, setImageProgressCompleted] = useState<ProgressStage[]>([])
  const [imageProgressFailedStage, setImageProgressFailedStage] = useState<ProgressStage | null>(null)
  const [imageProgressStatus, setImageProgressStatus] = useState<"idle" | "running" | "done" | "error">("idle")
  const [translationRegionIndex, setTranslationRegionIndex] = useState(0)
  const [translationRegionTotal, setTranslationRegionTotal] = useState(0)
  const [liveChunkProgress, setLiveChunkProgress] = useState<number | null>(null)
  const [livePhaseLabel, setLivePhaseLabel] = useState<string | null>(null)
  const [reliabilityHint, setReliabilityHint] = useState<string | null>(null)
  const [compareRevealActive, setCompareRevealActive] = useState(false)
  const [ocrConfidence, setOcrConfidence] = useState<number | null>(null)
  const [overlayBlocks, setOverlayBlocks] = useState<TextBlock[]>([])
  const [overlayTexts, setOverlayTexts] = useState<string[]>([])
  const [overlayStage, setOverlayStage] = useState<OverlayStage>("idle")
  const [overlayImageSize, setOverlayImageSize] = useState<{ width: number; height: number } | null>(null)
  const [overlayLabelSwapActive, setOverlayLabelSwapActive] = useState(false)
  const [overlayDemoRunning, setOverlayDemoRunning] = useState(false)
  const [overlayDemoPhase, setOverlayDemoPhase] = useState<"ocr" | "translation" | "rendering" | null>(null)
  const [demoModeActive, setDemoModeActive] = useState(false)
  const [demoModeIndex, setDemoModeIndex] = useState(0)
  const [detectedLang, setDetectedLang] = useState("")
  const [pipelineWarning, setPipelineWarning] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [ocrLoading, setOcrLoading] = useState(false)
  const [pdfLoading, setPdfLoading] = useState(false)
  const [showHistory, setShowHistory] = useState(false)
  const [showPdfJobs, setShowPdfJobs] = useState(false)
  const [showReliability, setShowReliability] = useState(false)
  const [showDemoGallery, setShowDemoGallery] = useState(false)
  const [showcaseMode, setShowcaseMode] = useState(false)
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
  const imageProgressTimersRef = useRef<number[]>([])
  const translationTickerRef = useRef<number | null>(null)
  const reliabilityHintTimersRef = useRef<number[]>([])
  const overlaySwapTimerRef = useRef<number | null>(null)
  const overlayDemoTimersRef = useRef<number[]>([])
  const compareFlashTimerRef = useRef<number | null>(null)
  const modalFlashTimerRef = useRef<number | null>(null)
  const demoModeTimerRef = useRef<number | null>(null)
  const compareRevealKeyRef = useRef<string | null>(null)
  const backendImageProgressActiveRef = useRef(false)
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

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null
      const inEditable = Boolean(
        target && (
          target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.tagName === "SELECT" ||
          target.isContentEditable
        )
      )
      if (e.key === "Escape" && comparisonModalOpen) {
        closeModal()
        return
      }
      if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
        if (!inEditable || workflow === "text") {
          handleTranslate()
        }
        return
      }
      if (!comparisonModalOpen || inEditable) return

      if (e.key === "+" || e.key === "=") {
        e.preventDefault()
        adjustComparisonModalZoom(0.16)
        return
      }
      if (e.key === "-") {
        e.preventDefault()
        adjustComparisonModalZoom(-0.16)
        return
      }
      if (e.key === "0") {
        e.preventDefault()
        resetComparisonModalZoom()
        return
      }

      if (comparisonModalView === "slider") {
        if (e.key === "ArrowLeft") {
          e.preventDefault()
          setModalSliderPercent2(prev => Math.max(0, prev - 4))
          return
        }
        if (e.key === "ArrowRight") {
          e.preventDefault()
          setModalSliderPercent2(prev => Math.min(100, prev + 4))
          return
        }
      }

      if (e.key.toLowerCase() === "f") {
        e.preventDefault()
        setModalView2(prev => (prev === "flash" ? "slider" : "flash"))
        return
      }

      if (e.key === " ") {
        e.preventDefault()
        setComparisonQuickToggle(true)
      }
    }
    window.addEventListener("keydown", handler)
    return () => window.removeEventListener("keydown", handler)
  }, [comparisonModalOpen, comparisonModalView, handleTranslate, workflow])

  useEffect(() => {
    if (!comparisonModalOpen) return
    const release = (e: KeyboardEvent) => {
      if (e.key === " ") {
        setComparisonQuickToggle(false)
      }
    }
    window.addEventListener("keyup", release)
    return () => window.removeEventListener("keyup", release)
  }, [comparisonModalOpen])

  // Body scroll lock is managed by useComparisonModal.

  const handleCopy = async () => {
    if (!result) return
    try {
      await navigator.clipboard.writeText(result)
      pushToast("Copied to clipboard", "success")
    } catch {
      pushToast("Copy failed — check browser permissions", "error")
    }
  }

  const handleDownload = useCallback(async (url: string, basename: string) => {
    try {
      const res = await fetch(url)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const blob = await res.blob()
      const ext = blob.type === "image/jpeg" ? "jpg"
        : blob.type === "image/webp" ? "webp"
        : blob.type === "image/gif"  ? "gif"
        : "png"
      const blobUrl = URL.createObjectURL(blob)
      const a = document.createElement("a")
      a.href = blobUrl
      a.download = `${basename}.${ext}`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(blobUrl)
    } catch (err) {
      pushToast(getErrorMessage(err, "Download failed"), "error")
    }
  }, [pushToast])

  const handleDownloadPDF = useCallback(async () => {
    if (!resultImageUrl && !result.trim()) {
      pushToast("Nothing to export", "error")
      return
    }

    try {
      const { jsPDF } = await import("jspdf")
      const pdf = new jsPDF({ orientation: "portrait", unit: "pt", format: "a4" })

      if (resultImageUrl) {
        const image = await new Promise<HTMLImageElement>((resolve, reject) => {
          const img = new Image()
          img.crossOrigin = "anonymous"
          img.onload = () => resolve(img)
          img.onerror = () => reject(new Error("Failed to prepare image for PDF export"))
          img.src = resultImageUrl
        })

        const pageWidth = pdf.internal.pageSize.getWidth()
        const pageHeight = pdf.internal.pageSize.getHeight()
        const scale = Math.min((pageWidth - 48) / image.width, (pageHeight - 70) / image.height)
        const drawWidth = image.width * scale
        const drawHeight = image.height * scale
        const x = (pageWidth - drawWidth) / 2
        const y = (pageHeight - drawHeight) / 2
        pdf.addImage(image, "PNG", x, y, drawWidth, drawHeight)
      } else {
        const pageWidth = pdf.internal.pageSize.getWidth()
        const lines = pdf.splitTextToSize(result, pageWidth - 60)
        pdf.setFont("helvetica", "normal")
        pdf.setFontSize(11)
        pdf.text(lines, 30, 40)
      }

      pdf.save("loklingo-export.pdf")
      pushToast("PDF exported", "success")
    } catch (err) {
      pushToast(getErrorMessage(err, "PDF export failed"), "error")
    }
  }, [pushToast, result, resultImageUrl])

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

  const clearImageProgressTimers = useCallback(() => {
    imageProgressTimersRef.current.forEach(timer => window.clearTimeout(timer))
    imageProgressTimersRef.current = []
  }, [])

  const clearReliabilityHintTimers = useCallback(() => {
    reliabilityHintTimersRef.current.forEach(timer => window.clearTimeout(timer))
    reliabilityHintTimersRef.current = []
  }, [])

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

  const startImageProgress = useCallback(() => {
    backendImageProgressActiveRef.current = false
    clearImageProgressTimers()
    clearReliabilityHintTimers()
    setTranslationRegionIndex(0)
    setTranslationRegionTotal(0)
    setLiveChunkProgress(null)
    setLivePhaseLabel(null)
    setReliabilityHint(null)
    setImageProgressStatus("running")
    setImageProgressFailedStage(null)
    setImageProgressStage("detecting")
    setImageProgressCompleted([])

    const toLayout = window.setTimeout(() => {
      setImageProgressCompleted(["detecting"])
      setImageProgressStage("layout")
    }, MOTION.imageProgress.toLayoutMs)

    const toLanguages = window.setTimeout(() => {
      setImageProgressCompleted(["detecting", "layout"])
      setImageProgressStage("languages")
    }, MOTION.imageProgress.toLanguagesMs)

    const toTranslation = window.setTimeout(() => {
      setImageProgressCompleted(["detecting", "layout", "languages"])
      setImageProgressStage("translating")
    }, MOTION.imageProgress.toTranslationMs)

    const toTypography = window.setTimeout(() => {
      setImageProgressCompleted(["detecting", "layout", "languages", "translating"])
      setImageProgressStage("typography")
    }, MOTION.imageProgress.toTypographyMs)

    const toRendering = window.setTimeout(() => {
      setImageProgressCompleted(["detecting", "layout", "languages", "translating", "typography"])
      setImageProgressStage("rendering")
    }, MOTION.imageProgress.toRenderingMs)

    const toRecoveryHint = window.setTimeout(() => {
      setReliabilityHint("Translation engine recovering...")
    }, 3400)
    const toRetryHint = window.setTimeout(() => {
      setReliabilityHint("Retrying unstable region...")
    }, 6200)
    const toFallbackHint = window.setTimeout(() => {
      setReliabilityHint("Switching rendering strategy...")
    }, 9000)

    imageProgressTimersRef.current = [toLayout, toLanguages, toTranslation, toTypography, toRendering]
    reliabilityHintTimersRef.current = [toRecoveryHint, toRetryHint, toFallbackHint]
  }, [clearImageProgressTimers, clearReliabilityHintTimers])

  const completeImageProgress = useCallback(() => {
    clearImageProgressTimers()
    clearReliabilityHintTimers()
    setImageProgressStatus("done")
    setImageProgressFailedStage(null)
    setImageProgressCompleted(["detecting", "layout", "languages", "translating", "typography", "rendering"])
    setImageProgressStage(null)
    setLiveChunkProgress(1)
    setLivePhaseLabel("Rendering typography")
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
    
    // Auto-open comparison modal after UI settles
    const autoOpenDelay = window.setTimeout(() => {
      const hasComparison = Boolean(compareOriginalUrl && compareOverlayUrl && compareLayoutUrl)
      if (!hasComparison && !resultImageUrl) return
      
      setModalFocus(hasComparison ? "layout" : "layout")
      setModalSliderTarget2("layout")
      setModalFlashTarget2("layout")
      setModalSliderPercent2(50)
      setModalView2(hasComparison ? "slider" : "gallery")
      setComparisonQuickToggle(false)
      resetComparisonModalZoom()
      openModal()
    }, MOTION.imageProgress.successSettleMs + 300)
    
    imageProgressTimersRef.current = [settle, autoOpenDelay]
  }, [clearImageProgressTimers, clearReliabilityHintTimers, compareOriginalUrl, compareOverlayUrl, compareLayoutUrl, resultImageUrl])

  const failImageProgress = useCallback(() => {
    clearImageProgressTimers()
    clearReliabilityHintTimers()
    setImageProgressStatus("error")
    setImageProgressFailedStage(imageProgressStage)
    setLivePhaseLabel("Pipeline stabilization in progress")

    const settle = window.setTimeout(() => {
      setImageProgressStatus("idle")
      setImageProgressFailedStage(null)
      setImageProgressCompleted([])
      setImageProgressStage(null)
      setTranslationRegionIndex(0)
      setTranslationRegionTotal(0)
      setLiveChunkProgress(null)
      setLivePhaseLabel(null)
    }, MOTION.imageProgress.errorSettleMs)
    imageProgressTimersRef.current = [settle]
  }, [clearImageProgressTimers, clearReliabilityHintTimers, imageProgressStage])

  const handleImageJobProgress = useCallback((job: JobProgressUpdate) => {
    if (!backendImageProgressActiveRef.current) {
      backendImageProgressActiveRef.current = true
      clearImageProgressTimers()
      clearReliabilityHintTimers()
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
  }, [clearImageProgressTimers, clearReliabilityHintTimers])

  useEffect(() => {
    return () => {
      clearImageProgressTimers()
    }
  }, [clearImageProgressTimers])

  useEffect(() => {
    return () => {
      clearReliabilityHintTimers()
    }
  }, [clearReliabilityHintTimers])

  useEffect(() => {
    if (translationTickerRef.current !== null) {
      window.clearInterval(translationTickerRef.current)
      translationTickerRef.current = null
    }

    if (!ocrLoading || imageProgressStatus !== "running" || imageProgressStage !== "translating") {
      return
    }

    const total = overlayBlocks.length
    if (total <= 0) return

    // eslint-disable-next-line react-hooks/set-state-in-effect
    setTranslationRegionTotal(total)
    setLivePhaseLabel("Preserving layout geometry")
    translationTickerRef.current = window.setInterval(() => {
      setTranslationRegionIndex(prev => {
        if (prev >= total) return total
        const next = prev + 1
        setLiveChunkProgress(next / total)
        return next
      })
    }, 190)

    return () => {
      if (translationTickerRef.current !== null) {
        window.clearInterval(translationTickerRef.current)
        translationTickerRef.current = null
      }
    }
  }, [imageProgressStage, imageProgressStatus, ocrLoading, overlayBlocks.length])

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
      if (demoModeTimerRef.current !== null) {
        window.clearTimeout(demoModeTimerRef.current)
      }
      if (translationTickerRef.current !== null) {
        window.clearInterval(translationTickerRef.current)
      }
    }
  }, [])

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null
      const inEditable = Boolean(
        target && (
          target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.tagName === "SELECT" ||
          target.isContentEditable
        )
      )
      if (inEditable) return
      if (event.shiftKey && !event.ctrlKey && !event.metaKey && !event.altKey && event.key.toLowerCase() === "d") {
        event.preventDefault()
        runOverlayDemo()
      }
    }
    window.addEventListener("keydown", handler)
    return () => window.removeEventListener("keydown", handler)
  }, [runOverlayDemo])

  const hasModalComparison = Boolean(compareOriginalUrl && compareOverlayUrl && compareLayoutUrl)

  // Keyboard shortcuts for comparison modal modes
  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (!comparisonModalOpen) return
      const target = event.target as HTMLElement | null
      const inEditable = Boolean(
        target && (
          target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.tagName === "SELECT" ||
          target.isContentEditable
        )
      )
      if (inEditable) return

      const key = event.key.toLowerCase()

      // Comparison mode shortcuts (only when modal is open)
      if (!event.ctrlKey && !event.metaKey && !event.shiftKey && !event.altKey) {
        if (key === "s") {
          event.preventDefault()
          if (hasModalComparison) setModalView2("slider")
        } else if (key === "g") {
          event.preventDefault()
          if (hasModalComparison) setModalView2("gallery")
        } else if (key === "f") {
          event.preventDefault()
          if (hasModalComparison) setModalView2("flash")
        } else if (key === "o") {
          event.preventDefault()
          setModalFocus("original")
          setComparisonQuickToggle(false)
        } else if (key === "v") {
          event.preventDefault()
          setModalFocus("overlay")
          setComparisonQuickToggle(false)
        } else if (key === "l") {
          event.preventDefault()
          setModalFocus("layout")
          setComparisonQuickToggle(false)
        } else if (key === "+" || key === "=") {
          event.preventDefault()
          adjustComparisonModalZoom(0.2)
        } else if (key === "-" || key === "_") {
          event.preventDefault()
          adjustComparisonModalZoom(-0.2)
        } else if (key === "0") {
          event.preventDefault()
          resetComparisonModalZoom()
        } else if (key === "escape") {
          event.preventDefault()
          closeComparisonModal()
        }
      }
    }
    window.addEventListener("keydown", handler)
    return () => window.removeEventListener("keydown", handler)
  }, [comparisonModalOpen, hasModalComparison, closeComparisonModal])

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
  ])

  // First-visit onboarding: Auto-load demo for new users
  useEffect(() => {
    const isFirstVisit = !localStorage.getItem(FIRST_VISIT_KEY)
    if (isFirstVisit) {
      localStorage.setItem(FIRST_VISIT_KEY, "true")
      // Auto-load first demo preset (manga) after a short delay to avoid jarring UX
      const demoTimer = window.setTimeout(() => {
        const demoPreset = DEMO_PRESETS[0]
        setWorkflow("image")
        setSourceLang(demoPreset.source)
        setTargetLang(demoPreset.target)
        
        // Load and process the demo image
        fetch(demoPreset.src)
          .then(res => res.blob())
          .then(blob => {
            const file = new File([blob], `demo-${demoPreset.id}.png`, { type: blob.type })
            runImageFile(file, demoPreset.source, demoPreset.target)
          })
          .catch(() => {
            // Silently fail - demo is optional
          })
      }, 800)

      return () => window.clearTimeout(demoTimer)
    }
  }, [])

  // Showcase mode: Auto-cycle through demos
  useEffect(() => {
    if (!showcaseMode) return

    let currentIndex = 0
    let timerId: number | null = null

    const loadNextDemo = () => {
      const preset = DEMO_PRESETS[currentIndex % DEMO_PRESETS.length]
      setWorkflow("image")
      setSourceLang(preset.source)
      setTargetLang(preset.target)

      fetch(preset.src)
        .then(res => res.blob())
        .then(blob => {
          const file = new File([blob], `demo-${preset.id}.png`, { type: blob.type })
          runImageFile(file, preset.source, preset.target)
          currentIndex++
          
          // Schedule next demo after processing + 6 seconds display time
          timerId = window.setTimeout(loadNextDemo, MOTION.imageProgress.successSettleMs + 6000)
        })
        .catch(() => {
          // Skip to next demo on error
          currentIndex++
          timerId = window.setTimeout(loadNextDemo, 2000)
        })
    }

    // Start showcase after 1 second
    const startTimer = window.setTimeout(loadNextDemo, 1000)

    return () => {
      window.clearTimeout(startTimer)
      if (timerId !== null) window.clearTimeout(timerId)
    }
  }, [showcaseMode])

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

  useEffect(() => {
    if (!demoModeActive || ocrLoading || pdfLoading || loading) {
      if (demoModeTimerRef.current !== null) {
        window.clearTimeout(demoModeTimerRef.current)
        demoModeTimerRef.current = null
      }
      return
    }

    const preset = DEMO_PRESETS[demoModeIndex % DEMO_PRESETS.length]
    const kickoff = window.setTimeout(() => {
      const cycle = demoModeIndex % 3
      if (cycle === 0) {
        setCompareView("side")
      } else if (cycle === 1) {
        setSliderTarget(demoModeIndex % 2 === 0 ? "layout" : "overlay")
        setCompareView("slider")
      } else {
        setSliderTarget(demoModeIndex % 2 === 0 ? "layout" : "overlay")
        setCompareView("slider")
      }
      void handleDemoPreset(preset)
    }, 0)
    demoModeTimerRef.current = window.setTimeout(() => {
      setDemoModeIndex(prev => (prev + 1) % DEMO_PRESETS.length)
    }, MOTION.demoCycleMs)

    return () => {
      window.clearTimeout(kickoff)
      if (demoModeTimerRef.current !== null) {
        window.clearTimeout(demoModeTimerRef.current)
        demoModeTimerRef.current = null
      }
    }
  }, [demoModeActive, demoModeIndex, handleDemoPreset, loading, ocrLoading, pdfLoading])

  const handleOCRFile = async (file: File) => {
    setPipelineWarning(null)
    if (demoModeActive) {
      setDemoModeActive(false)
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
        onToggleShowcase={() => setShowcaseMode(s => !s)}
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
          onToggleDemoMode={() => setDemoModeActive(prev => !prev)}
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
              onDownloadPDF={handleDownloadPDF}
              onCopy={handleCopy}
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
