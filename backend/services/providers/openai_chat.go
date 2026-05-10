package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxSuccessBodyBytes int64 = 2 * 1024 * 1024 // 2 MiB
	maxErrorBodyBytes   int64 = 16 * 1024       // 16 KiB
)

// OpenAIChatProvider implements TranslationProvider via the OpenAI chat
// completions API. It is wire-compatible with LiteLLM, OpenAI, Ollama (/v1),
// and any other OpenAI-compatible proxy — the only difference is the base URL,
// model, and optional API key.
type OpenAIChatProvider struct {
	name    string
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// NewOpenAIChatProvider constructs a provider for any OpenAI-chat-compatible
// backend.
//
//   - name     — stable identifier for logs/metrics (e.g. "litellm", "openai", "ollama").
//   - baseURL  — e.g. "http://litellm:4000" or "https://api.openai.com/v1".
//   - apiKey   — bearer token; may be empty for unauthenticated backends.
//   - model    — model string sent in the request body.
//   - timeout  — hard HTTP client deadline; ≤ 0 defaults to 300 s.
func NewOpenAIChatProvider(name, baseURL, apiKey, model string, timeout time.Duration) *OpenAIChatProvider {
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	return &OpenAIChatProvider{
		name:    name,
		baseURL: normalizeOpenAICompatBaseURL(baseURL),
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: timeout},
	}
}

func normalizeOpenAICompatBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	bareHost := !strings.Contains(trimmed, "://")
	if bareHost {
		trimmed = "http://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return strings.TrimRight(trimmed, "/")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if bareHost && parsed.Path == "" {
		parsed.Path = "/v1"
	}
	return strings.TrimRight(parsed.String(), "/")
}

func (p *OpenAIChatProvider) Name() string { return p.name }

// Translate makes a single POST /chat/completions call and returns the first
// non-empty assistant message. It honours ctx for cancellation and deadline.
func (p *OpenAIChatProvider) Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error) {
	start := time.Now()

	systemPrompt := fmt.Sprintf(
		"You are a professional translator. Translate the following text from %s to %s. Return only translated text.",
		langName(req.Source), langName(req.Target),
	)

	payload := chatCompletionRequest{
		Model: p.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: req.Text},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	url := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return TranslateResponse{}, &ProviderError{
			Provider:   p.name,
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(errBody)),
		}
	}

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxSuccessBodyBytes+1))
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("read response body: %w", err)
	}
	if int64(len(respBytes)) > maxSuccessBodyBytes {
		return TranslateResponse{}, fmt.Errorf("provider %s response body exceeds max size", p.name)
	}

	var chatResp chatCompletionResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return TranslateResponse{}, fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return TranslateResponse{}, fmt.Errorf("provider %s returned no choices", p.name)
	}

	for _, choice := range chatResp.Choices {
		if c := strings.TrimSpace(choice.Message.Content); c != "" {
			return TranslateResponse{
				Text:      c,
				LatencyMS: time.Since(start).Milliseconds(),
			}, nil
		}
	}

	return TranslateResponse{}, fmt.Errorf("provider %s returned empty choice content", p.name)
}

// ── shared OpenAI chat types ─────────────────────────────────────────────────

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// langName maps an ISO 639-1 code to a human-readable name used in the system
// prompt. Falls back to the raw code when no mapping is found.
func langName(code string) string {
	names := map[string]string{
		"af": "Afrikaans",
		"ar": "Arabic",
		"bn": "Bengali",
		"cs": "Czech",
		"da": "Danish",
		"de": "German",
		"el": "Greek",
		"en": "English",
		"es": "Spanish",
		"fa": "Persian",
		"fi": "Finnish",
		"fr": "French",
		"gu": "Gujarati",
		"he": "Hebrew",
		"hi": "Hindi",
		"hr": "Croatian",
		"hu": "Hungarian",
		"id": "Indonesian",
		"it": "Italian",
		"ja": "Japanese",
		"ko": "Korean",
		"ml": "Malayalam",
		"mr": "Marathi",
		"ms": "Malay",
		"nl": "Dutch",
		"no": "Norwegian",
		"pa": "Punjabi",
		"pl": "Polish",
		"pt": "Portuguese",
		"ro": "Romanian",
		"ru": "Russian",
		"sk": "Slovak",
		"sv": "Swedish",
		"sw": "Swahili",
		"ta": "Tamil",
		"te": "Telugu",
		"th": "Thai",
		"tr": "Turkish",
		"uk": "Ukrainian",
		"ur": "Urdu",
		"vi": "Vietnamese",
		"zh": "Chinese",
	}
	if n, ok := names[strings.ToLower(code)]; ok {
		return n
	}
	return code
}
