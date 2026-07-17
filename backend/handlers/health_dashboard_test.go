package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"

	"loklingo/backend/config"
	"loklingo/backend/jobs"
)

type dashboardTestStore struct{}

func (s *dashboardTestStore) Enqueue(context.Context, *jobs.Job) error { return nil }
func (s *dashboardTestStore) Get(context.Context, string) (*jobs.Job, error) {
	return nil, jobs.ErrNotFound
}
func (s *dashboardTestStore) Update(context.Context, *jobs.Job) error    { return nil }
func (s *dashboardTestStore) Dequeue(context.Context) (*jobs.Job, error) { return nil, nil }
func (s *dashboardTestStore) GetCached(context.Context, string, string, string) (string, error) {
	return "", jobs.ErrNotFound
}
func (s *dashboardTestStore) SetCached(context.Context, string, string, string, string) error {
	return nil
}
func (s *dashboardTestStore) QueueStats(context.Context) (jobs.QueueStats, error) {
	return jobs.QueueStats{QueueDepth: 2, InflightDepth: 1, StuckJobs: 0, RetryBacklog: 0, DeadLetterCount: 0}, nil
}

func TestHealthDashboardAndStartupChecks(t *testing.T) {
	depServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer depServer.Close()

	cfg := &config.Config{
		AppEnv:                   "development",
		OCRServiceURL:            depServer.URL,
		OllamaBaseURL:            depServer.URL,
		OllamaModel:              "qwen2.5-vl",
		PostgresDSN:              "",
		RedisURL:                 "redis://localhost:6379/0",
		GlobalRateLimitPerMinute: 120,
		WriteRateLimitPerMinute:  40,
		UploadRateLimitPerMinute: 12,
		SyncImageMaxInflight:     8,
	}
	store := &dashboardTestStore{}

	if err := CheckStartupDependencies(context.Background(), cfg, store, nil); err != nil {
		t.Fatalf("startup checks failed: %v", err)
	}

	app := fiber.New()
	app.Get("/api/v1/health/dashboard", NewHealthDashboardHandler(cfg, store, nil, time.Now().Add(-2*time.Minute)))

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/health/dashboard", nil))
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
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %#v", body["status"])
	}
	deps, ok := body["dependencies"].(map[string]any)
	if !ok {
		t.Fatalf("expected dependencies object, got %#v", body["dependencies"])
	}
	if deps["ocr"] == nil || deps["ollama"] == nil {
		t.Fatalf("expected ocr and ollama dependencies in dashboard: %#v", deps)
	}
}

func TestCheckStartupDependenciesReturnsErrorOnFailure(t *testing.T) {
	cfg := &config.Config{
		AppEnv:        "development",
		OCRServiceURL: "http://127.0.0.1:1",
		RedisURL:      "redis://localhost:6379/0",
	}
	store := &dashboardTestStore{}

	err := CheckStartupDependencies(context.Background(), cfg, store, nil)
	if err == nil {
		t.Fatal("expected startup dependency error")
	}
	if !errors.Is(err, redis.Nil) && err.Error() == "" {
		t.Fatal("expected non-empty startup error")
	}
}
