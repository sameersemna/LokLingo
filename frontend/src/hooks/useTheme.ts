import { useEffect, useState, type Dispatch, type SetStateAction } from "react"

const STORAGE_KEY = "loklingo-theme"

export type Theme = "light" | "dark"

function getInitialTheme(): Theme {
  if (typeof window === "undefined") return "light"
  const saved = localStorage.getItem(STORAGE_KEY)
  if (saved === "light" || saved === "dark") return saved
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"
}

/**
 * Manages the current UI theme and persists it to localStorage.
 * Mirrors the value to the `data-theme` attribute on <html> for CSS theming.
 *
 * Returns the current theme plus a React-style setter (supports updater
 * function) and a convenience `toggle` callback.
 */
export function useTheme(): [Theme, Dispatch<SetStateAction<Theme>>, () => void] {
  const [theme, setTheme] = useState<Theme>(getInitialTheme)

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme)
    localStorage.setItem(STORAGE_KEY, theme)
  }, [theme])

  return [theme, setTheme, () => setTheme(prev => (prev === "light" ? "dark" : "light"))]
}
