package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAIChatProviderTranslate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected path /chat/completions, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("expected Authorization header, got %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["model"] != "gpt-test" {
			t.Fatalf("expected model gpt-test, got %#v", body["model"])
		}

		messages, ok := body["messages"].([]any)
		if !ok || len(messages) != 2 {
			t.Fatalf("expected 2 messages, got %#v", body["messages"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"bonjour"}}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIChatProvider("litellm", server.URL, "test-key", "gpt-test", time.Second)
	translated, err := provider.Translate(context.Background(), TranslateRequest{
		Text:   "hello",
		Source: "en",
		Target: "fr",
	})
	if err != nil {
		t.Fatalf("Translate returned error: %v", err)
	}
	if translated.Text != "bonjour" {
		t.Fatalf("expected translated text %q, got %q", "bonjour", translated.Text)
	}
	if translated.LatencyMS < 0 {
		t.Fatalf("expected non-negative latency, got %d", translated.LatencyMS)
	}
}

func TestOpenAIChatProviderReturnsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary overload", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	provider := NewOpenAIChatProvider("litellm", server.URL, "", "gpt-test", time.Second)
	_, err := provider.Translate(context.Background(), TranslateRequest{
		Text:   "hello",
		Source: "en",
		Target: "fr",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Provider != "litellm" {
		t.Fatalf("expected provider name litellm, got %q", providerErr.Provider)
	}
	if providerErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", providerErr.StatusCode)
	}
	if !providerErr.IsTransient() {
		t.Fatal("expected 503 error to be transient")
	}
}

func TestNewOpenAIChatProviderNormalizesBareHostToV1BaseURL(t *testing.T) {
	provider := NewOpenAIChatProvider("ollama", "localhost:11435", "", "llama3.2", time.Second)
	if provider.baseURL != "http://localhost:11435/v1" {
		t.Fatalf("expected normalized base URL %q, got %q", "http://localhost:11435/v1", provider.baseURL)
	}
}

func TestNewOpenAIChatProviderPreservesExplicitV1Path(t *testing.T) {
	provider := NewOpenAIChatProvider("ollama", "http://localhost:11435/v1", "", "llama3.2", time.Second)
	if provider.baseURL != "http://localhost:11435/v1" {
		t.Fatalf("expected explicit /v1 path to be preserved, got %q", provider.baseURL)
	}
}
