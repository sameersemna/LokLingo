package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"loklingo/backend/jobs"
)

// imageJobStore records the last enqueued job.
type imageJobStore struct {
	mockStore
	job *jobs.Job
}

func (s *imageJobStore) Enqueue(_ context.Context, j *jobs.Job) error {
	s.job = j
	return nil
}

// newMultipartImageRequest builds a multipart/form-data POST request for image upload.
// Pass nil for imageData to omit the file field.
func newMultipartImageRequest(t *testing.T, imageData []byte, filename, target, source, mode string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if imageData != nil {
		if filename == "" {
			filename = "test.png"
		}
		fw, err := w.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := fw.Write(imageData); err != nil {
			t.Fatalf("write image data: %v", err)
		}
	}
	if target != "" {
		if err := w.WriteField("target", target); err != nil {
			t.Fatalf("write target field: %v", err)
		}
	}
	if source != "" {
		if err := w.WriteField("source", source); err != nil {
			t.Fatalf("write source field: %v", err)
		}
	}
	if mode != "" {
		if err := w.WriteField("mode", mode); err != nil {
			t.Fatalf("write mode field: %v", err)
		}
	}
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/jobs/image", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func newImageJobsApp(store jobs.Store) (*fiber.App, *JobsHandler) {
	h := NewJobsHandler(store, 25*1024*1024)
	app := fiber.New()
	app.Post("/jobs/image", h.CreateImageJob)
	return app, h
}

func TestCreateImageJob_MissingFile(t *testing.T) {
	app, _ := newImageJobsApp(&mockStore{})
	req := newMultipartImageRequest(t, nil, "", "de", "en", "")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateImageJob_MissingTarget(t *testing.T) {
	app, _ := newImageJobsApp(&mockStore{})
	req := newMultipartImageRequest(t, []byte("\x89PNG"), "img.png", "", "en", "")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateImageJob_InvalidMode(t *testing.T) {
	app, _ := newImageJobsApp(&mockStore{})
	req := newMultipartImageRequest(t, []byte("\x89PNG"), "img.png", "de", "en", "invalid")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid mode, got %d", resp.StatusCode)
	}
}

func TestCreateImageJob_ModeDefaultsToOverlay(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	// No mode field — should default to "overlay".
	req := newMultipartImageRequest(t, []byte("\x89PNG"), "img.png", "de", "en", "")
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.Mode != jobs.ModeOverlay {
		t.Fatalf("expected mode=%q, got %q", jobs.ModeOverlay, cs.job.Mode)
	}
}

func TestCreateImageJob_ModeOverlay(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	req := newMultipartImageRequest(t, []byte("\x89PNG"), "img.png", "de", "en", "overlay")
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.Mode != jobs.ModeOverlay {
		t.Fatalf("expected mode=%q, got %q", jobs.ModeOverlay, cs.job.Mode)
	}
}

func TestCreateImageJob_ModeLayout(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	req := newMultipartImageRequest(t, []byte("\x89PNG"), "img.png", "de", "en", "layout")
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.Mode != jobs.ModeLayout {
		t.Fatalf("expected mode=%q, got %q", jobs.ModeLayout, cs.job.Mode)
	}
}

func TestCreateImageJob_ValidRequest_Returns202AndJobID(t *testing.T) {
	app, _ := newImageJobsApp(&mockStore{})
	req := newMultipartImageRequest(t, []byte("\x89PNG"), "img.png", "de", "en", "overlay")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if result["job_id"] == "" {
		t.Fatalf("expected non-empty job_id, got: %s", body)
	}
}

func TestCreateImageJob_JobTypeIsImage(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	req := newMultipartImageRequest(t, []byte("\x89PNG"), "img.png", "de", "en", "")
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.Type != jobs.TypeImage {
		t.Fatalf("expected type=%q, got %q", jobs.TypeImage, cs.job.Type)
	}
	if cs.job.FilePath == "" {
		t.Fatal("expected non-empty FilePath on enqueued job")
	}
}

func TestCreateImageJob_SourceDefaultsToAuto(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	// No source field.
	req := newMultipartImageRequest(t, []byte("\x89PNG"), "img.png", "de", "", "")
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.Source != "auto" {
		t.Fatalf("expected source=auto, got %q", cs.job.Source)
	}
}

func TestCreateImageJob_FileTooLarge_Returns413(t *testing.T) {
	cs := &imageJobStore{}
	h := NewJobsHandler(cs, 4)
	app := fiber.New()
	app.Post("/jobs/image", h.CreateImageJob)

	req := newMultipartImageRequest(t, []byte("12345"), "img.png", "de", "en", "")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}
	if cs.job != nil {
		t.Fatal("expected no enqueued job when file is too large")
	}
}

func TestCreateImageJob_FileExtensionPreserved(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	req := newMultipartImageRequest(t, []byte("\xff\xd8\xff"), "photo.jpg", "de", "en", "")
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if len(cs.job.FilePath) < 4 || cs.job.FilePath[len(cs.job.FilePath)-4:] != ".jpg" {
		t.Fatalf("expected FilePath to end with .jpg, got %q", cs.job.FilePath)
	}
}
