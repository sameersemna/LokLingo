package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"loklingo/backend/internal/observability"
	"loklingo/backend/jobs"
)

type mockQueueMetricsStore struct {
	mockStore
	stats jobs.QueueStats
}

func (m *mockQueueMetricsStore) QueueStats(_ context.Context) (jobs.QueueStats, error) {
	return m.stats, nil
}

func TestReliabilityMetricsSummary_ReturnsPipelineAndQueue(t *testing.T) {
	observability.RecordOCRLatency(120)
	observability.RecordTranslationLatency(250)
	observability.RecordRenderLatency(310)
	observability.RecordQueueWaitLatency(80)
	observability.IncRetriesTotal()
	observability.IncFailoversTotal()
	observability.IncTimeoutsTotal()
	observability.IncChunkStartedTotal()
	observability.IncChunkCompletedTotal()
	observability.IncExportStartedTotal()
	observability.IncExportCompletedTotal()

	store := &mockQueueMetricsStore{stats: jobs.QueueStats{
		QueueDepth:      12,
		InflightDepth:   3,
		StuckJobs:       1,
		RetryBacklog:    4,
		DeadLetterCount: 2,
	}}

	h := NewReliabilityMetricsHandler(store)
	app := fiber.New()
	app.Get("/metrics/reliability", h.Summary)

	req := httptest.NewRequest(http.MethodGet, "/metrics/reliability", nil)
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
	if _, ok := body["pipeline"]; !ok {
		t.Fatal("expected pipeline in response")
	}
	if _, ok := body["queue"]; !ok {
		t.Fatal("expected queue in response")
	}
	if _, ok := body["alerts"]; !ok {
		t.Fatal("expected alerts in response")
	}

	queue, ok := body["queue"].(map[string]any)
	if !ok {
		t.Fatalf("expected queue object, got %T", body["queue"])
	}
	if got := int64(queue["queue_depth"].(float64)); got != 12 {
		t.Fatalf("expected queue_depth 12, got %d", got)
	}
	if got := int64(queue["dead_letter_volume"].(float64)); got != 2 {
		t.Fatalf("expected dead_letter_volume 2, got %d", got)
	}
}
