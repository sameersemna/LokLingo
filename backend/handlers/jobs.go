package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"loklingo/backend/internal/observability"
	"loklingo/backend/jobs"
)

type queueStatsProvider interface {
	QueueStats(ctx context.Context) (jobs.QueueStats, error)
}

// JobsHandler holds dependencies for the async job endpoints.
type JobsHandler struct {
	store             jobs.Store
	maxPDFUploadBytes int64
}

type deadLetterLister interface {
	ListDead(ctx context.Context, limit int) ([]jobs.DeadJobEntry, error)
}

type deadLetterReplayer interface {
	ReplayDead(ctx context.Context, id string) (*jobs.Job, error)
}

// NewJobsHandler constructs a JobsHandler.
func NewJobsHandler(store jobs.Store, maxPDFUploadBytes int64) *JobsHandler {
	if maxPDFUploadBytes <= 0 {
		maxPDFUploadBytes = 25 * 1024 * 1024
	}
	return &JobsHandler{store: store, maxPDFUploadBytes: maxPDFUploadBytes}
}

// CreateJob handles POST /api/v1/jobs.
// It accepts the same body as /api/v1/translate, enqueues the work, and
// immediately returns 202 Accepted with a job_id for polling.
func (h *JobsHandler) CreateJob(c *fiber.Ctx) error {
	var req struct {
		Text   string `json:"text"`
		Source string `json:"source"`
		Target string `json:"target"`
		Mode   string `json:"mode"`
	}
	if err := c.BodyParser(&req); err != nil {
		return errResponse(c, fiber.StatusBadRequest, "invalid request body")
	}
	text, err := normalizeAndValidateText(req.Text)
	if err != nil {
		if strings.HasPrefix(err.Error(), "text exceeds max length") {
			return errResponse(c, fiber.StatusRequestEntityTooLarge, err.Error())
		}
		return errResponse(c, fiber.StatusBadRequest, err.Error())
	}
	source, target, err := normalizeAndValidateSourceTarget(req.Source, req.Target)
	if err != nil {
		return errResponse(c, fiber.StatusBadRequest, err.Error())
	}
	mode, err := normalizeAndValidateMode(req.Mode)
	if err != nil {
		return errResponse(c, fiber.StatusBadRequest, err.Error())
	}

	now := time.Now()
	correlationID, _ := c.Locals("requestID").(string)
	if correlationID == "" {
		correlationID = uuid.NewString()
	}
	job := &jobs.Job{
		ID:            uuid.NewString(),
		CorrelationID: correlationID,
		Status:        jobs.StatusPending,
		Mode:          mode,
		Text:          text,
		Source:        source,
		Target:        target,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := h.store.Enqueue(c.Context(), job); err != nil {
		slog.Error("failed to enqueue job", "request_id", c.Locals("requestID"), "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to enqueue job")
	}

	queueDepth := int64(0)
	if provider, ok := h.store.(queueStatsProvider); ok {
		if stats, err := provider.QueueStats(c.Context()); err == nil {
			queueDepth = stats.QueueDepth
		}
	}
	observability.EmitLifecycleEvent("job_created", observability.LifecycleEvent{
		JobID:         job.ID,
		CorrelationID: job.CorrelationID,
		Provider:      "queue",
		QueueDepth:    queueDepth,
	})

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"job_id":         job.ID,
		"correlation_id": job.CorrelationID,
		"status":         job.Status,
	})
}

// GetJob handles GET /api/v1/jobs/:id.
// Returns the current state of the job, including the translated text when complete.
func (h *JobsHandler) GetJob(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return errResponse(c, fiber.StatusBadRequest, "job id is required")
	}

	job, err := h.store.Get(c.Context(), id)
	if errors.Is(err, jobs.ErrNotFound) {
		return errResponse(c, fiber.StatusNotFound, "job not found")
	}
	if err != nil {
		slog.Error("failed to retrieve job", "request_id", c.Locals("requestID"), "job_id", id, "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to retrieve job")
	}

	return c.JSON(buildJobResponse(job))
}

