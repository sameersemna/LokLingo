package middleware

import "github.com/gofiber/fiber/v2"

// InflightGate caps concurrent in-flight requests for selected routes.
// Requests above the cap are rejected with 429.
func InflightGate(maxInflight int, appliesToPath string) fiber.Handler {
	if maxInflight <= 0 {
		maxInflight = 1
	}
	sem := make(chan struct{}, maxInflight)

	return func(c *fiber.Ctx) error {
		if appliesToPath != "" && c.Path() != appliesToPath {
			return c.Next()
		}

		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
			return c.Next()
		default:
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "too many in-flight requests for this endpoint",
			})
		}
	}
}
