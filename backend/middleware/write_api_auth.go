package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// WriteAPIAuth protects write routes using either X-API-Token or
// Authorization: Bearer <token>.
//
// Behavior:
// - production + empty token: rejects all requests (misconfigured server)
// - non-production + empty token: allows all requests (developer convenience)
// - token set: requires a matching token value.
func WriteAPIAuth(appEnv, token string) fiber.Handler {
	token = strings.TrimSpace(token)
	isProd := strings.EqualFold(strings.TrimSpace(appEnv), "production")

	return func(c *fiber.Ctx) error {
		if token == "" {
			if isProd {
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
					"error": "server misconfigured: write API auth token missing",
				})
			}
			return c.Next()
		}

		headerToken := strings.TrimSpace(c.Get("X-API-Token"))
		if headerToken == "" {
			authz := strings.TrimSpace(c.Get("Authorization"))
			if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
				headerToken = strings.TrimSpace(authz[7:])
			}
		}

		if headerToken != token {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "unauthorized",
			})
		}
		return c.Next()
	}
}
