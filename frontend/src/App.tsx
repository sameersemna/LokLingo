import { useCallback, useEffect, useRef, useState } from "react"
import { translate, translateImage, uploadPDF } from "./api/translate"
import { getReadiness, type ReadinessResponse } from "./api/health"
import { getOCRMetrics, type MetricsWindow, type OCRMetricsResponse } from "./api/metrics"
import { PdfJobsPanel } from "./PdfJobsPanel"
import { DeadLetterOpsPanel } from "./DeadLetterOpsPanel"
import { saveStoredJob } from "./pdfJobsStorage"
import "./App.css"

const LANGUAGES = [
  { code: "auto", label: "Auto-detect" },
  { code: "en",   label: "English"     },
  { code: "de",   label: "German"      },
  { code: "fr",   label: "French"      },
  { code: "hi",   label: "Hindi"       },
  { code: "ur",   label: "Urdu"        },
  { code: "ar",   label: "Arabic"      },
  { code: "bn",   label: "Bengali"     },
  { code: "es",   label: "Spanish"     },
  { code: "zh",   label: "Chinese"     },
  { code: "ja",   label: "Japanese"    },
]

const TARGET_LANGUAGES = LANGUAGES.filter(l => l.code !== "auto")
const TRANSLATION_MODES = [
  { value: "overlay", label: "Balanced" },
  { value: "layout", label: "Keep layout" },
  { value: "ocr_only", label: "Extract text" },
] as const
const WORKFLOWS = [
  { value: "image", label: "Image", detail: "Hero workflow for instant visual translation", icon: "🖼" },
  { value: "pdf", label: "PDF", detail: "Queue full-document translation in background", icon: "📄" },
  { value: "text", label: "Text", detail: "Translate pasted or typed text", icon: "✍" },
] as const
const MAX_CHARS = 2000
const HISTORY_KEY = "loklingo-history"
const MAX_HISTORY = 10
const RELIABILITY_WINDOWS: MetricsWindow[] = ["1h", "6h", "24h", "7d", "30d"]
const PRESSURE_WARN_THRESHOLD = Number(import.meta.env.VITE_RELIABILITY_PRESSURE_WARN ?? 8)
const PRESSURE_CRITICAL_THRESHOLD = Number(import.meta.env.VITE_RELIABILITY_PRESSURE_CRITICAL ?? 20)

const DEMO_PRESETS = [
  { id: "cjk",   label: "CJK text",    emoji: "🈳", src: "/samples/cjk_vertical.png",       source: "auto", target: "en" },
  { id: "bold",  label: "Styled sign",  emoji: "🖋", src: "/samples/before_bold_style.png",  source: "en",   target: "de" },
  { id: "mixed", label: "Mixed styles", emoji: "🎨", src: "/samples/before_mixed_styles.png", source: "en",   target: "fr" },
  { id: "real",  label: "Real-world",   emoji: "📸", src: "/samples/realworld_overlay.jpg",   source: "auto", target: "en" },
] as const

interface Toast { id: number; msg: string; type: "success" | "error" }
interface HistoryEntry {
  id: number
  sourceText: string
  sourceLang: string
  targetLang: string
  result: string
  ts: number
}

type InputWorkflow = "image" | "pdf" | "text"

interface UploadSelection {
  kind: "image" | "pdf"
  name: string
  size: number
  previewUrl?: string
}

type ComparisonFocus = "original" | "overlay" | "layout"
type ProgressStage = "ocr" | "translation" | "rendering"

const IMAGE_PROGRESS_STAGES: Array<{ key: ProgressStage; label: string }> = [
  { key: "ocr", label: "OCR" },
  { key: "translation", label: "Translation" },
  { key: "rendering", label: "Rendering" },
]

function loadHistory(): HistoryEntry[] {
  try { return JSON.parse(localStorage.getItem(HISTORY_KEY) ?? "[]") } catch { return [] }
}
function saveHistory(h: HistoryEntry[]) {
  localStorage.setItem(HISTORY_KEY, JSON.stringify(h.slice(0, MAX_HISTORY)))
}

const LANG_KEY = "loklingo-langs"

function loadLangs() {
  try {
    const saved = JSON.parse(localStorage.getItem(LANG_KEY) ?? "{}")
    const src = LANGUAGES.find(l => l.code === saved.source)?.code ?? "auto"
    const tgt = TARGET_LANGUAGES.find(l => l.code === saved.target)?.code ?? "de"
    return { source: src, target: tgt }
  } catch { return { source: "auto", target: "de" } }
}

function pressureLevel(score: number): "normal" | "warn" | "critical" {
  if (score >= PRESSURE_CRITICAL_THRESHOLD) return "critical"
  if (score >= PRESSURE_WARN_THRESHOLD) return "warn"
  return "normal"
}

function formatFileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B"
  if (bytes < 1024) return `${bytes} B`
  const kb = bytes / 1024
  if (kb < 1024) return `${kb.toFixed(kb >= 100 ? 0 : 1)} KB`
  const mb = kb / 1024
  return `${mb.toFixed(mb >= 100 ? 0 : 1)} MB`
}

