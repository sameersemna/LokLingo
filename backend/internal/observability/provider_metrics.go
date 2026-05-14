package observability

import (
	"sync"
	"sync/atomic"
)

// providerCounters holds per-provider atomic reliability counters.
type providerCounters struct {
	success        atomic.Int64
	failures       atomic.Int64
	retries        atomic.Int64
	exhausted      atomic.Int64
	failovers      atomic.Int64
	timeouts       atomic.Int64
	latencyTotalMS atomic.Int64
	latencyCount   atomic.Int64
	latencyLE250   atomic.Int64
	latencyLE500   atomic.Int64
	latencyLE1000  atomic.Int64
	latencyLE2000  atomic.Int64
	latencyGT2000  atomic.Int64
}

type timeoutReasonCounters struct {
	count atomic.Int64
}

var timeoutReasonMetricsMap sync.Map

type renderFailureCounters struct {
	total atomic.Int64
}

var renderFailureMetricsMap sync.Map

type chunkCheckpointCounters struct {
	hits            atomic.Int64
	misses          atomic.Int64
	persistFailures atomic.Int64
	clears          atomic.Int64
	clearFailures   atomic.Int64
}

var chunkCheckpointMetrics chunkCheckpointCounters

type degradedModeCounters struct {
	total                atomic.Int64
	renderFallbacks      atomic.Int64
	ocrLowConfidence     atomic.Int64
	adaptiveReduceEvents atomic.Int64
	adaptiveBoostEvents  atomic.Int64
	adaptiveClampEvents  atomic.Int64
}

var degradedModeMetrics degradedModeCounters

// providerMetricsMap stores *providerCounters keyed by provider name.
var providerMetricsMap sync.Map

func getProviderCounters(name string) *providerCounters {
	v, _ := providerMetricsMap.LoadOrStore(name, &providerCounters{})
	return v.(*providerCounters)
}

// IncProviderSuccess records a successful translation call on provider.
func IncProviderSuccess(provider string) { getProviderCounters(provider).success.Add(1) }

// IncProviderFailure records a final failure on provider (all retries exhausted
// or a permanent error) before failing over to the next provider.
func IncProviderFailure(provider string) { getProviderCounters(provider).failures.Add(1) }

// RecordProviderRetry records one transient retry on provider.
func RecordProviderRetry(provider string) { getProviderCounters(provider).retries.Add(1) }

// IncProviderExhausted records that provider exhausted all retry attempts.
func IncProviderExhausted(provider string) { getProviderCounters(provider).exhausted.Add(1) }

// IncProviderFailover records that provider was chosen as a failover target
// (i.e. a previous provider failed and this one succeeded).
func IncProviderFailover(provider string) { getProviderCounters(provider).failovers.Add(1) }

// RecordProviderTimeout records timeout-classified failures by provider and reason.
func RecordProviderTimeout(provider, reason string) {
	if provider == "" {
		provider = "unknown"
	}
	getProviderCounters(provider).timeouts.Add(1)
	if reason == "" {
		reason = "unknown"
	}
	v, _ := timeoutReasonMetricsMap.LoadOrStore(reason, &timeoutReasonCounters{})
	v.(*timeoutReasonCounters).count.Add(1)
}

// IncRenderFailure records a rendering failure by mode (e.g. layout, fast).
func IncRenderFailure(mode string) {
	if mode == "" {
		mode = "unknown"
	}
	v, _ := renderFailureMetricsMap.LoadOrStore(mode, &renderFailureCounters{})
	v.(*renderFailureCounters).total.Add(1)
}

// IncChunkCheckpointHit records one checkpoint cache hit.
func IncChunkCheckpointHit() { chunkCheckpointMetrics.hits.Add(1) }

// IncChunkCheckpointMiss records one checkpoint cache miss.
func IncChunkCheckpointMiss() { chunkCheckpointMetrics.misses.Add(1) }

// IncChunkCheckpointPersistFailure records one failed checkpoint persist.
func IncChunkCheckpointPersistFailure() { chunkCheckpointMetrics.persistFailures.Add(1) }

