import { type HistoryEntry } from "../constants"

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
        <span>Recent translations</span>
        {history.length > 0 && (
          <button className="icon-btn" onClick={onClearAll}>
            Clear all
          </button>
        )}
      </div>
      {history.length === 0 ? (
        <p className="history-empty">No translations yet.</p>
      ) : (
        <ul className="history-list">
          {history.map(h => (
            <li
              key={h.id}
              className="history-item"
              onClick={() => {
                onRestore(h)
                onClose()
              }}
            >
              <div className="history-langs">
                {langLabel(h.sourceLang)} → {langLabel(h.targetLang)}
              </div>
              <div className="history-preview">{h.sourceText.slice(0, 80)}{h.sourceText.length > 80 ? "…" : ""}</div>
            </li>
          ))}
        </ul>
      )}
    </aside>
  )
}
