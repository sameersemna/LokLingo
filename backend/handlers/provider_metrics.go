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
	return c.JSON(fiber.Map{
		"providers": observability.SnapshotProviderMetrics(),
		"timeouts": fiber.Map{
			"by_reason": observability.SnapshotTimeoutReasonMetrics(),
		},
		"render_failures": observability.SnapshotRenderFailureMetrics(),
		"checkpoints":     observability.SnapshotChunkCheckpointMetrics(),
	})
}