// StreamJobEvents handles GET /api/v1/jobs/:id/events and streams structured
// job updates as Server-Sent Events until the job reaches a terminal state.
func (h *JobsHandler) StreamJobEvents(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return errResponse(c, fiber.StatusBadRequest, "job id is required")
	}

	// Validate existence up front to return proper HTTP status for unknown jobs.
	if _, err := h.store.Get(c.Context(), id); errors.Is(err, jobs.ErrNotFound) {
		return errResponse(c, fiber.StatusNotFound, "job not found")
	} else if err != nil {
		slog.Error("failed to retrieve job for event stream", "request_id", c.Locals("requestID"), "job_id", id, "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to retrieve job")
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	done := c.Context().Done()
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		ticker := time.NewTicker(800 * time.Millisecond)
		defer ticker.Stop()

		lastPayload := ""
		sendEvent := func(event string, payload fiber.Map) bool {
			b, err := json.Marshal(payload)
			if err != nil {
				return false
			}
			if _, err := w.WriteString("event: " + event + "\n"); err != nil {
				return false
			}
			if _, err := w.WriteString("data: " + string(b) + "\n\n"); err != nil {
				return false
			}
			if err := w.Flush(); err != nil {
				return false
			}
			return true
		}

		for {
			job, err := h.store.Get(context.Background(), id)
			if err != nil {
				payload := fiber.Map{"job_id": id, "event_type": "stream_error", "error": "job stream unavailable", "timestamp": time.Now().UTC()}
				_ = sendEvent("error", payload)
				return
			}

			payload := buildJobResponse(job)
			payload["event_type"] = eventTypeForJob(job)
			payload["timestamp"] = time.Now().UTC()

			encoded, _ := json.Marshal(payload)
			encodedStr := string(encoded)
			if encodedStr != lastPayload {
				if !sendEvent("progress", payload) {
					return
				}
				lastPayload = encodedStr
			}

			if job.Status == jobs.StatusCompleted || job.Status == jobs.StatusFailed {
				return
			}

			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	})

	return nil
}

func buildJobResponse(job *jobs.Job) fiber.Map {
	resp := fiber.Map{
		"job_id":         job.ID,
		"correlation_id": job.CorrelationID,
		"status":         job.Status,
		"mode":           job.Mode,
		"source":         job.Source,
		"target":         job.Target,
		"created_at":     job.CreatedAt,
		"updated_at":     job.UpdatedAt,
	}
	if job.Type == jobs.TypePDF {
		resp["total_pages"] = job.TotalPages
		resp["processed_pages"] = job.ProcessedPages
	}
	if job.Stage != "" {
		resp["stage"] = job.Stage
		resp["stage_message"] = job.StageMessage
		resp["stage_progress"] = job.StageProgress
	}
	if len(job.Warnings) > 0 {
		resp["warnings"] = job.Warnings
	}
	if job.OCRConfidence > 0 {
		resp["ocr_confidence"] = job.OCRConfidence
	}
	if job.Status == jobs.StatusCompleted {
		resp["translated_text"] = job.TranslatedText
		if job.ProcessingMethod != "" {
			resp["processing_method"] = job.ProcessingMethod
		}
		if job.OutputFilePath != "" {
			resp["output_file_path"] = job.OutputFilePath
			if job.Type == jobs.TypeImage {
				resp["image_url"] = fmt.Sprintf("/api/v1/jobs/%s/output", job.ID)
			}
		}
	}
	if job.Status == jobs.StatusFailed {
		resp["error"] = job.ErrorMsg
	}
	return resp
}

