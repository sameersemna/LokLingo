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
	"sync"
	"time"

	"loklingo/backend/config"
	"loklingo/backend/internal/observability"
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

	cbMu        sync.Mutex
	cbFailures  int
	cbOpenUntil time.Time
}

const (
	translationMaxErrorBodyBytes = 16 * 1024
)

var (
	translationMaxAttempts                   = 3
	translationInitialBackoff                = 200 * time.Millisecond
	translationMaxBackoff                    = 2 * time.Second
	translationMaxSuccessBody          int64 = 2 * 1024 * 1024
	translationCircuitBreakerThreshold       = 5
	translationCircuitBreakerOpenFor         = 5 * time.Second
)

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
	if err := s.circuitAllow(); err != nil {
		observability.IncLiteLLMCircuitReject()
		slog.Warn("litellm_circuit_reject")
		return "", err
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

	resp, err := s.doWithRetry(ctx, httpReq)
	if err != nil {
		if ctx.Err() == nil {
			s.circuitRecordFailure()
		}
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, readErr := io.ReadAll(io.LimitReader(resp.Body, translationMaxErrorBodyBytes))
		if readErr != nil {
			return "", fmt.Errorf("litellm status %d (error body read failed): %w", resp.StatusCode, readErr)
		}
		return "", fmt.Errorf("litellm status %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	respBytes, err := readBodyLimited(resp.Body, translationMaxSuccessBody)
	if err != nil {
		observability.IncLiteLLMResponseRejectedBody()
		slog.Warn("litellm_response_rejected", "reason", "body_too_large_or_unreadable", "err", err)
		s.circuitRecordFailure()
		return "", fmt.Errorf("decode response: %w", err)
	}

	var llmResp liteLLMResponse
	if err := json.Unmarshal(respBytes, &llmResp); err != nil {
		observability.IncLiteLLMResponseRejectedJSON()
		slog.Warn("litellm_response_rejected", "reason", "invalid_json", "err", err)
		s.circuitRecordFailure()
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(llmResp.Choices) == 0 {
		observability.IncLiteLLMResponseRejectedShape()
		slog.Warn("litellm_response_rejected", "reason", "missing_choices")
		s.circuitRecordFailure()
		return "", fmt.Errorf("litellm returned no choices")
	}
	result := ""
	for _, choice := range llmResp.Choices {
		if c := strings.TrimSpace(choice.Message.Content); c != "" {
			result = c
			break
		}
	}
	if result == "" {
		observability.IncLiteLLMResponseRejectedShape()
		slog.Warn("litellm_response_rejected", "reason", "empty_choice_content")
		s.circuitRecordFailure()
		return "", fmt.Errorf("litellm returned empty choice content")
	}
	s.circuitRecordSuccess()
	slog.Info("translation completed",
		"source", input.Source,
		"target", input.Target,
		"input_chars", len(input.Text),
		"output_chars", len(result),
		"latency_ms", time.Since(start).Milliseconds(),
	)
	return result, nil
}

func (s *translationService) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	backoff := translationInitialBackoff
	var lastErr error

	for attempt := 1; attempt <= translationMaxAttempts; attempt++ {
		clone := req.Clone(ctx)
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("reset request body: %w", err)
			}
			clone.Body = body
		}

		resp, err := s.client.Do(clone)
		if err == nil {
			if !isTransientStatus(resp.StatusCode) {
				return resp, nil
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("litellm transient status %d", resp.StatusCode)
			slog.Warn("litellm_retry", "attempt", attempt, "reason", "transient_status", "status", resp.StatusCode)
		} else {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("call litellm: %w", ctx.Err())
			}
			lastErr = fmt.Errorf("call litellm: %w", err)
			slog.Warn("litellm_retry", "attempt", attempt, "reason", "network_error", "err", err)
		}

		if attempt == translationMaxAttempts {
			break
		}

		observability.IncLiteLLMRetryAttempt()
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			observability.IncLiteLLMRetryCancelled()
			slog.Warn("litellm_retry_cancelled", "attempt", attempt, "err", ctx.Err())
			return nil, fmt.Errorf("call litellm: %w", ctx.Err())
		case <-timer.C:
		}
		if backoff < translationMaxBackoff {
			backoff *= 2
			if backoff > translationMaxBackoff {
				backoff = translationMaxBackoff
			}
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("call litellm: unknown failure")
	}
	observability.IncLiteLLMRetryExhausted()
	return nil, lastErr
}

func isTransientStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable || status == http.StatusBadGateway || status == http.StatusGatewayTimeout
}

func readBodyLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > maxBytes {
		return nil, fmt.Errorf("response body exceeds max size (%d bytes)", maxBytes)
	}
	return b, nil
}

func (s *translationService) circuitAllow() error {
	s.cbMu.Lock()
	defer s.cbMu.Unlock()
	if s.cbOpenUntil.After(time.Now()) {
		slog.Warn("litellm_circuit_open", "open_until", s.cbOpenUntil)
		return fmt.Errorf("litellm temporarily unavailable; circuit breaker open")
	}
	return nil
}

func (s *translationService) circuitRecordFailure() {
	s.cbMu.Lock()
	defer s.cbMu.Unlock()
	s.cbFailures++
	if s.cbFailures >= translationCircuitBreakerThreshold {
		s.cbOpenUntil = time.Now().Add(translationCircuitBreakerOpenFor)
		s.cbFailures = 0
		observability.IncLiteLLMCircuitOpened()
		slog.Warn("litellm_circuit_opened", "open_for_ms", translationCircuitBreakerOpenFor.Milliseconds())
	}
}

func (s *translationService) circuitRecordSuccess() {
	s.cbMu.Lock()
	defer s.cbMu.Unlock()
	hadOpenState := !s.cbOpenUntil.IsZero()
	s.cbFailures = 0
	if s.cbOpenUntil.Before(time.Now()) {
		s.cbOpenUntil = time.Time{}
	}
	if hadOpenState {
		observability.IncLiteLLMCircuitRecovered()
		slog.Info("litellm_circuit_recovered")
	}
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
