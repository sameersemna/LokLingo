package middleware

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
)

// HTTPLogger emits one structured log line per request with request tracing data.
func HTTPLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()

		status := c.Response().StatusCode()
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		attrs := []any{
			"request_id", c.Locals("requestID"),
			"method", c.Method(),
			"path", c.Path(),
			"status", status,
			"latency_ms", time.Since(start).Milliseconds(),
			"ip", c.IP(),
		}
		if ua := c.Get("User-Agent"); ua != "" {
			attrs = append(attrs, "user_agent", ua)
		}
		if err != nil && status < 400 {
			attrs = append(attrs, "err", err)
		}

		slog.Log(c.Context(), level, "http_request", attrs...)
		return err
	}
}
