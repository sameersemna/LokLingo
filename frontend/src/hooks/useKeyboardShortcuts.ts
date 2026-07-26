import { useEffect } from "react"
import type { ComparisonFocus, InputWorkflow } from "../constants"
import type { ModalView } from "./useComparisonModal"

export interface UseKeyboardShortcutsParams {
  comparisonModalOpen: boolean
  comparisonModalView: ModalView
  hasModalComparison: boolean
  workflow: InputWorkflow
  handleTranslate: () => void
  runOverlayDemo: () => void
  closeModal: () => void
  closeComparisonModal: () => void
  adjustComparisonModalZoom: (delta: number) => void
  resetComparisonModalZoom: () => void
  setModalSliderPercent: (updater: number | ((prev: number) => number)) => void
  setModalView: (updater: ModalView | ((prev: ModalView) => ModalView)) => void
  setComparisonQuickToggle: (on: boolean) => void
  setModalFocus: (focus: ComparisonFocus) => void
}

/**
 * All global keydown/keyup shortcuts: Escape/Ctrl+Enter/modal zoom/slider
 * arrows/flash-toggle in the main handler, the space-bar quick-toggle
 * release, Shift+D to replay the overlay demo, and the comparison modal's
 * single-letter mode shortcuts (s/g/f/o/v/l).
 */
export function useKeyboardShortcuts({
  comparisonModalOpen,
  comparisonModalView,
  hasModalComparison,
  workflow,
  handleTranslate,
  runOverlayDemo,
  closeModal,
  closeComparisonModal,
  adjustComparisonModalZoom,
  resetComparisonModalZoom,
  setModalSliderPercent,
  setModalView,
  setComparisonQuickToggle,
  setModalFocus,
}: UseKeyboardShortcutsParams): void {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null
      const inEditable = Boolean(
        target && (
          target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.tagName === "SELECT" ||
          target.isContentEditable
        )
      )
      if (e.key === "Escape" && comparisonModalOpen) {
        closeModal()
        return
      }
      if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
        if (!inEditable || workflow === "text") {
          handleTranslate()
        }
        return
      }
      if (!comparisonModalOpen || inEditable) return

      if (e.key === "+" || e.key === "=") {
        e.preventDefault()
        adjustComparisonModalZoom(0.16)
        return
      }
      if (e.key === "-") {
        e.preventDefault()
        adjustComparisonModalZoom(-0.16)
        return
      }
      if (e.key === "0") {
        e.preventDefault()
        resetComparisonModalZoom()
        return
      }

      if (comparisonModalView === "slider") {
        if (e.key === "ArrowLeft") {
          e.preventDefault()
          setModalSliderPercent(prev => Math.max(0, prev - 4))
          return
        }
        if (e.key === "ArrowRight") {
          e.preventDefault()
          setModalSliderPercent(prev => Math.min(100, prev + 4))
          return
        }
      }

      if (e.key.toLowerCase() === "f") {
        e.preventDefault()
        setModalView(prev => (prev === "flash" ? "slider" : "flash"))
        return
      }

      if (e.key === " ") {
        e.preventDefault()
        setComparisonQuickToggle(true)
      }
    }
    window.addEventListener("keydown", handler)
    return () => window.removeEventListener("keydown", handler)
  }, [comparisonModalOpen, comparisonModalView, handleTranslate, workflow, closeModal, adjustComparisonModalZoom, resetComparisonModalZoom, setModalSliderPercent, setModalView, setComparisonQuickToggle])

  useEffect(() => {
    if (!comparisonModalOpen) return
    const release = (e: KeyboardEvent) => {
      if (e.key === " ") {
        setComparisonQuickToggle(false)
      }
    }
    window.addEventListener("keyup", release)
    return () => window.removeEventListener("keyup", release)
  }, [comparisonModalOpen, setComparisonQuickToggle])

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null
      const inEditable = Boolean(
        target && (
          target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.tagName === "SELECT" ||
          target.isContentEditable
        )
      )
      if (inEditable) return
      if (event.shiftKey && !event.ctrlKey && !event.metaKey && !event.altKey && event.key.toLowerCase() === "d") {
        event.preventDefault()
        runOverlayDemo()
      }
    }
    window.addEventListener("keydown", handler)
    return () => window.removeEventListener("keydown", handler)
  }, [runOverlayDemo])

  // Keyboard shortcuts for comparison modal modes
  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (!comparisonModalOpen) return
      const target = event.target as HTMLElement | null
      const inEditable = Boolean(
        target && (
          target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.tagName === "SELECT" ||
          target.isContentEditable
        )
      )
      if (inEditable) return

      const key = event.key.toLowerCase()

      // Comparison mode shortcuts (only when modal is open)
      if (!event.ctrlKey && !event.metaKey && !event.shiftKey && !event.altKey) {
        if (key === "s") {
          event.preventDefault()
          if (hasModalComparison) setModalView("slider")
        } else if (key === "g") {
          event.preventDefault()
          if (hasModalComparison) setModalView("gallery")
        } else if (key === "f") {
          event.preventDefault()
          if (hasModalComparison) setModalView("flash")
        } else if (key === "o") {
          event.preventDefault()
          setModalFocus("original")
          setComparisonQuickToggle(false)
        } else if (key === "v") {
          event.preventDefault()
          setModalFocus("overlay")
          setComparisonQuickToggle(false)
        } else if (key === "l") {
          event.preventDefault()
          setModalFocus("layout")
          setComparisonQuickToggle(false)
        } else if (key === "+" || key === "=") {
          event.preventDefault()
          adjustComparisonModalZoom(0.2)
        } else if (key === "-" || key === "_") {
          event.preventDefault()
          adjustComparisonModalZoom(-0.2)
        } else if (key === "0") {
          event.preventDefault()
          resetComparisonModalZoom()
        } else if (key === "escape") {
          event.preventDefault()
          closeComparisonModal()
        }
      }
    }
    window.addEventListener("keydown", handler)
    return () => window.removeEventListener("keydown", handler)
  }, [comparisonModalOpen, hasModalComparison, closeComparisonModal, setModalView, setModalFocus, setComparisonQuickToggle, adjustComparisonModalZoom, resetComparisonModalZoom])
}
