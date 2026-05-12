package observability

import (
	"fmt"
	"testing"
	"time"
)

func TestLifecycleEventContract_RequiredFieldsAcrossRequiredEvents(t *testing.T) {
	correlationID := fmt.Sprintf("contract-corr-%d", time.Now().UnixNano())
	jobID := fmt.Sprintf("contract-job-%d", time.Now().UnixNano())

	requiredEvents := []string{
		"job_created",
		"upload_received",
		"ocr_started",
		"ocr_completed",
		"translation_started",
		"chunk_started",
		"chunk_completed",
		"chunk_retry",
		"provider_failover",
		"render_started",
		"render_completed",
		"export_started",
		"export_completed",
		"job_failed",
		"job_completed",
	}

	for idx, eventName := range requiredEvents {
		EmitLifecycleEvent(eventName, LifecycleEvent{
			JobID:         jobID,
			CorrelationID: correlationID,
			Provider:      "test-provider",
			DurationMS:    int64(idx + 1),
			RetryCount:    int64(idx),
			QueueDepth:    int64(10 + idx),
		})
	}

	events := SnapshotLifecycleEvents(correlationID, jobID, "", 100)
	if len(events) < len(requiredEvents) {
		t.Fatalf("expected at least %d events for contract test, got %d", len(requiredEvents), len(events))
	}

	seen := make(map[string]LifecycleEventRecord, len(requiredEvents))
	for _, ev := range events {
		if _, ok := seen[ev.Event]; !ok {
			seen[ev.Event] = ev
		}
	}

	for _, eventName := range requiredEvents {
		ev, ok := seen[eventName]
		if !ok {
			t.Fatalf("missing required lifecycle event %s", eventName)
		}
		if ev.JobID == "" {
			t.Fatalf("event %s missing job_id", eventName)
		}
		if ev.CorrelationID == "" {
			t.Fatalf("event %s missing correlation_id", eventName)
		}
		if ev.Provider == "" {
			t.Fatalf("event %s missing provider", eventName)
		}
		if ev.Timestamp == "" {
			t.Fatalf("event %s missing timestamp", eventName)
		}
		if ev.DurationMS < 0 {
			t.Fatalf("event %s has negative duration_ms", eventName)
		}
		if ev.RetryCount < 0 {
			t.Fatalf("event %s has negative retry_count", eventName)
		}
		if ev.QueueDepth < 0 {
			t.Fatalf("event %s has negative queue_depth", eventName)
		}
	}
}
