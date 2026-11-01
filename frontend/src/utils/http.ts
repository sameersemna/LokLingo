import { parseApiErrorBody, buildApiError } from "./errors"
import { getSessionToken, STORAGE_KEYS } from "./storage"

const DEFAULT_TIMEOUT_MS = 30_000

export interface FetchOptions extends Omit<RequestInit, "signal"> {
  timeoutMs?: number
  baseUrl?: string
  signal?: AbortSignal
  acceptedStatuses?: number[]
}

export async function apiFetch<T = unknown>(
  url: string,
  options: FetchOptions = {},
): Promise<T> {
  const { timeoutMs = DEFAULT_TIMEOUT_MS, baseUrl = "", acceptedStatuses = [], ...init } = options
  const fullUrl = baseUrl ? `${baseUrl}${url}` : url
  const headers = new Headers(init.headers)
  if (typeof window !== "undefined") {
    const destination = new URL(fullUrl, window.location.origin)
    const writeMethod = ["POST", "PUT", "PATCH", "DELETE"].includes((init.method ?? "GET").toUpperCase())
    if (writeMethod && destination.origin === window.location.origin && destination.pathname.startsWith("/api/")) {
      const writeToken = getSessionToken(STORAGE_KEYS.WRITE_TOKEN)
      if (writeToken && !headers.has("X-API-Token")) headers.set("X-API-Token", writeToken)
    }
  }

  const controller = new AbortController()
  const callerSignal = init.signal
  let timedOut = false

  const handleCallerAbort = () => controller.abort()
  if (callerSignal) {
    if (callerSignal.aborted) {
      controller.abort()
    } else {
      callerSignal.addEventListener("abort", handleCallerAbort, { once: true })
    }
  }

  const timeoutId = setTimeout(() => {
    timedOut = true
    controller.abort()
  }, timeoutMs)

  try {
    const res = await fetch(fullUrl, { ...init, headers, signal: controller.signal })

    if (!res.ok && !acceptedStatuses.includes(res.status)) {
      const body = await parseApiErrorBody(res)
      throw buildApiError(res.status, body, `HTTP ${res.status}`)
    }

    return res.json() as Promise<T>
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") {
      if (timedOut) {
        throw new Error("Request timed out. Please try again.", { cause: err })
      }
      throw err
    }
    if (err instanceof Error && err.name === "ApiError") {
      throw err
    }
    if (err instanceof Error) {
      throw new Error("Network request failed.", { cause: err })
    }
    throw new Error("Network request failed.", { cause: err })
  } finally {
    clearTimeout(timeoutId)
    if (callerSignal) {
      callerSignal.removeEventListener("abort", handleCallerAbort)
    }
  }
}

export function buildInternalHeaders(overrideToken?: string): HeadersInit {
  const headers: HeadersInit = {}
  const token = resolveInternalToken(overrideToken)
  if (token) {
    headers["X-Internal-Token"] = token
  }
  return headers
}

export function resolveInternalToken(overrideToken?: string): string | null {
  if (overrideToken && overrideToken.trim()) {
    return overrideToken.trim()
  }
  return getSessionToken(STORAGE_KEYS.INTERNAL_TOKEN) || null
}
