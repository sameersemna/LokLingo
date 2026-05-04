package handlers

import (
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"

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

	req.Text = strings.TrimSpace(req.Text)
	req.Source = strings.TrimSpace(req.Source)
	req.Target = strings.TrimSpace(req.Target)

	if req.Text == "" {
		return errResponse(c, fiber.StatusBadRequest, "text is required")
	}
	if req.Target == "" {
		return errResponse(c, fiber.StatusBadRequest, "target is required")
	}
	if req.Source == "" {
		req.Source = "auto"
	}

	// Check translation cache first.
	if cached, err := h.store.GetCached(c.Context(), req.Text, req.Source, req.Target); err == nil {
		slog.Info("cache hit (sync)", "request_id", c.Locals("requestID"))
		return c.JSON(TranslateResponse{
			TranslatedText: cached,
			Source:         req.Source,
			Target:         req.Target,
			Cached:         true,
		})
	}

	translated, err := h.service.Translate(services.TranslationInput{
		Text:   req.Text,
		Source: req.Source,
		Target: req.Target,
		Ctx:    c.Context(),
	})
	if err != nil {
		slog.Error("translation failed", "request_id", c.Locals("requestID"), "err", err)
		return errResponse(c, fiber.StatusBadGateway, "translation failed")
	}

	// Populate cache for future requests.
	if cerr := h.store.SetCached(c.Context(), req.Text, req.Source, req.Target, translated); cerr != nil {
		slog.Warn("failed to cache translation", "request_id", c.Locals("requestID"), "err", cerr)
	}

	return c.JSON(TranslateResponse{
		TranslatedText: translated,
		Source:         req.Source,
		Target:         req.Target,
	})
}
