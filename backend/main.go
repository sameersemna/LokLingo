package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"

	"loklingo/backend/config"
	"loklingo/backend/handlers"
	"loklingo/backend/middleware"
	"loklingo/backend/services"
)

func main() {
	cfg := config.Load()

	app := fiber.New(fiber.Config{
		AppName: "LokLingo API",
	})

	app.Use(logger.New())
	app.Use(middleware.RequestID())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,OPTIONS",
		AllowHeaders: "Content-Type,Authorization",
	}))
	app.Use(middleware.RateLimiter())

	app.Get("/health", handlers.HealthHandler)

	translationService := services.NewTranslationService(cfg)
	translateHandler := handlers.NewTranslateHandler(translationService)

	api := app.Group("/api/v1")
	api.Post("/translate", translateHandler.Translate)

	log.Printf("LokLingo backend starting on :%s", cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