func eventTypeForJob(job *jobs.Job) string {
	if job == nil {
		return "job_update"
	}
	if job.Status == jobs.StatusCompleted {
		return "job_complete"
	}
	if job.Status == jobs.StatusFailed {
		return "job_failed"
	}
	switch strings.ToLower(job.Stage) {
	case jobs.StageDetectingText:
		return "ocr_complete"
	case jobs.StageTranslating:
		return "translation_progress"
	case jobs.StageRetrying:
		return "retrying"
	case jobs.StageFallbackProvider:
		return "fallback_provider_active"
	case jobs.StageRendering, jobs.StageRebuildingLayout:
		return "rendering_progress"
	default:
		return "job_update"
	}
}

// CreatePDFJob handles POST /api/v1/jobs/pdf.
// Accepts multipart/form-data with a PDF file upload, saves it under
// jobs.PDFUploadDir, enqueues a translate_pdf job, and returns 202 Accepted
// with a job_id for polling. The file is NOT processed here.
func (h *JobsHandler) CreatePDFJob(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return errResponse(c, fiber.StatusBadRequest, "file is required (multipart field: file)")
	}
	if err := validatePDFUpload(file, h.maxPDFUploadBytes); err != nil {
		if strings.HasPrefix(err.Error(), "file exceeds max size") {
			return errResponse(c, fiber.StatusRequestEntityTooLarge, err.Error())
		}
		return errResponse(c, fiber.StatusBadRequest, err.Error())
	}
	source, target, err := normalizeAndValidateSourceTarget(c.FormValue("source"), c.FormValue("target"))
	if err != nil {
		return errResponse(c, fiber.StatusBadRequest, err.Error())
	}
	lang := strings.TrimSpace(c.FormValue("lang"))
	mode, err := normalizeAndValidatePDFMode(c.FormValue("mode"))
	if err != nil {
		return errResponse(c, fiber.StatusBadRequest, err.Error())
	}

	if err := os.MkdirAll(jobs.PDFUploadDir, 0o700); err != nil {
		slog.Error("failed to create upload dir", "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to prepare upload directory")
	}

	fileName := fmt.Sprintf("%s.pdf", uuid.NewString())
	filePath := filepath.Join(jobs.PDFUploadDir, fileName)
	if err := c.SaveFile(file, filePath); err != nil {
		slog.Error("failed to save uploaded pdf", "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to save uploaded file")
	}

	now := time.Now()
	correlationID, _ := c.Locals("requestID").(string)
	if correlationID == "" {
		correlationID = uuid.NewString()
	}
	job := &jobs.Job{
		ID:            uuid.NewString(),
		CorrelationID: correlationID,
		Type:          jobs.TypePDF,
		Status:        jobs.StatusPending,
		Mode:          mode,
		FilePath:      filePath,
		Lang:          lang,
		Source:        source,
		Target:        target,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := h.store.Enqueue(c.Context(), job); err != nil {
		slog.Error("failed to enqueue pdf job", "request_id", c.Locals("requestID"), "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to enqueue job")
	}

	queueDepth := int64(0)
	if provider, ok := h.store.(queueStatsProvider); ok {
		if stats, err := provider.QueueStats(c.Context()); err == nil {
			queueDepth = stats.QueueDepth
		}
	}
	observability.EmitLifecycleEvent("upload_received", observability.LifecycleEvent{
		JobID:         job.ID,
		CorrelationID: job.CorrelationID,
		Provider:      "pdf",
		QueueDepth:    queueDepth,
	})
	observability.EmitLifecycleEvent("job_created", observability.LifecycleEvent{
		JobID:         job.ID,
		CorrelationID: job.CorrelationID,
		Provider:      "queue",
		QueueDepth:    queueDepth,
	})

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"job_id":         job.ID,
		"correlation_id": job.CorrelationID,
	})
}

// isValidMode reports whether m is one of the accepted mode values.
func isValidMode(m string) bool {
	return m == jobs.ModeOverlay || m == jobs.ModeLayout
}

