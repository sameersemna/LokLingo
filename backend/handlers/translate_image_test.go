package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"loklingo/backend/jobs"
)

type translateImageTestStore struct {
	enqueued       *jobs.Job
	enqueueErr     error
	getCalls       int
	getErr         error
	completedAfter int
	finalStatus    jobs.Status
	finalError     string
}

func (s *translateImageTestStore) Enqueue(_ context.Context, job *jobs.Job) error {
	if s.enqueueErr != nil {
		return s.enqueueErr
	}
	s.enqueued = job
	return nil
}

func (s *translateImageTestStore) Get(_ context.Context, id string) (*jobs.Job, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	s.getCalls++
	status := jobs.StatusPending
	output := ""
	if s.getCalls >= s.completedAfter {
		if s.finalStatus != "" {
			status = s.finalStatus
		} else {
			status = jobs.StatusCompleted
		}
		if status == jobs.StatusCompleted {
			output = "/tmp/loklingo/images/out.png"
		}
	}
	return &jobs.Job{
		ID:             id,
		Type:           jobs.TypeImage,
		Status:         status,
		ErrorMsg:       s.finalError,
		OutputFilePath: output,
	}, nil
}

func (s *translateImageTestStore) Update(_ context.Context, _ *jobs.Job) error  { return nil }
func (s *translateImageTestStore) Dequeue(_ context.Context) (*jobs.Job, error) { return nil, nil }
func (s *translateImageTestStore) GetCached(_ context.Context, _, _, _ string) (string, error) {
	return "", jobs.ErrNotFound
}
func (s *translateImageTestStore) SetCached(_ context.Context, _, _, _, _ string) error { return nil }

func makeImageMultipartRequest(t *testing.T, target string, withFile bool, source string, mode string) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if withFile {
		part, err := writer.CreateFormFile("file", "sample.png")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write([]byte("not-a-real-png")); err != nil {
			t.Fatalf("write form file: %v", err)
		}
	}
	if target != "" {
		if err := writer.WriteField("target", target); err != nil {
			t.Fatalf("write target field: %v", err)
		}
	}
	if source != "" {
		if err := writer.WriteField("source", source); err != nil {
			t.Fatalf("write source field: %v", err)
		}
	}
	if mode != "" {
		if err := writer.WriteField("mode", mode); err != nil {
			t.Fatalf("write mode field: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/translate/image", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestTranslateImage_ReturnsImageURL(t *testing.T) {
	oldPollInterval := syncImagePollInterval
	oldPollTimeout := syncImagePollTimeout
	syncImagePollInterval = 1 * time.Millisecond
	syncImagePollTimeout = 2 * time.Second
	t.Cleanup(func() {
		syncImagePollInterval = oldPollInterval
		syncImagePollTimeout = oldPollTimeout
	})

	store := &translateImageTestStore{completedAfter: 1}
	h := NewTranslateHandler(nil, store)
	app := fiber.New()
	app.Post("/translate/image", h.TranslateImage)

	req := makeImageMultipartRequest(t, "de", true, "", jobs.ModeOverlay)
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
	if body["image_url"] == "" {
		t.Fatalf("expected image_url in response, got %#v", body)
	}
	if store.enqueued == nil {
		t.Fatal("expected image job to be enqueued")
	}
	if store.enqueued.Type != jobs.TypeImage {
		t.Fatalf("expected enqueued job type %q, got %q", jobs.TypeImage, store.enqueued.Type)
	}
}

func TestTranslateImage_ValidatesRequiredFields(t *testing.T) {
	store := &translateImageTestStore{completedAfter: 1}
	h := NewTranslateHandler(nil, store)
	app := fiber.New()
	app.Post("/translate/image", h.TranslateImage)

	missingFileReq := makeImageMultipartRequest(t, "de", false, "", jobs.ModeOverlay)
	missingFileResp, err := app.Test(missingFileReq)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if missingFileResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing file, got %d", missingFileResp.StatusCode)
	}

	missingTargetReq := makeImageMultipartRequest(t, "", true, "", jobs.ModeOverlay)
	missingTargetResp, err := app.Test(missingTargetReq)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if missingTargetResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing target, got %d", missingTargetResp.StatusCode)
	}

	invalidModeReq := makeImageMultipartRequest(t, "de", true, "", "invalid")
	invalidModeResp, err := app.Test(invalidModeReq)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if invalidModeResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid mode, got %d", invalidModeResp.StatusCode)
	}
}

func TestTranslateImage_DefaultsSourceAndMode(t *testing.T) {
	oldPollInterval := syncImagePollInterval
	oldPollTimeout := syncImagePollTimeout
	syncImagePollInterval = 1 * time.Millisecond
	syncImagePollTimeout = 2 * time.Second
	t.Cleanup(func() {
		syncImagePollInterval = oldPollInterval
		syncImagePollTimeout = oldPollTimeout
	})

	store := &translateImageTestStore{completedAfter: 1}
	h := NewTranslateHandler(nil, store)
	app := fiber.New()
	app.Post("/translate/image", h.TranslateImage)

	// No source/mode in form-data -> source defaults to auto, mode defaults to overlay.
	req := makeImageMultipartRequest(t, "fr", true, "", "")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if store.enqueued == nil {
		t.Fatal("expected image job to be enqueued")
	}
	if store.enqueued.Source != "auto" {
		t.Fatalf("expected source default auto, got %q", store.enqueued.Source)
	}
	if store.enqueued.Mode != jobs.ModeOverlay {
		t.Fatalf("expected mode default %q, got %q", jobs.ModeOverlay, store.enqueued.Mode)
	}
}

func TestTranslateImage_FailedJobReturnsBadGateway(t *testing.T) {
	oldPollInterval := syncImagePollInterval
	oldPollTimeout := syncImagePollTimeout
	syncImagePollInterval = 1 * time.Millisecond
	syncImagePollTimeout = 2 * time.Second
	t.Cleanup(func() {
		syncImagePollInterval = oldPollInterval
		syncImagePollTimeout = oldPollTimeout
	})

	store := &translateImageTestStore{
		completedAfter: 1,
		finalStatus:    jobs.StatusFailed,
		finalError:     "worker image OCR failed",
	}
	h := NewTranslateHandler(nil, store)
	app := fiber.New()
	app.Post("/translate/image", h.TranslateImage)

	req := makeImageMultipartRequest(t, "de", true, "en", jobs.ModeLayout)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", resp.StatusCode)
	}
}

func TestTranslateImage_TimeoutReturnsGatewayTimeout(t *testing.T) {
	oldPollInterval := syncImagePollInterval
	oldPollTimeout := syncImagePollTimeout
	syncImagePollInterval = 1 * time.Millisecond
	syncImagePollTimeout = 5 * time.Millisecond
	t.Cleanup(func() {
		syncImagePollInterval = oldPollInterval
		syncImagePollTimeout = oldPollTimeout
	})

	store := &translateImageTestStore{completedAfter: 9999}
	h := NewTranslateHandler(nil, store)
	app := fiber.New()
	app.Post("/translate/image", h.TranslateImage)

	req := makeImageMultipartRequest(t, "de", true, "auto", jobs.ModeOverlay)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", resp.StatusCode)
	}
}
