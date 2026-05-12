package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"strings"
	"time"

	"loklingo/backend/internal/observability"
	"loklingo/backend/services/providers"
)

// OrchestratorConfig tunes per-provider retry and cross-provider failover.
type OrchestratorConfig struct {
	// MaxRetriesPerProvider is the number of additional attempts after the first
	// failure on a single provider before moving to the next one.
	MaxRetriesPerProvider int
	// InitialBackoff is the delay before the first retry.
	InitialBackoff time.Duration
	// MaxBackoff caps the exponentially growing delay.
	MaxBackoff time.Duration
	// JitterFraction adds a random fraction of the current backoff to reduce
	// thundering-herd on a shared backend. Set to 0 to disable.
	JitterFraction float64
}

// DefaultOrchestratorConfig is the production-ready default tuning.
var DefaultOrchestratorConfig = OrchestratorConfig{
	MaxRetriesPerProvider: 3,
	InitialBackoff:        200 * time.Millisecond,
	MaxBackoff:            2 * time.Second,
	JitterFraction:        0.3,
}

// Orchestrator implements TranslationService with multi-provider retry and
// transparent failover. It tries each provider in registry order, applying
// exponential back-off with jitter on transient errors before moving to the
// next provider. Permanent errors (e.g. 400 Bad Request) skip remaining retries
// and immediately fail over.
//
// Stage notifications are delivered via a context value set by
// WithStageNotifier so callers can surface "retrying" / "fallback_provider"
// states to end-users without changing the TranslationService interface.
type Orchestrator struct {
	registry *providers.Registry
	cfg      OrchestratorConfig
}

// NewOrchestrator constructs an Orchestrator backed by the given registry.
func NewOrchestrator(registry *providers.Registry, cfg OrchestratorConfig) *Orchestrator {
	return &Orchestrator{registry: registry, cfg: cfg}
}

// Translate implements TranslationService. It attempts providers in order,
// retrying transient errors with exponential back-off + jitter on each
// provider before failing over to the next. All providers exhausted → error.
func (o *Orchestrator) Translate(input TranslationInput) (string, error) {
	ctx := input.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	req := providers.TranslateRequest{
		Text:   input.Text,
		Source: input.Source,
		Target: input.Target,
	}

	providerList := o.registry.Ordered()
	if len(providerList) == 0 {
		return "", fmt.Errorf("no translation providers configured")
	}

	var lastErr error
	for pi, p := range providerList {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		result, retries, err := o.tryProviderWithRetry(ctx, p, req, pi, len(providerList))
		if err == nil {
			if pi > 0 {
				observability.IncProviderFailover(p.Name())
				slog.Info("provider_failover_success",
					"provider", p.Name(),
					"failed_provider", providerList[pi-1].Name(),
					"retry_count", retries,
				)
			}
			return result.Text, nil
		}

		lastErr = err
		observability.IncProviderFailure(p.Name())
		timeoutReason := classifyTimeoutReason(err)
		if timeoutReason != "" {
			observability.RecordProviderTimeout(p.Name(), timeoutReason)
		}
		if pi < len(providerList)-1 {
			failoverReason := "provider_error"
			if timeoutReason != "" {
				failoverReason = "timeout_" + timeoutReason
			}
			notify(ctx, "fallback_provider",
				fmt.Sprintf("Switching to backup translation engine (%s)", providerList[pi+1].Name()),
				0,
			)
			slog.Warn("provider_failed_trying_next",
				"provider", p.Name(),
				"next_provider", providerList[pi+1].Name(),
				"retry_count", retries,
				"timeout_reason", timeoutReason,
				"failover_reason", failoverReason,
				"err", err,
			)
		}
	}

	return "", fmt.Errorf("all %d provider(s) exhausted: %w", len(providerList), lastErr)
}

