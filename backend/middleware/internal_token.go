package middleware

import (
	"crypto/subtle"

	"github.com/gofiber/fiber/v2"
)

// InternalToken returns a middleware that enforces a static bearer/header token
// on internal-only routes (e.g. /api/v1/metrics/ocr).
//
// The token is taken from the X-Internal-Token request header.
// If token is empty the middleware is a no-op (opt-in for local dev).
func InternalToken(token string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if token == "" {
			return c.Next()
		}
		// Constant-time comparison to avoid timing side-channels on the token.
		if subtle.ConstantTimeCompare([]byte(c.Get("X-Internal-Token")), []byte(token)) != 1 {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "unauthorized",
			})
		}
		return c.Next()
	}
}
