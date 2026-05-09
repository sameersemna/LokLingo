package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
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

func newMultipartImageRequestWithPartContentType(t *testing.T, imageData []byte, filename, target, partContentType string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	h.Set("Content-Type", partContentType)
	fw, err := w.CreatePart(h)
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	if _, err := fw.Write(imageData); err != nil {
		t.Fatalf("write image data: %v", err)
	}
	if err := w.WriteField("target", target); err != nil {
		t.Fatalf("write target: %v", err)
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

func TestCreateImageJob_InvalidFileType(t *testing.T) {
	app, _ := newImageJobsApp(&mockStore{})
	req := newMultipartImageRequest(t, []byte("not-an-image"), "doc.txt", "de", "en", "overlay")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid file type, got %d", resp.StatusCode)
	}
}

func TestCreateImageJob_ExtensionlessPNGAccepted(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	req := newMultipartImageRequest(t, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, "blob", "de", "en", "overlay")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 for extensionless png upload, got %d", resp.StatusCode)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if !strings.HasSuffix(cs.job.FilePath, ".png") {
		t.Fatalf("expected generated file path to use detected .png extension, got %q", cs.job.FilePath)
	}
}

func TestCreateImageJob_InvalidLanguageCodes(t *testing.T) {
	app, _ := newImageJobsApp(&mockStore{})
	req := newMultipartImageRequest(t, []byte("\x89PNG"), "img.png", "auto", "en", "overlay")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid target language, got %d", resp.StatusCode)
	}
}

func TestCreateImageJob_ContentTypeSpoofRejected(t *testing.T) {
	app, _ := newImageJobsApp(&mockStore{})
	req := newMultipartImageRequestWithPartContentType(t, []byte("hello"), "note.txt", "de", "image/png")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for content-type spoof, got %d", resp.StatusCode)
	}
}

func TestCreateImageJob_PathTraversalFilenameNeutralized(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	req := newMultipartImageRequest(t, []byte("\x89PNG"), "../../evil.png", "de", "en", "overlay")
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if strings.Contains(cs.job.FilePath, "evil") || strings.Contains(cs.job.FilePath, "..") {
		t.Fatalf("expected sanitized generated path, got %q", cs.job.FilePath)
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

func TestCreateImageJob_ModeOCROnly(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	req := newMultipartImageRequest(t, []byte("\x89PNG"), "img.png", "de", "en", string(jobs.ModeOCROnly))
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.Mode != jobs.ModeOCROnly {
		t.Fatalf("expected mode=%q, got %q", jobs.ModeOCROnly, cs.job.Mode)
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

// newMultipartImageRequestWithFields builds a multipart request including arbitrary
// extra string fields (e.g. "jpeg_quality", "bg_alpha", "text_padding").
func newMultipartImageRequestWithFields(t *testing.T, imageData []byte, filename, target string, extra map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(imageData); err != nil {
		t.Fatalf("write image data: %v", err)
	}
	if err := w.WriteField("target", target); err != nil {
		t.Fatalf("write target: %v", err)
	}
	for k, v := range extra {
		if err := w.WriteField(k, v); err != nil {
			t.Fatalf("write field %q: %v", k, err)
		}
	}
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/jobs/image", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestCreateImageJob_WithJPEGQuality(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	req := newMultipartImageRequestWithFields(t, []byte("\x89PNG"), "img.png", "de", map[string]string{
		"jpeg_quality": "75",
	})
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
	if cs.job.JPEGQuality != 75 {
		t.Fatalf("expected JPEGQuality=75, got %d", cs.job.JPEGQuality)
	}
}

func TestCreateImageJob_InvalidJPEGQuality_Ignored(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	req := newMultipartImageRequestWithFields(t, []byte("\x89PNG"), "img.png", "de", map[string]string{
		"jpeg_quality": "999",
	})
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.JPEGQuality != 0 {
		t.Fatalf("expected JPEGQuality=0 (ignored) for out-of-range value, got %d", cs.job.JPEGQuality)
	}
}

func TestCreateImageJob_WithBgAlpha(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	req := newMultipartImageRequestWithFields(t, []byte("\x89PNG"), "img.png", "de", map[string]string{
		"bg_alpha": "180",
	})
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.BgAlpha != 180 {
		t.Fatalf("expected BgAlpha=180, got %d", cs.job.BgAlpha)
	}
}

func TestCreateImageJob_InvalidBgAlpha_Ignored(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	req := newMultipartImageRequestWithFields(t, []byte("\x89PNG"), "img.png", "de", map[string]string{
		"bg_alpha": "300",
	})
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	// Out-of-range value leaves sentinel -1 ("not set").
	if cs.job.BgAlpha != -1 {
		t.Fatalf("expected BgAlpha=-1 (not set) for out-of-range value, got %d", cs.job.BgAlpha)
	}
}

func TestCreateImageJob_BgAlphaZero_AcceptedAsTransparent(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	// bg_alpha=0 is a valid value meaning "fully transparent background".
	req := newMultipartImageRequestWithFields(t, []byte("\x89PNG"), "img.png", "de", map[string]string{
		"bg_alpha": "0",
	})
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.BgAlpha != 0 {
		t.Fatalf("expected BgAlpha=0 (transparent), got %d", cs.job.BgAlpha)
	}
}

func TestCreateImageJob_BgAlpha_NotProvided_UsesDefault(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	// No bg_alpha field at all — sentinel -1 signals worker to use default.
	req := newMultipartImageRequestWithFields(t, []byte("\x89PNG"), "img.png", "de", map[string]string{})
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.BgAlpha != -1 {
		t.Fatalf("expected BgAlpha=-1 (default sentinel) when field not provided, got %d", cs.job.BgAlpha)
	}
}

func TestCreateImageJob_WithTextPadding(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	req := newMultipartImageRequestWithFields(t, []byte("\x89PNG"), "img.png", "de", map[string]string{
		"text_padding": "10",
	})
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.TextPadding != 10 {
		t.Fatalf("expected TextPadding=10, got %d", cs.job.TextPadding)
	}
}

func TestCreateImageJob_TextPaddingAboveMax_Ignored(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	req := newMultipartImageRequestWithFields(t, []byte("\x89PNG"), "img.png", "de", map[string]string{
		"text_padding": "99",
	})
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	// Out-of-range value leaves sentinel -1 ("not set").
	if cs.job.TextPadding != -1 {
		t.Fatalf("expected TextPadding=-1 (not set) for value>40, got %d", cs.job.TextPadding)
	}
}

func TestCreateImageJob_TextPaddingZero_AcceptedAsFlushToEdge(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	// text_padding=0 is a valid value meaning "no padding (flush to box edge)".
	req := newMultipartImageRequestWithFields(t, []byte("\x89PNG"), "img.png", "de", map[string]string{
		"text_padding": "0",
	})
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.TextPadding != 0 {
		t.Fatalf("expected TextPadding=0 (flush to edge), got %d", cs.job.TextPadding)
	}
}

func TestCreateImageJob_TextPadding_NotProvided_UsesDefault(t *testing.T) {
	cs := &imageJobStore{}
	app, _ := newImageJobsApp(cs)

	// No text_padding field — sentinel -1 signals worker to use default.
	req := newMultipartImageRequestWithFields(t, []byte("\x89PNG"), "img.png", "de", map[string]string{})
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if cs.job == nil {
		t.Fatal("expected job to be enqueued")
	}
	if cs.job.TextPadding != -1 {
		t.Fatalf("expected TextPadding=-1 (default sentinel) when field not provided, got %d", cs.job.TextPadding)
	}
}
