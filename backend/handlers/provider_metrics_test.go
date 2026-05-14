package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"loklingo/backend/internal/observability"
	"loklingo/backend/services"

	"github.com/gofiber/fiber/v2"
)

func TestProviderMetricsSummary_ReturnsPayload(t *testing.T) {
	observability.IncProviderSuccess("litellm")
	observability.RecordProviderRetry("litellm")
	observability.RecordProviderTimeout("litellm", "network_timeout")
	observability.IncRenderFailure("layout")
	observability.IncChunkCheckpointHit()
	observability.IncChunkCheckpointMiss()
	observability.IncChunkCheckpointPersistFailure()
	observability.IncChunkCheckpointClear()
	observability.IncChunkCheckpointClearFailure()

	h := NewProviderMetricsHandler()
	app := fiber.New()
	app.Get("/metrics/providers", h.Summary)

	req := httptest.NewRequest(http.MethodGet, "/metrics/providers", nil)
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

	if _, ok := body["providers"]; !ok {
		t.Fatal("expected providers key in response")
	}
	if _, ok := body["health"]; !ok {
		t.Fatal("expected health key in response")
	}
	if _, ok := body["timeouts"]; !ok {
		t.Fatal("expected timeouts key in response")
	}
	if _, ok := body["render_failures"]; !ok {
		t.Fatal("expected render_failures key in response")
	}
	if _, ok := body["checkpoints"]; !ok {
		t.Fatal("expected checkpoints key in response")
	}
	if _, ok := body["degraded_mode"]; !ok {
		t.Fatal("expected degraded_mode key in response")
	}
	if _, ok := body["provider_policy"]; !ok {
		t.Fatal("expected provider_policy key in response")
	}

	checkpoints, ok := body["checkpoints"].(map[string]any)
	if !ok {
		t.Fatalf("expected checkpoints to be an object, got %T", body["checkpoints"])
	}
	assertMetricAtLeast(t, checkpoints, "hit_total", 1)
	assertMetricAtLeast(t, checkpoints, "miss_total", 1)
	assertMetricAtLeast(t, checkpoints, "persist_failure_total", 1)
	assertMetricAtLeast(t, checkpoints, "clear_total", 1)
	assertMetricAtLeast(t, checkpoints, "clear_failure_total", 1)
}

type mockProviderHealthSnapshotter struct{}

func (m mockProviderHealthSnapshotter) SnapshotProviderHealth() []services.ProviderHealthSnapshot {
	return []services.ProviderHealthSnapshot{{Provider: "litellm", Score: 1.12}}
}

func TestProviderMetricsHealth_ReturnsSnapshot(t *testing.T) {
	h := NewProviderMetricsHandler(mockProviderHealthSnapshotter{})
	app := fiber.New()
	app.Get("/metrics/providers/health", h.Health)

	req := httptest.NewRequest(http.MethodGet, "/metrics/providers/health", nil)
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

	if _, ok := body["captured_at"]; !ok {
		t.Fatal("expected captured_at key in response")
	}
	if _, ok := body["provider_policy"]; !ok {
		t.Fatal("expected provider_policy key in response")
	}
	if _, ok := body["degraded_mode"]; !ok {
		t.Fatal("expected degraded_mode key in response")
	}
}

func TestProviderMetricsSummary_CheckpointCountersInterleavingRemainNumericAndAccumulate(t *testing.T) {
	h := NewProviderMetricsHandler()
	app := fiber.New()
	app.Get("/metrics/providers", h.Summary)

	before := fetchCheckpointMetrics(t, app)

	// Simulate mixed code paths incrementing counters in a non-uniform order.
	observability.IncChunkCheckpointHit()
	observability.IncChunkCheckpointMiss()
	observability.IncChunkCheckpointClearFailure()
	observability.IncChunkCheckpointHit()
	observability.IncChunkCheckpointPersistFailure()
	observability.IncChunkCheckpointClear()
	observability.IncChunkCheckpointMiss()
	observability.IncChunkCheckpointPersistFailure()
	observability.IncChunkCheckpointClear()

	after := fetchCheckpointMetrics(t, app)

	assertMetricDelta(t, before, after, "hit_total", 2)
	assertMetricDelta(t, before, after, "miss_total", 2)
	assertMetricDelta(t, before, after, "persist_failure_total", 2)
	assertMetricDelta(t, before, after, "clear_total", 2)
	assertMetricDelta(t, before, after, "clear_failure_total", 1)
}

func fetchCheckpointMetrics(t *testing.T, app *fiber.App) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics/providers", nil)
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
	checkpoints, ok := body["checkpoints"].(map[string]any)
	if !ok {
		t.Fatalf("expected checkpoints to be an object, got %T", body["checkpoints"])
	}
	return checkpoints
}

func assertMetricDelta(t *testing.T, before, after map[string]any, key string, expectedDelta int64) {
	t.Helper()
	beforeVal := metricValue(t, before, key)
	afterVal := metricValue(t, after, key)
	if afterVal-beforeVal != expectedDelta {
		t.Fatalf("expected %s delta %d, got before=%d after=%d", key, expectedDelta, beforeVal, afterVal)
	}
}

func metricValue(t *testing.T, obj map[string]any, key string) int64 {
	t.Helper()
	v, ok := obj[key]
	if !ok {
		t.Fatalf("expected %s in metrics payload", key)
	}
	n, ok := v.(float64)
	if !ok {
		t.Fatalf("expected %s to be numeric, got %T", key, v)
	}
	return int64(n)
}

func assertMetricAtLeast(t *testing.T, obj map[string]any, key string, min int64) {
	t.Helper()
	n := metricValue(t, obj, key)
	if int64(n) < min {
		t.Fatalf("expected %s >= %d, got %v", key, min, n)
	}
}
