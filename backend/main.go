package main

import (
	"log/slog"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"

	"loklingo/backend/config"
	"loklingo/backend/handlers"
	"loklingo/backend/middleware"
	"loklingo/backend/services"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

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

	slog.Info("LokLingo backend starting", "port", cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
