package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"

	"loklingo/backend/config"
)

type readinessCheck = func(context.Context) error

type ReadinessHandler struct {
	checks map[string]readinessCheck
}

type readinessDependency struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func NewReadinessHandler(cfg *config.Config) *ReadinessHandler {
	client := &http.Client{Timeout: 5 * time.Second}

	return NewReadinessHandlerWithChecks(map[string]readinessCheck{
		"config": func(ctx context.Context) error {
			return cfg.Validate()
		},
		"redis": func(ctx context.Context) error {
			opts, err := redis.ParseURL(cfg.RedisURL)
			if err != nil {
				return fmt.Errorf("parse redis url: %w", err)
			}
			rdb := redis.NewClient(opts)
			defer rdb.Close()
			if err := rdb.Ping(ctx).Err(); err != nil {
				return fmt.Errorf("ping redis: %w", err)
			}
			return nil
		},
		"ocr": func(ctx context.Context) error {
			return checkHTTPDependency(ctx, client, strings.TrimRight(cfg.OCRServiceURL, "/")+"/health", "")
		},
		"litellm": func(ctx context.Context) error {
			return checkHTTPDependency(ctx, client, strings.TrimRight(cfg.LiteLLMBaseURL, "/")+"/models", cfg.LiteLLMAPIKey)
		},
	})
}

func NewReadinessHandlerWithChecks(checks map[string]readinessCheck) *ReadinessHandler {
	return &ReadinessHandler{checks: checks}
}

func (h *ReadinessHandler) Ready(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	dependencies := make(map[string]readinessDependency, len(h.checks))
	statusCode := fiber.StatusOK
	status := "ok"

	for name, check := range h.checks {
		if err := check(ctx); err != nil {
			dependencies[name] = readinessDependency{Status: "error", Error: err.Error()}
			statusCode = fiber.StatusServiceUnavailable
			status = "degraded"
			continue
		}
		dependencies[name] = readinessDependency{Status: "ok"}
	}

	return c.Status(statusCode).JSON(fiber.Map{
		"status":       status,
		"service":      "loklingo-backend",
		"dependencies": dependencies,
	})
}

func checkHTTPDependency(ctx context.Context, client *http.Client, url string, apiKey string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return nil
}
