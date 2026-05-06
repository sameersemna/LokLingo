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

// captureStore wraps mockStore but records the last enqueued job.
type captureStore struct {
	mockStore
	job *jobs.Job
}

func (s *captureStore) Enqueue(_ context.Context, j *jobs.Job) error {
	s.job = j
	return nil
}

// newMultipartPDFRequest builds a multipart/form-data POST request.
// Pass nil for pdfData to omit the file field entirely.
func newMultipartPDFRequest(t *testing.T, pdfData []byte, target, source string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if pdfData != nil {
		fw, err := w.CreateFormFile("file", "test.pdf")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := fw.Write(pdfData); err != nil {
			t.Fatalf("write pdf data: %v", err)
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
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/jobs/pdf", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func newPDFJobsApp() (*fiber.App, *JobsHandler) {
	h := NewJobsHandler(&mockStore{}, 25*1024*1024)
	app := fiber.New()
	app.Post("/jobs/pdf", h.CreatePDFJob)
	return app, h
}

func TestCreatePDFJob_MissingFile(t *testing.T) {
	app, _ := newPDFJobsApp()

	req := newMultipartPDFRequest(t, nil, "de", "en")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreatePDFJob_MissingTarget(t *testing.T) {
	app, _ := newPDFJobsApp()

	req := newMultipartPDFRequest(t, []byte("%PDF-1.4"), "", "en")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreatePDFJob_ValidRequest_Returns202AndJobID(t *testing.T) {
	app, _ := newPDFJobsApp()

	req := newMultipartPDFRequest(t, []byte("%PDF-1.4"), "de", "en")
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

func TestCreatePDFJob_SourceDefaultsToAuto(t *testing.T) {
	cs := &captureStore{}
	h := NewJobsHandler(cs, 25*1024*1024)
	app := fiber.New()
	app.Post("/jobs/pdf", h.CreatePDFJob)

	// No source field in the request.
	req := newMultipartPDFRequest(t, []byte("%PDF-1.4"), "de", "")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.Source != "auto" {
		t.Fatalf("expected source=auto, got %q", cs.job.Source)
	}
}

func TestCreatePDFJob_JobTypeIsPDF(t *testing.T) {
	cs := &captureStore{}
	h := NewJobsHandler(cs, 25*1024*1024)
	app := fiber.New()
	app.Post("/jobs/pdf", h.CreatePDFJob)

	req := newMultipartPDFRequest(t, []byte("%PDF-1.4"), "de", "en")
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.Type != jobs.TypePDF {
		t.Fatalf("expected type=%q, got %q", jobs.TypePDF, cs.job.Type)
	}
	if cs.job.FilePath == "" {
		t.Fatal("expected non-empty FilePath on enqueued job")
	}
}

func TestCreatePDFJob_FileTooLarge_Returns413(t *testing.T) {
	cs := &captureStore{}
	h := NewJobsHandler(cs, 4)
	app := fiber.New()
	app.Post("/jobs/pdf", h.CreatePDFJob)

	req := newMultipartPDFRequest(t, []byte("12345"), "de", "en")
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

func newMultipartPDFRequestWithMode(t *testing.T, pdfData []byte, target, source, mode string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if pdfData != nil {
		fw, err := w.CreateFormFile("file", "test.pdf")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := fw.Write(pdfData); err != nil {
			t.Fatalf("write pdf data: %v", err)
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

	req := httptest.NewRequest(http.MethodPost, "/jobs/pdf", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestCreatePDFJob_ModeDefaultsToOverlay(t *testing.T) {
	cs := &captureStore{}
	h := NewJobsHandler(cs, 25*1024*1024)
	app := fiber.New()
	app.Post("/jobs/pdf", h.CreatePDFJob)

	// No mode field — should default to "overlay".
	req := newMultipartPDFRequest(t, []byte("%PDF-1.4"), "de", "en")
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

func TestCreatePDFJob_ModeLayout(t *testing.T) {
	cs := &captureStore{}
	h := NewJobsHandler(cs, 25*1024*1024)
	app := fiber.New()
	app.Post("/jobs/pdf", h.CreatePDFJob)

	req := newMultipartPDFRequestWithMode(t, []byte("%PDF-1.4"), "de", "en", "layout")
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
