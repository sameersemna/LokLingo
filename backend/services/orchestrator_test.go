package services

import (
	"context"
	"errors"
	"fmt"
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

func TestOrchestrator_ReordersProvidersByHealthAfterFailure(t *testing.T) {
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

	first, err := orchestrator.Translate(TranslationInput{
		Ctx:    context.Background(),
		Text:   "hello",
		Source: "en",
		Target: "es",
	})
	if err != nil {
		t.Fatalf("first Translate returned error: %v", err)
	}
	if first != "hola" {
		t.Fatalf("expected translated text %q, got %q", "hola", first)
	}
	if primary.CallCount() != 1 {
		t.Fatalf("expected first call to hit primary exactly once, got %d", primary.CallCount())
	}
	if secondary.CallCount() != 1 {
		t.Fatalf("expected first call to hit secondary once via failover, got %d", secondary.CallCount())
	}

	second, err := orchestrator.Translate(TranslationInput{
		Ctx:    context.Background(),
		Text:   "hello again",
		Source: "en",
		Target: "es",
	})
	if err != nil {
		t.Fatalf("second Translate returned error: %v", err)
	}
	if second != "hola" {
		t.Fatalf("expected translated text %q, got %q", "hola", second)
	}

	if primary.CallCount() != 1 {
		t.Fatalf("expected health ordering to avoid degraded primary on second call; primary calls=%d", primary.CallCount())
	}
	if secondary.CallCount() != 2 {
		t.Fatalf("expected healthy secondary to be preferred on second call; secondary calls=%d", secondary.CallCount())
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

func TestClassifyTimeoutReason_ContextDeadline(t *testing.T) {
	reason := classifyTimeoutReason(context.DeadlineExceeded)
	if reason != "context_deadline" {
		t.Fatalf("expected context_deadline, got %q", reason)
	}
}

func TestClassifyTimeoutReason_GatewayTimeoutPhrase(t *testing.T) {
	reason := classifyTimeoutReason(fmt.Errorf("provider status 504: gateway timeout"))
	if reason != "gateway_timeout" {
		t.Fatalf("expected gateway_timeout, got %q", reason)
	}
}

func TestClassifyTimeoutReason_ClientTimeoutPhrase(t *testing.T) {
	reason := classifyTimeoutReason(fmt.Errorf("net/http: request canceled (Client.Timeout exceeded while awaiting headers): i/o timeout"))
	if reason != "client_timeout" {
		t.Fatalf("expected client_timeout, got %q", reason)
	}
}

func TestClassifyTimeoutReason_NonTimeout(t *testing.T) {
	reason := classifyTimeoutReason(fmt.Errorf("provider returned 400 bad request"))
	if reason != "" {
		t.Fatalf("expected empty timeout reason, got %q", reason)
	}
}
