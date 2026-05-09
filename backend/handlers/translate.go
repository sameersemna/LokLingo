package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"loklingo/backend/jobs"
	"loklingo/backend/services"
)

// TranslateRequest is the body accepted by POST /api/v1/translate.
type TranslateRequest struct {
	Text   string `json:"text"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// TranslateResponse is the JSON response returned to the client.
type TranslateResponse struct {
	TranslatedText string `json:"translated_text"`
	Source         string `json:"source"`
	Target         string `json:"target"`
	Cached         bool   `json:"cached,omitempty"`
}

// TranslateHandler holds dependencies for the translate endpoint.
type TranslateHandler struct {
	service services.TranslationService
	store   jobs.Store
}

var syncImagePollTimeout = 120 * time.Second
var syncImagePollInterval = 600 * time.Millisecond
var syncImageMaxUploadBytes int64 = 25 * 1024 * 1024

// NewTranslateHandler constructs a TranslateHandler.
func NewTranslateHandler(service services.TranslationService, store jobs.Store) *TranslateHandler {
	return &TranslateHandler{service: service, store: store}
}

// Translate handles POST /api/v1/translate.
func (h *TranslateHandler) Translate(c *fiber.Ctx) error {
	var req TranslateRequest
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

	// Check translation cache first.
	if cached, err := h.store.GetCached(c.Context(), text, source, target); err == nil {
		slog.Info("cache hit (sync)", "request_id", c.Locals("requestID"))
		return c.JSON(TranslateResponse{
			TranslatedText: cached,
			Source:         source,
			Target:         target,
			Cached:         true,
		})
	}

	translated, err := h.service.Translate(services.TranslationInput{
		Text:   text,
		Source: source,
		Target: target,
		Ctx:    c.Context(),
	})
	if err != nil {
		slog.Error("translation failed", "request_id", c.Locals("requestID"), "err", err)
		return errResponse(c, fiber.StatusBadGateway, "translation failed")
	}

	// Populate cache for future requests.
	if cerr := h.store.SetCached(c.Context(), text, source, target, translated); cerr != nil {
		slog.Warn("failed to cache translation", "request_id", c.Locals("requestID"), "err", cerr)
	}

	return c.JSON(TranslateResponse{
		TranslatedText: translated,
		Source:         source,
		Target:         target,
	})
}

// TranslateImage handles POST /api/v1/translate/image.
//
// Accepts multipart/form-data:
//   - file: image upload
//   - source: optional (defaults to "auto")
//   - target: required
//   - mode: optional (overlay|layout, defaults to "overlay")
//
// Returns JSON:
//   - image_url: download URL of rendered image
func (h *TranslateHandler) TranslateImage(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return errResponse(c, fiber.StatusBadRequest, "file is required (multipart field: file)")
	}
	ext, err := validateImageUpload(file, syncImageMaxUploadBytes)
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

	mode, err := normalizeAndValidateImageMode(c.FormValue("mode"))
	if err != nil {
		return errResponse(c, fiber.StatusBadRequest, err.Error())
	}

	if err := os.MkdirAll(jobs.ImageUploadDir, 0o700); err != nil {
		slog.Error("failed to create image upload dir", "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to prepare upload directory")
	}

	if ext == "" {
		ext = ".bin"
	}
	uploadedPath := filepath.Join(jobs.ImageUploadDir, fmt.Sprintf("%s%s", uuid.NewString(), ext))
	if err := c.SaveFile(file, uploadedPath); err != nil {
		slog.Error("failed to save uploaded image", "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to save uploaded file")
	}

	now := time.Now()
	job := &jobs.Job{
		ID:        uuid.NewString(),
		Type:      jobs.TypeImage,
		Status:    jobs.StatusPending,
		Mode:      mode,
		FilePath:  uploadedPath,
		Source:    source,
		Target:    target,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.store.Enqueue(c.Context(), job); err != nil {
		slog.Error("failed to enqueue sync image translate job", "request_id", c.Locals("requestID"), "err", err)
		return errResponse(c, fiber.StatusInternalServerError, "failed to enqueue job")
	}

	deadline := time.Now().Add(syncImagePollTimeout)
	for time.Now().Before(deadline) {
		current, getErr := h.store.Get(c.Context(), job.ID)
		if errors.Is(getErr, jobs.ErrNotFound) {
			return errResponse(c, fiber.StatusNotFound, "job not found")
		}
		if getErr != nil {
			slog.Error("failed to retrieve image translate job", "request_id", c.Locals("requestID"), "job_id", job.ID, "err", getErr)
			return errResponse(c, fiber.StatusInternalServerError, "failed to retrieve job")
		}

		switch current.Status {
		case jobs.StatusCompleted:
			resp := fiber.Map{
				"translated_text": current.TranslatedText,
				"source":          firstNonEmpty(current.Source, source),
				"target":          firstNonEmpty(current.Target, target),
			}
			if current.OutputFilePath != "" {
				resp["image_url"] = fmt.Sprintf("/api/v1/jobs/%s/output", current.ID)
			}
			return c.JSON(resp)
		case jobs.StatusFailed:
			msg := strings.TrimSpace(current.ErrorMsg)
			if msg == "" {
				msg = "image translation failed"
			}
			return errResponse(c, fiber.StatusBadGateway, msg)
		}

		if err := c.Context().Err(); err != nil {
			return errResponse(c, fiber.StatusRequestTimeout, "request context cancelled")
		}

		time.Sleep(syncImagePollInterval)
	}

	return errResponse(c, fiber.StatusGatewayTimeout, "image translation timed out")
}

func firstNonEmpty(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v != "" {
		return v
	}
	return fallback
}
