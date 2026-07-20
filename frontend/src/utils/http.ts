import { parseApiErrorBody, buildApiError } from "./errors"

const DEFAULT_TIMEOUT_MS = 30_000

export interface FetchOptions extends Omit<RequestInit, "signal"> {
  timeoutMs?: number
  baseUrl?: string
}

export async function apiFetch<T = unknown>(
  url: string,
  options: FetchOptions = {},
): Promise<T> {
  const { timeoutMs = DEFAULT_TIMEOUT_MS, baseUrl = "", ...init } = options
  const fullUrl = baseUrl ? `${baseUrl}${url}` : url

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
    const res = await fetch(fullUrl, { ...init, signal: controller.signal })

    if (!res.ok) {
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
    if (err instanceof Error && err.message.startsWith("HTTP ")) {
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
  const envToken = import.meta.env.VITE_INTERNAL_TOKEN as string | undefined
  if (envToken && envToken.trim()) {
    return envToken.trim()
  }
  return null
}
