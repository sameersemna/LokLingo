package providers

// Package providers defines the TranslationProvider interface and its
// implementations. Each provider makes a single translation attempt;
// retry logic and cross-provider failover live in the Orchestrator.

import "context"

// TranslateRequest carries the text and language pair for one translation call.
type TranslateRequest struct {
	Text   string
	Source string
	Target string
}

// TranslateResponse carries the translated text returned by a provider.
type TranslateResponse struct {
	Text      string
	LatencyMS int64
}

// TranslationProvider is the low-level interface every translation backend must
// implement. Each call represents a single attempt with no internal retry.
type TranslationProvider interface {
	// Name returns a stable, lowercase identifier used in logs and metrics
	// (e.g. "litellm", "openai", "ollama").
	Name() string

	// Translate performs one translation attempt. ctx carries the deadline and
	// cancellation signal. Implementations must respect ctx.Done().
	Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error)
}
