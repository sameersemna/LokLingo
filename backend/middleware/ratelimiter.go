package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// RateLimiter returns a middleware that caps requests per IP.
// max requests per window can be tuned via environment; these defaults are
// intentionally conservative for a self-hosted translation service.
func RateLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        60,
		Expiration: 1 * time.Minute,
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
