package handlers

import (
	"fmt"
	"strings"

	"loklingo/backend/internal/observability"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OCRMetricsHandler serves lightweight OCR analytics from Postgres.
type OCRMetricsHandler struct {
	pool *pgxpool.Pool
}

type reliabilityEventCount struct {
	Integration string `json:"integration"`
	EventName   string `json:"event_name"`
	Reason      string `json:"reason"`
	Count       int64  `json:"count"`
}

func NewOCRMetricsHandler(pool *pgxpool.Pool) *OCRMetricsHandler {
	return &OCRMetricsHandler{pool: pool}
}

// parseWindow maps the ?window= query param to a Postgres interval string.
// Accepted values: 1h, 6h, 24h, 7d, 30d. Defaults to "24h" → "24 hours".
func parseWindow(raw string) (label string, interval string, ok bool) {
	switch raw {
	case "1h":
		return "last_1h", "1 hours", true
	case "6h":
		return "last_6h", "6 hours", true
	case "", "24h":
		return "last_24h", "24 hours", true
	case "7d":
		return "last_7d", "7 days", true
	case "30d":
		return "last_30d", "30 days", true
	}
	return "", "", false
}

func (h *OCRMetricsHandler) Summary(c *fiber.Ctx) error {
	if h.pool == nil {
		return errResponse(c, fiber.StatusServiceUnavailable, "ocr metrics unavailable")
	}

	windowLabel, pgInterval, ok := parseWindow(c.Query("window", "24h"))
	if !ok {
		return errResponse(c, fiber.StatusBadRequest, "invalid window; accepted: 1h, 6h, 24h, 7d, 30d")
	}

	// Summary row: total events + latency percentiles.
	summaryQuery := fmt.Sprintf(`
		SELECT
			COUNT(*)::bigint,
			COALESCE(percentile_cont(0.50) WITHIN GROUP (ORDER BY total_ms), 0)::float8,
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY total_ms), 0)::float8,
			COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY total_ms), 0)::float8
		FROM ocr_events
		WHERE recorded_at >= now() - interval '%s'`, pgInterval)

	var eventsTotal int64
	var p50MS, p95MS, p99MS float64
	if err := h.pool.QueryRow(c.Context(), summaryQuery).Scan(&eventsTotal, &p50MS, &p95MS, &p99MS); err != nil {
		return errResponse(c, fiber.StatusInternalServerError, "failed to query ocr metrics")
	}

	// Per-outcome breakdown.
	outcomeQuery := fmt.Sprintf(`
		SELECT outcome, COUNT(*)::bigint
		FROM ocr_events
		WHERE recorded_at >= now() - interval '%s'
		GROUP BY outcome`, pgInterval)

	rows, err := h.pool.Query(c.Context(), outcomeQuery)
	if err != nil {
		return errResponse(c, fiber.StatusInternalServerError, "failed to query ocr outcome breakdown")
	}
	defer rows.Close()

	outcomes := map[string]int64{}
	var succeededTotal int64
	for rows.Next() {
		var outcome string
		var count int64
		if err := rows.Scan(&outcome, &count); err != nil {
			continue
		}
		outcomes[outcome] = count
		if outcome == "succeeded" {
			succeededTotal = count
		}
	}

	successRate := float64(0)
	if eventsTotal > 0 {
		successRate = (float64(succeededTotal) * 100) / float64(eventsTotal)
	}

	reliabilityCounts, err := h.queryReliabilityCounts(c, pgInterval)
	if err != nil {
		return errResponse(c, fiber.StatusInternalServerError, "failed to query reliability metrics")
	}

	return c.JSON(fiber.Map{
		"window": fiber.Map{
			"name": windowLabel,
		},
		"events_total":     eventsTotal,
		"succeeded_total":  succeededTotal,
		"success_rate_pct": successRate,
		"outcomes":         outcomes,
		"reliability":      observability.SnapshotReliability(),
		"reliability_windowed": fiber.Map{
			"events": reliabilityCounts,
		},
		"latency": fiber.Map{
			"p50_total_ms": p50MS,
			"p95_total_ms": p95MS,
			"p99_total_ms": p99MS,
		},
	})
}

func (h *OCRMetricsHandler) queryReliabilityCounts(c *fiber.Ctx, pgInterval string) ([]reliabilityEventCount, error) {
	q := fmt.Sprintf(`
		SELECT integration, event_name, reason, COUNT(*)::bigint
		FROM reliability_events
		WHERE recorded_at >= now() - interval '%s'
		GROUP BY integration, event_name, reason
		ORDER BY integration, event_name, reason`, pgInterval)

	rows, err := h.pool.Query(c.Context(), q)
	if err != nil {
		// Allow mixed-version deployments where migrations have not yet been applied.
		if strings.Contains(strings.ToLower(err.Error()), "reliability_events") {
			return []reliabilityEventCount{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	counts := make([]reliabilityEventCount, 0, 8)
	for rows.Next() {
		var row reliabilityEventCount
		if err := rows.Scan(&row.Integration, &row.EventName, &row.Reason, &row.Count); err != nil {
			continue
		}
		counts = append(counts, row)
	}
	return counts, rows.Err()
}
