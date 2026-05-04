package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"loklingo/backend/config"
)

func TestTranslationService_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %s", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "Hallo Welt"},
			}},
		})
	}))
	defer ts.Close()

	svc := NewTranslationService(&config.Config{
		LiteLLMBaseURL: ts.URL,
		LiteLLMAPIKey:  "test-key",
		LiteLLMModel:   "gpt-4o-mini",
	})

	got, err := svc.Translate(TranslationInput{Text: "Hello world", Source: "en", Target: "de"})
	if err != nil {
		t.Fatalf("Translate returned error: %v", err)
	}
	if got != "Hallo Welt" {
		t.Fatalf("expected translated text, got %q", got)
	}
}

func TestTranslationService_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer ts.Close()

	svc := NewTranslationService(&config.Config{
		LiteLLMBaseURL: ts.URL,
		LiteLLMAPIKey:  "test-key",
		LiteLLMModel:   "gpt-4o-mini",
	})

	_, err := svc.Translate(TranslationInput{Text: "Hello world", Source: "en", Target: "de"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
