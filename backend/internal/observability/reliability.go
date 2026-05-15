package observability

import (
	"sync"
	"sync/atomic"
)

// ReliabilitySnapshot captures in-memory reliability counters for upstream integrations.
type ReliabilitySnapshot struct {
	LiteLLM LiteLLMMetrics `json:"litellm"`
	OCR     OCRMetrics     `json:"ocr"`
}

type LiteLLMMetrics struct {
	RetryAttemptsTotal         int64 `json:"retry_attempts_total"`
	RetryCancelledTotal        int64 `json:"retry_cancelled_total"`
	RetryExhaustedTotal        int64 `json:"retry_exhausted_total"`
	CircuitRejectTotal         int64 `json:"circuit_reject_total"`
	CircuitOpenedTotal         int64 `json:"circuit_opened_total"`
	CircuitRecoveredTotal      int64 `json:"circuit_recovered_total"`
	ResponseRejectedTotal      int64 `json:"response_rejected_total"`
	ResponseRejectedBodyTotal  int64 `json:"response_rejected_body_total"`
	ResponseRejectedJSONTotal  int64 `json:"response_rejected_json_total"`
	ResponseRejectedShapeTotal int64 `json:"response_rejected_shape_total"`
}

type OCRMetrics struct {
	RetryAttemptsTotal         int64                      `json:"retry_attempts_total"`
	RetryAfterHonoredTotal     int64                      `json:"retry_after_honored_total"`
	RetryCancelledTotal        int64                      `json:"retry_cancelled_total"`
	RetryExhaustedTotal        int64                      `json:"retry_exhausted_total"`
	ResponseRejectedTotal      int64                      `json:"response_rejected_total"`
	ResponseRejectedBodyTotal  int64                      `json:"response_rejected_body_total"`
	ResponseRejectedJSONTotal  int64                      `json:"response_rejected_json_total"`
	ResponseRejectedShapeTotal int64                      `json:"response_rejected_shape_total"`
	ProviderUsage              []OCRProviderUsageSnapshot `json:"provider_usage"`
	ProviderLatencyMS          OCRAggregateMetric         `json:"provider_latency_ms"`
	ProviderConfidence         OCRAggregateMetric         `json:"provider_confidence"`
	ProviderRetriesTotal       int64                      `json:"provider_retries_total"`
	ProviderFallbackCount      int64                      `json:"provider_fallback_count"`
}

type OCRProviderUsageSnapshot struct {
	Provider string `json:"provider"`
	Count    int64  `json:"count"`
}

type OCRAggregateMetric struct {
	Total float64 `json:"total"`
	Count int64   `json:"count"`
	Avg   float64 `json:"avg"`
}

type ocrProviderUsageCounter struct {
	count atomic.Int64
}

var (
	liteLLMRetryAttempts         atomic.Int64
	liteLLMRetryCancelled        atomic.Int64
	liteLLMRetryExhausted        atomic.Int64
	liteLLMCircuitReject         atomic.Int64
	liteLLMCircuitOpened         atomic.Int64
	liteLLMCircuitRecovered      atomic.Int64
	liteLLMResponseRejected      atomic.Int64
	liteLLMResponseRejectedBody  atomic.Int64
	liteLLMResponseRejectedJSON  atomic.Int64
	liteLLMResponseRejectedShape atomic.Int64

	ocrRetryAttempts         atomic.Int64
	ocrRetryAfterHonored     atomic.Int64
	ocrRetryCancelled        atomic.Int64
	ocrRetryExhausted        atomic.Int64
	ocrResponseRejected      atomic.Int64
	ocrResponseRejectedBody  atomic.Int64
	ocrResponseRejectedJSON  atomic.Int64
	ocrResponseRejectedShape atomic.Int64

	ocrProviderLatencyTotalMS  atomic.Int64
	ocrProviderLatencyCount    atomic.Int64
	ocrProviderConfidenceMilli atomic.Int64
	ocrProviderConfidenceCount atomic.Int64
	ocrProviderRetriesTotal    atomic.Int64
	ocrProviderFallbackTotal   atomic.Int64
	ocrProviderUsageMap        sync.Map
)

func IncLiteLLMRetryAttempt()     { liteLLMRetryAttempts.Add(1) }
func IncLiteLLMRetryCancelled()   { liteLLMRetryCancelled.Add(1) }
func IncLiteLLMRetryExhausted()   { liteLLMRetryExhausted.Add(1) }
func IncLiteLLMCircuitReject()    { liteLLMCircuitReject.Add(1) }
func IncLiteLLMCircuitOpened()    { liteLLMCircuitOpened.Add(1) }
func IncLiteLLMCircuitRecovered() { liteLLMCircuitRecovered.Add(1) }
func IncLiteLLMResponseRejectedBody() {
	liteLLMResponseRejected.Add(1)
	liteLLMResponseRejectedBody.Add(1)
}
func IncLiteLLMResponseRejectedJSON() {
	liteLLMResponseRejected.Add(1)
	liteLLMResponseRejectedJSON.Add(1)
}
func IncLiteLLMResponseRejectedShape() {
	liteLLMResponseRejected.Add(1)
	liteLLMResponseRejectedShape.Add(1)
}

