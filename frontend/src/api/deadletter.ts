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

function resolveInternalToken(overrideToken?: string): string | null {
  if (overrideToken && overrideToken.trim()) {
    return overrideToken.trim()
  }
  const envToken = import.meta.env.VITE_INTERNAL_TOKEN as string | undefined
  if (envToken && envToken.trim()) {
    return envToken.trim()
  }
  return null
}

function buildHeaders(overrideToken?: string): HeadersInit {
  const headers: HeadersInit = {}
  const token = resolveInternalToken(overrideToken)
  if (token) {
    headers["X-Internal-Token"] = token
  }
  return headers
}

async function parseApiError(res: Response, fallback: string): Promise<Error> {
  const payload = await res.json().catch(() => null)
  if (payload && typeof payload === "object" && "error" in payload) {
    const msg = (payload as { error?: unknown }).error
    if (typeof msg === "string" && msg.trim()) {
      return new Error(msg)
    }
  }
  return new Error(fallback)
}

export async function listDeadJobs(limit = 25, token?: string): Promise<DeadLetterListResponse> {
  const safeLimit = Math.max(1, Math.min(200, Math.floor(limit)))
  const res = await fetch(`/api/v1/jobs/dead?limit=${safeLimit}`, {
    method: "GET",
    headers: buildHeaders(token),
    signal: AbortSignal.timeout(10_000),
  })
  if (!res.ok) {
    throw await parseApiError(res, `Failed to load dead-letter jobs (HTTP ${res.status})`)
  }
  return res.json() as Promise<DeadLetterListResponse>
}

export async function replayDeadJob(jobId: string, token?: string): Promise<ReplayDeadJobResponse> {
  const id = jobId.trim()
  if (!id) {
    throw new Error("job id is required")
  }
  const res = await fetch(`/api/v1/jobs/${encodeURIComponent(id)}/replay`, {
    method: "POST",
    headers: buildHeaders(token),
    signal: AbortSignal.timeout(10_000),
  })
  if (!res.ok) {
    throw await parseApiError(res, `Failed to replay job (HTTP ${res.status})`)
  }
  return res.json() as Promise<ReplayDeadJobResponse>
}
