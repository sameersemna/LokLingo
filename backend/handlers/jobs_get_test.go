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

	"loklingo/backend/jobs"
)

type getJobStore struct {
	job *jobs.Job
	err error
}

func (s *getJobStore) Enqueue(_ context.Context, _ *jobs.Job) error { return nil }
func (s *getJobStore) Get(_ context.Context, _ string) (*jobs.Job, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.job, nil
}
func (s *getJobStore) Update(_ context.Context, _ *jobs.Job) error  { return nil }
func (s *getJobStore) Dequeue(_ context.Context) (*jobs.Job, error) { return nil, nil }
func (s *getJobStore) GetCached(_ context.Context, _, _, _ string) (string, error) {
	return "", jobs.ErrNotFound
}
func (s *getJobStore) SetCached(_ context.Context, _, _, _, _ string) error { return nil }

func TestGetJob_PDFProgressFieldsIncluded(t *testing.T) {
	now := time.Now()
	store := &getJobStore{job: &jobs.Job{
		ID:             "job-123",
		Type:           jobs.TypePDF,
		Status:         jobs.StatusProcessing,
		Source:         "en",
		Target:         "de",
		TotalPages:     12,
		ProcessedPages: 5,
		CreatedAt:      now,
		UpdatedAt:      now,
	}}

	h := NewJobsHandler(store, 1024)
	app := fiber.New()
	app.Get("/jobs/:id", h.GetJob)

	req := httptest.NewRequest(http.MethodGet, "/jobs/job-123", nil)
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

	if body["total_pages"] != float64(12) {
		t.Fatalf("expected total_pages=12, got %#v", body["total_pages"])
	}
	if body["processed_pages"] != float64(5) {
		t.Fatalf("expected processed_pages=5, got %#v", body["processed_pages"])
	}
}

func TestGetJob_NotFound(t *testing.T) {
	store := &getJobStore{err: jobs.ErrNotFound}
	h := NewJobsHandler(store, 1024)
	app := fiber.New()
	app.Get("/jobs/:id", h.GetJob)

	req := httptest.NewRequest(http.MethodGet, "/jobs/missing", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetJob_ModeIncludedInResponse(t *testing.T) {
	now := time.Now()
	store := &getJobStore{job: &jobs.Job{
		ID:        "job-456",
		Type:      jobs.TypeText,
		Status:    jobs.StatusCompleted,
		Mode:      jobs.ModeLayout,
		Source:    "en",
		Target:    "fr",
		CreatedAt: now,
		UpdatedAt: now,
	}}

	h := NewJobsHandler(store, 1024)
	app := fiber.New()
	app.Get("/jobs/:id", h.GetJob)

	req := httptest.NewRequest(http.MethodGet, "/jobs/job-456", nil)
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
	if body["mode"] != jobs.ModeLayout {
		t.Fatalf("expected mode=%q, got %#v", jobs.ModeLayout, body["mode"])
	}
}

func TestGetJob_ModeDefaultsToOverlayWhenEmpty(t *testing.T) {
	now := time.Now()
	// Job with no Mode set (zero value) — simulates legacy jobs.
	store := &getJobStore{job: &jobs.Job{
		ID:        "job-789",
		Status:    jobs.StatusPending,
		Source:    "en",
		Target:    "de",
		CreatedAt: now,
		UpdatedAt: now,
	}}

	h := NewJobsHandler(store, 1024)
	app := fiber.New()
	app.Get("/jobs/:id", h.GetJob)

	req := httptest.NewRequest(http.MethodGet, "/jobs/job-789", nil)
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
	// Empty string for legacy jobs: the field is present but blank.
	if _, ok := body["mode"]; !ok {
		t.Fatal("expected mode field to be present in response")
	}
}

func TestGetJob_StoreError(t *testing.T) {
	store := &getJobStore{err: errors.New("redis down")}
	h := NewJobsHandler(store, 1024)
	app := fiber.New()
	app.Get("/jobs/:id", h.GetJob)

	req := httptest.NewRequest(http.MethodGet, "/jobs/job-123", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}