function App() {
  const [theme, setTheme] = useState<"light" | "dark">(() => {
    const saved = localStorage.getItem("loklingo-theme")
    if (saved === "light" || saved === "dark") return saved
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"
  })
  const [sourceText, setSourceText] = useState("")
  const [sourceLang, setSourceLang] = useState(() => loadLangs().source)
  const [targetLang, setTargetLang] = useState(() => loadLangs().target)
  const [mode, setMode] = useState<string>("overlay")
  const [workflow, setWorkflow] = useState<InputWorkflow>("image")
  const [uploadSelection, setUploadSelection] = useState<UploadSelection | null>(null)
  const [uploadDragActive, setUploadDragActive] = useState(false)
  const [result, setResult] = useState("")
  const [resultKind, setResultKind] = useState<"translation" | "ocr">("translation")
  const [resultImageUrl, setResultImageUrl] = useState<string | null>(null)
  const [compareOriginalUrl, setCompareOriginalUrl] = useState<string | null>(null)
  const [compareOverlayUrl, setCompareOverlayUrl] = useState<string | null>(null)
  const [compareLayoutUrl, setCompareLayoutUrl] = useState<string | null>(null)
  const [compareView, setCompareView] = useState<"side" | "slider">("side")
  const [sliderTarget, setSliderTarget] = useState<"overlay" | "layout">("layout")
  const [sliderPercent, setSliderPercent] = useState(50)
  const [comparisonModalOpen, setComparisonModalOpen] = useState(false)
  const [comparisonModalFocus, setComparisonModalFocus] = useState<ComparisonFocus>("layout")
  const [comparisonModalView, setComparisonModalView] = useState<"gallery" | "slider">("gallery")
  const [comparisonModalSliderTarget, setComparisonModalSliderTarget] = useState<"overlay" | "layout">("layout")
  const [comparisonModalSliderPercent, setComparisonModalSliderPercent] = useState(50)
  const [comparisonModalZoom, setComparisonModalZoom] = useState(1)
  const [comparisonModalPan, setComparisonModalPan] = useState({ x: 0, y: 0 })
  const [comparisonModalPanning, setComparisonModalPanning] = useState(false)
  const [imageProgressStage, setImageProgressStage] = useState<ProgressStage | null>(null)
  const [imageProgressCompleted, setImageProgressCompleted] = useState<ProgressStage[]>([])
  const [imageProgressStatus, setImageProgressStatus] = useState<"idle" | "running" | "done" | "error">("idle")
  const [detectedLang, setDetectedLang] = useState("")
  const [loading, setLoading] = useState(false)
  const [ocrLoading, setOcrLoading] = useState(false)
  const [pdfLoading, setPdfLoading] = useState(false)
  const [showHistory, setShowHistory] = useState(false)
  const [showPdfJobs, setShowPdfJobs] = useState(false)
  const [showReliability, setShowReliability] = useState(false)
  const [showDeadOps, setShowDeadOps] = useState(false)
  const [pdfJobsVersion, setPdfJobsVersion] = useState(0)
  const [metricsWindow, setMetricsWindow] = useState<MetricsWindow>("24h")
  const [metrics, setMetrics] = useState<OCRMetricsResponse | null>(null)
  const [metricsLoading, setMetricsLoading] = useState(false)
  const [metricsError, setMetricsError] = useState<string | null>(null)
  const [metricsUpdatedAt, setMetricsUpdatedAt] = useState<number | null>(null)
  const [history, setHistory] = useState<HistoryEntry[]>(loadHistory)
  const [toasts, setToasts] = useState<Toast[]>([])
  const [readiness, setReadiness] = useState<ReadinessResponse | null>(null)
  const [showStatusDetail, setShowStatusDetail] = useState(false)
  const [readinessMetrics, setReadinessMetrics] = useState<OCRMetricsResponse | null>(null)
  const [readinessMetricsLoading, setReadinessMetricsLoading] = useState(false)
  const [readinessMetricsError, setReadinessMetricsError] = useState<string | null>(null)
  const [readinessPressureDelta, setReadinessPressureDelta] = useState(0)
  const [readinessPressureTrend, setReadinessPressureTrend] = useState<"up" | "down" | "flat">("flat")
  const [readinessPressureSeries, setReadinessPressureSeries] = useState<number[]>([])
  const [readinessPressureLevel, setReadinessPressureLevel] = useState<"normal" | "warn" | "critical">("normal")
  const readinessPressureRef = useRef<number | null>(null)
  const toastId = useRef(0)
  const histId = useRef(history.length)
  const fileRef = useRef<HTMLInputElement>(null)
  const chipRef = useRef<HTMLDivElement>(null)
  const compareSectionRef = useRef<HTMLElement>(null)
  const modalPanOriginRef = useRef<{ pointerX: number; pointerY: number; panX: number; panY: number } | null>(null)
  const imageProgressTimersRef = useRef<number[]>([])

  useEffect(() => {
    return () => {
      if (compareOriginalUrl) URL.revokeObjectURL(compareOriginalUrl)
    }
  }, [compareOriginalUrl])

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
    document.documentElement.setAttribute("data-theme", theme)
    localStorage.setItem("loklingo-theme", theme)
  }, [theme])

  useEffect(() => {
    localStorage.setItem(LANG_KEY, JSON.stringify({ source: sourceLang, target: targetLang }))
  }, [sourceLang, targetLang])

  useEffect(() => {
    let cancelled = false
    const check = () => getReadiness().then(r => { if (!cancelled) setReadiness(r) }).catch(() => {})
    check()
    const interval = setInterval(check, 30_000)
    return () => { cancelled = true; clearInterval(interval) }
  }, [])

  useEffect(() => {
    if (!showStatusDetail) return
    const handler = (e: MouseEvent) => {
      if (chipRef.current && !chipRef.current.contains(e.target as Node)) {
        setShowStatusDetail(false)
      }
    }
    document.addEventListener("mousedown", handler)
    return () => document.removeEventListener("mousedown", handler)
  }, [showStatusDetail])

  useEffect(() => {
    if (!showStatusDetail) return

    let cancelled = false
    const fetchPopoverMetrics = async () => {
      setReadinessMetricsLoading(true)
      try {
        const data = await getOCRMetrics("1h")
        if (cancelled) return

        const nextPressure =
          data.reliability.litellm.retry_attempts_total +
          data.reliability.litellm.circuit_opened_total +
          data.reliability.ocr.retry_attempts_total +
          data.reliability.ocr.response_rejected_total
        const prevPressure = readinessPressureRef.current
        if (prevPressure === null) {
          setReadinessPressureDelta(0)
          setReadinessPressureTrend("flat")
        } else {
          const delta = nextPressure - prevPressure
          setReadinessPressureDelta(delta)
          setReadinessPressureTrend(delta > 0 ? "up" : delta < 0 ? "down" : "flat")
        }
        readinessPressureRef.current = nextPressure
        setReadinessPressureSeries(prev => [...prev.slice(-11), nextPressure])
        setReadinessPressureLevel(pressureLevel(nextPressure))

        setReadinessMetrics(data)
        setReadinessMetricsError(null)
      } catch (err) {
        if (cancelled) return
        setReadinessMetricsError(err instanceof Error ? err.message : "Failed to load reliability summary")
      } finally {
        if (!cancelled) {
          setReadinessMetricsLoading(false)
        }
      }
    }

    fetchPopoverMetrics()
    const interval = setInterval(fetchPopoverMetrics, 60_000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [showStatusDetail])

  useEffect(() => {
    if (!showReliability) return

    let cancelled = false
    const fetchMetrics = async () => {
      setMetricsLoading(true)
      try {
        const data = await getOCRMetrics(metricsWindow)
        if (cancelled) return
        setMetrics(data)
        setMetricsError(null)
        setMetricsUpdatedAt(Date.now())
      } catch (err) {
        if (cancelled) return
        setMetricsError(err instanceof Error ? err.message : "Failed to load reliability metrics")
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

  const pushToast = useCallback((msg: string, type: "success" | "error") => {
    const id = ++toastId.current
    setToasts(t => [...t, { id, msg, type }])
    setTimeout(() => setToasts(t => t.filter(x => x.id !== id)), 3500)
  }, [])

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
      const textMode = mode === "ocr_only" ? "overlay" : mode
      const res = await translate({ text: sourceText, source: sourceLang, target: targetLang, mode: textMode })
      setResult(res.translated_text)
      setResultKind("translation")
      setResultImageUrl(res.image_url ?? null)
      // If auto-detect was used, reflect what the backend resolved it to
      if (sourceLang === "auto" && res.source && res.source !== "auto") {
        setDetectedLang(res.source)
      }
      const entry: HistoryEntry = {
        id: ++histId.current,
        sourceText,
        sourceLang,
        targetLang,
        result: res.translated_text,
        ts: Date.now(),
      }
      setHistory(h => { const next = [entry, ...h].slice(0, MAX_HISTORY); saveHistory(next); return next })
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "Translation failed", "error")
    } finally {
      setLoading(false)
    }
  }, [sourceText, sourceLang, targetLang, mode, pushToast, compareOriginalUrl])

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && comparisonModalOpen) {
        setComparisonModalOpen(false)
        return
      }
      if ((e.ctrlKey || e.metaKey) && e.key === "Enter") handleTranslate()
    }
    window.addEventListener("keydown", handler)
    return () => window.removeEventListener("keydown", handler)
  }, [handleTranslate, comparisonModalOpen])

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
      pushToast(err instanceof Error ? err.message : "Download failed", "error")
    }
  }, [pushToast])

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
    setComparisonModalFocus(hasComparison ? focus : "layout")
    setComparisonModalSliderTarget(focus === "overlay" ? "overlay" : "layout")
    setComparisonModalSliderPercent(50)
    setComparisonModalView(hasComparison ? "slider" : "gallery")
    setComparisonModalZoom(1)
    setComparisonModalPan({ x: 0, y: 0 })
    setComparisonModalPanning(false)
    setComparisonModalOpen(true)
  }, [compareOriginalUrl, compareOverlayUrl, compareLayoutUrl, resultImageUrl])

  const closeComparisonModal = () => {
    setComparisonModalPanning(false)
    setComparisonModalOpen(false)
  }

  const clearImageProgressTimers = useCallback(() => {
    imageProgressTimersRef.current.forEach(timer => window.clearTimeout(timer))
    imageProgressTimersRef.current = []
  }, [])

  const startImageProgress = useCallback(() => {
    clearImageProgressTimers()
    setImageProgressStatus("running")
    setImageProgressStage("ocr")
    setImageProgressCompleted([])

    const toTranslation = window.setTimeout(() => {
      setImageProgressCompleted(["ocr"])
      setImageProgressStage("translation")
    }, 900)

    const toRendering = window.setTimeout(() => {
      setImageProgressCompleted(["ocr", "translation"])
      setImageProgressStage("rendering")
    }, 2200)

    imageProgressTimersRef.current = [toTranslation, toRendering]
  }, [clearImageProgressTimers])

  const completeImageProgress = useCallback(() => {
    clearImageProgressTimers()
    setImageProgressStatus("done")
    setImageProgressCompleted(["ocr", "translation", "rendering"])
    setImageProgressStage(null)

    const settle = window.setTimeout(() => {
      setImageProgressStatus("idle")
      setImageProgressCompleted([])
      setImageProgressStage(null)
    }, 3600)
    imageProgressTimersRef.current = [settle]
  }, [clearImageProgressTimers])

  const failImageProgress = useCallback(() => {
    clearImageProgressTimers()
    setImageProgressStatus("error")
    setImageProgressStage(null)

    const settle = window.setTimeout(() => {
      setImageProgressStatus("idle")
      setImageProgressCompleted([])
      setImageProgressStage(null)
    }, 4200)
    imageProgressTimersRef.current = [settle]
  }, [clearImageProgressTimers])

  useEffect(() => {
    return () => {
      clearImageProgressTimers()
    }
  }, [clearImageProgressTimers])

  const hasModalComparison = Boolean(compareOriginalUrl && compareOverlayUrl && compareLayoutUrl)
  const modalImageSrc =
    comparisonModalFocus === "original"
      ? compareOriginalUrl
      : comparisonModalFocus === "overlay"
      ? compareOverlayUrl
      : compareLayoutUrl ?? resultImageUrl
  const clampModalZoom = (value: number) => Math.max(0.6, Math.min(3, Number(value.toFixed(2))))
  const clampModalPan = (x: number, y: number, zoom: number) => {
    const maxOffset = Math.max(0, (zoom - 1) * 520)
    return {
      x: Math.max(-maxOffset, Math.min(maxOffset, x)),
      y: Math.max(-maxOffset, Math.min(maxOffset, y)),
    }
  }
  const modalTransform = `translate(${comparisonModalPan.x}px, ${comparisonModalPan.y}px) scale(${comparisonModalZoom})`

  const adjustComparisonModalZoom = (delta: number) => {
    const next = clampModalZoom(comparisonModalZoom + delta)
    setComparisonModalZoom(next)
    if (next <= 1) {
      setComparisonModalPan({ x: 0, y: 0 })
      setComparisonModalPanning(false)
      modalPanOriginRef.current = null
    }
  }

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
    setComparisonModalPanning(true)
  }

  const handleComparisonModalPointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!modalPanOriginRef.current || !comparisonModalPanning) return
    const dx = event.clientX - modalPanOriginRef.current.pointerX
    const dy = event.clientY - modalPanOriginRef.current.pointerY
    setComparisonModalPan(clampModalPan(modalPanOriginRef.current.panX + dx, modalPanOriginRef.current.panY + dy, comparisonModalZoom))
  }

  const handleComparisonModalPointerUp = (event: React.PointerEvent<HTMLDivElement>) => {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    modalPanOriginRef.current = null
    setComparisonModalPanning(false)
  }

  const runImageFile = async (file: File, src: string, tgt: string) => {
    setOcrLoading(true)
    startImageProgress()
    try {
      const originalUrl = URL.createObjectURL(file)
      if (compareOriginalUrl) URL.revokeObjectURL(compareOriginalUrl)
      setCompareOriginalUrl(originalUrl)

      if (mode === "ocr_only") {
        const ocrRes = await translateImage(file, src, tgt, "ocr_only")
        setResult(ocrRes.translated_text)
        setResultKind("ocr")
        setResultImageUrl(ocrRes.image_url ?? null)
        setCompareOverlayUrl(null)
        setCompareLayoutUrl(null)
        if (src === "auto" && ocrRes.source && ocrRes.source !== "auto") {
          setDetectedLang(ocrRes.source)
        }
        completeImageProgress()
        pushToast("OCR extracted", "success")
        return
      }

      const overlayRes = await translateImage(file, src, tgt, "overlay")
      const layoutRes = await translateImage(file, src, tgt, "layout")

      if (!overlayRes.image_url || !layoutRes.image_url) {
        throw new Error("Image comparison requires both overlay and layout outputs")
      }

      const activeRes = mode === "layout" ? layoutRes : overlayRes
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
      completeImageProgress()
      pushToast("Image translated", "success")
    } catch (err) {
      failImageProgress()
      pushToast(err instanceof Error ? err.message : "Image translation failed", "error")
    } finally {
      setOcrLoading(false)
      if (fileRef.current) {
        fileRef.current.value = ""
        fileRef.current.accept = "image/*,application/pdf"
      }
    }
  }

  const handleDemoPreset = async (preset: typeof DEMO_PRESETS[number]) => {
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
      pushToast(err instanceof Error ? err.message : "Demo preset failed", "error")
    }
  }

  const handleOCRFile = async (file: File) => {
    // PDF files → async translate_pdf job (enqueue + track in PDF Jobs panel)
    if (file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf')) {
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
        const { job_id } = await uploadPDF(file, sourceLang, targetLang, mode)
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
        pushToast(err instanceof Error ? err.message : 'PDF upload failed', 'error')
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

  return (
    <div className="app">
      <header className="app-header">
        <div className="header-row">
          <h1>LokLingo</h1>
          <div className="header-actions">
            <button
              className={`icon-btn${showPdfJobs ? " is-active" : ""}`}
              type="button"
              onClick={() => { setShowPdfJobs(p => !p); setShowHistory(false) }}
              aria-pressed={showPdfJobs}
              title="PDF translation jobs"
            >
              📄 PDF Jobs
            </button>
            <button
              className={`icon-btn${showReliability ? " is-active" : ""}`}
              type="button"
              onClick={() => setShowReliability(v => !v)}
              aria-pressed={showReliability}
              title="Reliability telemetry"
            >
              📊 Reliability
            </button>
            <button
              className={`icon-btn${showDeadOps ? " is-active" : ""}`}
              type="button"
              onClick={() => setShowDeadOps(v => !v)}
              aria-pressed={showDeadOps}
              title="Dead-letter operations"
            >
              🛠 Dead Ops
            </button>
            <button
              className={`icon-btn${showHistory ? " is-active" : ""}`}
              type="button"
              onClick={() => { setShowHistory(h => !h); setShowPdfJobs(false) }}
              aria-pressed={showHistory}
              title="Translation history"
            >
              ⏱ History {history.length > 0 && <span className="badge">{history.length}</span>}
            </button>
            <button
              className="theme-btn"
              type="button"
              aria-label="Toggle color scheme"
              aria-pressed={theme === "dark"}
              onClick={() => setTheme(t => t === "dark" ? "light" : "dark")}
            >
              {theme === "dark" ? "☀ Light" : "☾ Dark"}
            </button>
          </div>
        </div>
        <p className="tagline">Self-hosted translation &mdash; no limits</p>
        <div className="readiness-chip-wrap" ref={chipRef}>
          <button
            type="button"
            className={`readiness-chip readiness-${readinessState}${showStatusDetail ? " is-open" : ""}`}
            onClick={() => setShowStatusDetail(s => !s)}
            aria-expanded={showStatusDetail}
            aria-haspopup="true"
          >
            {readinessLabel} <span className="chip-caret">{showStatusDetail ? "▲" : "▼"}</span>
          </button>
          {showStatusDetail && (
            <div className="readiness-popover" role="status">
              <div className="readiness-popover-title">Dependency status</div>
              {readiness === null ? (
                <p className="readiness-popover-checking">Fetching…</p>
              ) : (
                <>
                  <ul className="readiness-dep-list">
                    {Object.entries(readiness.dependencies)
                      .sort(([a], [b]) => a.localeCompare(b))
                      .map(([name, dep]) => (
                        <li key={name} className={`readiness-dep-item dep-${dep.status}`}>
                          <span className="dep-dot" />
                          <span className="dep-name">{name}</span>
                          {dep.status === "error" && dep.error && (
                            <span className="dep-error" title={dep.error}>
                              {dep.error.length > 60 ? dep.error.slice(0, 57) + "…" : dep.error}
                            </span>
                          )}
                        </li>
                      ))}
                  </ul>

                  <div className="readiness-mini-reliability">
                    <div className="mini-rel-header">
                      <div className="mini-rel-title">Reliability (1h)</div>
                      {readinessMetrics && (
                        <span className={`mini-rel-trend mini-rel-trend-${readinessPressureTrend} mini-rel-level-${readinessPressureLevel}`}>
                          {readinessPressureTrend === "up" ? "▲" : readinessPressureTrend === "down" ? "▼" : "■"}
                          {readinessPressureDelta === 0 ? "0" : readinessPressureDelta > 0 ? `+${readinessPressureDelta}` : `${readinessPressureDelta}`} {readinessPressureLabel}
                        </span>
                      )}
                    </div>
                    {readinessMetricsLoading && <p className="mini-rel-state">Loading...</p>}
                    {readinessMetricsError && <p className="mini-rel-state mini-rel-state-error">{readinessMetricsError}</p>}
                    {readinessMetrics && !readinessMetricsLoading && !readinessMetricsError && (
                      <>
                        {miniSparklinePoints && (
                          <div className={`mini-rel-sparkline-wrap mini-rel-level-${readinessPressureLevel}`} aria-hidden="true">
                            <svg className={`mini-rel-sparkline mini-rel-level-${readinessPressureLevel}`} viewBox="0 0 120 28" preserveAspectRatio="none">
                              <polyline points={miniSparklinePoints} />
                            </svg>
                          </div>
                        )}
                        <div className="mini-rel-grid">
                          <div className="mini-rel-item">
                            <span>LiteLLM retries</span>
                            <strong>{readinessMetrics.reliability.litellm.retry_attempts_total}</strong>
                          </div>
                          <div className="mini-rel-item">
                            <span>LiteLLM circuit opens</span>
                            <strong>{readinessMetrics.reliability.litellm.circuit_opened_total}</strong>
                          </div>
                          <div className="mini-rel-item">
                            <span>OCR retries</span>
                            <strong>{readinessMetrics.reliability.ocr.retry_attempts_total}</strong>
                          </div>
                          <div className="mini-rel-item">
                            <span>OCR response rejects</span>
                            <strong>{readinessMetrics.reliability.ocr.response_rejected_total}</strong>
                          </div>
                        </div>
                        <button
                          type="button"
                          className="mini-rel-open-btn"
                          onClick={() => {
                            setShowReliability(true)
                            setShowStatusDetail(false)
                          }}
                        >
                          Open full Reliability panel
                        </button>
                      </>
                    )}
                  </div>
                </>
              )}
            </div>
          )}
        </div>
      </header>

      {/* PDF Jobs panel */}
      {showPdfJobs && (
        <PdfJobsPanel key={pdfJobsVersion} onToast={pushToast} />
      )}

      {/* Reliability telemetry panel */}
      {showReliability && (
        <section className="reliability-panel" aria-live="polite">
          <div className="reliability-header">
            <div>
              <h2>Reliability telemetry</h2>
              <p>Live counters and windowed event aggregates from /api/v1/metrics/ocr.</p>
            </div>
            <label className="reliability-window-control">
              Window
              <select value={metricsWindow} onChange={e => setMetricsWindow(e.target.value as MetricsWindow)}>
                {RELIABILITY_WINDOWS.map(w => (
                  <option key={w} value={w}>{w}</option>
                ))}
              </select>
            </label>
          </div>

          {metricsLoading && <p className="reliability-state">Loading reliability metrics...</p>}
          {metricsError && <p className="reliability-state reliability-state-error">{metricsError}</p>}

          {metrics && (
            <>
              <div className="reliability-summary-grid">
                <article className="reliability-card">
                  <h3>LiteLLM</h3>
                  <dl>
                    <div><dt>Retries</dt><dd>{metrics.reliability.litellm.retry_attempts_total}</dd></div>
                    <div><dt>Cancelled retries</dt><dd>{metrics.reliability.litellm.retry_cancelled_total}</dd></div>
                    <div><dt>Exhausted retries</dt><dd>{metrics.reliability.litellm.retry_exhausted_total}</dd></div>
                    <div><dt>Circuit opened</dt><dd>{metrics.reliability.litellm.circuit_opened_total}</dd></div>
                    <div><dt>Circuit rejects</dt><dd>{metrics.reliability.litellm.circuit_reject_total}</dd></div>
                    <div><dt>Response rejects</dt><dd>{metrics.reliability.litellm.response_rejected_total}</dd></div>
                  </dl>
                </article>

                <article className="reliability-card">
                  <h3>OCR</h3>
                  <dl>
                    <div><dt>Retries</dt><dd>{metrics.reliability.ocr.retry_attempts_total}</dd></div>
                    <div><dt>Retry-After honored</dt><dd>{metrics.reliability.ocr.retry_after_honored_total}</dd></div>
                    <div><dt>Cancelled retries</dt><dd>{metrics.reliability.ocr.retry_cancelled_total}</dd></div>
                    <div><dt>Exhausted retries</dt><dd>{metrics.reliability.ocr.retry_exhausted_total}</dd></div>
                    <div><dt>Response rejects</dt><dd>{metrics.reliability.ocr.response_rejected_total}</dd></div>
                    <div><dt>OCR events ({metrics.window.name})</dt><dd>{metrics.events_total}</dd></div>
                  </dl>
                </article>
              </div>

              <div className="reliability-windowed-table-wrap">
                <div className="reliability-windowed-title">Windowed reliability events ({metrics.window.name})</div>
                {metrics.reliability_windowed.events.length === 0 ? (
                  <p className="reliability-state">No reliability events recorded in this window.</p>
                ) : (
                  <table className="reliability-table">
                    <thead>
                      <tr>
                        <th>Integration</th>
                        <th>Event</th>
                        <th>Reason</th>
                        <th>Count</th>
                      </tr>
                    </thead>
                    <tbody>
                      {metrics.reliability_windowed.events.slice(0, 12).map((event, idx) => (
                        <tr key={`${event.integration}:${event.event_name}:${event.reason}:${idx}`}>
                          <td>{event.integration}</td>
                          <td>{event.event_name}</td>
                          <td>{event.reason || "-"}</td>
                          <td>{event.count}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              {metricsUpdatedAt && (
                <p className="reliability-updated">Updated {new Date(metricsUpdatedAt).toLocaleTimeString()}</p>
              )}
            </>
          )}
        </section>
      )}

      {showDeadOps && (
        <DeadLetterOpsPanel onToast={pushToast} />
      )}

      {/* History drawer */}
      {showHistory && (
        <section className="history-panel">
          <div className="history-header">
            <span>Recent translations</span>
            {history.length > 0 && (
              <button className="icon-btn" onClick={() => { setHistory([]); saveHistory([]) }}>
                Clear all
              </button>
            )}
          </div>
          {history.length === 0 ? (
            <p className="history-empty">No translations yet.</p>
          ) : (
            <ul className="history-list">
              {history.map(h => (
                <li key={h.id} className="history-item" onClick={() => {
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
                  setShowHistory(false)
                }}>
                  <div className="history-langs">
                    {langLabel(h.sourceLang)} → {langLabel(h.targetLang)}
                  </div>
                  <div className="history-preview">{h.sourceText.slice(0, 80)}{h.sourceText.length > 80 ? "…" : ""}</div>
                </li>
              ))}
            </ul>
          )}
        </section>
      )}

      <main className="translator">
        <div className="lang-selectors">
          <div className="lang-select-wrap">
            <select
              aria-label="Source language"
              value={sourceLang}
              onChange={e => { setSourceLang(e.target.value); setDetectedLang("") }}
            >
              {LANGUAGES.map(l => (
                <option key={l.code} value={l.code}>{l.label}</option>
              ))}
            </select>
            {detectedLang && (
              <span className="detected-badge">✓ {langLabel(detectedLang)}</span>
            )}
          </div>

          <button
            className="swap-btn"
            title={sourceLang === "auto" ? "Cannot swap when source is Auto-detect" : "Swap languages"}
            disabled={sourceLang === "auto"}
            onClick={handleSwap}
          >
            ⇄
          </button>

          <select
            aria-label="Target language"
            value={targetLang}
            onChange={e => setTargetLang(e.target.value)}
          >
            {TARGET_LANGUAGES.map(l => (
              <option key={l.code} value={l.code}>{l.label}</option>
            ))}
          </select>
        </div>

        <div className="mode-row">
          <label className="mode-label" htmlFor="translation-mode">Style</label>
          <select
            id="translation-mode"
            className="mode-select"
            aria-label="Translation mode"
            value={mode}
            onChange={e => setMode(e.target.value)}
          >
            {TRANSLATION_MODES.map(m => (
              <option key={m.value} value={m.value}>{m.label}</option>
            ))}
          </select>
        </div>
        <p className="mode-help" role="note" aria-live="polite">
          {mode === "ocr_only"
            ? "Extract text without generating a translated image."
            : "Balanced is faster, while Keep layout preserves the original structure."}
        </p>

        <input
          ref={fileRef}
          type="file"
          accept="image/*,application/pdf"
          className="ocr-input"
          id="ocr-file"
          onChange={e => e.target.files?.[0] && handleOCRFile(e.target.files[0])}
        />

        <section className="workflow-switch-wrap" aria-label="Input workflow">
          <div className="workflow-switch-heading">Choose workflow: Image, PDF, or Text</div>
          <div className="workflow-switch">
          {WORKFLOWS.map(item => (
            <button
              key={item.value}
              type="button"
              className={`workflow-card${workflow === item.value ? " is-active" : ""}${item.value === "image" ? " is-primary" : ""}`}
              onClick={() => setWorkflow(item.value)}
              aria-pressed={workflow === item.value}
            >
              <span className="workflow-card-top">
                <span className="workflow-card-icon" aria-hidden="true">{item.icon}</span>
                <span className="workflow-card-title">{item.label}</span>
                {item.value === "image" && <span className="workflow-card-badge">Primary</span>}
              </span>
              <span className="workflow-card-detail">{item.detail}</span>
            </button>
          ))}
          </div>
        </section>

        <section
          className={`upload-hero${uploadDragActive ? " is-drag-active" : ""}${workflow === "image" ? " is-image-primary" : ""}${workflow === "pdf" ? " is-pdf-focus" : ""}${workflow === "text" ? " is-text-focus" : ""}`}
          onDragEnter={e => {
            e.preventDefault()
            e.stopPropagation()
            if (!ocrLoading && !pdfLoading) setUploadDragActive(true)
          }}
          onDragOver={e => {
            e.preventDefault()
            e.stopPropagation()
            if (!ocrLoading && !pdfLoading) setUploadDragActive(true)
          }}
          onDragLeave={e => {
            e.preventDefault()
            e.stopPropagation()
            setUploadDragActive(false)
          }}
          onDrop={handleUploadDrop}
          aria-label="Upload image or PDF"
        >
          <div className="upload-hero-head">
            <h2>Image Translation</h2>
            <span className="upload-hero-pill">Primary workflow</span>
          </div>
          <p className="upload-hero-subtitle">{uploadHeroSubtitle}</p>

          {uploadSelection?.kind === "image" && uploadSelection.previewUrl ? (
            <div className="upload-preview upload-preview-image">
              <img src={uploadSelection.previewUrl} alt="Selected upload preview" />
              <div className="upload-preview-meta">
                <strong>{uploadSelection.name}</strong>
                <span>{formatFileSize(uploadSelection.size)} · Image</span>
              </div>
            </div>
          ) : uploadSelection?.kind === "pdf" ? (
            <div className="upload-preview upload-preview-pdf">
              <div className="upload-preview-file-icon" aria-hidden="true">PDF</div>
              <div className="upload-preview-meta">
                <strong>{uploadSelection.name}</strong>
                <span>{formatFileSize(uploadSelection.size)} · PDF queued for background translation</span>
              </div>
            </div>
          ) : (
            <button
              type="button"
              className={`upload-preview upload-preview-empty upload-drop-target${uploadDragActive ? " is-drag-active" : ""}`}
              disabled={ocrLoading || pdfLoading}
              onClick={() => openUploadPicker(uploadBrowseAccept)}
            >
              <strong>{uploadDragActive ? "Release to upload" : uploadEmptyTitle}</strong>
              <span>{uploadEmptySupport}</span>
            </button>
          )}

          <div className="upload-hero-actions">
            <button
              type="button"
              className="upload-primary-btn"
              disabled={ocrLoading || pdfLoading}
              onClick={() => openUploadPicker("image/*")}
            >
              {ocrLoading
                ? "Translating image..."
                : "Upload image"}
            </button>
            <button
              type="button"
              className="icon-btn"
              disabled={ocrLoading || pdfLoading}
              onClick={() => openUploadPicker("application/pdf")}
            >
              {pdfLoading ? "Uploading PDF..." : "Upload PDF"}
            </button>
            <button
              type="button"
              className="icon-btn"
              onClick={() => setWorkflow("text")}
            >
              Use text input
            </button>
          </div>
        </section>

        {imageProgressStatus !== "idle" && (
          <section className={`image-progress image-progress-${imageProgressStatus}`} aria-live="polite">
            <div className="image-progress-header">
              <strong>
                {imageProgressStatus === "running"
                  ? "Image pipeline in progress"
                  : imageProgressStatus === "done"
                  ? "Image pipeline completed"
                  : "Image pipeline failed"}
              </strong>
              <span className="image-progress-summary">
                {imageProgressStatus === "running"
                  ? `Active stage: ${IMAGE_PROGRESS_STAGES.find(stage => stage.key === imageProgressStage)?.label ?? "OCR"}`
                  : imageProgressStatus === "done"
                  ? "All stages finished"
                  : "Job stopped before completion"}
              </span>
            </div>
            <ol className="image-progress-stages">
              {IMAGE_PROGRESS_STAGES.map(stage => {
                const done = imageProgressCompleted.includes(stage.key)
                const active = imageProgressStatus === "running" && imageProgressStage === stage.key
                const failed = imageProgressStatus === "error" && !done
                return (
                  <li
                    key={stage.key}
                    className={`image-progress-stage${done ? " is-done" : ""}${active ? " is-active" : ""}${failed ? " is-failed" : ""}`}
                  >
                    <span className="image-progress-dot" aria-hidden="true" />
                    <span>{stage.label}</span>
                    {done && <span className="image-progress-state">Done</span>}
                    {active && <span className="image-progress-state">Running</span>}
                    {failed && <span className="image-progress-state">Waiting</span>}
                  </li>
                )
              })}
            </ol>
          </section>
        )}

        {/* Comparison spotlight */}
        {ocrLoading && compareOriginalUrl && (
          <div className="comparison-loading comparison-loading-spotlight">
            <span className="spinner" aria-hidden="true" />
            {imageProgressStage === "ocr"
              ? "Running OCR..."
              : imageProgressStage === "translation"
              ? "Running translation..."
              : imageProgressStage === "rendering"
              ? "Rendering results..."
              : mode === "ocr_only"
              ? "Extracting text..."
              : "Preparing visual comparison — overlay and layout are processing..."}
          </div>
        )}

        {!ocrLoading && compareOriginalUrl && compareOverlayUrl && compareLayoutUrl && (
          <section
            ref={compareSectionRef}
            className="comparison-wrap comparison-wrap--full comparison-hero"
            aria-label="Image comparison"
          >
            <div className="comparison-intro">
              <h3>Visual Quality Comparison</h3>
              <p>Compare original, overlay, and layout outputs instantly.</p>
            </div>
            <div className="comparison-controls">
              <div className="comparison-segment" role="group" aria-label="Comparison layout">
                <button
                  type="button"
                  className={`icon-btn${compareView === "side" ? " is-active" : ""}`}
                  onClick={() => setCompareView("side")}
                >
                  Side-by-side
                </button>
                <button
                  type="button"
                  className={`icon-btn${compareView === "slider" ? " is-active" : ""}`}
                  onClick={() => setCompareView("slider")}
                >
                  Slider
                </button>
              </div>
              {compareView === "slider" && (
                <div className="comparison-segment" role="group" aria-label="Slider target">
                  <button
                    type="button"
                    className={`icon-btn${sliderTarget === "overlay" ? " is-active" : ""}`}
                    onClick={() => setSliderTarget("overlay")}
                  >
                    vs Overlay
                  </button>
                  <button
                    type="button"
                    className={`icon-btn${sliderTarget === "layout" ? " is-active" : ""}`}
                    onClick={() => setSliderTarget("layout")}
                  >
                    vs Layout
                  </button>
                </div>
              )}
            </div>

            {compareView === "side" ? (
              <div className="comparison-grid">
                <figure className="compare-card">
                  <figcaption>Original</figcaption>
                  <img
                    src={compareOriginalUrl}
                    alt="Original image"
                    loading="lazy"
                    className="compare-clickable"
                    onClick={() => openComparisonModal("original")}
                  />
                </figure>
                <figure className="compare-card">
                  <div className="compare-card-header">
                    <figcaption>Overlay result</figcaption>
                    <button
                      type="button"
                      className="icon-btn compare-dl-btn"
                      title="Download overlay result"
                      onClick={() => handleDownload(compareOverlayUrl, "overlay-translation")}
                    >
                      ⬇ Download
                    </button>
                  </div>
                  <img
                    src={compareOverlayUrl}
                    alt="Overlay translation result"
                    loading="lazy"
                    className="compare-clickable"
                    onClick={() => openComparisonModal("overlay")}
                  />
                </figure>
                <figure className="compare-card">
                  <div className="compare-card-header">
                    <figcaption>Layout result</figcaption>
                    <button
                      type="button"
                      className="icon-btn compare-dl-btn"
                      title="Download layout result"
                      onClick={() => handleDownload(compareLayoutUrl, "layout-translation")}
                    >
                      ⬇ Download
                    </button>
                  </div>
                  <img
                    src={compareLayoutUrl}
                    alt="Layout translation result"
                    loading="lazy"
                    className="compare-clickable"
                    onClick={() => openComparisonModal("layout")}
                  />
                </figure>
              </div>
            ) : (
              <div className="compare-slider-wrap">
                <div className="compare-slider-frame" aria-live="polite">
                  <img
                    className="compare-slider-base"
                    src={compareOriginalUrl}
                    alt="Original image"
                    loading="lazy"
                  />
                  <div
                    className="compare-slider-overlay"
                    style={{ width: `${sliderPercent}%` }}
                    aria-hidden="true"
                  >
                    <img
                      src={sliderTarget === "overlay" ? compareOverlayUrl : compareLayoutUrl}
                      alt=""
                      loading="lazy"
                    />
                  </div>
                  <div className="compare-slider-handle" style={{ left: `${sliderPercent}%` }} aria-hidden="true" />
                </div>
                <label className="compare-slider-label">
                  Original vs {sliderTarget === "overlay" ? "overlay" : "layout"} — drag to reveal
                  <input
                    type="range"
                    min={0}
                    max={100}
                    value={sliderPercent}
                    onChange={e => setSliderPercent(Number(e.target.value))}
                  />
                </label>
                <div className="compare-dl-row">
                  <button
                    type="button"
                    className="icon-btn"
                    onClick={() => handleDownload(compareOverlayUrl, "overlay-translation")}
                  >
                    ⬇ Download Overlay
                  </button>
                  <button
                    type="button"
                    className="icon-btn"
                    onClick={() => handleDownload(compareLayoutUrl, "layout-translation")}
                  >
                    ⬇ Download Layout
                  </button>
                </div>
              </div>
            )}
          </section>
        )}

        {/* Demo presets */}
        <div className="demo-presets" role="group" aria-label="Demo presets">
          <span className="demo-presets-label">Sample images:</span>
          {DEMO_PRESETS.map(preset => (
            <button
              key={preset.id}
              type="button"
              className="demo-preset-btn"
              disabled={ocrLoading}
              onClick={() => handleDemoPreset(preset)}
              title={`${preset.label} — ${preset.source === "auto" ? "auto" : preset.source} → ${preset.target}`}
            >
              <span className="demo-preset-thumb-wrap">
                <img
                  className="demo-preset-thumb"
                  src={preset.src}
                  alt=""
                  loading="lazy"
                  aria-hidden="true"
                />
              </span>
              <span className="demo-preset-info">
                <span className="demo-preset-emoji">{preset.emoji}</span>
                <span className="demo-preset-name">{preset.label}</span>
                <span className="demo-preset-langs">
                  {preset.source === "auto" ? "auto" : preset.source.toUpperCase()} → {preset.target.toUpperCase()}
                </span>
              </span>
            </button>
          ))}
        </div>

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
                      {resultKind === "ocr" ? "OCR extraction only" : "Translated text"}
                    </div>
                  )}
                  <div className="result-text">{result}</div>
                  {resultImageUrl && !(compareOverlayUrl && compareLayoutUrl) && (
                    <div className="result-image-wrap">
                      <img
                        className="result-image compare-clickable"
                        src={resultImageUrl}
                        alt="Translated output preview"
                        loading="lazy"
                        onClick={() => openComparisonModal("layout")}
                      />
                    </div>
                  )}
                </>
              )}
            </div>
            {result && !loading && (
              <div className="panel-footer">
                <span className="char-count">{result.length} chars</span>
                <button className="icon-btn" title="Copy translation" onClick={handleCopy}>
                  ⎘ Copy
                </button>
              </div>
            )}
          </div>
        </div>

        <div className="actions">
          <span className="hint">
            {workflow === "text"
              ? "Ctrl+Enter to translate text"
              : "Image/PDF upload starts processing automatically"}
          </span>
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

      {comparisonModalOpen && modalImageSrc && (
        <div
          className="comparison-modal"
          role="dialog"
          aria-modal="true"
          aria-label="Translation comparison viewer"
          onClick={closeComparisonModal}
        >
          <div className="comparison-modal-card" onClick={e => e.stopPropagation()}>
            <div className="comparison-modal-header">
              <strong>Translation Quality Viewer</strong>
              <div className="comparison-modal-header-actions">
                <button
                  type="button"
                  className="icon-btn"
                  onClick={() => adjustComparisonModalZoom(-0.2)}
                  title="Zoom out"
                >
                  −
                </button>
                <span className="comparison-modal-zoom-label">{Math.round(comparisonModalZoom * 100)}%</span>
                <button
                  type="button"
                  className="icon-btn"
                  onClick={() => adjustComparisonModalZoom(0.2)}
                  title="Zoom in"
                >
                  +
                </button>
                <button
                  type="button"
                  className="icon-btn"
                  onClick={() => {
                    setComparisonModalZoom(1)
                    setComparisonModalPan({ x: 0, y: 0 })
                    setComparisonModalPanning(false)
                    modalPanOriginRef.current = null
                  }}
                  title="Reset zoom"
                >
                  Reset
                </button>
                <button
                  type="button"
                  className="icon-btn"
                  onClick={closeComparisonModal}
                  aria-label="Close comparison viewer"
                >
                  ✕
                </button>
              </div>
            </div>

            {hasModalComparison && (
              <div className="comparison-modal-toolbar">
                <div className="comparison-segment" role="group" aria-label="Modal comparison mode">
                  <button
                    type="button"
                    className={`icon-btn${comparisonModalView === "gallery" ? " is-active" : ""}`}
                    onClick={() => setComparisonModalView("gallery")}
                  >
                    Gallery
                  </button>
                  <button
                    type="button"
                    className={`icon-btn${comparisonModalView === "slider" ? " is-active" : ""}`}
                    onClick={() => setComparisonModalView("slider")}
                  >
                    Slider
                  </button>
                </div>
                <div className="comparison-segment" role="group" aria-label="Focused image">
                  <button
                    type="button"
                    className={`icon-btn${comparisonModalFocus === "original" ? " is-active" : ""}`}
                    onClick={() => setComparisonModalFocus("original")}
                  >
                    Original
                  </button>
                  <button
                    type="button"
                    className={`icon-btn${comparisonModalFocus === "overlay" ? " is-active" : ""}`}
                    onClick={() => setComparisonModalFocus("overlay")}
                  >
                    Overlay
                  </button>
                  <button
                    type="button"
                    className={`icon-btn${comparisonModalFocus === "layout" ? " is-active" : ""}`}
                    onClick={() => setComparisonModalFocus("layout")}
                  >
                    Layout
                  </button>
                </div>
              </div>
            )}

            {hasModalComparison && comparisonModalView === "slider" ? (
              <div className="comparison-modal-body">
                <div
                  className={`comparison-modal-stage${comparisonModalZoom > 1 ? " is-pannable" : ""}${comparisonModalPanning ? " is-panning" : ""}`}
                  onWheel={handleComparisonModalWheel}
                  onPointerDown={handleComparisonModalPointerDown}
                  onPointerMove={handleComparisonModalPointerMove}
                  onPointerUp={handleComparisonModalPointerUp}
                  onPointerLeave={handleComparisonModalPointerUp}
                >
                <div className="compare-slider-frame comparison-modal-slider-frame" aria-live="polite">
                  <img
                    className="compare-slider-base"
                    src={compareOriginalUrl ?? ""}
                    alt="Original image"
                    loading="lazy"
                    style={{ transform: modalTransform, transformOrigin: "center" }}
                  />
                  <div
                    className="compare-slider-overlay"
                    style={{ width: `${comparisonModalSliderPercent}%` }}
                    aria-hidden="true"
                  >
                    <img
                      src={comparisonModalSliderTarget === "overlay" ? compareOverlayUrl ?? "" : compareLayoutUrl ?? ""}
                      alt=""
                      loading="lazy"
                      style={{ transform: modalTransform, transformOrigin: "center" }}
                    />
                  </div>
                  <div className="compare-slider-handle" style={{ left: `${comparisonModalSliderPercent}%` }} aria-hidden="true" />
                </div>
                </div>
                <label className="compare-slider-label">
                  Original vs {comparisonModalSliderTarget === "overlay" ? "overlay" : "layout"} — drag to reveal
                  <input
                    type="range"
                    min={0}
                    max={100}
                    value={comparisonModalSliderPercent}
                    onChange={e => setComparisonModalSliderPercent(Number(e.target.value))}
                  />
                </label>
                <div className="comparison-segment" role="group" aria-label="Modal slider target">
                  <button
                    type="button"
                    className={`icon-btn${comparisonModalSliderTarget === "overlay" ? " is-active" : ""}`}
                    onClick={() => setComparisonModalSliderTarget("overlay")}
                  >
                    Compare Overlay
                  </button>
                  <button
                    type="button"
                    className={`icon-btn${comparisonModalSliderTarget === "layout" ? " is-active" : ""}`}
                    onClick={() => setComparisonModalSliderTarget("layout")}
                  >
                    Compare Layout
                  </button>
                </div>
              </div>
            ) : (
              <div className="comparison-modal-body">
                <div
                  className={`comparison-modal-stage${comparisonModalZoom > 1 ? " is-pannable" : ""}${comparisonModalPanning ? " is-panning" : ""}`}
                  onWheel={handleComparisonModalWheel}
                  onPointerDown={handleComparisonModalPointerDown}
                  onPointerMove={handleComparisonModalPointerMove}
                  onPointerUp={handleComparisonModalPointerUp}
                  onPointerLeave={handleComparisonModalPointerUp}
                >
                  <img
                    src={modalImageSrc}
                    alt="Comparison preview"
                    className="comparison-modal-image"
                    style={{ transform: modalTransform }}
                  />
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

export default App
