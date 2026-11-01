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
  return apiFetch<ReadinessResponse>("/ready", { timeoutMs: 7000, acceptedStatuses: [503] })
}
