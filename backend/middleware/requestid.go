package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const RequestIDHeader = "X-Request-ID"

// RequestID injects a unique request ID into every request context and response header.
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Get(RequestIDHeader)
		if id == "" {
			id = uuid.New().String()
		}
		c.Set(RequestIDHeader, id)
		c.Locals("requestID", id)
		return c.Next()
	}
}
