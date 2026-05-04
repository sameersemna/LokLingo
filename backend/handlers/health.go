package handlers

import "github.com/gofiber/fiber/v2"

// HealthHandler returns a simple liveness probe response.
func HealthHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok", "service": "loklingo-backend"})
}