// tryProviderWithRetry makes up to (1 + MaxRetriesPerProvider) calls to p,
// sleeping between attempts with exponential back-off + jitter on transient
// errors. Returns the response, total retry count, and any unrecoverable error.
func (o *Orchestrator) tryProviderWithRetry(
	ctx context.Context,
	p providers.TranslationProvider,
	req providers.TranslateRequest,
	providerIdx, providerTotal int,
) (providers.TranslateResponse, int, error) {
	backoff := o.cfg.InitialBackoff
	var lastErr error

	for attempt := 0; attempt <= o.cfg.MaxRetriesPerProvider; attempt++ {
		if ctx.Err() != nil {
			return providers.TranslateResponse{}, attempt, ctx.Err()
		}

		start := time.Now()
		resp, err := p.Translate(ctx, req)
		latencyMS := time.Since(start).Milliseconds()

		if err == nil {
			observability.IncProviderSuccess(p.Name())
			observability.RecordProviderLatency(p.Name(), latencyMS)
			slog.Info("provider_translate_ok",
				"provider", p.Name(),
				"latency_ms", latencyMS,
				"attempt", attempt,
			)
			return resp, attempt, nil
		}

		observability.RecordProviderRetry(p.Name())
		timeoutReason := classifyTimeoutReason(err)
		if timeoutReason != "" {
			observability.RecordProviderTimeout(p.Name(), timeoutReason)
		}

		if !isOrchestratorTransientError(err) {
			// Permanent error — don't retry this provider.
			slog.Warn("provider_permanent_error",
				"provider", p.Name(),
				"attempt", attempt,
				"timeout_reason", timeoutReason,
				"err", err,
			)
			return providers.TranslateResponse{}, attempt, err
		}

		lastErr = err

		if attempt == o.cfg.MaxRetriesPerProvider {
			break
		}

		// Compute back-off with optional jitter.
		sleep := backoff
		if o.cfg.JitterFraction > 0 {
			// #nosec G404 — non-cryptographic jitter is intentional.
			sleep += time.Duration(float64(backoff) * o.cfg.JitterFraction * rand.Float64())
		}

		// Emit a "retrying" stage notification for real-time UI feedback.
		retryMsg := "Retrying translation…"
		if providerTotal > 1 {
			retryMsg = fmt.Sprintf("Retrying translation on %s (attempt %d)…", p.Name(), attempt+2)
		}
		notify(ctx, "retrying", retryMsg, 0)

		slog.Warn("provider_transient_backoff",
			"provider", p.Name(),
			"attempt", attempt+1,
			"max_attempts", o.cfg.MaxRetriesPerProvider+1,
			"backoff_ms", sleep.Milliseconds(),
			"timeout_reason", timeoutReason,
			"err", err,
		)

		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return providers.TranslateResponse{}, attempt, ctx.Err()
		case <-timer.C:
		}

		backoff *= 2
		if backoff > o.cfg.MaxBackoff {
			backoff = o.cfg.MaxBackoff
		}
	}

	observability.IncProviderExhausted(p.Name())
	return providers.TranslateResponse{}, o.cfg.MaxRetriesPerProvider,
		fmt.Errorf("provider %s retry exhausted: %w", p.Name(), lastErr)
}

func classifyTimeoutReason(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "context_deadline"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "network_timeout"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "gateway timeout") || strings.Contains(msg, "status 504"):
		return "gateway_timeout"
	case strings.Contains(msg, "client.timeout exceeded") || strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "timed out"):
		return "client_timeout"
	case strings.Contains(msg, "context deadline exceeded"):
		return "context_deadline"
	default:
		return ""
	}
}

// isOrchestratorTransientError returns true when err is worth retrying on the
// same provider (rate-limit, gateway, network timeout, etc.). Permanent errors
// like 400 Bad Request cause an immediate failover without retry.
func isOrchestratorTransientError(err error) bool {
	if err == nil {
		return false
	}
	var pe *providers.ProviderError
	if errors.As(err, &pe) {
		return pe.IsTransient()
	}
	// Go net/http timeout errors.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	// String-based classification for wrapped errors.
	msg := strings.ToLower(err.Error())
	transientPhrases := []string{
		"context deadline exceeded",
		"client.timeout exceeded",
		"connection reset",
		"connection refused",
		"i/o timeout",
		"timed out",
		"timeout",
		"rate limit",
		"rate_limit",
		"temporarily unavailable",
		"service unavailable",
		"bad gateway",
		"gateway timeout",
	}
	for _, phrase := range transientPhrases {
		if strings.Contains(msg, phrase) {
			return true
		}
	}
	return false
}
