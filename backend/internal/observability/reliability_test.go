package observability

import "testing"

func TestSnapshotReliability_LiteLLMCountersIncrease(t *testing.T) {
	before := SnapshotReliability()

	IncLiteLLMRetryAttempt()
	IncLiteLLMRetryCancelled()
	IncLiteLLMRetryExhausted()
	IncLiteLLMCircuitReject()
	IncLiteLLMCircuitOpened()
	IncLiteLLMCircuitRecovered()
	IncLiteLLMResponseRejectedBody()
	IncLiteLLMResponseRejectedJSON()
	IncLiteLLMResponseRejectedShape()

	after := SnapshotReliability()

	if got := after.LiteLLM.RetryAttemptsTotal - before.LiteLLM.RetryAttemptsTotal; got != 1 {
		t.Fatalf("retry attempts delta = %d, want 1", got)
	}
	if got := after.LiteLLM.RetryCancelledTotal - before.LiteLLM.RetryCancelledTotal; got != 1 {
		t.Fatalf("retry cancelled delta = %d, want 1", got)
	}
	if got := after.LiteLLM.RetryExhaustedTotal - before.LiteLLM.RetryExhaustedTotal; got != 1 {
		t.Fatalf("retry exhausted delta = %d, want 1", got)
	}
	if got := after.LiteLLM.CircuitRejectTotal - before.LiteLLM.CircuitRejectTotal; got != 1 {
		t.Fatalf("circuit reject delta = %d, want 1", got)
	}
	if got := after.LiteLLM.CircuitOpenedTotal - before.LiteLLM.CircuitOpenedTotal; got != 1 {
		t.Fatalf("circuit opened delta = %d, want 1", got)
	}
	if got := after.LiteLLM.CircuitRecoveredTotal - before.LiteLLM.CircuitRecoveredTotal; got != 1 {
		t.Fatalf("circuit recovered delta = %d, want 1", got)
	}
	if got := after.LiteLLM.ResponseRejectedTotal - before.LiteLLM.ResponseRejectedTotal; got != 3 {
		t.Fatalf("response rejected total delta = %d, want 3", got)
	}
	if got := after.LiteLLM.ResponseRejectedBodyTotal - before.LiteLLM.ResponseRejectedBodyTotal; got != 1 {
		t.Fatalf("response rejected body delta = %d, want 1", got)
	}
	if got := after.LiteLLM.ResponseRejectedJSONTotal - before.LiteLLM.ResponseRejectedJSONTotal; got != 1 {
		t.Fatalf("response rejected json delta = %d, want 1", got)
	}
	if got := after.LiteLLM.ResponseRejectedShapeTotal - before.LiteLLM.ResponseRejectedShapeTotal; got != 1 {
		t.Fatalf("response rejected shape delta = %d, want 1", got)
	}
}

func TestSnapshotReliability_OCRCountersIncrease(t *testing.T) {
	before := SnapshotReliability()

	IncOCRRetryAttempt()
	IncOCRRetryAfterHonored()
	IncOCRRetryCancelled()
	IncOCRRetryExhausted()
	IncOCRResponseRejectedBody()
	IncOCRResponseRejectedJSON()
	IncOCRResponseRejectedShape()
	IncOCRProviderFallbackBudgetExhausted()

	after := SnapshotReliability()

	if got := after.OCR.RetryAttemptsTotal - before.OCR.RetryAttemptsTotal; got != 1 {
		t.Fatalf("retry attempts delta = %d, want 1", got)
	}
	if got := after.OCR.RetryAfterHonoredTotal - before.OCR.RetryAfterHonoredTotal; got != 1 {
		t.Fatalf("retry-after honored delta = %d, want 1", got)
	}
	if got := after.OCR.RetryCancelledTotal - before.OCR.RetryCancelledTotal; got != 1 {
		t.Fatalf("retry cancelled delta = %d, want 1", got)
	}
	if got := after.OCR.RetryExhaustedTotal - before.OCR.RetryExhaustedTotal; got != 1 {
		t.Fatalf("retry exhausted delta = %d, want 1", got)
	}
	if got := after.OCR.ResponseRejectedTotal - before.OCR.ResponseRejectedTotal; got != 3 {
		t.Fatalf("response rejected total delta = %d, want 3", got)
	}
	if got := after.OCR.ResponseRejectedBodyTotal - before.OCR.ResponseRejectedBodyTotal; got != 1 {
		t.Fatalf("response rejected body delta = %d, want 1", got)
	}
	if got := after.OCR.ResponseRejectedJSONTotal - before.OCR.ResponseRejectedJSONTotal; got != 1 {
		t.Fatalf("response rejected json delta = %d, want 1", got)
	}
	if got := after.OCR.ResponseRejectedShapeTotal - before.OCR.ResponseRejectedShapeTotal; got != 1 {
		t.Fatalf("response rejected shape delta = %d, want 1", got)
	}
	if got := after.OCR.ProviderFallbackBudgetExhaustedTotal - before.OCR.ProviderFallbackBudgetExhaustedTotal; got != 1 {
		t.Fatalf("fallback budget exhausted delta = %d, want 1", got)
	}
}
