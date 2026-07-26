import { useEffect, useRef, useState } from "react"
import { MOTION } from "../motion"

export function useAnimatedCount(target: number, durationMs = MOTION.animatedCounterMs): number {
  const [value, setValue] = useState(target)
  const valueRef = useRef(target)

  useEffect(() => {
    valueRef.current = value
  }, [value])

  useEffect(() => {
    let frame = 0
    const start = performance.now()
    const initial = valueRef.current
    const delta = target - initial
    if (delta === 0) return

    const tick = (now: number) => {
      const elapsed = now - start
      const progress = Math.min(1, elapsed / durationMs)
      const eased = 1 - Math.pow(1 - progress, 3)
      setValue(Math.round(initial + delta * eased))
      if (progress < 1) {
        frame = window.requestAnimationFrame(tick)
      }
    }

    frame = window.requestAnimationFrame(tick)
    return () => {
      if (frame) {
        window.cancelAnimationFrame(frame)
      }
    }
  }, [target, durationMs])

  return value
}
