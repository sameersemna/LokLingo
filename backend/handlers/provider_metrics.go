package handlers

import (
	"loklingo/backend/internal/observability"

	"github.com/gofiber/fiber/v2"
)

// ProviderMetricsHandler serves translation-provider reliability metrics.
type ProviderMetricsHandler struct{}

func NewProviderMetricsHandler() *ProviderMetricsHandler {
	return &ProviderMetricsHandler{}
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
			"provider":       p.Provider,
			"success_rate":   successRate,
			"retry_rate":     retryRate,
			"timeout_rate":   timeoutRate,
			"failover_rate":  failoverRate,
			"avg_latency_ms": p.AvgLatencyMS,
			"latency_buckets": p.LatencyBuckets,
		})
	}

	return c.JSON(fiber.Map{
		"providers": providers,
		"health":    health,
		"timeouts": fiber.Map{
			"by_reason": observability.SnapshotTimeoutReasonMetrics(),
		},
		"render_failures": observability.SnapshotRenderFailureMetrics(),
		"checkpoints":     observability.SnapshotChunkCheckpointMetrics(),
	})
}
