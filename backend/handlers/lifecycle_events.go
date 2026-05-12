package handlers

import (
	"strconv"
	"time"

	"loklingo/backend/internal/observability"

	"github.com/gofiber/fiber/v2"
)

// LifecycleEventsHandler serves recent lifecycle events for debugging and incident timelines.
type LifecycleEventsHandler struct{}

func NewLifecycleEventsHandler() *LifecycleEventsHandler {
	return &LifecycleEventsHandler{}
}

// List handles GET /api/v1/metrics/lifecycle/events.
func (h *LifecycleEventsHandler) List(c *fiber.Ctx) error {
	correlationID := c.Query("correlation_id")
	jobID := c.Query("job_id")
	event := c.Query("event")

	limit := 100
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err == nil && parsed > 0 {
			limit = parsed
		}
	}

	events := observability.SnapshotLifecycleEvents(correlationID, jobID, event, limit)

	return c.JSON(fiber.Map{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"filters": fiber.Map{
			"correlation_id": correlationID,
			"job_id":         jobID,
			"event":          event,
			"limit":          limit,
		},
		"count":  len(events),
		"events": events,
	})
}
