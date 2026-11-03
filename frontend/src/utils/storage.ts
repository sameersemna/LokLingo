export const STORAGE_KEYS = {
  HISTORY: "loklingo-history",
  OCR_VISUAL_CONTROLS: "loklingo-ocr-visual-controls",
  FIRST_VISIT: "loklingo-first-visit",
  LANGUAGE_SELECTION: "loklingo-langs",
  THEME: "loklingo-theme",
  PDF_JOBS: "loklingo-pdf-jobs",
  INTERNAL_TOKEN: "loklingo-internal-token",
  WRITE_TOKEN: "loklingo-write-token",
  DEAD_OPS_AUTO_REFRESH: "loklingo-dead-ops-auto-refresh",
  OCR_ENGINE: "loklingo-ocr-engine",
} as const

export type StorageKey = (typeof STORAGE_KEYS)[keyof typeof STORAGE_KEYS]

export function getSessionToken(key: typeof STORAGE_KEYS.INTERNAL_TOKEN | typeof STORAGE_KEYS.WRITE_TOKEN): string {
  try {
    return sessionStorage.getItem(key)?.trim() ?? ""
  } catch {
    return ""
  }
}

export function saveSessionCredentials(writeToken: string, internalToken: string): void {
  const previousWrite = getSessionToken(STORAGE_KEYS.WRITE_TOKEN)
  const previousInternal = getSessionToken(STORAGE_KEYS.INTERNAL_TOKEN)
  try {
    if (writeToken.trim()) sessionStorage.setItem(STORAGE_KEYS.WRITE_TOKEN, writeToken.trim())
    else sessionStorage.removeItem(STORAGE_KEYS.WRITE_TOKEN)
    if (internalToken.trim()) sessionStorage.setItem(STORAGE_KEYS.INTERNAL_TOKEN, internalToken.trim())
    else sessionStorage.removeItem(STORAGE_KEYS.INTERNAL_TOKEN)
  } catch (error) {
    if (previousWrite) sessionStorage.setItem(STORAGE_KEYS.WRITE_TOKEN, previousWrite)
    else sessionStorage.removeItem(STORAGE_KEYS.WRITE_TOKEN)
    if (previousInternal) sessionStorage.setItem(STORAGE_KEYS.INTERNAL_TOKEN, previousInternal)
    else sessionStorage.removeItem(STORAGE_KEYS.INTERNAL_TOKEN)
    throw error
  }
  removeStorageItem(STORAGE_KEYS.INTERNAL_TOKEN)
}

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
