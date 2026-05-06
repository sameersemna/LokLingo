package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"loklingo/backend/config"
)

type TranslationInput struct {
	Text   string
	Source string
	Target string
	// Ctx carries the request context for timeout / cancellation.
	Ctx context.Context
}

type TranslationService interface {
	Translate(input TranslationInput) (string, error)
}

type translationService struct {
	cfg    *config.Config
	client *http.Client
}

func NewTranslationService(cfg *config.Config) TranslationService {
	timeout := time.Duration(cfg.LiteLLMRequestTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second // 5-minute safe default
	}
	return &translationService{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

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

func (s *translationService) Translate(input TranslationInput) (string, error) {
	ctx := input.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	start := time.Now()

	systemPrompt := fmt.Sprintf(
		"You are a professional translator. Translate the following text from %s to %s. Return only translated text.",
		langName(input.Source), langName(input.Target),
	)

	payload := liteLLMRequest{
		Model: s.cfg.LiteLLMModel,
		Messages: []liteLLMMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: input.Text},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(s.cfg.LiteLLMBaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.LiteLLMAPIKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call litellm: %w", err)
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
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(llmResp.Choices) == 0 {
		return "", fmt.Errorf("litellm returned no choices")
	}

	result := strings.TrimSpace(llmResp.Choices[0].Message.Content)
	slog.Info("translation completed",
		"source", input.Source,
		"target", input.Target,
		"input_chars", len(input.Text),
		"output_chars", len(result),
		"latency_ms", time.Since(start).Milliseconds(),
	)
	return result, nil
}

func langName(code string) string {
	names := map[string]string{
		"en":   "English",
		"de":   "German",
		"fr":   "French",
		"hi":   "Hindi",
		"ur":   "Urdu",
		"ar":   "Arabic",
		"bn":   "Bengali",
		"es":   "Spanish",
		"zh":   "Chinese",
		"ja":   "Japanese",
		"auto": "Auto",
	}
	if name, ok := names[strings.ToLower(code)]; ok {
		return name
	}
	return code
}
