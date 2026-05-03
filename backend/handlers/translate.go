package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"loklingo/backend/config"
)

// TranslateRequest is the body accepted by POST /api/v1/translate.
type TranslateRequest struct {
	SourceText string `json:"source_text"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
}

// TranslateResponse is the JSON response returned to the client.
type TranslateResponse struct {
	TranslatedText string `json:"translated_text"`
	SourceLang     string `json:"source_lang"`
	TargetLang     string `json:"target_lang"`
}

// liteLLMRequest mirrors the OpenAI chat completion request body.
type liteLLMRequest struct {
	Model    string           `json:"model"`
	Messages []liteLLMMessage `json:"messages"`
}

type liteLLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type liteLLMResponse struct {
	Choices []struct {
		Message liteLLMMessage `json:"message"`
	} `json:"choices"`
}

// TranslateHandler holds dependencies for the translate endpoint.
type TranslateHandler struct {
	cfg    *config.Config
	client *http.Client
}

// NewTranslateHandler constructs a TranslateHandler.
func NewTranslateHandler(cfg *config.Config) *TranslateHandler {
	return &TranslateHandler{
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// Translate handles POST /api/v1/translate.
func (h *TranslateHandler) Translate(c *fiber.Ctx) error {
	var req TranslateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	req.SourceText = strings.TrimSpace(req.SourceText)
	req.SourceLang = strings.TrimSpace(req.SourceLang)
	req.TargetLang = strings.TrimSpace(req.TargetLang)

	if req.SourceText == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "source_text is required",
		})
	}
	if req.TargetLang == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "target_lang is required",
		})
	}

	translated, err := h.callLiteLLM(req)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": fmt.Sprintf("translation failed: %v", err),
		})
	}

	return c.JSON(TranslateResponse{
		TranslatedText: translated,
		SourceLang:     req.SourceLang,
		TargetLang:     req.TargetLang,
	})
}

func (h *TranslateHandler) callLiteLLM(req TranslateRequest) (string, error) {
	systemPrompt := fmt.Sprintf(
		"You are a professional translator. Translate the following text from %s to %s. Return only the translated text, nothing else.",
		langName(req.SourceLang), langName(req.TargetLang),
	)

	payload := liteLLMRequest{
		Model: h.cfg.LiteLLMModel,
		Messages: []liteLLMMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: req.SourceText},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	url := strings.TrimRight(h.cfg.LiteLLMBaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+h.cfg.LiteLLMAPIKey)

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("litellm status %d: %s", resp.StatusCode, string(respBytes))
	}

	var llmResp liteLLMResponse
	if err := json.Unmarshal(respBytes, &llmResp); err != nil {
		return "", fmt.Errorf("unmarshal: %w", err)
	}

	if len(llmResp.Choices) == 0 {
		return "", fmt.Errorf("no choices returned from litellm")
	}

	return strings.TrimSpace(llmResp.Choices[0].Message.Content), nil
}

// langName maps ISO 639-1 codes to readable names for the prompt.
func langName(code string) string {
	names := map[string]string{
		"en": "English",
		"de": "German",
		"fr": "French",
		"hi": "Hindi",
		"ur": "Urdu",
		"ar": "Arabic",
		"bn": "Bengali",
		"es": "Spanish",
		"zh": "Chinese",
		"ja": "Japanese",
	}
	if name, ok := names[strings.ToLower(code)]; ok {
		return name
	}
	return code
}
