import { useCallback, useEffect, useRef, useState } from "react"
import { translate, translateImage, uploadPDF } from "./api/translate"
import { getReadiness, type ReadinessResponse } from "./api/health"
import { PdfJobsPanel, saveStoredJob } from "./PdfJobsPanel"
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
  { value: "overlay", label: "Basic (fast)" },
  { value: "layout", label: "Layout-preserving" },
] as const
const MAX_CHARS = 2000
const HISTORY_KEY = "loklingo-history"
const MAX_HISTORY = 10

interface Toast { id: number; msg: string; type: "success" | "error" }
interface HistoryEntry {
  id: number
  sourceText: string
  sourceLang: string
  targetLang: string
  result: string
  ts: number
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
  const [result, setResult] = useState("")
  const [resultImageUrl, setResultImageUrl] = useState<string | null>(null)
  const [compareOriginalUrl, setCompareOriginalUrl] = useState<string | null>(null)
  const [compareOverlayUrl, setCompareOverlayUrl] = useState<string | null>(null)
  const [compareLayoutUrl, setCompareLayoutUrl] = useState<string | null>(null)
  const [compareView, setCompareView] = useState<"side" | "slider">("side")
  const [sliderTarget, setSliderTarget] = useState<"overlay" | "layout">("overlay")
  const [sliderPercent, setSliderPercent] = useState(50)
  const [detectedLang, setDetectedLang] = useState("")
  const [loading, setLoading] = useState(false)
  const [ocrLoading, setOcrLoading] = useState(false)
  const [pdfLoading, setPdfLoading] = useState(false)
  const [showHistory, setShowHistory] = useState(false)
  const [showPdfJobs, setShowPdfJobs] = useState(false)
  const pdfJobsKey = useRef(0)
  const [history, setHistory] = useState<HistoryEntry[]>(loadHistory)
  const [toasts, setToasts] = useState<Toast[]>([])
  const [readiness, setReadiness] = useState<ReadinessResponse | null>(null)
  const [showStatusDetail, setShowStatusDetail] = useState(false)
  const toastId = useRef(0)
  const histId = useRef(history.length)
  const fileRef = useRef<HTMLInputElement>(null)
  const chipRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    return () => {
      if (compareOriginalUrl) URL.revokeObjectURL(compareOriginalUrl)
    }
  }, [compareOriginalUrl])

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

  const pushToast = useCallback((msg: string, type: "success" | "error") => {
    const id = ++toastId.current
    setToasts(t => [...t, { id, msg, type }])
    setTimeout(() => setToasts(t => t.filter(x => x.id !== id)), 3500)
  }, [])

  const handleTranslate = useCallback(async () => {
    if (!sourceText.trim()) return
    setLoading(true)
    setResult("")
    setResultImageUrl(null)
    if (compareOriginalUrl) {
      URL.revokeObjectURL(compareOriginalUrl)
      setCompareOriginalUrl(null)
    }
    setCompareOverlayUrl(null)
    setCompareLayoutUrl(null)
    setDetectedLang("")
    try {
      const res = await translate({ text: sourceText, source: sourceLang, target: targetLang, mode })
      setResult(res.translated_text)
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
  }, [sourceText, sourceLang, targetLang, mode, pushToast])

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "Enter") handleTranslate()
    }
    window.addEventListener("keydown", handler)
    return () => window.removeEventListener("keydown", handler)
  }, [handleTranslate])

  const handleCopy = async () => {
    if (!result) return
    try {
      await navigator.clipboard.writeText(result)
      pushToast("Copied to clipboard", "success")
    } catch {
      pushToast("Copy failed — check browser permissions", "error")
    }
  }

  const handleSwap = () => {
    if (sourceLang === "auto") return
    setSourceLang(targetLang)
    setTargetLang(sourceLang)
    setSourceText(result)
    setResult(sourceText)
    setResultImageUrl(null)
    if (compareOriginalUrl) {
      URL.revokeObjectURL(compareOriginalUrl)
      setCompareOriginalUrl(null)
    }
    setCompareOverlayUrl(null)
    setCompareLayoutUrl(null)
    setDetectedLang("")
  }

  const handleOCRFile = async (file: File) => {
    // PDF files → async translate_pdf job (enqueue + track in PDF Jobs panel)
    if (file.type === 'application/pdf' || file.name.toLowerCase().endsWith('.pdf')) {
      setPdfLoading(true)
      try {
        const { job_id } = await uploadPDF(file, sourceLang, targetLang, mode)
        saveStoredJob({
          job_id,
          filename: file.name,
          source: sourceLang,
          target: targetLang,
          submittedAt: Date.now(),
        })
        // Force panel refresh by bumping key, then show it
        pdfJobsKey.current += 1
        setShowPdfJobs(true)
        setShowHistory(false)
        pushToast('PDF uploaded — translating in background', 'success')
      } catch (err) {
        pushToast(err instanceof Error ? err.message : 'PDF upload failed', 'error')
      } finally {
        setPdfLoading(false)
        if (fileRef.current) fileRef.current.value = ""
      }
      return
    }

    if (!file.type.startsWith("image/")) {
      pushToast("Please upload an image or PDF file", "error")
      return
    }
    setOcrLoading(true)
    try {
      const originalUrl = URL.createObjectURL(file)
      if (compareOriginalUrl) {
        URL.revokeObjectURL(compareOriginalUrl)
      }
      setCompareOriginalUrl(originalUrl)

      const [overlayRes, layoutRes] = await Promise.all([
        translateImage(file, sourceLang, targetLang, "overlay"),
        translateImage(file, sourceLang, targetLang, "layout"),
      ])

      if (!overlayRes.image_url || !layoutRes.image_url) {
        throw new Error("Image comparison requires both overlay and layout outputs")
      }

      const activeRes = mode === "layout" ? layoutRes : overlayRes
      setResult(activeRes.translated_text)
      setResultImageUrl(activeRes.image_url ?? null)
      setCompareOverlayUrl(overlayRes.image_url)
      setCompareLayoutUrl(layoutRes.image_url)

      if (sourceLang === "auto") {
        const resolved = overlayRes.source && overlayRes.source !== "auto"
          ? overlayRes.source
          : layoutRes.source && layoutRes.source !== "auto"
          ? layoutRes.source
          : ""
        if (resolved) setDetectedLang(resolved)
      }
      pushToast("Image translated", "success")
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "Image translation failed", "error")
    } finally {
      setOcrLoading(false)
      if (fileRef.current) fileRef.current.value = ""
    }
  }

  const langLabel = (code: string) =>
    LANGUAGES.find(l => l.code === code)?.label ?? code.toUpperCase()

  const charCount = sourceText.length
  const nearLimit = charCount > MAX_CHARS * 0.8
  const readinessState = readiness?.status ?? "checking"
  const readinessLabel =
    readinessState === "ok" ? "System healthy" :
    readinessState === "degraded" ? "System degraded" :
    "Checking system"

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
              )}
            </div>
          )}
        </div>
      </header>

      {/* PDF Jobs panel */}
      {showPdfJobs && (
        <PdfJobsPanel key={pdfJobsKey.current} onToast={pushToast} />
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
            onChange={e => setMode(e.target.value)}
          >
            {TRANSLATION_MODES.map(m => (
              <option key={m.value} value={m.value}>{m.label}</option>
            ))}
          </select>
        </div>

        <div className="panels">
          {/* Source panel */}
          <div className="panel">
            <textarea
              aria-label="Source text"
              placeholder="Enter text to translate…"
              value={sourceText}
              maxLength={MAX_CHARS}
              onChange={e => setSourceText(e.target.value)}
              rows={8}
            />
            <div className="panel-footer">
              <span className={`char-count${nearLimit ? " near-limit" : ""}`}>
                {charCount}/{MAX_CHARS} · {sourceText.trim() ? sourceText.trim().split(/\s+/).length : 0}w
              </span>
              <div className="panel-footer-actions">
                {/* OCR upload button */}
                <input
                  ref={fileRef}
                  type="file"
                  accept="image/*,application/pdf"
                  className="ocr-input"
                  id="ocr-file"
                  onChange={e => e.target.files?.[0] && handleOCRFile(e.target.files[0])}
                />
                <label htmlFor="ocr-file" className="icon-btn" title="Extract text from image (OCR) or queue a PDF translation">
                  {ocrLoading
                    ? <><span className="spinner spinner-sm" aria-hidden="true" /> Translating image…</>
                    : pdfLoading
                    ? <><span className="spinner spinner-sm" aria-hidden="true" /> Uploading…</>
                    : "📷 Image / PDF"
                  }
                </label>
                {sourceText && (
                  <button
                    className="icon-btn"
                    title="Clear"
                    onClick={() => {
                      setSourceText("")
                      setResult("")
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
          <div className="panel">
            <div className="output-body">
              {loading || pdfLoading ? (
                <span className="status">
                  <span className="spinner" aria-hidden="true" />
                  {pdfLoading ? 'Translating PDF…' : 'Translating…'}
                </span>
              ) : (
                <>
                  <div className="result-text">{result}</div>
                  {compareOriginalUrl && compareOverlayUrl && compareLayoutUrl && (
                    <section className="comparison-wrap" aria-label="Image comparison">
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
                              Compare Overlay
                            </button>
                            <button
                              type="button"
                              className={`icon-btn${sliderTarget === "layout" ? " is-active" : ""}`}
                              onClick={() => setSliderTarget("layout")}
                            >
                              Compare Layout
                            </button>
                          </div>
                        )}
                      </div>

                      {compareView === "side" ? (
                        <div className="comparison-grid">
                          <figure className="compare-card">
                            <figcaption>Original</figcaption>
                            <img src={compareOriginalUrl} alt="Original image" loading="lazy" />
                          </figure>
                          <figure className="compare-card">
                            <figcaption>Overlay result</figcaption>
                            <img src={compareOverlayUrl} alt="Overlay translation result" loading="lazy" />
                          </figure>
                          <figure className="compare-card">
                            <figcaption>Layout result</figcaption>
                            <img src={compareLayoutUrl} alt="Layout translation result" loading="lazy" />
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
                            Reveal {sliderTarget === "overlay" ? "overlay" : "layout"} result
                            <input
                              type="range"
                              min={0}
                              max={100}
                              value={sliderPercent}
                              onChange={e => setSliderPercent(Number(e.target.value))}
                            />
                          </label>
                        </div>
                      )}
                    </section>
                  )}
                  {resultImageUrl && (
                    <div className="result-image-wrap">
                      <img
                        className="result-image"
                        src={resultImageUrl}
                        alt="Translated output preview"
                        loading="lazy"
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
          <span className="hint">Ctrl+Enter to translate</span>
          <button
            className="translate-btn"
            onClick={handleTranslate}
            disabled={loading || pdfLoading || !sourceText.trim()}
          >
            {loading
              ? <><span className="spinner spinner-sm spinner-white" aria-hidden="true" /> Translating…</>
              : pdfLoading
              ? <><span className="spinner spinner-sm spinner-white" aria-hidden="true" /> Translating PDF…</>
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
}

export default App
