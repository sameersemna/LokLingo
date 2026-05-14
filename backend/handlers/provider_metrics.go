package handlers

import (
	"loklingo/backend/internal/observability"
	"loklingo/backend/services"
	"time"

	"github.com/gofiber/fiber/v2"
)

type providerHealthSnapshotter interface {
	SnapshotProviderHealth() []services.ProviderHealthSnapshot
}

// ProviderMetricsHandler serves translation-provider reliability metrics.
type ProviderMetricsHandler struct {
	healthSnapshotter providerHealthSnapshotter
}

func NewProviderMetricsHandler(healthSnapshotter ...providerHealthSnapshotter) *ProviderMetricsHandler {
	h := &ProviderMetricsHandler{}
	if len(healthSnapshotter) > 0 {
		h.healthSnapshotter = healthSnapshotter[0]
	}
	return h
}

// Summary handles GET /api/v1/metrics/providers.
func (h *ProviderMetricsHandler) Summary(c *fiber.Ctx) error {
	providers := observability.SnapshotProviderMetrics()
	health := make([]fiber.Map, 0, len(providers))
	for _, p := range providers {
		total := p.SuccessTotal + p.FailureTotal
		successRate := 0.0
		retryRate := 0.0
		timeoutRate := 0.0
		failoverRate := 0.0
		if total > 0 {
			successRate = float64(p.SuccessTotal) / float64(total)
			retryRate = float64(p.RetryTotal) / float64(total)
			timeoutRate = float64(p.TimeoutTotal) / float64(total)
			failoverRate = float64(p.FailoverTotal) / float64(total)
		}
		health = append(health, fiber.Map{
			"provider":        p.Provider,
			"success_rate":    successRate,
			"retry_rate":      retryRate,
			"timeout_rate":    timeoutRate,
			"failover_rate":   failoverRate,
			"avg_latency_ms":  p.AvgLatencyMS,
			"latency_buckets": p.LatencyBuckets,
		})
	}

	providerPolicy := []services.ProviderHealthSnapshot{}
	if h.healthSnapshotter != nil {
		providerPolicy = h.healthSnapshotter.SnapshotProviderHealth()
	}

	return c.JSON(fiber.Map{
		"providers":       providers,
		"health":          health,
		"provider_policy": providerPolicy,
		"timeouts": fiber.Map{
			"by_reason": observability.SnapshotTimeoutReasonMetrics(),
		},
		"render_failures": observability.SnapshotRenderFailureMetrics(),
		"checkpoints":     observability.SnapshotChunkCheckpointMetrics(),
		"degraded_mode":   observability.SnapshotDegradedModeMetrics(),
	})
}

// Health handles GET /api/v1/metrics/providers/health.
func (h *ProviderMetricsHandler) Health(c *fiber.Ctx) error {
	providerPolicy := []services.ProviderHealthSnapshot{}
	if h.healthSnapshotter != nil {
		providerPolicy = h.healthSnapshotter.SnapshotProviderHealth()
	}

	return c.JSON(fiber.Map{
		"captured_at":       time.Now().UTC().Format(time.RFC3339),
		"provider_policy":   providerPolicy,
		"degraded_mode":     observability.SnapshotDegradedModeMetrics(),
		"timeout_by_reason": observability.SnapshotTimeoutReasonMetrics(),
	})
}