// IncChunkCheckpointClear records one terminal checkpoint cleanup run.
func IncChunkCheckpointClear() { chunkCheckpointMetrics.clears.Add(1) }

// IncChunkCheckpointClearFailure records one failed terminal checkpoint cleanup.
func IncChunkCheckpointClearFailure() { chunkCheckpointMetrics.clearFailures.Add(1) }

// IncDegradedMode records a graceful degradation event in processing paths.
func IncDegradedMode() { degradedModeMetrics.total.Add(1) }

// IncRenderFallback records rendering fallback activations.
func IncRenderFallback() {
	degradedModeMetrics.renderFallbacks.Add(1)
	degradedModeMetrics.total.Add(1)
}

// IncOCRLowConfidence records low-confidence OCR continuation events.
func IncOCRLowConfidence() {
	degradedModeMetrics.ocrLowConfidence.Add(1)
	degradedModeMetrics.total.Add(1)
}

// IncAdaptiveConcurrencyReduce records queue-pressure concurrency reductions.
func IncAdaptiveConcurrencyReduce() { degradedModeMetrics.adaptiveReduceEvents.Add(1) }

// IncAdaptiveConcurrencyBoost records low-pressure concurrency boosts.
func IncAdaptiveConcurrencyBoost() { degradedModeMetrics.adaptiveBoostEvents.Add(1) }

// IncAdaptiveConcurrencyClamp records severe-overload clamped concurrency events.
func IncAdaptiveConcurrencyClamp() { degradedModeMetrics.adaptiveClampEvents.Add(1) }

// RecordProviderLatency records a successful call latency in milliseconds.
func RecordProviderLatency(provider string, ms int64) {
	c := getProviderCounters(provider)
	c.latencyTotalMS.Add(ms)
	c.latencyCount.Add(1)
	switch {
	case ms <= 250:
		c.latencyLE250.Add(1)
	case ms <= 500:
		c.latencyLE500.Add(1)
	case ms <= 1000:
		c.latencyLE1000.Add(1)
	case ms <= 2000:
		c.latencyLE2000.Add(1)
	default:
		c.latencyGT2000.Add(1)
	}
}

// ProviderSnapshot captures per-provider counters at a point in time.
type ProviderSnapshot struct {
	Provider       string                 `json:"provider"`
	SuccessTotal   int64                  `json:"success_total"`
	FailureTotal   int64                  `json:"failure_total"`
	RetryTotal     int64                  `json:"retry_total"`
	ExhaustedTotal int64                  `json:"exhausted_total"`
	FailoverTotal  int64                  `json:"failover_total"`
	TimeoutTotal   int64                  `json:"timeout_total"`
	AvgLatencyMS   float64                `json:"avg_latency_ms"`
	LatencyBuckets ProviderLatencyBuckets `json:"latency_buckets"`
}

// ProviderLatencyBuckets is a compact latency distribution for provider health views.
type ProviderLatencyBuckets struct {
	LE250MS  int64 `json:"le_250_ms"`
	LE500MS  int64 `json:"le_500_ms"`
	LE1000MS int64 `json:"le_1000_ms"`
	LE2000MS int64 `json:"le_2000_ms"`
	GT2000MS int64 `json:"gt_2000_ms"`
}

// TimeoutReasonSnapshot captures timeout frequencies grouped by reason.
type TimeoutReasonSnapshot struct {
	Reason string `json:"reason"`
	Total  int64  `json:"total"`
}

// RenderFailureSnapshot captures render failures grouped by render mode.
type RenderFailureSnapshot struct {
	Mode  string `json:"mode"`
	Total int64  `json:"total"`
}

// ChunkCheckpointSnapshot captures resume checkpoint health counters.
type ChunkCheckpointSnapshot struct {
	HitTotal            int64 `json:"hit_total"`
	MissTotal           int64 `json:"miss_total"`
	PersistFailureTotal int64 `json:"persist_failure_total"`
	ClearTotal          int64 `json:"clear_total"`
	ClearFailureTotal   int64 `json:"clear_failure_total"`
}

