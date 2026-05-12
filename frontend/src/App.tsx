import { useCallback, useEffect, useRef, useState } from "react"
import { translate, translateImage, uploadPDF, type JobProgressUpdate } from "./api/translate"
import { extractTextFromImage, type TextBlock } from "./api/ocr"
import { getReadiness, type ReadinessResponse } from "./api/health"
import { getOCRMetrics, getProviderMetrics, type MetricsWindow, type OCRMetricsResponse, type ProviderMetricsResponse } from "./api/metrics"
import { PdfJobsPanel } from "./PdfJobsPanel"
import { DeadLetterOpsPanel } from "./DeadLetterOpsPanel"
import { saveStoredJob } from "./pdfJobsStorage"
import { checkpointOperatorHint, formatPercent, levelLabel, safePercent, type CheckpointThresholds } from "./checkpointReliability"
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
type ProductMode = "fast" | "studio" | "extract"
const PRODUCT_MODE_BACKEND_MAP: Record<ProductMode, "overlay" | "layout" | "ocr_only"> = {
  fast: "overlay",
  studio: "layout",
  extract: "ocr_only",
}
const PRODUCT_MODES = [
  { value: "fast", label: "Fast", detail: "Quick visual draft" },
  { value: "studio", label: "Studio", detail: "Premium layout fidelity" },
  { value: "extract", label: "Extract", detail: "Text-first output" },
] as const
const WORKFLOWS = [
  { value: "image", label: "Image", detail: "Hero workflow for instant visual translation", icon: "🖼" },
  { value: "pdf", label: "PDF", detail: "Queue full-document translation in background", icon: "📄" },
  { value: "text", label: "Text", detail: "Translate pasted or typed text", icon: "✍" },
] as const
const MAX_CHARS = 2000
const HISTORY_KEY = "loklingo-history"
const MAX_HISTORY = 10
const OCR_VIS_KEY = "loklingo-ocr-visual-controls"
const FIRST_VISIT_KEY = "loklingo-first-visit"
const RELIABILITY_WINDOWS: MetricsWindow[] = ["1h", "6h", "24h", "7d", "30d"]
const PRESSURE_WARN_THRESHOLD = Number(import.meta.env.VITE_RELIABILITY_PRESSURE_WARN ?? 8)
const PRESSURE_CRITICAL_THRESHOLD = Number(import.meta.env.VITE_RELIABILITY_PRESSURE_CRITICAL ?? 20)
const CHECKPOINT_HIT_WARN_THRESHOLD = Number(import.meta.env.VITE_CHECKPOINT_HIT_WARN ?? 70)
const CHECKPOINT_HIT_CRITICAL_THRESHOLD = Number(import.meta.env.VITE_CHECKPOINT_HIT_CRITICAL ?? 40)
const CHECKPOINT_PERSIST_FAILURE_WARN_THRESHOLD = Number(import.meta.env.VITE_CHECKPOINT_PERSIST_FAILURE_WARN ?? 5)
const CHECKPOINT_PERSIST_FAILURE_CRITICAL_THRESHOLD = Number(import.meta.env.VITE_CHECKPOINT_PERSIST_FAILURE_CRITICAL ?? 15)

