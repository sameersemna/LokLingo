package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"loklingo/backend/config"
	internalservices "loklingo/backend/internal/services"
	"loklingo/backend/jobs"
)

type healthComponent struct {
	Status    string         `json:"status"`
	LatencyMS int64          `json:"latency_ms,omitempty"`
	Error     string         `json:"error,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

type healthDashboardPayload struct {
	Service       string                     `json:"service"`
	Status        string                     `json:"status"`
	Timestamp     time.Time                  `json:"timestamp"`
	UptimeSeconds int64                      `json:"uptime_seconds,omitempty"`
	RequestID     string                     `json:"request_id,omitempty"`
	Dependencies  map[string]healthComponent `json:"dependencies"`
}

// NewHealthDashboardHandler returns a centralized health endpoint for LAN ops.
func NewHealthDashboardHandler(cfg *config.Config, store jobs.Store, pgPool *pgxpool.Pool, startedAt time.Time) fiber.Handler {
	client := &http.Client{Timeout: 5 * time.Second}
	return func(c *fiber.Ctx) error {
		payload := collectHealthDashboard(c.Context(), cfg, store, pgPool, client, startedAt)
		statusCode := fiber.StatusOK
		if payload.Status != "ok" {
			statusCode = fiber.StatusServiceUnavailable
		}
		if id, ok := c.Locals("requestID").(string); ok && id != "" {
			payload.RequestID = id
		}
		return c.Status(statusCode).JSON(payload)
	}
}

// CheckStartupDependencies validates critical runtime dependencies before the
// API starts serving requests. It is intended to fail fast so Docker restart
// policy can retry the container when LAN dependencies are not ready.
func CheckStartupDependencies(ctx context.Context, cfg *config.Config, store jobs.Store, pgPool *pgxpool.Pool) error {
	client := &http.Client{Timeout: 5 * time.Second}
	var lastErr error
	for attempt := 1; attempt <= 10; attempt++ {
		payload := collectHealthDashboard(ctx, cfg, store, pgPool, client, time.Time{})

		deps := make([]string, 0, len(payload.Dependencies))
		for name, dep := range payload.Dependencies {
			switch dep.Status {
			case "ok", "disabled", "warning":
				continue
			default:
				if dep.Error != "" {
					deps = append(deps, fmt.Sprintf("%s: %s", name, dep.Error))
				} else {
					deps = append(deps, fmt.Sprintf("%s: %s", name, dep.Status))
				}
			}
		}
		if len(deps) == 0 {
			return nil
		}
		lastErr = fmt.Errorf("startup dependency checks failed: %s", strings.Join(deps, "; "))
		if attempt < 10 {
			select {
			case <-ctx.Done():
				return lastErr
			case <-time.After(3 * time.Second):
			}
		}
	}
	return lastErr
}

func collectHealthDashboard(ctx context.Context, cfg *config.Config, store jobs.Store, pgPool *pgxpool.Pool, client *http.Client, startedAt time.Time) healthDashboardPayload {
	deps := map[string]healthComponent{
		"backend": {Status: "ok"},
		"queues":  probeQueues(ctx, store),
		"ocr":     probeOCR(ctx, cfg, client),
		"ollama":  probeOllama(ctx, cfg, client),
		"db":      probeDB(ctx, cfg, pgPool),
	}

	status := "ok"
	for _, dep := range deps {
		switch dep.Status {
		case "down", "error":
			status = "degraded"
		case "warning":
			if status == "ok" {
				status = "degraded"
			}
		}
	}

	payload := healthDashboardPayload{
		Service:      "loklingo-backend",
		Status:       status,
		Timestamp:    time.Now().UTC(),
		Dependencies: deps,
	}
	if !startedAt.IsZero() {
		payload.UptimeSeconds = int64(time.Since(startedAt).Seconds())
	}
	return payload
}

func probeQueues(ctx context.Context, store jobs.Store) healthComponent {
	provider, ok := store.(interface {
		QueueStats(context.Context) (jobs.QueueStats, error)
	})
	if !ok {
		return healthComponent{Status: "disabled"}
	}

	start := time.Now()
	stats, err := provider.QueueStats(ctx)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return healthComponent{Status: "error", LatencyMS: latency, Error: err.Error()}
	}

	status := "ok"
	if stats.StuckJobs > 0 || stats.DeadLetterCount > 0 {
		status = "warning"
	}
	return healthComponent{
		Status:    status,
		LatencyMS: latency,
		Details: map[string]any{
			"queue_depth":       stats.QueueDepth,
			"inflight_depth":    stats.InflightDepth,
			"stuck_jobs":        stats.StuckJobs,
			"retry_backlog":     stats.RetryBacklog,
			"dead_letter_count": stats.DeadLetterCount,
		},
	}
}

func probeHTTP(ctx context.Context, client *http.Client, url string, headers map[string]string) healthComponent {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return healthComponent{Status: "error", Error: err.Error()}
	}
	for key, value := range headers {
		if value != "" {
			req.Header.Set(key, value)
		}
	}
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return healthComponent{Status: "error", LatencyMS: latency, Error: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return healthComponent{Status: "down", LatencyMS: latency, Error: fmt.Sprintf("unexpected status %d", resp.StatusCode)}
	}
	return healthComponent{Status: "ok", LatencyMS: latency}
}

func probeOCR(ctx context.Context, cfg *config.Config, client *http.Client) healthComponent {
	baseURL := internalservices.NormalizeOCRServiceURL(cfg.OCRServiceURL)
	if baseURL == "" {
		return healthComponent{Status: "down", Error: "OCR service URL is not configured"}
	}
	return probeHTTP(ctx, client, strings.TrimRight(baseURL, "/")+"/health", nil)
}

func probeOllama(ctx context.Context, cfg *config.Config, client *http.Client) healthComponent {
	baseURL := strings.TrimSpace(cfg.OllamaBaseURL)
	model := strings.TrimSpace(cfg.OllamaModel)
	if baseURL == "" || model == "" {
		return healthComponent{Status: "disabled"}
	}
	return probeHTTP(ctx, client, strings.TrimRight(baseURL, "/")+"/api/tags", nil)
}

func probeDB(ctx context.Context, cfg *config.Config, pgPool *pgxpool.Pool) healthComponent {
	if strings.TrimSpace(cfg.PostgresDSN) == "" || pgPool == nil {
		return healthComponent{Status: "disabled"}
	}
	start := time.Now()
	if err := pgPool.Ping(ctx); err != nil {
		return healthComponent{Status: "error", LatencyMS: time.Since(start).Milliseconds(), Error: err.Error()}
	}
	return healthComponent{Status: "ok", LatencyMS: time.Since(start).Milliseconds()}
}
