package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestGetJob_ImageJobIncludesImageURL(t *testing.T) {
	now := time.Now()
	store := &getJobStore{job: &jobs.Job{
		ID:             "job-image-1",
		Type:           jobs.TypeImage,
		Status:         jobs.StatusCompleted,
		Mode:           jobs.ModeOverlay,
		Source:         "en",
		Target:         "de",
		TranslatedText: "Hallo",
		OutputFilePath: "/tmp/loklingo/images/out_translated.png",
		CreatedAt:      now,
		UpdatedAt:      now,
	}}

	h := NewJobsHandler(store, 1024)
	app := fiber.New()
	app.Get("/jobs/:id", h.GetJob)

	req := httptest.NewRequest(http.MethodGet, "/jobs/job-image-1", nil)
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
	if body["image_url"] != "/api/v1/jobs/job-image-1/output" {
		t.Fatalf("expected image_url for image job, got %#v", body["image_url"])
	}
	if body["output_file_path"] != "/tmp/loklingo/images/out_translated.png" {
		t.Fatalf("expected output_file_path to remain present, got %#v", body["output_file_path"])
	}
}

func TestGetJob_NonImageJobDoesNotIncludeImageURL(t *testing.T) {
	now := time.Now()
	store := &getJobStore{job: &jobs.Job{
		ID:             "job-text-1",
		Type:           jobs.TypeText,
		Status:         jobs.StatusCompleted,
		Mode:           jobs.ModeOverlay,
		Source:         "en",
		Target:         "de",
		TranslatedText: "Hallo Welt",
		OutputFilePath: "/tmp/loklingo/images/out_translated.png",
		CreatedAt:      now,
		UpdatedAt:      now,
	}}

	h := NewJobsHandler(store, 1024)
	app := fiber.New()
	app.Get("/jobs/:id", h.GetJob)

	req := httptest.NewRequest(http.MethodGet, "/jobs/job-text-1", nil)
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
	if _, ok := body["image_url"]; ok {
		t.Fatalf("did not expect image_url for non-image job, got %#v", body["image_url"])
	}
}

// ---- DownloadJobOutput tests ----

func newDownloadApp(store jobs.Store) *fiber.App {
	h := NewJobsHandler(store, 1024)
	app := fiber.New()
	app.Get("/jobs/:id/output", h.DownloadJobOutput)
	return app
}

func TestDownloadJobOutput_ServesFile(t *testing.T) {
	// Write a real file inside ImageUploadDir so SendFile can serve it.
	if err := os.MkdirAll(jobs.ImageUploadDir, 0o700); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}
	tmpFile := filepath.Join(jobs.ImageUploadDir, "test_output_translated.png")
	if err := os.WriteFile(tmpFile, []byte("\x89PNG\r\n\x1a\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile) })

	now := time.Now()
	store := &getJobStore{job: &jobs.Job{
		ID:             "job-dl",
		Type:           jobs.TypeImage,
		Status:         jobs.StatusCompleted,
		OutputFilePath: tmpFile,
		CreatedAt:      now,
		UpdatedAt:      now,
	}}

	app := newDownloadApp(store)
	req := httptest.NewRequest(http.MethodGet, "/jobs/job-dl/output", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("expected Content-Type image/png, got %q", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd == "" {
		t.Fatal("expected Content-Disposition header to be set")
	}
}

func TestDownloadJobOutput_JobNotFound(t *testing.T) {
	store := &getJobStore{err: jobs.ErrNotFound}
	app := newDownloadApp(store)

	req := httptest.NewRequest(http.MethodGet, "/jobs/missing/output", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDownloadJobOutput_NotCompleted(t *testing.T) {
	now := time.Now()
	store := &getJobStore{job: &jobs.Job{
		ID:        "job-pending",
		Type:      jobs.TypeImage,
		Status:    jobs.StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}}
	app := newDownloadApp(store)

	req := httptest.NewRequest(http.MethodGet, "/jobs/job-pending/output", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDownloadJobOutput_PathTraversalRejected(t *testing.T) {
	now := time.Now()
	store := &getJobStore{job: &jobs.Job{
		ID:             "job-evil",
		Type:           jobs.TypeImage,
		Status:         jobs.StatusCompleted,
		OutputFilePath: "/etc/passwd",
		CreatedAt:      now,
		UpdatedAt:      now,
	}}
	app := newDownloadApp(store)

	req := httptest.NewRequest(http.MethodGet, "/jobs/job-evil/output", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestDownloadJobOutput_OutputFileMissing(t *testing.T) {
	now := time.Now()
	store := &getJobStore{job: &jobs.Job{
		ID:             "job-gone",
		Type:           jobs.TypeImage,
		Status:         jobs.StatusCompleted,
		OutputFilePath: filepath.Join(jobs.ImageUploadDir, "nonexistent_translated.png"),
		CreatedAt:      now,
		UpdatedAt:      now,
	}}
	app := newDownloadApp(store)

	req := httptest.NewRequest(http.MethodGet, "/jobs/job-gone/output", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "output file not found" {
		t.Fatalf("unexpected error message: %#v", body["error"])
	}
}
