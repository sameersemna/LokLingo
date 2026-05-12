package providers

import "time"

// NewLiteLLMProvider constructs a TranslationProvider targeting a LiteLLM
// OpenAI-compatible endpoint.
func NewLiteLLMProvider(baseURL, apiKey, model string, timeout time.Duration) TranslationProvider {
	return NewOpenAIChatProvider("litellm", baseURL, apiKey, model, timeout)
}

// NewOpenAICompatProvider constructs a TranslationProvider targeting an
// OpenAI-compatible backend.
func NewOpenAICompatProvider(baseURL, apiKey, model string, timeout time.Duration) TranslationProvider {
	return NewOpenAIChatProvider("openai_compat", baseURL, apiKey, model, timeout)
}

// NewOllamaProvider constructs a TranslationProvider targeting an Ollama
// OpenAI-compatible endpoint (usually /v1/chat/completions).
func NewOllamaProvider(baseURL, model string, timeout time.Duration) TranslationProvider {
	// Ollama commonly runs without authentication.
	return NewOpenAIChatProvider("ollama", baseURL, "", model, timeout)
}
