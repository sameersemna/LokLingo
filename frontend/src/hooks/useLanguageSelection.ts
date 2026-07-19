import { useEffect, useState, type Dispatch, type SetStateAction } from "react"
import { LANGUAGES, TARGET_LANGUAGES } from "../constants"

const STORAGE_KEY = "loklingo-langs"

interface PersistedLangs {
  source?: string
  target?: string
}

function loadLangs(): { source: string; target: string } {
  try {
    const saved = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "{}") as PersistedLangs
    const src = LANGUAGES.find(l => l.code === saved.source)?.code ?? "auto"
    const tgt = TARGET_LANGUAGES.find(l => l.code === saved.target)?.code ?? "de"
    return { source: src, target: tgt }
  } catch {
    return { source: "auto", target: "de" }
  }
}

/**
 * Tracks the source and target language for translation requests and
 * persists the selection to localStorage.
 */
export function useLanguageSelection(): {
  source: string
  target: string
  setSource: Dispatch<SetStateAction<string>>
  setTarget: Dispatch<SetStateAction<string>>
  swap: () => boolean
} {
  const [source, setSource] = useState(() => loadLangs().source)
  const [target, setTarget] = useState(() => loadLangs().target)

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ source, target }))
  }, [source, target])

  const swap = (): boolean => {
    if (source === "auto") return false
    setSource(target)
    setTarget(source)
    return true
  }

  return { source, target, setSource, setTarget, swap }
}
