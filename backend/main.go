package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"

	"loklingo/backend/config"
	"loklingo/backend/handlers"
)

func main() {
	cfg := config.Load()

	app := fiber.New(fiber.Config{
		AppName: "LokLingo API",
	})

	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,OPTIONS",
		AllowHeaders: "Content-Type,Authorization",
	}))

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "loklingo-backend"})
	})

	api := app.Group("/api/v1")
	api.Post("/translate", handlers.NewTranslateHandler(cfg).Translate)

	log.Printf("LokLingo backend starting on :%s", cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
