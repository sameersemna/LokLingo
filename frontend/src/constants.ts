export const LANGUAGES = [
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

export const TARGET_LANGUAGES = LANGUAGES.filter(l => l.code !== "auto")

export type ProductMode = "fast" | "studio" | "extract"

export const PRODUCT_MODE_BACKEND_MAP: Record<ProductMode, "overlay" | "layout" | "ocr_only"> = {
  fast: "overlay",
  studio: "layout",
  extract: "ocr_only",
}

export const PRODUCT_MODES = [
  { value: "fast", label: "Fast", detail: "Quick visual draft" },
  { value: "studio", label: "Studio", detail: "Premium layout fidelity" },
  { value: "extract", label: "Text-first output", detail: "" },
] as const

export const WORKFLOWS = [
  { value: "image", label: "Image", detail: "Hero workflow for instant visual translation", icon: "🖼" },
  { value: "pdf", label: "PDF", detail: "Queue full-document translation in background", icon: "📄" },
  { value: "text", label: "Text", detail: "Translate pasted or typed text", icon: "✍" },
] as const

export const MAX_CHARS = 2000
export const HISTORY_KEY = "loklingo-history"
export const MAX_HISTORY = 10
export const OCR_VIS_KEY = "loklingo-ocr-visual-controls"
export const FIRST_VISIT_KEY = "loklingo-first-visit"

export const RELIABILITY_WINDOWS = ["1h", "6h", "24h", "7d", "30d"] as const

export type MetricsWindow = (typeof RELIABILITY_WINDOWS)[number]

export const PRESSURE_WARN_THRESHOLD = Number(import.meta.env.VITE_RELIABILITY_PRESSURE_WARN ?? 8)
export const PRESSURE_CRITICAL_THRESHOLD = Number(import.meta.env.VITE_RELIABILITY_PRESSURE_CRITICAL ?? 20)
export const CHECKPOINT_HIT_WARN_THRESHOLD = Number(import.meta.env.VITE_CHECKPOINT_HIT_WARN ?? 70)
export const CHECKPOINT_HIT_CRITICAL_THRESHOLD = Number(import.meta.env.VITE_CHECKPOINT_HIT_CRITICAL ?? 40)
export const CHECKPOINT_PERSIST_FAILURE_WARN_THRESHOLD = Number(import.meta.env.VITE_CHECKPOINT_PERSIST_FAILURE_WARN ?? 5)
export const CHECKPOINT_PERSIST_FAILURE_CRITICAL_THRESHOLD = Number(import.meta.env.VITE_CHECKPOINT_PERSIST_FAILURE_CRITICAL ?? 15)

export interface HistoryEntry {
  id: number
  sourceText: string
  sourceLang: string
  targetLang: string
  result: string
  ts: number
}

export type InputWorkflow = "image" | "pdf" | "text"

export interface UploadSelection {
  kind: "image" | "pdf"
  name: string
  size: number
  previewUrl?: string
}

export type ComparisonFocus = "original" | "overlay" | "layout"

export type ProgressStage = "detecting" | "layout" | "languages" | "translating" | "typography" | "rendering"

export type OverlayStage = "idle" | "ocr" | "translated"

export interface LiveProgressDetail {
  stage?: ProgressStage
  region?: number
  totalRegions?: number
  chunkProgress?: number
  phaseLabel?: string
}

export const TRUST_SIGNALS = [
  "Local processing",
  "GPU acceleration",
  "No cloud dependency",
  "OCR confidence",
  "Layout preservation",
] as const

export const IMAGE_PROGRESS_STAGES: Array<{ key: ProgressStage; label: string }> = [
  { key: "detecting", label: "Detecting text" },
  { key: "layout", label: "Understanding layout" },
  { key: "languages", label: "Detecting languages" },
  { key: "translating", label: "Translating content" },
  { key: "typography", label: "Rebuilding typography" },
  { key: "rendering", label: "Rendering final image" },
]

export const OCR_STAGGER_PRESETS = [
  { id: "fast", label: "Fast", ms: 30 },
  { id: "default", label: "Default", ms: 70 },
  { id: "slow", label: "Slow", ms: 140 },
] as const

export const DEMO_PRESETS = [
  { id: "manga",      label: "Manga frames",     emoji: "📚", category: "Manga", src: "/samples/cjk_vertical.png",       source: "ja", target: "en" },
  { id: "menu",       label: "Restaurant menu",  emoji: "🍽", category: "Menus", src: "/samples/before_bold_style.png",  source: "auto", target: "en" },
  { id: "anime",      label: "Anime/stylized",   emoji: "🎨", category: "Anime", src: "/samples/before_mixed_styles.png", source: "en", target: "es" },
  { id: "signs",      label: "Street signs",     emoji: "🚩", category: "Signs", src: "/samples/realworld_overlay.jpg",   source: "auto", target: "en" },
  { id: "infographic", label: "Infographics",    emoji: "📊", category: "Infographics", src: "/samples/cjk_vertical.png",  source: "auto", target: "en" },
  { id: "screenshot", label: "UI screenshots",   emoji: "🖥", category: "Screenshots", src: "/samples/before_bold_style.png", source: "en", target: "fr" },
  { id: "poster",     label: "Posters & art",    emoji: "🎬", category: "Posters", src: "/samples/before_mixed_styles.png", source: "en", target: "de" },
  { id: "document",   label: "Documents/PDFs",  emoji: "📄", category: "PDFs", src: "/samples/realworld_overlay.jpg",   source: "auto", target: "en" },
] as const

export function pressureLevel(score: number): "normal" | "warn" | "critical" {
  if (score >= PRESSURE_CRITICAL_THRESHOLD) return "critical"
  if (score >= PRESSURE_WARN_THRESHOLD) return "warn"
  return "normal"
}