const DEMO_PRESETS = [
  // Manga & Comics
  { id: "manga",      label: "Manga frames",     emoji: "📚", category: "Manga", src: "/samples/cjk_vertical.png",       source: "ja", target: "en" },
  // Restaurant Menus
  { id: "menu",       label: "Restaurant menu",  emoji: "🍽", category: "Menus", src: "/samples/before_bold_style.png",  source: "auto", target: "en" },
  // Anime & Stylized Art
  { id: "anime",      label: "Anime/stylized",   emoji: "🎨", category: "Anime", src: "/samples/before_mixed_styles.png", source: "en", target: "es" },
  // Street Signs
  { id: "signs",      label: "Street signs",     emoji: "🚩", category: "Signs", src: "/samples/realworld_overlay.jpg",   source: "auto", target: "en" },
  // Infographics
  { id: "infographic", label: "Infographics",    emoji: "📊", category: "Infographics", src: "/samples/cjk_vertical.png",  source: "auto", target: "en" },
  // Screenshots
  { id: "screenshot", label: "UI screenshots",   emoji: "🖥", category: "Screenshots", src: "/samples/before_bold_style.png", source: "en", target: "fr" },
  // Posters
  { id: "poster",     label: "Posters & art",    emoji: "🎬", category: "Posters", src: "/samples/before_mixed_styles.png", source: "en", target: "de" },
  // PDFs (using same images as proxy)
  { id: "document",   label: "Documents/PDFs",  emoji: "📄", category: "PDFs", src: "/samples/realworld_overlay.jpg",   source: "auto", target: "en" },
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
type ProgressStage = "detecting" | "layout" | "languages" | "translating" | "typography" | "rendering"
type OverlayStage = "idle" | "ocr" | "translated"
type LiveProgressDetail = {
  stage?: ProgressStage
  region?: number
  totalRegions?: number
  chunkProgress?: number
  phaseLabel?: string
}

const TRUST_SIGNALS = [
  "Local processing",
  "GPU acceleration",
  "No cloud dependency",
  "OCR confidence",
  "Layout preservation",
] as const

const IMAGE_PROGRESS_STAGES: Array<{ key: ProgressStage; label: string }> = [
  { key: "detecting", label: "Detecting text" },
  { key: "layout", label: "Understanding layout" },
  { key: "languages", label: "Detecting languages" },
  { key: "translating", label: "Translating content" },
  { key: "typography", label: "Rebuilding typography" },
  { key: "rendering", label: "Rendering final image" },
]

const OCR_STAGGER_PRESETS = [
  { id: "fast", label: "Fast", ms: 30 },
  { id: "default", label: "Default", ms: 70 },
  { id: "slow", label: "Slow", ms: 140 },
] as const

const MOTION = {
  animatedCounterMs: 520,
  toastMs: 3500,
  flashIntervalMs: 1100,
  modalFlashIntervalMs: 950,
  overlaySwapLabelMs: 420,
  overlayDemo: {
    toTranslationMs: 550,
    clearSwapMs: 900,
    toRenderingMs: 1020,
    settleMs: 1520,
  },
  imageProgress: {
    toLayoutMs: 620,
    toLanguagesMs: 1220,
    toTranslationMs: 1880,
    toTypographyMs: 2760,
    toRenderingMs: 3600,
    successSettleMs: 3600,
    errorSettleMs: 4200,
  },
  demoCycleMs: 5200,
} as const

function isRenderableOCRBlock(block: TextBlock): boolean {
  const [x1, y1, x2, y2] = block.bbox
  return Number.isFinite(x1) && Number.isFinite(y1) && Number.isFinite(x2) && Number.isFinite(y2) && Math.abs(x2 - x1) >= 4 && Math.abs(y2 - y1) >= 4
}

function mapTranslatedLinesToBlocks(blocks: TextBlock[], translatedText: string): string[] {
  const translatedLines = translatedText
    .split("\n")
    .map(line => line.trim())
    .filter(Boolean)

  const mapped = new Array<string>(blocks.length).fill("")
  let lineIdx = 0

  for (let i = 0; i < blocks.length; i++) {
    const source = blocks[i].text.trim()
    if (!source) {
      mapped[i] = ""
      continue
    }
    mapped[i] = translatedLines[lineIdx] ?? source
    lineIdx += 1
  }

  return mapped
}

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

function loadOCRVisualizationControls(): { show: boolean; staggerMs: number } {
  try {
    const raw = JSON.parse(localStorage.getItem(OCR_VIS_KEY) ?? "{}") as { show?: unknown; staggerMs?: unknown }
    const show = typeof raw.show === "boolean" ? raw.show : true
    const parsedStagger = Number(raw.staggerMs)
    const staggerMs = Number.isFinite(parsedStagger)
      ? Math.max(20, Math.min(220, Math.round(parsedStagger)))
      : 70
    return { show, staggerMs }
  } catch {
    return { show: true, staggerMs: 70 }
  }
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

function hasRTLText(text: string): boolean {
  return /[\u0590-\u08FF]/.test(text)
}

function hasVerticalTypography(blocks: TextBlock[]): boolean {
  return blocks.some(block => {
    const [x1, y1, x2, y2] = block.bbox
    const width = Math.abs(x2 - x1)
    const height = Math.abs(y2 - y1)
    return height > width * 1.35
  })
}

function getPipelineHelpMessage(err: unknown): string | null {
  const raw = err instanceof Error ? err.message : String(err ?? "")
  const message = raw.toLowerCase()

  if (message.includes("unsupported api version")) {
    return "Backend API version mismatch. Update the frontend/backend pair or restart services with matching versions."
  }
  if (message.includes("failed to fetch") || message.includes("networkerror") || message.includes("network error")) {
    return "Cannot reach translation services right now. Check backend containers and network connectivity."
  }
  if (message.includes("http 5")) {
    return "Translation backend returned a server error. Retry in a moment or inspect backend logs."
  }

  return null
}

function mapBackendImageStage(stage?: string): ProgressStage | undefined {
  switch ((stage ?? "").toLowerCase()) {
    case "detecting_text":
      return "detecting"
    case "understanding_layout":
      return "layout"
    case "detecting_languages":
      return "languages"
    case "translating":
      return "translating"
    case "retrying":
      return "translating"
    case "fallback_provider":
      return "translating"
    case "rebuilding_layout":
      return "typography"
    case "rendering":
      return "rendering"
    case "completed":
      return "rendering"
    default:
      return undefined
  }
}

function mapBackendStageNarrative(job: JobProgressUpdate): string | null {
  const stage = (job.stage ?? "").toLowerCase()
  const message = (job.stage_message ?? "").toLowerCase()

  if (stage === "detecting_text") return "Detecting text regions"
  if (stage === "understanding_layout") return "Preserving layout geometry"
  if (stage === "detecting_languages") return "Detecting languages and script direction"
  if (stage === "translating") return "Translating content"
  if (stage === "rebuilding_layout") return "Rebuilding typography"
  if (stage === "rendering") return "Rendering final image"

  if (message.includes("layout")) return "Preserving layout geometry"
  if (message.includes("language")) return "Detecting languages and script direction"
  if (message.includes("translat")) return "Translating content"
  if (message.includes("render")) return "Rendering final image"
  if (message.includes("typography") || message.includes("rebuild")) return "Rebuilding typography"

  return null
}

function mapBackendReliabilityHint(job: JobProgressUpdate): string | null {
  const stage = (job.stage ?? "").toLowerCase()
  const message = (job.stage_message ?? "").toLowerCase()
  if (stage === "retrying" || message.includes("retry")) {
    return "Retrying unstable region..."
  }
  if (stage === "fallback_provider" || message.includes("backup") || message.includes("switching")) {
    return "Switching rendering strategy..."
  }
  return null
}

function useAnimatedCount(target: number, durationMs = MOTION.animatedCounterMs): number {
  const [value, setValue] = useState(target)
  const valueRef = useRef(target)

  useEffect(() => {
    valueRef.current = value
  }, [value])

  useEffect(() => {
    let frame = 0
    const start = performance.now()
    const initial = valueRef.current
    const delta = target - initial
    if (delta === 0) return

    const tick = (now: number) => {
      const elapsed = now - start
      const progress = Math.min(1, elapsed / durationMs)
      const eased = 1 - Math.pow(1 - progress, 3)
      setValue(Math.round(initial + delta * eased))
      if (progress < 1) {
        frame = window.requestAnimationFrame(tick)
      }
    }

    frame = window.requestAnimationFrame(tick)
    return () => {
      if (frame) {
        window.cancelAnimationFrame(frame)
      }
    }
  }, [target, durationMs])

  return value
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
  const [mode, setMode] = useState<ProductMode>("fast")
  const [workflow, setWorkflow] = useState<InputWorkflow>("image")
  const [uploadSelection, setUploadSelection] = useState<UploadSelection | null>(null)
  const [uploadDragActive, setUploadDragActive] = useState(false)
  const [result, setResult] = useState("")
  const [resultKind, setResultKind] = useState<"translation" | "ocr">("translation")
  const [resultImageUrl, setResultImageUrl] = useState<string | null>(null)
  const [compareOriginalUrl, setCompareOriginalUrl] = useState<string | null>(null)
  const [compareOverlayUrl, setCompareOverlayUrl] = useState<string | null>(null)
  const [compareLayoutUrl, setCompareLayoutUrl] = useState<string | null>(null)
  const [compareView, setCompareView] = useState<"side" | "slider" | "flash">("side")
  const [sliderTarget, setSliderTarget] = useState<"overlay" | "layout">("layout")
  const [sliderPercent, setSliderPercent] = useState(50)
  const [comparisonModalOpen, setComparisonModalOpen] = useState(false)
  const [comparisonModalFocus, setComparisonModalFocus] = useState<ComparisonFocus>("layout")
  const [comparisonModalView, setComparisonModalView] = useState<"gallery" | "slider" | "flash">("gallery")
  const [comparisonModalSliderTarget, setComparisonModalSliderTarget] = useState<"overlay" | "layout">("layout")
  const [comparisonModalSliderPercent, setComparisonModalSliderPercent] = useState(50)
  const [comparisonModalFlashTarget, setComparisonModalFlashTarget] = useState<"overlay" | "layout">("layout")
  const [comparisonModalFlashToggle, setComparisonModalFlashToggle] = useState(false)
  const [comparisonQuickToggle, setComparisonQuickToggle] = useState(false)
  const [comparisonModalZoom, setComparisonModalZoom] = useState(1)
  const [comparisonModalPan, setComparisonModalPan] = useState({ x: 0, y: 0 })
  const [comparisonModalPanning, setComparisonModalPanning] = useState(false)
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
  const [showOCROverlay, setShowOCROverlay] = useState(() => loadOCRVisualizationControls().show)
  const [ocrStaggerMs, setOcrStaggerMs] = useState(() => loadOCRVisualizationControls().staggerMs)
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
  const sliderDraggingRef = useRef(false)
  const modalSliderDraggingRef = useRef(false)
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
    setComparisonModalFocus("layout")
    setComparisonModalSliderTarget("layout")
    setComparisonModalFlashTarget("layout")
  }, [ocrLoading, compareOriginalUrl, compareOverlayUrl, compareLayoutUrl])

  useEffect(() => {
    const canFlash = Boolean(compareOriginalUrl && compareOverlayUrl && compareLayoutUrl)
    if (!comparisonModalOpen || comparisonModalView !== "flash" || !canFlash) {
      if (modalFlashTimerRef.current !== null) {
        window.clearInterval(modalFlashTimerRef.current)
        modalFlashTimerRef.current = null
      }
      return
    }
    modalFlashTimerRef.current = window.setInterval(() => {
      setComparisonModalFlashToggle(prev => !prev)
    }, MOTION.modalFlashIntervalMs)
    return () => {
      if (modalFlashTimerRef.current !== null) {
        window.clearInterval(modalFlashTimerRef.current)
        modalFlashTimerRef.current = null
      }
    }
  }, [comparisonModalOpen, comparisonModalView, compareOriginalUrl, compareOverlayUrl, compareLayoutUrl])

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme)
    localStorage.setItem("loklingo-theme", theme)
  }, [theme])

  useEffect(() => {
    localStorage.setItem(LANG_KEY, JSON.stringify({ source: sourceLang, target: targetLang }))
  }, [sourceLang, targetLang])

  useEffect(() => {
    localStorage.setItem(OCR_VIS_KEY, JSON.stringify({ show: showOCROverlay, staggerMs: ocrStaggerMs }))
  }, [showOCROverlay, ocrStaggerMs])

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
    setTimeout(() => setToasts(t => t.filter(x => x.id !== id)), MOTION.toastMs)
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
  }, [sourceText, sourceLang, targetLang, mode, workflow, pushToast, compareOriginalUrl])

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
        setComparisonModalOpen(false)
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
        setComparisonModalZoom(prev => Math.max(0.6, Math.min(3, Number((prev + 0.16).toFixed(2)))))
        return
      }
      if (e.key === "-") {
        e.preventDefault()
        setComparisonModalZoom(prev => Math.max(0.6, Math.min(3, Number((prev - 0.16).toFixed(2)))))
        return
      }
      if (e.key === "0") {
        e.preventDefault()
        setComparisonModalZoom(1)
        setComparisonModalPan({ x: 0, y: 0 })
        setComparisonModalPanning(false)
        modalPanOriginRef.current = null
        return
      }

      if (comparisonModalView === "slider") {
        if (e.key === "ArrowLeft") {
          e.preventDefault()
          setComparisonModalSliderPercent(prev => Math.max(0, prev - 4))
          return
        }
        if (e.key === "ArrowRight") {
          e.preventDefault()
          setComparisonModalSliderPercent(prev => Math.min(100, prev + 4))
          return
        }
      }

      if (e.key.toLowerCase() === "f") {
        e.preventDefault()
        setComparisonModalView(prev => (prev === "flash" ? "slider" : "flash"))
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

  useEffect(() => {
    if (!comparisonModalOpen) return
    const previous = document.body.style.overflow
    document.body.style.overflow = "hidden"
    return () => {
      document.body.style.overflow = previous
    }
  }, [comparisonModalOpen])

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
      pushToast(err instanceof Error ? err.message : "PDF export failed", "error")
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
    setComparisonModalFocus(hasComparison ? focus : "layout")
    setComparisonModalSliderTarget(focus === "overlay" ? "overlay" : "layout")
    setComparisonModalFlashTarget(focus === "overlay" ? "overlay" : "layout")
    setComparisonModalSliderPercent(50)
    setComparisonModalView(hasComparison ? "slider" : "gallery")
    setComparisonQuickToggle(false)
    setComparisonModalZoom(1)
    setComparisonModalPan({ x: 0, y: 0 })
    setComparisonModalPanning(false)
    setComparisonModalOpen(true)
  }, [compareOriginalUrl, compareOverlayUrl, compareLayoutUrl, resultImageUrl])

  const closeComparisonModal = () => {
    setComparisonModalPanning(false)
    setComparisonQuickToggle(false)
    setComparisonModalOpen(false)
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
      
      setComparisonModalFocus(hasComparison ? "layout" : "layout")
      setComparisonModalSliderTarget("layout")
      setComparisonModalFlashTarget("layout")
      setComparisonModalSliderPercent(50)
      setComparisonModalView(hasComparison ? "slider" : "gallery")
      setComparisonQuickToggle(false)
      setComparisonModalZoom(1)
      setComparisonModalPan({ x: 0, y: 0 })
      setComparisonModalPanning(false)
      setComparisonModalOpen(true)
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
          if (hasModalComparison) setComparisonModalView("slider")
        } else if (key === "g") {
          event.preventDefault()
          if (hasModalComparison) setComparisonModalView("gallery")
        } else if (key === "f") {
          event.preventDefault()
          if (hasModalComparison) setComparisonModalView("flash")
        } else if (key === "o") {
          event.preventDefault()
          setComparisonModalFocus("original")
          setComparisonQuickToggle(false)
        } else if (key === "v") {
          event.preventDefault()
          setComparisonModalFocus("overlay")
          setComparisonQuickToggle(false)
        } else if (key === "l") {
          event.preventDefault()
          setComparisonModalFocus("layout")
          setComparisonQuickToggle(false)
        } else if (key === "+" || key === "=") {
          event.preventDefault()
          adjustComparisonModalZoom(0.2)
        } else if (key === "-" || key === "_") {
          event.preventDefault()
          adjustComparisonModalZoom(-0.2)
        } else if (key === "0") {
          event.preventDefault()
          setComparisonModalZoom(1)
          setComparisonModalPan({ x: 0, y: 0 })
          setComparisonModalPanning(false)
          modalPanOriginRef.current = null
        } else if (key === "escape") {
          event.preventDefault()
          closeComparisonModal()
        }
      }
    }
    window.addEventListener("keydown", handler)
    return () => window.removeEventListener("keydown", handler)
  }, [comparisonModalOpen, hasModalComparison, closeComparisonModal])

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

  const effectiveModalFocus: ComparisonFocus = comparisonQuickToggle ? "original" : comparisonModalFocus
  const modalImageSrc =
    effectiveModalFocus === "original"
      ? compareOriginalUrl
      : effectiveModalFocus === "overlay"
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

  const runImageFile = useCallback(async (file: File, src: string, tgt: string) => {
    setOcrLoading(true)
    setPipelineWarning(null)
    startImageProgress()
    resetOCRVisualization()
    setOcrConfidence(null)

    let ocrBlocks: TextBlock[] = []
    let translatedOverlayText = ""
    const ocrExtractionPromise = extractTextFromImage(file, src)
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
        const ocrRes = await translateImage(file, src, tgt, requestMode, handleImageJobProgress)
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

      const overlayRes = await translateImage(file, src, tgt, "overlay", handleImageJobProgress)
      const layoutRes = await translateImage(file, src, tgt, "layout", handleImageJobProgress)

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
      const friendlyWarning = getPipelineHelpMessage(err) ?? "Translation engine recovering. Please retry in a moment if processing does not complete."
      setPipelineWarning(friendlyWarning)
      failImageProgress()
      pushToast("Image translation paused. Retry to continue.", "error")
    } finally {
      void ocrExtractionPromise
      setOcrLoading(false)
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
      pushToast(err instanceof Error ? err.message : "Demo preset failed", "error")
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
              className={`icon-btn${showDemoGallery ? " is-active" : ""}`}
              type="button"
              onClick={() => setShowDemoGallery(g => !g)}
              aria-pressed={showDemoGallery}
              title="Try demo examples"
            >
              🎨 Demos
            </button>
            <button
              className={`icon-btn${showcaseMode ? " is-active" : ""}`}
              type="button"
              onClick={() => setShowcaseMode(s => !s)}
              aria-pressed={showcaseMode}
              title="Auto-play demo showcase"
            >
              ▶ Showcase
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

      {/* Demo Gallery panel */}
      {showDemoGallery && (
        <section className="demo-gallery-panel" aria-label="Demo gallery - try examples">
          <div className="demo-gallery-header">
            <h2>Try Examples</h2>
            <button
              type="button"
              className="icon-btn"
              onClick={() => setShowDemoGallery(false)}
              aria-label="Close gallery"
            >
              ✕
            </button>
          </div>
          <div className="demo-gallery-grid">
            {DEMO_PRESETS.map(preset => (
              <button
                key={preset.id}
                className="demo-card"
                onClick={() => {
                  setWorkflow("image")
                  setSourceLang(preset.source)
                  setTargetLang(preset.target)
                  setShowDemoGallery(false)
                  // Load the preset image
                  fetch(preset.src)
                    .then(res => res.blob())
                    .then(blob => {
                      const file = new File([blob], `demo-${preset.id}.png`, { type: blob.type })
                      runImageFile(file, preset.source, preset.target)
                    })
                    .catch(err => pushToast(`Failed to load demo: ${err.message}`, "error"))
                }}
                title={`Load demo: ${preset.label}`}
              >
                <div className="demo-card-emoji">{preset.emoji}</div>
                <div className="demo-card-label">{preset.label}</div>
                <div className="demo-card-category">{preset.category}</div>
                <div className="demo-card-image-wrapper">
                  <img
                    src={preset.src}
                    alt={preset.label}
                    loading="lazy"
                    className="demo-card-image"
                  />
                </div>
              </button>
            ))}
          </div>
        </section>
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

              <div className="reliability-windowed-table-wrap">
                <div className="reliability-windowed-title">Provider reliability (live)</div>
                {providerMetricsError && <p className="reliability-state reliability-state-error">{providerMetricsError}</p>}
                {providerMetrics && providerMetrics.providers.length === 0 && (
                  <p className="reliability-state">No provider calls observed yet.</p>
                )}
                {providerMetrics && providerMetrics.providers.length > 0 && (
                  <table className="reliability-table">
                    <thead>
                      <tr>
                        <th>Provider</th>
                        <th>Success</th>
                        <th>Failures</th>
                        <th>Retries</th>
                        <th>Timeouts</th>
                        <th>Failovers</th>
                        <th>Avg latency (ms)</th>
                      </tr>
                    </thead>
                    <tbody>
                      {providerMetrics.providers.map(provider => (
                        <tr key={provider.provider}>
                          <td>{provider.provider}</td>
                          <td>{provider.success_total}</td>
                          <td>{provider.failure_total}</td>
                          <td>{provider.retry_total}</td>
                          <td>{provider.timeout_total}</td>
                          <td>{provider.failover_total}</td>
                          <td>{Math.round(provider.avg_latency_ms)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              {providerMetrics && providerMetrics.timeouts.by_reason.length > 0 && (
                <div className="reliability-windowed-table-wrap">
                  <div className="reliability-windowed-title">Timeout reasons (live)</div>
                  <table className="reliability-table">
                    <thead>
                      <tr>
                        <th>Reason</th>
                        <th>Count</th>
                      </tr>
                    </thead>
                    <tbody>
                      {providerMetrics.timeouts.by_reason.map(reason => (
                        <tr key={reason.reason}>
                          <td>{reason.reason}</td>
                          <td>{reason.total}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}

              {providerMetrics && (
                <div className="reliability-windowed-table-wrap">
                  <div className="reliability-windowed-title">Checkpoint health (live)</div>
                  {(() => {
                    const checkpointThresholds: CheckpointThresholds = {
                      hitWarn: CHECKPOINT_HIT_WARN_THRESHOLD,
                      hitCritical: CHECKPOINT_HIT_CRITICAL_THRESHOLD,
                      persistFailureWarn: CHECKPOINT_PERSIST_FAILURE_WARN_THRESHOLD,
                      persistFailureCritical: CHECKPOINT_PERSIST_FAILURE_CRITICAL_THRESHOLD,
                    }
                    const checkpointEvents = providerMetrics.checkpoints.hit_total + providerMetrics.checkpoints.miss_total
                    const checkpointHitRate = safePercent(providerMetrics.checkpoints.hit_total, checkpointEvents)
                    const persistFailureRate = safePercent(providerMetrics.checkpoints.persist_failure_total, checkpointEvents)
                    const operatorHint = checkpointOperatorHint(checkpointHitRate, persistFailureRate, checkpointThresholds)
                    const evaluatedAt = metricsUpdatedAt ? new Date(metricsUpdatedAt).toLocaleTimeString() : null

                    return (
                    <>
                  <table className="reliability-table">
                    <thead>
                      <tr>
                        <th>Metric</th>
                        <th>Total</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr>
                        <td>Checkpoint hits</td>
                        <td>{providerMetrics.checkpoints.hit_total}</td>
                      </tr>
                      <tr>
                        <td>Checkpoint misses</td>
                        <td>{providerMetrics.checkpoints.miss_total}</td>
                      </tr>
                      <tr>
                        <td>Persist failures</td>
                        <td>{providerMetrics.checkpoints.persist_failure_total}</td>
                      </tr>
                      <tr>
                        <td>Cleanup runs</td>
                        <td>{providerMetrics.checkpoints.clear_total}</td>
                      </tr>
                      <tr>
                        <td>Cleanup failures</td>
                        <td>{providerMetrics.checkpoints.clear_failure_total}</td>
                      </tr>
                      <tr>
                        <td>Hit rate</td>
                        <td>
                          <span className={`reliability-rate-badge reliability-rate-level-${operatorHint.hitLevel}`}>
                            {formatPercent(checkpointHitRate)} - {levelLabel(operatorHint.hitLevel)}
                          </span>
                        </td>
                      </tr>
                      <tr>
                        <td>Persist failure rate</td>
                        <td>
                          <span className={`reliability-rate-badge reliability-rate-level-${operatorHint.persistLevel}`}>
                            {formatPercent(persistFailureRate)} - {levelLabel(operatorHint.persistLevel)}
                          </span>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                    <p className={`reliability-operator-hint reliability-operator-hint-${operatorHint.level}`}>
                      {operatorHint.message}
                      {evaluatedAt && (
                        <span className="reliability-operator-hint-meta"> Evaluated at {evaluatedAt}.</span>
                      )}
                    </p>
                    </>
                    )
                  })()}
                </div>
              )}

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
          <label className="mode-label" htmlFor="translation-mode">Mode</label>
          <select
            id="translation-mode"
            className="mode-select"
            aria-label="Translation mode"
            value={mode}
            onChange={e => setMode(e.target.value as ProductMode)}
          >
            {PRODUCT_MODES.map(m => (
              <option key={m.value} value={m.value}>{m.label}</option>
            ))}
          </select>
        </div>
        <p className="mode-help" role="note" aria-live="polite">
          {PRODUCT_MODES.find(m => m.value === mode)?.detail}
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
            <h2>Visual localization</h2>
            <div className="upload-hero-head-right">
              <span className="upload-hero-pill">Local by default</span>
              <button
                type="button"
                className={`upload-backend-chip upload-backend-${uploadBackendState}`}
                onClick={() => {
                  setShowReliability(true)
                  setShowStatusDetail(false)
                }}
                title={uploadBackendHint}
              >
                <span className="upload-backend-dot" aria-hidden="true" />
                <span>{uploadBackendLabel}</span>
              </button>
            </div>
          </div>
          <p className="upload-hero-subtitle">{uploadHeroSubtitle}</p>

          <div className="upload-trust-row" aria-label="Trust indicators">
            {TRUST_SIGNALS.map(signal => (
              <span key={signal} className="upload-trust-pill">{signal}</span>
            ))}
          </div>

          {uploadSelection?.kind === "image" && uploadSelection.previewUrl ? (
            <div className="upload-preview upload-preview-image">
              <div className="upload-preview-image-stage">
                <div className="ocr-pipeline-legend" aria-label="Visual pipeline legend">
                  <span className={`ocr-pipeline-step${overlayStage !== "idle" ? " is-done" : ""}${legendActiveStage === "ocr" ? " is-active" : ""}`}>Scan</span>
                  <span className={`ocr-pipeline-step${overlayStage === "translated" ? " is-done" : ""}${legendActiveStage === "translation" ? " is-active" : ""}`}>Translate</span>
                  <span className={`ocr-pipeline-step${!ocrLoading && overlayStage === "translated" ? " is-done" : ""}${legendActiveStage === "rendering" ? " is-active" : ""}`}>Render</span>
                </div>
                <img
                  src={uploadSelection.previewUrl}
                  alt="Selected upload preview"
                  onLoad={event => {
                    const img = event.currentTarget
                    if (img.naturalWidth > 0 && img.naturalHeight > 0) {
                      setOverlayImageSize({ width: img.naturalWidth, height: img.naturalHeight })
                    }
                  }}
                />
                {showOCROverlay && overlayImageSize && overlayBlocks.length > 0 && overlayStage !== "idle" && (
                  <div className={`ocr-visualization-overlay ocr-visualization-${overlayStage}${overlayLabelSwapActive ? " is-switching" : ""}`} aria-hidden="true">
                    {overlayBlocks.map((block, idx) => {
                      const [bx1, by1, bx2, by2] = block.bbox
                      const left = (Math.min(bx1, bx2) / overlayImageSize.width) * 100
                      const top = (Math.min(by1, by2) / overlayImageSize.height) * 100
                      const width = (Math.abs(bx2 - bx1) / overlayImageSize.width) * 100
                      const height = (Math.abs(by2 - by1) / overlayImageSize.height) * 100
                      const label = overlayStage === "translated" ? (overlayTexts[idx] || block.text) : block.text
                      return (
                        <div
                          key={`${idx}-${block.bbox.join("-")}`}
                          className={`ocr-visualization-box${overlayStage === "translated" ? " is-translated" : ""}`}
                          style={{ left: `${left}%`, top: `${top}%`, width: `${width}%`, height: `${height}%`, animationDelay: `${idx * ocrStaggerMs}ms` }}
                        >
                          <span
                            className={`ocr-visualization-label${overlayStage === "translated" ? " is-translated" : ""}`}
                            style={{ animationDelay: `${idx * ocrStaggerMs + 120}ms` }}
                          >
                            {label}
                          </span>
                        </div>
                      )
                    })}
                  </div>
                )}
                {showOCROverlay && overlayBlocks.length > 0 && overlayStage !== "idle" && (
                  <div className={`ocr-visualization-badge ocr-visualization-badge-${overlayStage}`}>
                    {overlayStage === "ocr" ? `Detected ${overlayBlocks.length} regions` : `Translated ${overlayBlocks.length} regions`}
                  </div>
                )}
              </div>
              <div className="upload-preview-meta">
                <strong>{uploadSelection.name}</strong>
                <span>{formatFileSize(uploadSelection.size)} · Image</span>
                {overlayStage === "ocr" && (
                  <span className="upload-preview-stage-note">Animating scan regions…</span>
                )}
                {overlayStage === "translated" && (
                  <span className="upload-preview-stage-note">Regions now show translated text.</span>
                )}
                {!showOCROverlay && overlayStage !== "idle" && (
                  <span className="upload-preview-stage-note">Visual overlay hidden.</span>
                )}
              </div>
            </div>
          ) : uploadSelection?.kind === "pdf" ? (
            <div className="upload-preview upload-preview-pdf">
              <div className="upload-preview-file-icon" aria-hidden="true">PDF</div>
              <div className="upload-preview-meta">
                <strong>{uploadSelection.name}</strong>
                <span>{formatFileSize(uploadSelection.size)} · queued for background translation</span>
              </div>
            </div>
          ) : (
            <div className="upload-preview upload-preview-empty-panel">
              <button
                type="button"
                className={`upload-preview upload-preview-empty upload-drop-target${uploadDragActive ? " is-drag-active" : ""}`}
                disabled={ocrLoading || pdfLoading}
                onClick={() => openUploadPicker(uploadBrowseAccept)}
              >
                <strong>{uploadDragActive ? "Release to upload" : uploadEmptyTitle}</strong>
                <span>{uploadEmptySupport}</span>
              </button>
              <div className="upload-example-grid" aria-label="Example files">
                {DEMO_PRESETS.map(preset => (
                  <button
                    key={preset.id}
                    type="button"
                    className="upload-example-card"
                    disabled={ocrLoading || demoModeActive}
                    onClick={() => handleDemoPreset(preset)}
                    title={`Load ${preset.label}`}
                  >
                    <img src={preset.src} alt="" aria-hidden="true" loading="lazy" />
                    <span className="upload-example-meta">
                      <strong>{preset.label}</strong>
                      <span>{preset.source === "auto" ? "auto" : preset.source.toUpperCase()} → {preset.target.toUpperCase()}</span>
                    </span>
                  </button>
                ))}
              </div>
            </div>
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
            <button
              type="button"
              className={`icon-btn${showOCROverlay ? " is-active" : ""}`}
              onClick={() => setShowOCROverlay(v => !v)}
              aria-pressed={showOCROverlay}
              title="Show or hide visual overlay"
            >
              {showOCROverlay ? "Hide overlay" : "Show overlay"}
            </button>
            <label className="ocr-stagger-control">
              Region stagger
              <input
                type="range"
                min={20}
                max={220}
                step={10}
                value={ocrStaggerMs}
                onChange={event => setOcrStaggerMs(Number(event.target.value))}
              />
              <span>{ocrStaggerMs} ms</span>
            </label>
            <div className="ocr-stagger-presets" role="group" aria-label="Region stagger presets">
              {OCR_STAGGER_PRESETS.map(preset => (
                <button
                  key={preset.id}
                  type="button"
                  className={`icon-btn${ocrStaggerMs === preset.ms ? " is-active" : ""}`}
                  onClick={() => setOcrStaggerMs(preset.ms)}
                >
                  {preset.label}
                </button>
              ))}
            </div>
            <button
              type="button"
              className={`icon-btn ocr-demo-btn${overlayDemoRunning ? " is-active" : ""}`}
              onClick={runOverlayDemo}
              disabled={ocrLoading || overlayStage === "idle" || overlayBlocks.length === 0}
              title="Replay visual pipeline stages on the current preview (Shift+D)"
            >
              {overlayDemoRunning ? "Replaying demo..." : "Replay demo"}
            </button>
            <span className="ocr-demo-hint" aria-live="polite">Shortcut: Shift+D</span>
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
                  ? `Active stage: ${IMAGE_PROGRESS_STAGES.find(stage => stage.key === imageProgressStage)?.label ?? "Detecting text"}`
                  : imageProgressStatus === "done"
                  ? "All stages finished"
                  : "Job stopped before completion"}
              </span>
            </div>
            <div className="image-progress-meter" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(stageProgressRatio * 100)}>
              <div className="image-progress-meter-fill" style={{ transform: `scaleX(${stageProgressRatio})` }} />
            </div>
            <div className="image-progress-live-row">
              <span>{imageProgressStage === "translating" ? translationProgressText : (livePhaseLabel ?? "Rendering typography")}</span>
              <span>{Math.round(((liveChunkProgress ?? stageProgressRatio) * 100))}%</span>
            </div>
            {reliabilityHint && (
              <div className="image-progress-reliability" role="status">{reliabilityHint}</div>
            )}
            <ol className="image-progress-stages">
              {IMAGE_PROGRESS_STAGES.map(stage => {
                const done = imageProgressCompleted.includes(stage.key)
                const active = imageProgressStatus === "running" && imageProgressStage === stage.key
                const failed = imageProgressStatus === "error" && imageProgressFailedStage === stage.key
                return (
                  <li
                    key={stage.key}
                    className={`image-progress-stage${done ? " is-done" : ""}${active ? " is-active" : ""}${failed ? " is-failed" : ""}`}
                  >
                    <span className="image-progress-dot" aria-hidden="true" />
                    <span>{stage.label}</span>
                    {done && <span className="image-progress-state">Done</span>}
                    {active && <span className="image-progress-state">Running</span>}
                    {failed && <span className="image-progress-state">Failed</span>}
                  </li>
                )
              })}
            </ol>
          </section>
        )}

        {(overlayBlocks.length > 0 || imageProgressStatus === "running") && (
          <section className="intelligence-panel" aria-live="polite">
            <div className="intelligence-header">
              <strong>Live intelligence</strong>
              <span>{imageProgressStatus === "running" ? "Analyzing in real time" : "Analysis complete"}</span>
            </div>
            <div className="intelligence-grid">
              <article className="intelligence-card">
                <span className="intelligence-label">Text regions detected</span>
                <strong>{animatedRegionCount}</strong>
              </article>
              <article className="intelligence-card">
                <span className="intelligence-label">Languages</span>
                <strong>{animatedLanguageCount}</strong>
                <div className="intelligence-tags">
                  {intelligenceLanguageCodes.length === 0 ? (
                    <span className="intelligence-tag">Waiting…</span>
                  ) : (
                    intelligenceLanguageCodes.map(code => (
                      <span key={code} className="intelligence-tag">{langLabel(code)}</span>
                    ))
                  )}
                </div>
              </article>
              <article className="intelligence-card">
                <span className="intelligence-label">Vertical typography</span>
                <strong>{animatedVerticalCount ? "Detected" : "None"}</strong>
              </article>
              <article className="intelligence-card">
                <span className="intelligence-label">RTL text</span>
                <strong>{animatedRTLCount ? "Detected" : "None"}</strong>
              </article>
              <article className="intelligence-card">
                <span className="intelligence-label">Confidence</span>
                <strong>{animatedConfidence}%</strong>
              </article>
            </div>
          </section>
        )}

        {pipelineWarning && (
          <section className="status-banner" role="status" aria-live="polite">
            <span className="status-banner-icon" aria-hidden="true">⚠</span>
            <span>{pipelineWarning}</span>
            <button
              type="button"
              className="icon-btn"
              onClick={() => {
                setShowReliability(true)
                setShowStatusDetail(false)
              }}
            >
              Open reliability
            </button>
            <button
              type="button"
              className="icon-btn"
              onClick={() => setPipelineWarning(null)}
            >
              Dismiss
            </button>
          </section>
        )}

        {/* Comparison spotlight */}
        {ocrLoading && compareOriginalUrl && (
          <div className="comparison-loading comparison-loading-spotlight">
            <span className="spinner" aria-hidden="true" />
            {imageProgressStage === "detecting"
              ? "Detecting text regions..."
              : imageProgressStage === "layout"
              ? "Understanding layout and reading order..."
              : imageProgressStage === "languages"
              ? "Detecting language families and script direction..."
              : imageProgressStage === "translating"
              ? "Translating content..."
              : imageProgressStage === "typography"
              ? "Rebuilding typography and styling..."
              : imageProgressStage === "rendering"
              ? "Rendering final image..."
              : mode === "extract"
              ? "Extracting text..."
              : "Preparing visual comparison — overlay and layout are processing..."}
          </div>
        )}

        {!ocrLoading && compareOriginalUrl && compareOverlayUrl && compareLayoutUrl && (
          <section
            ref={compareSectionRef}
            className={`comparison-wrap comparison-wrap--full comparison-hero${compareRevealActive ? " comparison-reveal" : ""}`}
            aria-label="Image comparison"
          >
            <div className="comparison-intro">
              <div className="comparison-kicker">Comparison first</div>
              <h3>Original, Fast, Studio</h3>
              <p>See the source, the quick pass, and the premium layout result side by side.</p>
            </div>
            <div className="comparison-controls">
              <div className="comparison-segment" role="group" aria-label="Comparison layout">
                <button
                  type="button"
                  className={`icon-btn${compareView === "side" ? " is-active" : ""}`}
                  onClick={() => setCompareView("side")}
                >
                  Grid
                </button>
                <button
                  type="button"
                  className={`icon-btn${compareView === "slider" ? " is-active" : ""}`}
                  onClick={() => setCompareView("slider")}
                >
                  Slider
                </button>
                <button
                  type="button"
                  className="icon-btn"
                  onClick={() => {
                    setComparisonModalFocus("layout")
                    setComparisonModalSliderTarget("layout")
                    setComparisonModalFlashTarget("layout")
                    setComparisonModalView("slider")
                    setComparisonModalOpen(true)
                  }}
                >
                  Fullscreen
                </button>
              </div>
              {compareView === "slider" && (
                <div className="comparison-segment" role="group" aria-label="Slider target">
                  <button
                    type="button"
                    className={`icon-btn${sliderTarget === "overlay" ? " is-active" : ""}`}
                    onClick={() => setSliderTarget("overlay")}
                  >
                    Fast
                  </button>
                  <button
                    type="button"
                    className={`icon-btn${sliderTarget === "layout" ? " is-active" : ""}`}
                    onClick={() => setSliderTarget("layout")}
                  >
                    Studio
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
                    <figcaption>Fast</figcaption>
                    <button
                      type="button"
                      className="icon-btn compare-dl-btn"
                      title="Download fast result"
                      onClick={() => handleDownload(compareOverlayUrl, "overlay-translation")}
                    >
                      ⬇ Download
                    </button>
                  </div>
                  <img
                    src={compareOverlayUrl}
                    alt="Fast translation result"
                    loading="lazy"
                    className="compare-clickable"
                    onClick={() => openComparisonModal("overlay")}
                  />
                </figure>
                <figure className="compare-card">
                  <div className="compare-card-header">
                    <figcaption>Studio</figcaption>
                    <button
                      type="button"
                      className="icon-btn compare-dl-btn"
                      title="Download studio result"
                      onClick={() => handleDownload(compareLayoutUrl, "layout-translation")}
                    >
                      ⬇ Download
                    </button>
                  </div>
                  <img
                    src={compareLayoutUrl}
                    alt="Studio translation result"
                    loading="lazy"
                    className="compare-clickable"
                    onClick={() => openComparisonModal("layout")}
                  />
                </figure>
              </div>
            ) : compareView === "slider" ? (
              <div className="compare-slider-wrap">
                <div
                  className="compare-slider-frame"
                  aria-live="polite"
                  onMouseMove={(e) => {
                    if (!sliderDraggingRef.current) return
                    const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
                    const x = e.clientX - rect.left
                    const percent = Math.max(0, Math.min(100, (x / rect.width) * 100))
                    setSliderPercent(percent)
                  }}
                  onMouseDown={(e) => {
                    if (e.button !== 0) return
                    sliderDraggingRef.current = true
                    const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
                    const x = e.clientX - rect.left
                    setSliderPercent(Math.max(0, Math.min(100, (x / rect.width) * 100)))
                  }}
                  onMouseUp={() => { sliderDraggingRef.current = false }}
                  onMouseLeave={() => { sliderDraggingRef.current = false }}
                  onTouchMove={(e) => {
                    if (!sliderDraggingRef.current) return
                    const touch = e.touches[0]
                    if (!touch) return
                    const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
                    const x = touch.clientX - rect.left
                    const percent = Math.max(0, Math.min(100, (x / rect.width) * 100))
                    setSliderPercent(percent)
                  }}
                  onTouchStart={(e) => {
                    const touch = e.touches[0]
                    if (!touch) return
                    sliderDraggingRef.current = true
                    const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
                    const x = touch.clientX - rect.left
                    setSliderPercent(Math.max(0, Math.min(100, (x / rect.width) * 100)))
                  }}
                  onTouchEnd={() => { sliderDraggingRef.current = false }}
                >
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
                  Original vs {sliderTarget === "overlay" ? "Fast" : "Studio"} — drag to reveal
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
                    ⬇ Download Fast
                  </button>
                  <button
                    type="button"
                    className="icon-btn"
                    onClick={() => handleDownload(compareLayoutUrl, "layout-translation")}
                  >
                    ⬇ Download Studio
                  </button>
                </div>
              </div>
            ) : (
              <div className="compare-slider-wrap compare-slider-wrap--compact">
                <div className="compare-slider-frame compare-slider-frame--compact" aria-live="polite">
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
                      src={compareOverlayUrl}
                      alt="Fast comparison image"
                      loading="lazy"
                    />
                  </div>
                  <div className="compare-slider-handle" style={{ left: `${sliderPercent}%` }} aria-hidden="true" />
                </div>
              </div>
            )}
          </section>
        )}

        {/* Demo presets */}
        <div className="demo-presets" role="group" aria-label="Demo presets">
          <span className="demo-presets-label">Sample images:</span>
          <button
            type="button"
            className={`icon-btn demo-mode-btn${demoModeActive ? " is-active" : ""}`}
            onClick={() => setDemoModeActive(prev => !prev)}
            title="Cycle sample images automatically"
          >
            {demoModeActive ? "⏹ Stop demo" : "▶ Demo mode"}
          </button>
          {DEMO_PRESETS.map(preset => (
            <button
              key={preset.id}
              type="button"
              className="demo-preset-btn"
              disabled={ocrLoading || demoModeActive}
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
                      {resultKind === "ocr" ? "Extracted text" : "Translated text"}
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
              <div className="panel-footer panel-footer-export">
                <span className="char-count">{result.length} chars</span>
                <div className="export-action-bar" role="group" aria-label="Export actions">
                  <button
                    className="icon-btn"
                    title="Download PNG"
                    disabled={!resultImageUrl}
                    onClick={() => resultImageUrl && handleDownload(resultImageUrl, "loklingo-output")}
                  >
                    🖼 PNG
                  </button>
                  <button className="icon-btn" title="Download PDF" onClick={handleDownloadPDF}>
                    📄 PDF
                  </button>
                  <button className="icon-btn" title="Copy translation" onClick={handleCopy}>
                    ⎘ Copy text
                  </button>
                  <button
                    className="icon-btn"
                    title="Open comparison workspace"
                    disabled={!(compareOriginalUrl || resultImageUrl)}
                    onClick={() => openComparisonModal("layout")}
                  >
                    ⟲ Open compare
                  </button>
                </div>
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
                {/* Mode and focus selectors */}
                <div className="comparison-toolbar-section">
                  <div className="comparison-segment" role="group" aria-label="Modal comparison mode">
                    <button
                      type="button"
                      className={`icon-btn${comparisonModalView === "gallery" ? " is-active" : ""}`}
                      onClick={() => setComparisonModalView("gallery")}
                      title="Gallery view (G)"
                    >
                      Gallery
                    </button>
                    <button
                      type="button"
                      className={`icon-btn${comparisonModalView === "slider" ? " is-active" : ""}`}
                      onClick={() => setComparisonModalView("slider")}
                      title="Slider view (S)"
                    >
                      Slider
                    </button>
                    <button
                      type="button"
                      className={`icon-btn${comparisonModalView === "flash" ? " is-active" : ""}`}
                      onClick={() => {
                        setComparisonModalFlashToggle(false)
                        setComparisonModalView("flash")
                      }}
                      title="Flash toggle (F)"
                    >
                      Flash
                    </button>
                  </div>
                  <div className="comparison-segment" role="group" aria-label="Focused image">
                    <button
                      type="button"
                      className={`icon-btn${comparisonModalFocus === "original" ? " is-active" : ""}`}
                      onClick={() => setComparisonModalFocus("original")}
                      title="Original image (O)"
                    >
                      Original
                    </button>
                    <button
                      type="button"
                      className={`icon-btn${comparisonModalFocus === "overlay" ? " is-active" : ""}`}
                      onClick={() => setComparisonModalFocus("overlay")}
                      title="Overlay/Fast result (V)"
                    >
                      Fast
                    </button>
                    <button
                      type="button"
                      className={`icon-btn${comparisonModalFocus === "layout" ? " is-active" : ""}`}
                      onClick={() => setComparisonModalFocus("layout")}
                      title="Studio/Layout result (L)"
                    >
                      Studio
                    </button>
                  </div>
                </div>

                {/* Quick toggle and action buttons */}
                <div className="comparison-toolbar-section">
                  <button
                    type="button"
                    className={`icon-btn${comparisonQuickToggle ? " is-active" : ""}`}
                    onMouseDown={() => setComparisonQuickToggle(true)}
                    onMouseUp={() => setComparisonQuickToggle(false)}
                    onMouseLeave={() => setComparisonQuickToggle(false)}
                    onTouchStart={() => setComparisonQuickToggle(true)}
                    onTouchEnd={() => setComparisonQuickToggle(false)}
                    title="Press and hold for before/after toggle"
                  >
                    Before / After
                  </button>
                  <button
                    type="button"
                    className="icon-btn"
                    onClick={() => {
                      const img = modalImageSrc
                      if (img) handleDownload(img, "comparison-result")
                    }}
                    title="Download this image"
                  >
                    ⬇ Download
                  </button>
                  <button
                    type="button"
                    className="icon-btn"
                    onClick={() => {
                      const elem = document.querySelector(".comparison-modal-card") as HTMLElement | null
                      if (elem?.requestFullscreen) elem.requestFullscreen()
                    }}
                    title="Fullscreen view"
                  >
                    ⛶ Fullscreen
                  </button>
                </div>

                <span className="comparison-shortcuts-hint" aria-hidden="true">
                  <span className="hint-label">Keyboard:</span> G/S/F (modes) | O/V/L (views) | +/- (zoom) | ESC (close)
                </span>
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
                <div
                  className="compare-slider-frame comparison-modal-slider-frame"
                  aria-live="polite"
                  onMouseMove={(e) => {
                    if (!modalSliderDraggingRef.current) return
                    const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
                    const x = e.clientX - rect.left
                    const percent = Math.max(0, Math.min(100, (x / rect.width) * 100))
                    setComparisonModalSliderPercent(percent)
                  }}
                  onMouseDown={(e) => {
                    if (e.button !== 0) return
                    modalSliderDraggingRef.current = true
                    const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
                    const x = e.clientX - rect.left
                    setComparisonModalSliderPercent(Math.max(0, Math.min(100, (x / rect.width) * 100)))
                  }}
                  onMouseUp={() => { modalSliderDraggingRef.current = false }}
                  onMouseLeave={() => { modalSliderDraggingRef.current = false }}
                  onTouchMove={(e) => {
                    if (!modalSliderDraggingRef.current) return
                    const touch = e.touches[0]
                    if (!touch) return
                    const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
                    const x = touch.clientX - rect.left
                    const percent = Math.max(0, Math.min(100, (x / rect.width) * 100))
                    setComparisonModalSliderPercent(percent)
                  }}
                  onTouchStart={(e) => {
                    const touch = e.touches[0]
                    if (!touch) return
                    modalSliderDraggingRef.current = true
                    const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect()
                    const x = touch.clientX - rect.left
                    setComparisonModalSliderPercent(Math.max(0, Math.min(100, (x / rect.width) * 100)))
                  }}
                  onTouchEnd={() => { modalSliderDraggingRef.current = false }}
                >
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
            ) : hasModalComparison && comparisonModalView === "flash" ? (
              <div className="comparison-modal-body">
                <div
                  className={`comparison-modal-stage${comparisonModalZoom > 1 ? " is-pannable" : ""}${comparisonModalPanning ? " is-panning" : ""}`}
                  onWheel={handleComparisonModalWheel}
                  onPointerDown={handleComparisonModalPointerDown}
                  onPointerMove={handleComparisonModalPointerMove}
                  onPointerUp={handleComparisonModalPointerUp}
                  onPointerLeave={handleComparisonModalPointerUp}
                >
                  <div className="compare-flash-frame comparison-modal-flash-frame">
                    <img
                      className="compare-flash-image compare-flash-base"
                      src={compareOriginalUrl ?? ""}
                      alt="Original image"
                      style={{ transform: modalTransform, transformOrigin: "center" }}
                    />
                    <img
                      className={`compare-flash-image compare-flash-top${comparisonModalFlashToggle ? " is-visible" : ""}`}
                      src={comparisonModalFlashTarget === "overlay" ? compareOverlayUrl ?? "" : compareLayoutUrl ?? ""}
                      alt="Flash comparison"
                      style={{ transform: modalTransform, transformOrigin: "center" }}
                    />
                    <div className="compare-flash-badge">
                      {comparisonModalFlashToggle ? `Showing ${comparisonModalFlashTarget}` : "Showing original"}
                    </div>
                  </div>
                </div>
                <div className="comparison-segment" role="group" aria-label="Modal flash target">
                  <button
                    type="button"
                    className={`icon-btn${comparisonModalFlashTarget === "overlay" ? " is-active" : ""}`}
                    onClick={() => setComparisonModalFlashTarget("overlay")}
                  >
                    Flash Overlay
                  </button>
                  <button
                    type="button"
                    className={`icon-btn${comparisonModalFlashTarget === "layout" ? " is-active" : ""}`}
                    onClick={() => setComparisonModalFlashTarget("layout")}
                  >
                    Flash Layout
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
