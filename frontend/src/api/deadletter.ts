import { apiFetch, buildInternalHeaders } from "../utils/http"

export interface DeadLetterJobItem {
  job_id: string
  reason?: string
  status?: string
  type?: string
  mode?: string
  source?: string
  target?: string
  attempt?: number
  max_attempts?: number
  dead_lettered_at?: string
  updated_at?: string
}

export interface DeadLetterListResponse {
  count: number
  jobs: DeadLetterJobItem[]
}

export interface ReplayDeadJobResponse {
  job_id: string
  status: string
  attempt: number
  max_attempts: number
}

export async function listDeadJobs(limit = 25, token?: string): Promise<DeadLetterListResponse> {
  const safeLimit = Math.max(1, Math.min(200, Math.floor(limit)))
  return apiFetch<DeadLetterListResponse>(`/api/v1/jobs/dead?limit=${safeLimit}`, {
    method: "GET",
    headers: buildInternalHeaders(token),
    timeoutMs: 10_000,
  })
}

export async function replayDeadJob(jobId: string, token?: string): Promise<ReplayDeadJobResponse> {
  const id = jobId.trim()
  if (!id) {
    throw new Error("job id is required")
  }
  return apiFetch<ReplayDeadJobResponse>(`/api/v1/jobs/${encodeURIComponent(id)}/replay`, {
    method: "POST",
    headers: buildInternalHeaders(token),
    timeoutMs: 10_000,
  })
}
