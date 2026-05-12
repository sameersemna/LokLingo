package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"loklingo/backend/internal/observability"
)

func TestLifecycleEventsList_FiltersByCorrelationID(t *testing.T) {
	h := NewLifecycleEventsHandler()
	app := fiber.New()
	app.Get("/metrics/lifecycle/events", h.List)

	corrA := fmt.Sprintf("corr-a-%d", time.Now().UnixNano())
	corrB := fmt.Sprintf("corr-b-%d", time.Now().UnixNano())

	observability.EmitLifecycleEvent("chunk_started", observability.LifecycleEvent{
		JobID:         "job-a",
		CorrelationID: corrA,
		Provider:      "litellm",
		QueueDepth:    3,
	})
	observability.EmitLifecycleEvent("chunk_completed", observability.LifecycleEvent{
		JobID:         "job-b",
		CorrelationID: corrB,
		Provider:      "ollama",
		QueueDepth:    4,
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics/lifecycle/events?correlation_id="+corrA+"&limit=10", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	eventsRaw, ok := body["events"].([]any)
	if !ok {
		t.Fatalf("expected events array, got %T", body["events"])
	}
	if len(eventsRaw) == 0 {
		t.Fatal("expected at least one filtered lifecycle event")
	}

	for _, item := range eventsRaw {
		event, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("expected event object, got %T", item)
		}
		if got := event["correlation_id"].(string); got != corrA {
			t.Fatalf("expected correlation_id %s, got %s", corrA, got)
		}
	}
}
