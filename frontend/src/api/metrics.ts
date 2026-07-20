import { apiFetch, buildInternalHeaders } from "../utils/http"

export type MetricsWindow = "1h" | "6h" | "24h" | "7d" | "30d"

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

export interface ProviderMetricSnapshot {
  provider: string
  success_total: number
  failure_total: number
  retry_total: number
  exhausted_total: number
  failover_total: number
  timeout_total: number
  avg_latency_ms: number
}

export interface TimeoutReasonSnapshot {
  reason: string
  total: number
}

export interface RenderFailureSnapshot {
  mode: string
  total: number
}

export interface CheckpointMetricsSnapshot {
  hit_total: number
  miss_total: number
  persist_failure_total: number
  clear_total: number
  clear_failure_total: number
}

export interface ProviderMetricsResponse {
  providers: ProviderMetricSnapshot[]
  timeouts: {
    by_reason: TimeoutReasonSnapshot[]
  }
  render_failures: RenderFailureSnapshot[]
  checkpoints: CheckpointMetricsSnapshot
}

export async function getOCRMetrics(window: MetricsWindow): Promise<OCRMetricsResponse> {
  return apiFetch<OCRMetricsResponse>(`/api/v1/metrics/ocr?window=${encodeURIComponent(window)}`, {
    headers: buildInternalHeaders(),
    timeoutMs: 7000,
  })
}

export async function getProviderMetrics(): Promise<ProviderMetricsResponse> {
  return apiFetch<ProviderMetricsResponse>("/api/v1/metrics/providers", {
    headers: buildInternalHeaders(),
    timeoutMs: 7000,
  })
}
