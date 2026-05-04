import { useEffect, useState } from "react"
import { translate } from "./api/translate"
import "./App.css"

const LANGUAGES = [
  { code: "en", label: "English" },
  { code: "de", label: "German" },
  { code: "fr", label: "French" },
  { code: "hi", label: "Hindi" },
  { code: "ur", label: "Urdu" },
  { code: "ar", label: "Arabic" },
  { code: "bn", label: "Bengali" },
  { code: "es", label: "Spanish" },
  { code: "zh", label: "Chinese" },
  { code: "ja", label: "Japanese" },
]

function App() {
  const [theme, setTheme] = useState<"light" | "dark">(() => {
    const saved = localStorage.getItem("loklingo-theme")
    if (saved === "light" || saved === "dark") return saved
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"
  })
  const [sourceText, setSourceText] = useState("")
  const [sourceLang, setSourceLang] = useState("en")
  const [targetLang, setTargetLang] = useState("de")
  const [result, setResult] = useState("")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme)
    localStorage.setItem("loklingo-theme", theme)
  }, [theme])

  const handleTranslate = async () => {
    if (!sourceText.trim()) return
    setLoading(true)
    setError("")
    setResult("")
    try {
      const res = await translate({
        text: sourceText,
        source: sourceLang,
        target: targetLang,
      })
      setResult(res.translated_text)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Translation failed")
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="app">
      <header className="app-header">
        <div className="header-row">
          <h1>LokLingo</h1>
          <button
            className="theme-btn"
            type="button"
            onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
          >
            {theme === "dark" ? "Light" : "Dark"} mode
          </button>
        </div>
        <p className="tagline">Self-hosted translation &mdash; no limits</p>
      </header>

      <main className="translator">
        <div className="lang-selectors">
          <select value={sourceLang} onChange={e => setSourceLang(e.target.value)}>
            {LANGUAGES.map(l => (
              <option key={l.code} value={l.code}>{l.label}</option>
            ))}
          </select>

          <button
            className="swap-btn"
            title="Swap languages"
            onClick={() => {
              setSourceLang(targetLang)
              setTargetLang(sourceLang)
              setSourceText(result)
              setResult(sourceText)
            }}
          >
            &harr;
          </button>

          <select value={targetLang} onChange={e => setTargetLang(e.target.value)}>
            {LANGUAGES.map(l => (
              <option key={l.code} value={l.code}>{l.label}</option>
            ))}
          </select>
        </div>

        <div className="panels">
          <div className="panel">
            <textarea
              placeholder="Enter text to translate..."
              value={sourceText}
              onChange={e => setSourceText(e.target.value)}
              rows={8}
            />
          </div>

          <div className="panel output">
            {loading ? (
              <span className="status">Translating...</span>
            ) : error ? (
              <span className="status error">{error}</span>
            ) : (
              <div className="result-text">{result}</div>
            )}
          </div>
        </div>

        <button
          className="translate-btn"
          onClick={handleTranslate}
          disabled={loading || !sourceText.trim()}
        >
          {loading && <span className="spinner" aria-hidden="true" />}
          {loading ? "Translating…" : "Translate"}
        </button>
      </main>
    </div>
  )
}

export default App
