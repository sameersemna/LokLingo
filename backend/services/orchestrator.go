package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
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

	mu             sync.Mutex
	providerHealth map[string]float64
}

// ProviderHealthSnapshot represents a provider health score used for dynamic
// ordering and failover decisions.
type ProviderHealthSnapshot struct {
	Provider string  `json:"provider"`
	Score    float64 `json:"score"`
}

// NewOrchestrator constructs an Orchestrator backed by the given registry.
func NewOrchestrator(registry *providers.Registry, cfg OrchestratorConfig) *Orchestrator {
	return &Orchestrator{
		registry:       registry,
		cfg:            cfg,
		providerHealth: make(map[string]float64),
	}
}

const (
	providerHealthDefaultScore    = 1.0
	providerHealthMaxScore        = 1.5
	providerHealthMinScore        = 0.05
	providerHealthSuccessTarget   = 1.2
	providerHealthTransientTarget = 0.35
	providerHealthPermanentTarget = 0.15
	providerHealthSmoothing       = 0.22
)

func clampProviderHealth(v float64) float64 {
	if v < providerHealthMinScore {
		return providerHealthMinScore
	}
	if v > providerHealthMaxScore {
		return providerHealthMaxScore
	}
	return v
}

func blendProviderHealth(current, target float64) float64 {
	if current <= 0 {
		current = providerHealthDefaultScore
	}
	next := (1-providerHealthSmoothing)*current + providerHealthSmoothing*target
	return clampProviderHealth(next)
}

func (o *Orchestrator) currentProviderHealth(name string) float64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	score, ok := o.providerHealth[name]
	if !ok || score <= 0 {
		return providerHealthDefaultScore
	}
	return score
}

func (o *Orchestrator) updateProviderHealth(name string, target float64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	current := o.providerHealth[name]
	o.providerHealth[name] = blendProviderHealth(current, target)
}

func (o *Orchestrator) orderedProvidersByHealth() []providers.TranslationProvider {
	ordered := o.registry.Ordered()
	if len(ordered) <= 1 {
		return ordered
	}

	type candidate struct {
		provider    providers.TranslationProvider
		baseIndex   int
		healthScore float64
	}
	items := make([]candidate, 0, len(ordered))
	for idx, p := range ordered {
		items = append(items, candidate{provider: p, baseIndex: idx, healthScore: o.currentProviderHealth(p.Name())})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].healthScore == items[j].healthScore {
			return items[i].baseIndex < items[j].baseIndex
		}
		return items[i].healthScore > items[j].healthScore
	})
	out := make([]providers.TranslationProvider, 0, len(items))
	for _, it := range items {
		out = append(out, it.provider)
	}
	return out
}

// SnapshotProviderHealth returns current provider scores, sorted by score
// descending (and registration order for ties).
func (o *Orchestrator) SnapshotProviderHealth() []ProviderHealthSnapshot {
	ordered := o.orderedProvidersByHealth()
	if len(ordered) == 0 {
		return nil
	}
	out := make([]ProviderHealthSnapshot, 0, len(ordered))
	for _, p := range ordered {
		out = append(out, ProviderHealthSnapshot{
			Provider: p.Name(),
			Score:    o.currentProviderHealth(p.Name()),
		})
	}
	return out
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

	providerList := o.orderedProvidersByHealth()
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
			o.updateProviderHealth(p.Name(), providerHealthSuccessTarget)
			if pi > 0 {
				observability.IncProviderFailover(p.Name())
				observability.EmitLifecycleEventFromContext(ctx, "provider_failover", observability.LifecycleEvent{Provider: p.Name(), RetryCount: int64(retries)})
				slog.Info("provider_failover_success",
					"provider", p.Name(),
					"failed_provider", providerList[pi-1].Name(),
					"retry_count", retries,
				)
			}
			return result.Text, nil
		}

		lastErr = err
		if isOrchestratorTransientError(err) {
			o.updateProviderHealth(p.Name(), providerHealthTransientTarget)
		} else {
			o.updateProviderHealth(p.Name(), providerHealthPermanentTarget)
		}
		observability.IncProviderFailure(p.Name())
		timeoutReason := classifyTimeoutReason(err)
		if timeoutReason != "" {
			observability.RecordProviderTimeout(p.Name(), timeoutReason)
			observability.IncTimeoutsTotal()
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
		observability.IncRetriesTotal()
		timeoutReason := classifyTimeoutReason(err)
		if timeoutReason != "" {
			observability.RecordProviderTimeout(p.Name(), timeoutReason)
			observability.IncTimeoutsTotal()
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
