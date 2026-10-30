import { useCallback, useEffect, useEffectEvent, useRef, useState } from "react"
import { DEMO_PRESETS, FIRST_VISIT_KEY, type InputWorkflow } from "../constants"
import { MOTION } from "../motion"

export interface UseDemoShowcaseParams {
  runImageFile: (file: File, source: string, target: string) => Promise<void>
  handleDemoPreset: (preset: typeof DEMO_PRESETS[number]) => Promise<void>
  setWorkflow: (workflow: InputWorkflow) => void
  setSourceLang: (value: string) => void
  setTargetLang: (value: string) => void
  setCompareView: (view: "side" | "slider" | "flash") => void
  setSliderTarget: (target: "overlay" | "layout") => void
  ocrLoading: boolean
  pdfLoading: boolean
  loading: boolean
}

export interface UseDemoShowcaseApi {
  showcaseMode: boolean
  toggleShowcase: () => void
  demoModeActive: boolean
  toggleDemoMode: () => void
  stopDemoMode: () => void
}

/**
 * First-visit onboarding auto-load, the "Showcase" auto-cycle-through-demos
 * mode, and the comparison view's own demo-cycling mode. All three load a
 * DEMO_PRESETS entry and run it through the image pipeline on a timer.
 */
export function useDemoShowcase({
  runImageFile,
  handleDemoPreset,
  setWorkflow,
  setSourceLang,
  setTargetLang,
  setCompareView,
  setSliderTarget,
  ocrLoading,
  pdfLoading,
  loading,
}: UseDemoShowcaseParams): UseDemoShowcaseApi {
  const [showcaseMode, setShowcaseMode] = useState(false)
  const [demoModeActive, setDemoModeActive] = useState(false)
  const [demoModeIndex, setDemoModeIndex] = useState(0)
  const demoModeTimerRef = useRef<number | null>(null)

  const loadDemo = useEffectEvent(async (preset: typeof DEMO_PRESETS[number]) => {
    setWorkflow("image")
    setSourceLang(preset.source)
    setTargetLang(preset.target)
    const response = await fetch(preset.src)
    const blob = await response.blob()
    const file = new File([blob], `demo-${preset.id}.png`, { type: blob.type })
    await runImageFile(file, preset.source, preset.target)
  })

  // First-visit onboarding: Auto-load demo for new users
  useEffect(() => {
    const isFirstVisit = !localStorage.getItem(FIRST_VISIT_KEY)
    if (isFirstVisit) {
      localStorage.setItem(FIRST_VISIT_KEY, "true")
      // Auto-load first demo preset (manga) after a short delay to avoid jarring UX
      const demoTimer = window.setTimeout(() => {
        loadDemo(DEMO_PRESETS[0])
          .catch(error => {
            console.warn("Onboarding demo failed", error)
          })
      }, 800)

      return () => window.clearTimeout(demoTimer)
    }
  }, [])

  // Showcase mode: Auto-cycle through demos
  useEffect(() => {
    if (!showcaseMode) return

    let currentIndex = 0
    let timerId: number | null = null
    let cancelled = false

    const loadNextDemo = () => {
      if (cancelled) return
      const preset = DEMO_PRESETS[currentIndex % DEMO_PRESETS.length]
      loadDemo(preset)
        .then(() => {
          if (cancelled) return
          currentIndex++

          // Schedule next demo after processing + 6 seconds display time
          timerId = window.setTimeout(loadNextDemo, MOTION.imageProgress.successSettleMs + 6000)
        })
        .catch(() => {
          if (cancelled) return
          // Skip to next demo on error
          currentIndex++
          timerId = window.setTimeout(loadNextDemo, 2000)
        })
    }

    // Start showcase after 1 second
    const startTimer = window.setTimeout(loadNextDemo, 1000)

    return () => {
      cancelled = true
      window.clearTimeout(startTimer)
      if (timerId !== null) window.clearTimeout(timerId)
    }
  }, [showcaseMode])

  useEffect(() => {
    if (!demoModeActive || ocrLoading || pdfLoading || loading) {
      if (demoModeTimerRef.current !== null) {
        window.clearTimeout(demoModeTimerRef.current)
        demoModeTimerRef.current = null
      }
      return
    }

    const preset = DEMO_PRESETS[demoModeIndex % DEMO_PRESETS.length]
    const kickoff = window.setTimeout(() => {
      const cycle = demoModeIndex % 3
      if (cycle === 0) {
        setCompareView("side")
      } else if (cycle === 1) {
        setSliderTarget(demoModeIndex % 2 === 0 ? "layout" : "overlay")
        setCompareView("slider")
      } else {
        setSliderTarget(demoModeIndex % 2 === 0 ? "layout" : "overlay")
        setCompareView("slider")
      }
      void handleDemoPreset(preset)
    }, 0)
    demoModeTimerRef.current = window.setTimeout(() => {
      setDemoModeIndex(prev => (prev + 1) % DEMO_PRESETS.length)
    }, MOTION.demoCycleMs)

    return () => {
      window.clearTimeout(kickoff)
      if (demoModeTimerRef.current !== null) {
        window.clearTimeout(demoModeTimerRef.current)
        demoModeTimerRef.current = null
      }
    }
  }, [demoModeActive, demoModeIndex, handleDemoPreset, loading, ocrLoading, pdfLoading, setCompareView, setSliderTarget])

  const toggleShowcase = useCallback(() => setShowcaseMode(s => !s), [])
  const toggleDemoMode = useCallback(() => setDemoModeActive(prev => !prev), [])
  const stopDemoMode = useCallback(() => setDemoModeActive(false), [])

  return { showcaseMode, toggleShowcase, demoModeActive, toggleDemoMode, stopDemoMode }
}
