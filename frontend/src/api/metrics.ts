export type MetricsWindow = '1h' | '6h' | '24h' | '7d' | '30d'

export interface ReliabilitySnapshotCounterGroup {
  retry_attempts_total: number
  retry_cancelled_total: number
  retry_exhausted_total: number
  response_rejected_total: number
  response_rejected_body_total: number
  response_rejected_json_total: number
  response_rejected_shape_total: number
}

export interface LiteLLMReliabilitySnapshot extends ReliabilitySnapshotCounterGroup {
  circuit_reject_total: number
  circuit_opened_total: number
  circuit_recovered_total: number
}

export interface OCRReliabilitySnapshot extends ReliabilitySnapshotCounterGroup {
  retry_after_honored_total: number
}

export interface ReliabilityWindowedEvent {
  integration: string
  event_name: string
  reason: string
  count: number
}

export interface OCRMetricsResponse {
  window: {
    name: string
  }
  events_total: number
  succeeded_total: number
  success_rate_pct: number
  outcomes: Record<string, number>
  reliability: {
    litellm: LiteLLMReliabilitySnapshot
    ocr: OCRReliabilitySnapshot
  }
  reliability_windowed: {
    events: ReliabilityWindowedEvent[]
  }
  latency: {
    p50_total_ms: number
    p95_total_ms: number
    p99_total_ms: number
  }
}

function parseErrorMessage(status: number, payload: unknown): string {
  if (typeof payload === 'object' && payload !== null && 'error' in payload) {
    const v = (payload as { error?: unknown }).error
    if (typeof v === 'string' && v.trim()) {
      return v
    }
  }
  return `Metrics request returned HTTP ${status}`
}

export async function getOCRMetrics(window: MetricsWindow): Promise<OCRMetricsResponse> {
  const internalToken = import.meta.env.VITE_INTERNAL_TOKEN as string | undefined
  const headers: HeadersInit = {}
  if (internalToken && internalToken.trim()) {
    headers['X-Internal-Token'] = internalToken.trim()
  }

  const res = await fetch(`/api/v1/metrics/ocr?window=${encodeURIComponent(window)}`, {
    headers,
    signal: AbortSignal.timeout(7000),
  })

  if (!res.ok) {
    const payload = await res.json().catch(() => null)
    throw new Error(parseErrorMessage(res.status, payload))
  }

  return res.json() as Promise<OCRMetricsResponse>
}
