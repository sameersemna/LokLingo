export const STORAGE_KEYS = {
  HISTORY: "loklingo-history",
  OCR_VISUAL_CONTROLS: "loklingo-ocr-visual-controls",
  FIRST_VISIT: "loklingo-first-visit",
  LANGUAGE_SELECTION: "loklingo-langs",
  THEME: "loklingo-theme",
  PDF_JOBS: "loklingo-pdf-jobs",
  INTERNAL_TOKEN: "loklingo-internal-token",
  DEAD_OPS_AUTO_REFRESH: "loklingo-dead-ops-auto-refresh",
} as const

export type StorageKey = (typeof STORAGE_KEYS)[keyof typeof STORAGE_KEYS]

export function getStorageItem<T>(key: StorageKey, fallback: T): T {
  try {
    const raw = localStorage.getItem(key)
    if (raw === null) return fallback
    return JSON.parse(raw) as T
  } catch {
    return fallback
  }
}

export function setStorageItem<T>(key: StorageKey, value: T): void {
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch {
    // Storage full or unavailable — silently degrade
  }
}

export function removeStorageItem(key: StorageKey): void {
  try {
    localStorage.removeItem(key)
  } catch {
    // Storage unavailable — silently degrade
  }
}
