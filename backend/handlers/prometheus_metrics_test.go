package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"loklingo/backend/internal/observability"
	"loklingo/backend/jobs"
)

func TestPrometheusMetricsExpose_ReturnsTextFormat(t *testing.T) {
	observability.RecordOCRLatency(100)
	observability.RecordTranslationLatency(200)
	observability.RecordRenderLatency(300)
	observability.IncRetriesTotal()
	observability.IncChunkStartedTotal()
	observability.IncChunkCompletedTotal()
	observability.IncProviderSuccess("litellm")
	observability.RecordProviderRetry("litellm")
	observability.RecordProviderTimeout("litellm", "network_timeout")

	store := &mockQueueMetricsStore{stats: jobs.QueueStats{QueueDepth: 7, InflightDepth: 2, RetryBacklog: 1, DeadLetterCount: 0, StuckJobs: 0}}
	h := NewPrometheusMetricsHandler(store, mockProviderHealthSnapshotter{})
	app := fiber.New()
	app.Get("/metrics/prometheus", h.Expose)

	req := httptest.NewRequest(http.MethodGet, "/metrics/prometheus", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("expected text/plain content type, got %s", ct)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	body := string(bodyBytes)
	if !strings.Contains(body, "loklingo_pipeline_retries_total") {
		t.Fatal("expected retries metric in output")
	}
	if !strings.Contains(body, "loklingo_queue_depth") {
		t.Fatal("expected queue depth metric in output")
	}
	if !strings.Contains(body, "loklingo_provider_success_total") {
		t.Fatal("expected provider success metric in output")
	}
	if !strings.Contains(body, "loklingo_degraded_mode_total") {
		t.Fatal("expected degraded mode metric in output")
	}
	if !strings.Contains(body, "loklingo_provider_policy_score") {
		t.Fatal("expected provider policy score metric in output")
	}
}
