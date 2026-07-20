import { apiFetch } from "../utils/http"

export interface DependencyStatus {
  status: "ok" | "error"
  error?: string
}

export interface ReadinessResponse {
  status: "ok" | "degraded"
  service: string
  dependencies: Record<string, DependencyStatus>
}

export async function getReadiness(): Promise<ReadinessResponse> {
  const res = await fetch("/ready", { signal: AbortSignal.timeout(7000) })
  if (!res.ok && res.status !== 503) {
    throw new Error(`Readiness check returned HTTP ${res.status}`)
  }
  return res.json()
}
