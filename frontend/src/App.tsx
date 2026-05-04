import { useCallback, useEffect, useRef, useState } from "react"
import { translate } from "./api/translate"
import { extractText } from "./api/ocr"
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

function App() {
  const [theme, setTheme] = useState<"light" | "dark">(() => {
    const saved = localStorage.getItem("loklingo-theme")
    if (saved === "light" || saved === "dark") return saved
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"
  })
  const [sourceText, setSourceText] = useState("")
  const [sourceLang, setSourceLang] = useState("auto")
  const [targetLang, setTargetLang] = useState("de")
  const [result, setResult] = useState("")
  const [detectedLang, setDetectedLang] = useState("")
  const [loading, setLoading] = useState(false)
  const [ocrLoading, setOcrLoading] = useState(false)
  const [showHistory, setShowHistory] = useState(false)
  const [history, setHistory] = useState<HistoryEntry[]>(loadHistory)
  const [toasts, setToasts] = useState<Toast[]>([])
  const toastId = useRef(0)
  const histId = useRef(history.length)
  const fileRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme)
    localStorage.setItem("loklingo-theme", theme)
  }, [theme])

  const pushToast = useCallback((msg: string, type: "success" | "error") => {
    const id = ++toastId.current
    setToasts(t => [...t, { id, msg, type }])
    setTimeout(() => setToasts(t => t.filter(x => x.id !== id)), 3500)
  }, [])

  const handleTranslate = useCallback(async () => {
    if (!sourceText.trim()) return
    setLoading(true)
    setResult("")
    setDetectedLang("")
    try {
      const res = await translate({ text: sourceText, source: sourceLang, target: targetLang })
      setResult(res.translated_text)
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
  }, [sourceText, sourceLang, targetLang, pushToast])

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
    setDetectedLang("")
  }

  const handleOCRFile = async (file: File) => {
    if (!file.type.startsWith("image/")) {
      pushToast("Please upload an image file", "error")
      return
    }
    setOcrLoading(true)
    try {
      const res = await extractText(file, sourceLang === 'auto' ? 'auto' : sourceLang)
      setSourceText(res.text)
      pushToast(`OCR complete — confidence ${Math.round(res.confidence * 100)}%`, "success")
    } catch (err) {
      pushToast(err instanceof Error ? err.message : "OCR failed", "error")
    } finally {
      setOcrLoading(false)
      if (fileRef.current) fileRef.current.value = ""
    }
  }

  const langLabel = (code: string) =>
    LANGUAGES.find(l => l.code === code)?.label ?? code.toUpperCase()

  const charCount = sourceText.length
  const nearLimit = charCount > MAX_CHARS * 0.8

  return (
    <div className="app">
      <header className="app-header">
        <div className="header-row">
          <h1>LokLingo</h1>
          <div className="header-actions">
            <button
              className="icon-btn"
              type="button"
              onClick={() => setShowHistory(h => !h)}
              title="Translation history"
            >
              ⏱ History {history.length > 0 && <span className="badge">{history.length}</span>}
            </button>
            <button
              className="theme-btn"
              type="button"
              aria-label="Toggle color scheme"
              onClick={() => setTheme(t => t === "dark" ? "light" : "dark")}
            >
              {theme === "dark" ? "☀ Light" : "☾ Dark"}
            </button>
          </div>
        </div>
        <p className="tagline">Self-hosted translation &mdash; no limits</p>
      </header>

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
              <span className="detected-badge">Detected: {langLabel(detectedLang)}</span>
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
                {charCount}/{MAX_CHARS}
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
                <label htmlFor="ocr-file" className="icon-btn" title="Extract text from image (OCR)">
                  {ocrLoading
                    ? <><span className="spinner spinner-sm" aria-hidden="true" /> Extracting…</>
                    : "📷 OCR"
                  }
                </label>
                {sourceText && (
                  <button
                    className="icon-btn"
                    title="Clear"
                    onClick={() => { setSourceText(""); setResult(""); setDetectedLang("") }}
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
              {loading ? (
                <span className="status">
                  <span className="spinner" aria-hidden="true" />
                  Translating…
                </span>
              ) : (
                <div className="result-text">{result}</div>
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
            disabled={loading || !sourceText.trim()}
          >
            {loading ? "Translating…" : "Translate"}
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