// DegradedModeSnapshot captures graceful degradation and adaptive concurrency events.
type DegradedModeSnapshot struct {
	TotalEvents            int64 `json:"total_events"`
	RenderFallbackEvents   int64 `json:"render_fallback_events"`
	OCRLowConfidenceEvents int64 `json:"ocr_low_confidence_events"`
	AdaptiveReduceEvents   int64 `json:"adaptive_reduce_events"`
	AdaptiveBoostEvents    int64 `json:"adaptive_boost_events"`
	AdaptiveClampEvents    int64 `json:"adaptive_clamp_events"`
}

// SnapshotProviderMetrics returns a snapshot for every provider that has been
// observed since process start.
func SnapshotProviderMetrics() []ProviderSnapshot {
	var out []ProviderSnapshot
	providerMetricsMap.Range(func(k, v any) bool {
		name := k.(string)
		c := v.(*providerCounters)
		cnt := c.latencyCount.Load()
		avg := 0.0
		if cnt > 0 {
			avg = float64(c.latencyTotalMS.Load()) / float64(cnt)
		}
		out = append(out, ProviderSnapshot{
			Provider:       name,
			SuccessTotal:   c.success.Load(),
			FailureTotal:   c.failures.Load(),
			RetryTotal:     c.retries.Load(),
			ExhaustedTotal: c.exhausted.Load(),
			FailoverTotal:  c.failovers.Load(),
			TimeoutTotal:   c.timeouts.Load(),
			AvgLatencyMS:   avg,
			LatencyBuckets: ProviderLatencyBuckets{
				LE250MS:  c.latencyLE250.Load(),
				LE500MS:  c.latencyLE500.Load(),
				LE1000MS: c.latencyLE1000.Load(),
				LE2000MS: c.latencyLE2000.Load(),
				GT2000MS: c.latencyGT2000.Load(),
			},
		})
		return true
	})
	return out
}

// SnapshotTimeoutReasonMetrics returns timeout frequency by classified reason.
func SnapshotTimeoutReasonMetrics() []TimeoutReasonSnapshot {
	var out []TimeoutReasonSnapshot
	timeoutReasonMetricsMap.Range(func(k, v any) bool {
		reason := k.(string)
		c := v.(*timeoutReasonCounters)
		out = append(out, TimeoutReasonSnapshot{Reason: reason, Total: c.count.Load()})
		return true
	})
	return out
}

// SnapshotRenderFailureMetrics returns render failure counts by mode.
func SnapshotRenderFailureMetrics() []RenderFailureSnapshot {
	var out []RenderFailureSnapshot
	renderFailureMetricsMap.Range(func(k, v any) bool {
		mode := k.(string)
		c := v.(*renderFailureCounters)
		out = append(out, RenderFailureSnapshot{Mode: mode, Total: c.total.Load()})
		return true
	})
	return out
}

// SnapshotChunkCheckpointMetrics returns chunk checkpoint health counters.
func SnapshotChunkCheckpointMetrics() ChunkCheckpointSnapshot {
	return ChunkCheckpointSnapshot{
		HitTotal:            chunkCheckpointMetrics.hits.Load(),
		MissTotal:           chunkCheckpointMetrics.misses.Load(),
		PersistFailureTotal: chunkCheckpointMetrics.persistFailures.Load(),
		ClearTotal:          chunkCheckpointMetrics.clears.Load(),
		ClearFailureTotal:   chunkCheckpointMetrics.clearFailures.Load(),
	}
}

// SnapshotDegradedModeMetrics returns graceful degradation and adaptive events.
func SnapshotDegradedModeMetrics() DegradedModeSnapshot {
	return DegradedModeSnapshot{
		TotalEvents:            degradedModeMetrics.total.Load(),
		RenderFallbackEvents:   degradedModeMetrics.renderFallbacks.Load(),
		OCRLowConfidenceEvents: degradedModeMetrics.ocrLowConfidence.Load(),
		AdaptiveReduceEvents:   degradedModeMetrics.adaptiveReduceEvents.Load(),
		AdaptiveBoostEvents:    degradedModeMetrics.adaptiveBoostEvents.Load(),
		AdaptiveClampEvents:    degradedModeMetrics.adaptiveClampEvents.Load(),
	}
}
