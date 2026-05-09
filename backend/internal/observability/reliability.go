package observability

import "sync/atomic"

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
	RetryAttemptsTotal         int64 `json:"retry_attempts_total"`
	RetryAfterHonoredTotal     int64 `json:"retry_after_honored_total"`
	RetryCancelledTotal        int64 `json:"retry_cancelled_total"`
	RetryExhaustedTotal        int64 `json:"retry_exhausted_total"`
	ResponseRejectedTotal      int64 `json:"response_rejected_total"`
	ResponseRejectedBodyTotal  int64 `json:"response_rejected_body_total"`
	ResponseRejectedJSONTotal  int64 `json:"response_rejected_json_total"`
	ResponseRejectedShapeTotal int64 `json:"response_rejected_shape_total"`
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
		},
	}
}
