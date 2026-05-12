package handlers

import (
	"context"
	"time"

	"loklingo/backend/internal/observability"
	"loklingo/backend/jobs"

	"github.com/gofiber/fiber/v2"
)

type queueMetricsProvider interface {
	QueueStats(ctx context.Context) (jobs.QueueStats, error)
}

// ReliabilityMetricsHandler serves end-to-end pipeline and queue reliability metrics.
type ReliabilityMetricsHandler struct {
	store jobs.Store
}

func NewReliabilityMetricsHandler(store jobs.Store) *ReliabilityMetricsHandler {
	return &ReliabilityMetricsHandler{store: store}
}

// Summary handles GET /api/v1/metrics/reliability.
func (h *ReliabilityMetricsHandler) Summary(c *fiber.Ctx) error {
	queue := jobs.QueueStats{}
	if provider, ok := h.store.(queueMetricsProvider); ok {
		stats, err := provider.QueueStats(c.Context())
		if err == nil {
			queue = stats
		}
	}

	pipeline := observability.SnapshotPipelineMetrics()
	alerts := []fiber.Map{
		{"metric": "queue_depth", "warn": 500, "critical": 1000, "note": "Sustained depth indicates worker under-capacity or provider latency."},
		{"metric": "stuck_jobs", "warn": 1, "critical": 5, "note": "Jobs in inflight >10m need triage and potential requeue tuning."},
		{"metric": "retry_backlog", "warn": 50, "critical": 200, "note": "Growing retry backlog can predict rising error rates."},
		{"metric": "dead_letter_volume", "warn": 1, "critical": 10, "note": "Dead-letter growth should trigger incident review."},
	}

	return c.JSON(fiber.Map{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"pipeline":  pipeline,
		"queue":     queue,
		"alerts":    alerts,
	})
}
