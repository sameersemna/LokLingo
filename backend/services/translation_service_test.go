package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestTranslationService_RetriesTransientStatus(t *testing.T) {
	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"temporary outage"}`))
			return
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
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestTranslationService_ContextCancelled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "Late response"},
			}},
		})
	}))
	defer ts.Close()

	svc := NewTranslationService(&config.Config{
		LiteLLMBaseURL: ts.URL,
		LiteLLMAPIKey:  "test-key",
		LiteLLMModel:   "gpt-4o-mini",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := svc.Translate(TranslationInput{Text: "Hello world", Source: "en", Target: "de", Ctx: ctx})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected context deadline error, got %v", err)
	}
}

func TestTranslationService_ResponseTooLarge(t *testing.T) {
	oldLimit := translationMaxSuccessBody
	translationMaxSuccessBody = 64
	t.Cleanup(func() {
		translationMaxSuccessBody = oldLimit
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"` + strings.Repeat("a", 1024) + `"}}]}`))
	}))
	defer ts.Close()

	svc := NewTranslationService(&config.Config{
		LiteLLMBaseURL: ts.URL,
		LiteLLMAPIKey:  "test-key",
		LiteLLMModel:   "gpt-4o-mini",
	})

	_, err := svc.Translate(TranslationInput{Text: "Hello", Source: "en", Target: "de"})
	if err == nil || !strings.Contains(err.Error(), "response body exceeds max size") {
		t.Fatalf("expected bounded response size error, got %v", err)
	}
}

func TestTranslationService_EmptyChoiceContentRejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{"role": "assistant", "content": "   "},
			}},
		})
	}))
	defer ts.Close()

	svc := NewTranslationService(&config.Config{
		LiteLLMBaseURL: ts.URL,
		LiteLLMAPIKey:  "test-key",
		LiteLLMModel:   "gpt-4o-mini",
	})

	_, err := svc.Translate(TranslationInput{Text: "Hello", Source: "en", Target: "de"})
	if err == nil || !strings.Contains(err.Error(), "empty choice content") {
		t.Fatalf("expected empty content validation error, got %v", err)
	}
}

func TestTranslationService_CircuitBreakerOpens(t *testing.T) {
	oldAttempts := translationMaxAttempts
	oldBackoff := translationInitialBackoff
	oldMaxBackoff := translationMaxBackoff
	oldThreshold := translationCircuitBreakerThreshold
	oldOpenFor := translationCircuitBreakerOpenFor
	translationMaxAttempts = 1
	translationInitialBackoff = 5 * time.Millisecond
	translationMaxBackoff = 5 * time.Millisecond
	translationCircuitBreakerThreshold = 2
	translationCircuitBreakerOpenFor = 200 * time.Millisecond
	t.Cleanup(func() {
		translationMaxAttempts = oldAttempts
		translationInitialBackoff = oldBackoff
		translationMaxBackoff = oldMaxBackoff
		translationCircuitBreakerThreshold = oldThreshold
		translationCircuitBreakerOpenFor = oldOpenFor
	})

	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"busy"}`))
	}))
	defer ts.Close()

	svc := NewTranslationService(&config.Config{
		LiteLLMBaseURL: ts.URL,
		LiteLLMAPIKey:  "test-key",
		LiteLLMModel:   "gpt-4o-mini",
	})

	for i := 0; i < 2; i++ {
		_, _ = svc.Translate(TranslationInput{Text: "Hello", Source: "en", Target: "de"})
	}

	_, err := svc.Translate(TranslationInput{Text: "Hello", Source: "en", Target: "de"})
	if err == nil || !strings.Contains(err.Error(), "circuit breaker open") {
		t.Fatalf("expected circuit breaker open error, got %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected no upstream call when breaker is open, got %d requests", requestCount)
	}
}

func TestTranslationService_CancellationRaceStress(t *testing.T) {
	oldAttempts := translationMaxAttempts
	oldBackoff := translationInitialBackoff
	oldMaxBackoff := translationMaxBackoff
	translationMaxAttempts = 2
	translationInitialBackoff = 5 * time.Millisecond
	translationMaxBackoff = 10 * time.Millisecond
	t.Cleanup(func() {
		translationMaxAttempts = oldAttempts
		translationInitialBackoff = oldBackoff
		translationMaxBackoff = oldMaxBackoff
	})

	var mu sync.Mutex
	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"busy"}`))
	}))
	defer ts.Close()

	svc := NewTranslationService(&config.Config{
		LiteLLMBaseURL: ts.URL,
		LiteLLMAPIKey:  "test-key",
		LiteLLMModel:   "gpt-4o-mini",
	})

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			_, err := svc.Translate(TranslationInput{Text: fmt.Sprintf("text-%d", idx), Source: "en", Target: "de", Ctx: ctx})
			if err == nil {
				errs <- fmt.Errorf("expected cancellation error")
				return
			}
			if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "circuit breaker open") {
				errs <- fmt.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("stress assertion failed: %v", err)
	}
}