// CreateImageJob handles POST /api/v1/jobs/image.
// Accepts multipart/form-data with an image file upload, saves it under
// jobs.ImageUploadDir, enqueues a translate_image job, and returns 202 Accepted
// with a job_id for polling. The file is NOT processed here.
func (h *JobsHandler) CreateImageJob(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return errResponse(c, fiber.StatusBadRequest, "file is required (multipart field: file)")
	}
	ext, err := validateImageUpload(file, h.maxPDFUploadBytes)
	if err != nil {
		if strings.HasPrefix(err.Error(), "file exceeds max size") {
			return errResponse(c, fiber.StatusRequestEntityTooLarge, err.Error())
		}
		return errResponse(c, fiber.StatusBadRequest, err.Error())
	}

	source, target, err := normalizeAndValidateSourceTarget(c.FormValue("source"), c.FormValue("target"))
	if err != nil {
		return errResponse(c, fiber.StatusBadRequest, err.Error())
	}
	lang := strings.TrimSpace(c.FormValue("lang"))

	mode, err := normalizeAndValidateImageMode(c.FormValue("mode"))
	if err != nil {
		return errResponse(c, fiber.StatusBadRequest, err.Error())
	}

	jpegQuality := 0
	if qStr := c.FormValue("jpeg_quality"); qStr != "" {
		if q, err := strconv.Atoi(qStr); err == nil && q >= 1 && q <= 100 {
			jpegQuality = q
		}
	}

	bgAlpha := -1 // -1 means "not set" (use service default 220)
	if aStr := c.FormValue("bg_alpha"); aStr != "" {
		if a, err := strconv.Atoi(aStr); err == nil && a >= 0 && a <= 255 {
			bgAlpha = a
		}
	}

	textPadding := -1 // -1 means "not set" (use service default 6)
	if pStr := c.FormValue("text_padding"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p >= 0 && p <= 40 {
			textPadding = p
		}
	}

	if err := os.MkdirAll(jobs.ImageUploadDir, 0o700); err != nil {
		slog.Error("failed to create image upload dir", "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to prepare upload directory")
	}

	if ext == "" {
		ext = ".bin"
	}
	fileName := fmt.Sprintf("%s%s", uuid.NewString(), ext)
	filePath := filepath.Join(jobs.ImageUploadDir, fileName)
	if err := c.SaveFile(file, filePath); err != nil {
		slog.Error("failed to save uploaded image", "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to save uploaded file")
	}

	now := time.Now()
	correlationID, _ := c.Locals("requestID").(string)
	if correlationID == "" {
		correlationID = uuid.NewString()
	}
	job := &jobs.Job{
		ID:            uuid.NewString(),
		CorrelationID: correlationID,
		Type:          jobs.TypeImage,
		Status:        jobs.StatusPending,
		Mode:          mode,
		FilePath:      filePath,
		Lang:          lang,
		Source:        source,
		Target:        target,
		JPEGQuality:   jpegQuality,
		BgAlpha:       bgAlpha,
		TextPadding:   textPadding,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := h.store.Enqueue(c.Context(), job); err != nil {
		slog.Error("failed to enqueue image job", "request_id", c.Locals("requestID"), "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to enqueue job")
	}

	queueDepth := int64(0)
	if provider, ok := h.store.(queueStatsProvider); ok {
		if stats, err := provider.QueueStats(c.Context()); err == nil {
			queueDepth = stats.QueueDepth
		}
	}
	observability.EmitLifecycleEvent("upload_received", observability.LifecycleEvent{
		JobID:         job.ID,
		CorrelationID: job.CorrelationID,
		Provider:      "image",
		QueueDepth:    queueDepth,
	})
	observability.EmitLifecycleEvent("job_created", observability.LifecycleEvent{
		JobID:         job.ID,
		CorrelationID: job.CorrelationID,
		Provider:      "queue",
		QueueDepth:    queueDepth,
	})

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"job_id":         job.ID,
		"correlation_id": job.CorrelationID,
	})
}

