import { type HistoryEntry } from "../constants"
import { History, RotateCcw, Trash2 } from "lucide-react"

interface HistoryPanelProps {
  history: HistoryEntry[]
  onClearAll: () => void
  onRestore: (entry: HistoryEntry) => void
  onClose: () => void
  langLabel: (code: string) => string
}

export function HistoryPanel({ history, onClearAll, onRestore, onClose, langLabel }: HistoryPanelProps) {
  return (
    <aside className="history-panel" aria-label="Recent translations">
      <div className="history-header">
        <h2>Recent translations</h2>
        {history.length > 0 && (
          <button className="icon-btn" onClick={() => { if (window.confirm("Clear recent translations from this browser?")) onClearAll() }}>
            <Trash2 size={16} aria-hidden="true" /> Clear all
          </button>
        )}
      </div>
      {history.length === 0 ? (
        <p className="history-empty"><History size={32} aria-hidden="true" /> No translations yet.</p>
      ) : (
        <ul className="history-list">
          {history.map(h => (
            <li
              key={h.id}
              className="history-item"
            >
              <div className="history-entry-head"><div className="history-langs">
                {langLabel(h.sourceLang)} → {langLabel(h.targetLang)}
              </div><button type="button" className="icon-btn" onClick={() => { onRestore(h); onClose() }}><RotateCcw size={15} aria-hidden="true" /> Restore</button></div>
              <div className="history-preview">{h.sourceText.slice(0, 80)}{h.sourceText.length > 80 ? "…" : ""}</div>
              <div className="history-result-preview" dir="auto">{h.result.slice(0, 140)}{h.result.length > 140 ? "..." : ""}</div>
            </li>
          ))}
        </ul>
      )}
    </aside>
  )
}
