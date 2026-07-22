import {
  type OCRMetricsResponse,
  type ProviderMetricsResponse,
  type MetricsWindow,
} from "../api/metrics"
import { RELIABILITY_WINDOWS } from "../constants"
import { type CheckpointThresholds, formatPercent, levelLabel, checkpointOperatorHint, safePercent } from "../checkpointReliability"

interface ReliabilityTelemetryProps {
  visible: boolean
  window: MetricsWindow
  onWindowChange: (w: MetricsWindow) => void
  loading: boolean
  error: string | null
  metrics: OCRMetricsResponse | null
  providerMetrics: ProviderMetricsResponse | null
  providerError: string | null
  updatedAt: number | null
}

export function ReliabilityTelemetry({
  visible,
  window: metricsWindow,
  onWindowChange,
  loading,
  error,
  metrics,
  providerMetrics,
  providerError,
  updatedAt,
}: ReliabilityTelemetryProps) {
  if (!visible) return null

  return (
    <section className="reliability-panel" aria-live="polite">
      <div className="reliability-header">
        <div>
          <h2>Reliability telemetry</h2>
          <p>Live counters and windowed event aggregates from /api/v1/metrics/ocr.</p>
        </div>
        <label className="reliability-window-control">
          Window
          <select value={metricsWindow} onChange={e => onWindowChange(e.target.value as MetricsWindow)}>
            {RELIABILITY_WINDOWS.map(w => (
              <option key={w} value={w}>{w}</option>
            ))}
          </select>
        </label>
      </div>

      {loading && <p className="reliability-state">Loading reliability metrics...</p>}
      {error && <p className="reliability-state reliability-state-error">{error}</p>}

      {metrics && (
        <>
          <div className="reliability-summary-grid">
            <article className="reliability-card">
              <h3>LiteLLM</h3>
              <dl>
                <div><dt>Retries</dt><dd>{metrics.reliability.litellm.retry_attempts_total}</dd></div>
                <div><dt>Cancelled retries</dt><dd>{metrics.reliability.litellm.retry_cancelled_total}</dd></div>
                <div><dt>Exhausted retries</dt><dd>{metrics.reliability.litellm.retry_exhausted_total}</dd></div>
                <div><dt>Circuit opened</dt><dd>{metrics.reliability.litellm.circuit_opened_total}</dd></div>
                <div><dt>Circuit rejects</dt><dd>{metrics.reliability.litellm.circuit_reject_total}</dd></div>
                <div><dt>Response rejects</dt><dd>{metrics.reliability.litellm.response_rejected_total}</dd></div>
              </dl>
            </article>

            <article className="reliability-card">
              <h3>OCR</h3>
              <dl>
                <div><dt>Retries</dt><dd>{metrics.reliability.ocr.retry_attempts_total}</dd></div>
                <div><dt>Retry-After honored</dt><dd>{metrics.reliability.ocr.retry_after_honored_total}</dd></div>
                <div><dt>Cancelled retries</dt><dd>{metrics.reliability.ocr.retry_cancelled_total}</dd></div>
                <div><dt>Exhausted retries</dt><dd>{metrics.reliability.ocr.retry_exhausted_total}</dd></div>
                <div><dt>Response rejects</dt><dd>{metrics.reliability.ocr.response_rejected_total}</dd></div>
                <div><dt>OCR events ({metrics.window.name})</dt><dd>{metrics.events_total}</dd></div>
              </dl>
            </article>
          </div>

          <div className="reliability-windowed-table-wrap">
            <div className="reliability-windowed-title">Windowed reliability events ({metrics.window.name})</div>
            {metrics.reliability_windowed.events.length === 0 ? (
              <p className="reliability-state">No reliability events recorded in this window.</p>
            ) : (
              <table className="reliability-table">
                <thead>
                  <tr>
                    <th>Integration</th>
                    <th>Event</th>
                    <th>Reason</th>
                    <th>Count</th>
                  </tr>
                </thead>
                <tbody>
                  {metrics.reliability_windowed.events.slice(0, 12).map((event, idx) => (
                    <tr key={`${event.integration}:${event.event_name}:${event.reason}:${idx}`}>
                      <td>{event.integration}</td>
                      <td>{event.event_name}</td>
                      <td>{event.reason || "-"}</td>
                      <td>{event.count}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          {providerMetrics && (
            <>
              <div className="reliability-windowed-table-wrap">
                <div className="reliability-windowed-title">Provider reliability (live)</div>
                {providerError && <p className="reliability-state reliability-state-error">{providerError}</p>}
                {providerMetrics.providers.length === 0 && (
                  <p className="reliability-state">No provider calls observed yet.</p>
                )}
                {providerMetrics.providers.length > 0 && (
                  <table className="reliability-table">
                    <thead>
                      <tr>
                        <th>Provider</th>
                        <th>Success</th>
                        <th>Failures</th>
                        <th>Retries</th>
                        <th>Timeouts</th>
                        <th>Failovers</th>
                        <th>Avg latency (ms)</th>
                      </tr>
                    </thead>
                    <tbody>
                      {providerMetrics.providers.map(provider => (
                        <tr key={provider.provider}>
                          <td>{provider.provider}</td>
                          <td>{provider.success_total}</td>
                          <td>{provider.failure_total}</td>
                          <td>{provider.retry_total}</td>
                          <td>{provider.timeout_total}</td>
                          <td>{provider.failover_total}</td>
                          <td>{Math.round(provider.avg_latency_ms)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>

              {providerMetrics.timeouts.by_reason.length > 0 && (
                <div className="reliability-windowed-table-wrap">
                  <div className="reliability-windowed-title">Timeout reasons (live)</div>
                  <table className="reliability-table">
                    <thead>
                      <tr>
                        <th>Reason</th>
                        <th>Count</th>
                      </tr>
                    </thead>
                    <tbody>
                      {providerMetrics.timeouts.by_reason.map(reason => (
                        <tr key={reason.reason}>
                          <td>{reason.reason}</td>
                          <td>{reason.total}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}

              {providerMetrics && (
                <div className="reliability-windowed-table-wrap">
                  <div className="reliability-windowed-title">Checkpoint health (live)</div>
                  {(() => {
                    const checkpointThresholds: CheckpointThresholds = {
                      hitWarn: 70,
                      hitCritical: 40,
                      persistFailureWarn: 5,
                      persistFailureCritical: 15,
                    }
                    const checkpointEvents = providerMetrics.checkpoints.hit_total + providerMetrics.checkpoints.miss_total
                    const checkpointHitRate = safePercent(providerMetrics.checkpoints.hit_total, checkpointEvents)
                    const persistFailureRate = safePercent(providerMetrics.checkpoints.persist_failure_total, checkpointEvents)
                    const operatorHint = checkpointOperatorHint(checkpointHitRate, persistFailureRate, checkpointThresholds)
                    const evaluatedAt = updatedAt ? new Date(updatedAt).toLocaleTimeString() : null

                    return (
                      <>
                        <table className="reliability-table">
                          <thead>
                            <tr>
                              <th>Metric</th>
                              <th>Total</th>
                            </tr>
                          </thead>
                          <tbody>
                            <tr>
                              <td>Checkpoint hits</td>
                              <td>{providerMetrics.checkpoints.hit_total}</td>
                            </tr>
                            <tr>
                              <td>Checkpoint misses</td>
                              <td>{providerMetrics.checkpoints.miss_total}</td>
                            </tr>
                            <tr>
                              <td>Persist failures</td>
                              <td>{providerMetrics.checkpoints.persist_failure_total}</td>
                            </tr>
                            <tr>
                              <td>Cleanup runs</td>
                              <td>{providerMetrics.checkpoints.clear_total}</td>
                            </tr>
                            <tr>
                              <td>Cleanup failures</td>
                              <td>{providerMetrics.checkpoints.clear_failure_total}</td>
                            </tr>
                            <tr>
                              <td>Hit rate</td>
                              <td>
                                <span className={`reliability-rate-badge reliability-rate-level-${operatorHint.hitLevel}`}>
                                  {formatPercent(checkpointHitRate)} - {levelLabel(operatorHint.hitLevel)}
                                </span>
                              </td>
                            </tr>
                            <tr>
                              <td>Persist failure rate</td>
                              <td>
                                <span className={`reliability-rate-badge reliability-rate-level-${operatorHint.persistLevel}`}>
                                  {formatPercent(persistFailureRate)} - {levelLabel(operatorHint.persistLevel)}
                                </span>
                              </td>
                            </tr>
                          </tbody>
                        </table>
                        <p className={`reliability-operator-hint reliability-operator-hint-${operatorHint.level}`}>
                          {operatorHint.message}
                          {evaluatedAt && (
                            <span className="reliability-operator-hint-meta"> Evaluated at {evaluatedAt}.</span>
                          )}
                        </p>
                      </>
                    )
                  })()}
                </div>
              )}
            </>
          )}

          {updatedAt && (
            <p className="reliability-updated">Updated {new Date(updatedAt).toLocaleTimeString()}</p>
          )}
        </>
      )}
    </section>
  )
}
