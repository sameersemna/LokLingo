import { useCallback, useRef, useState } from "react"
import { MOTION } from "../motion"

export interface Toast {
  id: number
  msg: string
  type: "success" | "error"
}

/**
 * A small toast queue with auto-dismiss after MOTION.toastMs.
 * Returns the active toasts and a `push` callback to enqueue new ones.
 */
export function useToasts(): {
  toasts: Toast[]
  push: (msg: string, type: Toast["type"]) => void
  dismiss: (id: number) => void
} {
  const [toasts, setToasts] = useState<Toast[]>([])
  const idRef = useRef(0)

  const push = useCallback((msg: string, type: Toast["type"]) => {
    const id = ++idRef.current
    setToasts(prev => [...prev, { id, msg, type }])
    window.setTimeout(() => {
      setToasts(prev => prev.filter(t => t.id !== id))
    }, MOTION.toastMs)
  }, [])

  const dismiss = useCallback((id: number) => {
    setToasts(prev => prev.filter(t => t.id !== id))
  }, [])

  return { toasts, push, dismiss }
}
