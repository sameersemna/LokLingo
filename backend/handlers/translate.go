package handlers

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// TranslateRequest defines the expected POST body for /translate.
type TranslateRequest struct {
	SourceText string `json:"source_text"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
}

// TranslateResponse is returned by the /translate endpoint.
type TranslateResponse struct {
	TranslatedText string `json:"translated_text"`
	SourceLang     string `json:"source_lang"`
	TargetLang     string `json:"target_lang"`
	Model          string `json:"model"`
}

// Translate handles POST /api/v1/translate.
// At this scaffold stage it returns a stubbed translation.
// Real LLM routing via LiteLLM will be wired here later.
func Translate(c *fiber.Ctx) error {
	var req TranslateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	if strings.TrimSpace(req.SourceText) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "source_text must not be empty",
		})
	}
	if strings.TrimSpace(req.SourceLang) == "" || strings.TrimSpace(req.TargetLang) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "source_lang and target_lang are required",
		})
	}

	// Stub: replace with LiteLLM call
	translated := fmt.Sprintf("[%s→%s] %s", strings.ToUpper(req.SourceLang), strings.ToUpper(req.TargetLang), req.SourceText)

	return c.JSON(TranslateResponse{
		TranslatedText: translated,
		SourceLang:     req.SourceLang,
		TargetLang:     req.TargetLang,
		Model:          "stub",
	})
}
