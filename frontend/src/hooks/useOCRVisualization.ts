import { useEffect, useState, type Dispatch, type SetStateAction } from "react"
import { OCR_VIS_KEY } from "../constants"

const STAGGER_MIN = 20
const STAGGER_MAX = 220
const STAGGER_DEFAULT = 70

interface PersistedControls {
  show?: unknown
  staggerMs?: unknown
}

function load(): { show: boolean; staggerMs: number } {
  try {
    const raw = JSON.parse(localStorage.getItem(OCR_VIS_KEY) ?? "{}") as PersistedControls
    const show = typeof raw.show === "boolean" ? raw.show : true
    const parsedStagger = Number(raw.staggerMs)
    const staggerMs = Number.isFinite(parsedStagger)
      ? Math.max(STAGGER_MIN, Math.min(STAGGER_MAX, Math.round(parsedStagger)))
      : STAGGER_DEFAULT
    return { show, staggerMs }
  } catch {
    return { show: true, staggerMs: STAGGER_DEFAULT }
  }
}

export interface OCRVisualizationApi {
  show: boolean
  setShow: Dispatch<SetStateAction<boolean>>
  staggerMs: number
  setStaggerMs: Dispatch<SetStateAction<number>>
  reset: () => void
}

/**
 * Persisted user preferences for the OCR overlay visualization: visibility
 * toggle and the per-block stagger animation interval (ms).
 */
export function useOCRVisualization(): OCRVisualizationApi {
  const initial = load()
  const [show, setShow] = useState<boolean>(initial.show)
  const [staggerMs, setStaggerMs] = useState<number>(initial.staggerMs)

  useEffect(() => {
    localStorage.setItem(OCR_VIS_KEY, JSON.stringify({ show, staggerMs }))
  }, [show, staggerMs])

  const reset = () => {
    setShow(true)
    setStaggerMs(STAGGER_DEFAULT)
  }

  return { show, setShow, staggerMs, setStaggerMs, reset }
}
