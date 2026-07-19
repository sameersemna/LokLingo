import { useCallback, useEffect, useRef, useState } from "react"
import { HISTORY_KEY, MAX_HISTORY, type HistoryEntry } from "../constants"

function loadHistory(): HistoryEntry[] {
  try {
    return JSON.parse(localStorage.getItem(HISTORY_KEY) ?? "[]") as HistoryEntry[]
  } catch {
    return []
  }
}

function saveHistory(entries: HistoryEntry[]): void {
  localStorage.setItem(HISTORY_KEY, JSON.stringify(entries.slice(0, MAX_HISTORY)))
}

export interface HistoryApi {
  history: HistoryEntry[]
  add: (entry: Omit<HistoryEntry, "id" | "ts">) => HistoryEntry
  clear: () => void
}

/**
 * Translation history backed by localStorage (capped at MAX_HISTORY entries).
 * Entries are kept in newest-first order.
 */
export function useHistory(): HistoryApi {
  const [history, setHistory] = useState<HistoryEntry[]>(loadHistory)
  const nextIdRef = useRef(history.length)

  useEffect(() => {
    saveHistory(history)
  }, [history])

  const add = useCallback(
    (entry: Omit<HistoryEntry, "id" | "ts">): HistoryEntry => {
      const next: HistoryEntry = {
        ...entry,
        id: ++nextIdRef.current,
        ts: Date.now(),
      }
      setHistory(prev => [next, ...prev].slice(0, MAX_HISTORY))
      return next
    },
    [],
  )

  const clear = useCallback(() => setHistory([]), [])

  return { history, add, clear }
}
