package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"loklingo/backend/services/providers"
)

func TestOrchestratorRetriesTransientProviderThenSucceeds(t *testing.T) {
	registry := providers.NewRegistry()
	provider := providers.NewTransientThenSuccessMock(
		"litellm",
		2,
		&providers.ProviderError{Provider: "litellm", StatusCode: 503, Body: "busy"},
		"bonjour",
	)
	registry.Register(provider)

	orchestrator := NewOrchestrator(registry, OrchestratorConfig{
		MaxRetriesPerProvider: 2,
		InitialBackoff:        time.Millisecond,
		MaxBackoff:            2 * time.Millisecond,
		JitterFraction:        0,
	})

	var stages []string
	ctx := WithStageNotifier(context.Background(), func(stage, _ string, _ float64) {
		stages = append(stages, stage)
	})

	translated, err := orchestrator.Translate(TranslationInput{
		Ctx:    ctx,
		Text:   "hello",
		Source: "en",
		Target: "fr",
	})
	if err != nil {
		t.Fatalf("Translate returned error: %v", err)
	}
	if translated != "bonjour" {
		t.Fatalf("expected translated text %q, got %q", "bonjour", translated)
	}
	if provider.CallCount() != 3 {
		t.Fatalf("expected 3 provider calls, got %d", provider.CallCount())
	}

	retryCount := 0
	for _, stage := range stages {
		if stage == "retrying" {
			retryCount++
		}
	}
	if retryCount != 2 {
		t.Fatalf("expected 2 retry notifications, got %d (%v)", retryCount, stages)
	}
}

func TestOrchestratorFailsOverToNextProvider(t *testing.T) {
	registry := providers.NewRegistry()
	primary := providers.NewFailingMockProvider("litellm", &providers.ProviderError{Provider: "litellm", StatusCode: 503, Body: "busy"})
	secondary := providers.NewMockProvider("openai", "hola")
	registry.Register(primary)
	registry.Register(secondary)

	orchestrator := NewOrchestrator(registry, OrchestratorConfig{
		MaxRetriesPerProvider: 0,
		InitialBackoff:        time.Millisecond,
		MaxBackoff:            time.Millisecond,
		JitterFraction:        0,
	})

	var stages []string
	ctx := WithStageNotifier(context.Background(), func(stage, _ string, _ float64) {
		stages = append(stages, stage)
	})

	translated, err := orchestrator.Translate(TranslationInput{
		Ctx:    ctx,
		Text:   "hello",
		Source: "en",
		Target: "es",
	})
	if err != nil {
		t.Fatalf("Translate returned error: %v", err)
	}
	if translated != "hola" {
		t.Fatalf("expected translated text %q, got %q", "hola", translated)
	}
	if primary.CallCount() != 1 {
		t.Fatalf("expected primary provider to be called once, got %d", primary.CallCount())
	}
	if secondary.CallCount() != 1 {
		t.Fatalf("expected secondary provider to be called once, got %d", secondary.CallCount())
	}

	foundFallback := false
	for _, stage := range stages {
		if stage == "fallback_provider" {
			foundFallback = true
			break
		}
	}
	if !foundFallback {
		t.Fatalf("expected fallback_provider notification, got %v", stages)
	}
}

func TestOrchestratorHonorsContextDeadline(t *testing.T) {
	registry := providers.NewRegistry()
	provider := providers.NewMockProvider("litellm", "bonjour")
	provider.Latencies = []time.Duration{50 * time.Millisecond}
	registry.Register(provider)

	orchestrator := NewOrchestrator(registry, OrchestratorConfig{
		MaxRetriesPerProvider: 0,
		InitialBackoff:        time.Millisecond,
		MaxBackoff:            time.Millisecond,
		JitterFraction:        0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := orchestrator.Translate(TranslationInput{
		Ctx:    ctx,
		Text:   "hello",
		Source: "en",
		Target: "fr",
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
}

func TestOrchestratorSupportsConcurrentCalls(t *testing.T) {
	registry := providers.NewRegistry()
	provider := providers.NewMockProvider("litellm", "ok")
	registry.Register(provider)

	orchestrator := NewOrchestrator(registry, OrchestratorConfig{
		MaxRetriesPerProvider: 0,
		InitialBackoff:        time.Millisecond,
		MaxBackoff:            time.Millisecond,
		JitterFraction:        0,
	})

	const workers = 8
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			translated, err := orchestrator.Translate(TranslationInput{
				Ctx:    context.Background(),
				Text:   "hello",
				Source: "en",
				Target: "de",
			})
			if err != nil {
				errCh <- err
				return
			}
			if translated != "ok" {
				errCh <- errors.New("unexpected translated text")
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent translate failed: %v", err)
		}
	}
	if provider.CallCount() != workers {
		t.Fatalf("expected %d provider calls, got %d", workers, provider.CallCount())
	}
}
