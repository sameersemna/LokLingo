package handlers

import "github.com/gofiber/fiber/v2"

// errResponse returns a structured JSON error body with the request ID when available.
func errResponse(c *fiber.Ctx, status int, msg string) error {
	body := fiber.Map{"error": msg}
	if id, ok := c.Locals("requestID").(string); ok && id != "" {
		body["request_id"] = id
	}
	return c.Status(status).JSON(body)
}
