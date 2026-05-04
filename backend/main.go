package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"

	"loklingo/backend/config"
	"loklingo/backend/handlers"
	"loklingo/backend/jobs"
	"loklingo/backend/middleware"
	"loklingo/backend/services"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := config.Load()

	// --- Job store (Redis) ---
	jobStore, err := jobs.NewRedisStore(cfg.RedisURL)
	if err != nil {
		slog.Error("failed to connect to Redis", "err", err)
		os.Exit(1)
	}

	// --- Worker ---
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	translationService := services.NewTranslationService(cfg)
	worker := jobs.NewWorker(jobStore, translationService)
	go worker.Run(ctx)

	// --- HTTP server ---
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

	translateHandler := handlers.NewTranslateHandler(translationService)
	jobsHandler := handlers.NewJobsHandler(jobStore)

	api := app.Group("/api/v1")
	api.Post("/translate", translateHandler.Translate)
	api.Post("/jobs", jobsHandler.CreateJob)
	api.Get("/jobs/:id", jobsHandler.GetJob)

	slog.Info("LokLingo backend starting", "port", cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
