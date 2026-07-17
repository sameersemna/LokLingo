package middleware

import "github.com/gofiber/fiber/v2"

// SecurityHeaders adds defensive HTTP response headers for API responses.
// These complement nginx CSP/HSTS on the frontend edge.
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("Referrer-Policy", "no-referrer")
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Set("Cross-Origin-Resource-Policy", "same-site")
		c.Set("X-XSS-Protection", "0") // modern browsers rely on CSP; disable legacy XSS auditor
		// Cache-Control for API responses — avoid caching authenticated/dynamic data.
		if c.Path() != "/health" && c.Path() != "/ready" {
			c.Set("Cache-Control", "no-store")
		}
		return c.Next()
	}
}