func IncOCRRetryAttempt()      { ocrRetryAttempts.Add(1) }
func IncOCRRetryAfterHonored() { ocrRetryAfterHonored.Add(1) }
func IncOCRRetryCancelled()    { ocrRetryCancelled.Add(1) }
func IncOCRRetryExhausted()    { ocrRetryExhausted.Add(1) }
func IncOCRResponseRejectedBody() {
	ocrResponseRejected.Add(1)
	ocrResponseRejectedBody.Add(1)
}
func IncOCRResponseRejectedJSON() {
	ocrResponseRejected.Add(1)
	ocrResponseRejectedJSON.Add(1)
}
func IncOCRResponseRejectedShape() {
	ocrResponseRejected.Add(1)
	ocrResponseRejectedShape.Add(1)
}

func getOCRProviderUsageCounter(provider string) *ocrProviderUsageCounter {
	if provider == "" {
		provider = "unknown"
	}
	v, _ := ocrProviderUsageMap.LoadOrStore(provider, &ocrProviderUsageCounter{})
	return v.(*ocrProviderUsageCounter)
}

func IncOCRProviderUsed(provider string) {
	getOCRProviderUsageCounter(provider).count.Add(1)
}

func RecordOCRProviderLatency(ms int64) {
	if ms < 0 {
		ms = 0
	}
	ocrProviderLatencyTotalMS.Add(ms)
	ocrProviderLatencyCount.Add(1)
}

func RecordOCRProviderConfidence(confidence float64) {
	if confidence <= 0 {
		return
	}
	ocrProviderConfidenceMilli.Add(int64(confidence * 1000))
	ocrProviderConfidenceCount.Add(1)
}

func IncOCRProviderRetries(n int64) {
	if n > 0 {
		ocrProviderRetriesTotal.Add(n)
	}
}

func IncOCRProviderFallback() {
	ocrProviderFallbackTotal.Add(1)
}

func snapshotOCRProviderUsage() []OCRProviderUsageSnapshot {
	out := make([]OCRProviderUsageSnapshot, 0, 3)
	ocrProviderUsageMap.Range(func(k, v any) bool {
		provider := k.(string)
		counter := v.(*ocrProviderUsageCounter)
		out = append(out, OCRProviderUsageSnapshot{Provider: provider, Count: counter.count.Load()})
		return true
	})
	return out
}

func buildOCRAggregate(total float64, count int64) OCRAggregateMetric {
	avg := 0.0
	if count > 0 {
		avg = total / float64(count)
	}
	return OCRAggregateMetric{Total: total, Count: count, Avg: avg}
}

func SnapshotReliability() ReliabilitySnapshot {
	return ReliabilitySnapshot{
		LiteLLM: LiteLLMMetrics{
			RetryAttemptsTotal:         liteLLMRetryAttempts.Load(),
			RetryCancelledTotal:        liteLLMRetryCancelled.Load(),
			RetryExhaustedTotal:        liteLLMRetryExhausted.Load(),
			CircuitRejectTotal:         liteLLMCircuitReject.Load(),
			CircuitOpenedTotal:         liteLLMCircuitOpened.Load(),
			CircuitRecoveredTotal:      liteLLMCircuitRecovered.Load(),
			ResponseRejectedTotal:      liteLLMResponseRejected.Load(),
			ResponseRejectedBodyTotal:  liteLLMResponseRejectedBody.Load(),
			ResponseRejectedJSONTotal:  liteLLMResponseRejectedJSON.Load(),
			ResponseRejectedShapeTotal: liteLLMResponseRejectedShape.Load(),
		},
		OCR: OCRMetrics{
			RetryAttemptsTotal:         ocrRetryAttempts.Load(),
			RetryAfterHonoredTotal:     ocrRetryAfterHonored.Load(),
			RetryCancelledTotal:        ocrRetryCancelled.Load(),
			RetryExhaustedTotal:        ocrRetryExhausted.Load(),
			ResponseRejectedTotal:      ocrResponseRejected.Load(),
			ResponseRejectedBodyTotal:  ocrResponseRejectedBody.Load(),
			ResponseRejectedJSONTotal:  ocrResponseRejectedJSON.Load(),
			ResponseRejectedShapeTotal: ocrResponseRejectedShape.Load(),
			ProviderUsage:              snapshotOCRProviderUsage(),
			ProviderLatencyMS:          buildOCRAggregate(float64(ocrProviderLatencyTotalMS.Load()), ocrProviderLatencyCount.Load()),
			ProviderConfidence:         buildOCRAggregate(float64(ocrProviderConfidenceMilli.Load())/1000.0, ocrProviderConfidenceCount.Load()),
			ProviderRetriesTotal:       ocrProviderRetriesTotal.Load(),
			ProviderFallbackCount:      ocrProviderFallbackTotal.Load(),
		},
	}
}
