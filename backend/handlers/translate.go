package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"

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
}

// TranslateHandler holds dependencies for the translate endpoint.
type TranslateHandler struct {
	service services.TranslationService
}

// NewTranslateHandler constructs a TranslateHandler.
func NewTranslateHandler(service services.TranslationService) *TranslateHandler {
	return &TranslateHandler{
		service: service,
	}
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

	translated, err := h.service.Translate(services.TranslationInput{
		Text:   req.Text,
		Source: req.Source,
		Target: req.Target,
	})
	if err != nil {
		return errResponse(c, fiber.StatusBadGateway, "translation failed")
	}

	return c.JSON(TranslateResponse{
		TranslatedText: translated,
		Source:         req.Source,
		Target:         req.Target,
	})
}