func (h *JobsHandler) DownloadJobOutput(c *fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return errResponse(c, fiber.StatusBadRequest, "job id is required")
	}

	job, err := h.store.Get(c.Context(), id)
	if errors.Is(err, jobs.ErrNotFound) {
		return errResponse(c, fiber.StatusNotFound, "job not found")
	}
	if err != nil {
		slog.Error("failed to retrieve job", "request_id", c.Locals("requestID"), "job_id", id, "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to retrieve job")
	}

	if job.Status != jobs.StatusCompleted || job.OutputFilePath == "" {
		return errResponse(c, fiber.StatusNotFound, "output not available")
	}

	// Guard against directory traversal: the output must live under ImageUploadDir.
	clean := filepath.Clean(job.OutputFilePath)
	allowedDir := filepath.Clean(jobs.ImageUploadDir)
	rel, relErr := filepath.Rel(allowedDir, clean)
	if relErr != nil || rel == "." || rel == "" || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		slog.Error("output_file_path outside allowed dir", "path", clean)
		return errResponse(c, fiber.StatusForbidden, "output not accessible")
	}

	if _, err := os.Stat(clean); err != nil {
		return errResponse(c, fiber.StatusNotFound, "output file not found")
	}

	ext := filepath.Ext(clean)
	contentType := "application/octet-stream"
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".gif":
		contentType = "image/gif"
	case ".webp":
		contentType = "image/webp"
	}

	c.Set("Content-Type", contentType)
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(clean)))
	return c.SendFile(clean)
}

// ListDeadJobs handles GET /api/v1/jobs/dead.
// It returns dead-lettered jobs with their last failure reason.
func (h *JobsHandler) ListDeadJobs(c *fiber.Ctx) error {
	lister, ok := h.store.(deadLetterLister)
	if !ok {
		return errResponse(c, fiber.StatusNotImplemented, "dead-letter listing not supported by this store")
	}

	limit := 25
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return errResponse(c, fiber.StatusBadRequest, "limit must be a positive integer")
		}
		if n > 200 {
			n = 200
		}
		limit = n
	}

	entries, err := lister.ListDead(c.Context(), limit)
	if err != nil {
		slog.Error("failed to list dead-letter jobs", "request_id", c.Locals("requestID"), "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to list dead-letter jobs")
	}

	items := make([]fiber.Map, 0, len(entries))
	for _, e := range entries {
		item := fiber.Map{
			"job_id": e.ID,
			"reason": e.Reason,
		}
		if e.Job != nil {
			item["status"] = e.Job.Status
			item["type"] = e.Job.Type
			item["mode"] = e.Job.Mode
			item["source"] = e.Job.Source
			item["target"] = e.Job.Target
			item["attempt"] = e.Job.Attempt
			item["max_attempts"] = e.Job.MaxAttempts
			item["dead_lettered_at"] = e.Job.DeadLetteredAt
			item["updated_at"] = e.Job.UpdatedAt
		}
		items = append(items, item)
	}

	return c.JSON(fiber.Map{
		"count": len(items),
		"jobs":  items,
	})
}

// ReplayDeadJob handles POST /api/v1/jobs/:id/replay.
// It removes a dead-lettered job from dead-letter storage and requeues it.
func (h *JobsHandler) ReplayDeadJob(c *fiber.Ctx) error {
	replayer, ok := h.store.(deadLetterReplayer)
	if !ok {
		return errResponse(c, fiber.StatusNotImplemented, "dead-letter replay not supported by this store")
	}

	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		return errResponse(c, fiber.StatusBadRequest, "job id is required")
	}

	job, err := replayer.ReplayDead(c.Context(), id)
	if errors.Is(err, jobs.ErrNotFound) {
		return errResponse(c, fiber.StatusNotFound, "job not found")
	}
	if err != nil {
		slog.Error("failed to replay dead-letter job", "request_id", c.Locals("requestID"), "job_id", id, "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to replay job")
	}

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"job_id":       job.ID,
		"status":       job.Status,
		"attempt":      job.Attempt,
		"max_attempts": job.MaxAttempts,
	})
}
