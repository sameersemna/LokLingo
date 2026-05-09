package middleware

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

func buildRateLimiter(max int, expiration time.Duration, shouldApply func(*fiber.Ctx) bool) fiber.Handler {
	if max <= 0 {
		max = 1
	}
	if expiration <= 0 {
		expiration = time.Minute
	}
	if shouldApply == nil {
		shouldApply = func(*fiber.Ctx) bool { return true }
	}

	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: expiration,
		Next: func(c *fiber.Ctx) bool {
			return !shouldApply(c)
		},
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded — try again in a minute",
			})
		},
	})
}

// GlobalRateLimiter caps requests per IP for most routes while skipping
// health/readiness probes.
func GlobalRateLimiter(maxPerMinute int) fiber.Handler {
	return buildRateLimiter(maxPerMinute, time.Minute, func(c *fiber.Ctx) bool {
		path := c.Path()
		if path == "/health" || path == "/ready" {
			return false
		}
		return true
	})
}

// WriteRateLimiter caps write-route requests per IP.
func WriteRateLimiter(maxPerMinute int) fiber.Handler {
	return buildRateLimiter(maxPerMinute, time.Minute, func(c *fiber.Ctx) bool {
		return c.Method() == fiber.MethodPost
	})
}

// UploadRateLimiter applies stricter limits to upload-heavy endpoints.
func UploadRateLimiter(maxPerMinute int) fiber.Handler {
	uploadPaths := map[string]struct{}{
		"/api/v1/translate/image": {},
		"/api/v1/jobs/pdf":        {},
		"/api/v1/jobs/image":      {},
	}
	return buildRateLimiter(maxPerMinute, time.Minute, func(c *fiber.Ctx) bool {
		if c.Method() != fiber.MethodPost {
			return false
		}
		path := strings.TrimSpace(c.Path())
		_, ok := uploadPaths[path]
		return ok
	})
}
