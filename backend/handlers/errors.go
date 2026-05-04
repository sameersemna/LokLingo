package handlers

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
)

// errResponse returns a structured JSON error body with a machine-readable code,
// human-readable message, and the request ID when available.
func errResponse(c *fiber.Ctx, status int, msg string) error {
	body := fiber.Map{
		"error": msg,
		"code":  http.StatusText(status),
	}
	if id, ok := c.Locals("requestID").(string); ok && id != "" {
		body["request_id"] = id
	}
	return c.Status(status).JSON(body)
}
