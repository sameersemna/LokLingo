package main

import (
	"context"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"

	"loklingo/backend/config"
	"loklingo/backend/handlers"
	internalservices "loklingo/backend/internal/services"
	"loklingo/backend/jobs"
	"loklingo/backend/middleware"
	"loklingo/backend/services"
	"loklingo/backend/services/providers"

	"loklingo/backend/internal/pglog"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	jsonHandler := slog.NewJSONHandler(os.Stdout, nil)

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	// Connect to Postgres for the analytics sink (optional).
	var pgPool *pgxpool.Pool
	if cfg.PostgresDSN != "" {
		var err error
		pgPool, err = pgxpool.New(context.Background(), cfg.PostgresDSN)
		if err != nil {
			slog.New(jsonHandler).Warn("postgres analytics sink disabled: could not connect", "err", err)
		} else {
			slog.New(jsonHandler).Info("postgres analytics sink enabled")
		}
	}

	slog.SetDefault(slog.New(pglog.New(jsonHandler, pgPool)))

	slog.Info("LokLingo starting", "env", cfg.AppEnv)

	if cfg.AppEnv == "production" && strings.Contains(cfg.RedisURL, "localhost") {
		slog.Error("misconfiguration: REDIS_URL points to localhost in production", "redis_url", cfg.RedisURL)
		os.Exit(1)
	}

	// --- Job store (Redis) ---
	jobStore, err := jobs.NewRedisStore(cfg.RedisURL)
	if err != nil {
		slog.Error("failed to connect to Redis", "err", err)
		os.Exit(1)
	}

	// --- Worker ---
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	providerRegistry := providers.NewRegistry()
	providerTimeout := time.Duration(cfg.LiteLLMRequestTimeoutSeconds) * time.Second
	if cfg.LiteLLMBaseURL != "" && cfg.LiteLLMModel != "" {
		providerRegistry.Register(providers.NewLiteLLMProvider(cfg.LiteLLMBaseURL, cfg.LiteLLMAPIKey, cfg.LiteLLMModel, providerTimeout))
	}
	if cfg.OllamaBaseURL != "" && cfg.OllamaModel != "" {
		providerRegistry.Register(providers.NewOllamaProvider(cfg.OllamaBaseURL, cfg.OllamaModel, providerTimeout))
	}
	if cfg.OpenAICompatBaseURL != "" && cfg.OpenAICompatModel != "" {
		providerRegistry.Register(providers.NewOpenAICompatProvider(cfg.OpenAICompatBaseURL, cfg.OpenAICompatAPIKey, cfg.OpenAICompatModel, providerTimeout))
	}
	translationService := services.NewOrchestrator(providerRegistry, services.DefaultOrchestratorConfig)
	pdfService := internalservices.NewPDFService()
	ocrClient := internalservices.NewPluggableOCRClient(
		cfg.OCRServiceURL,
		cfg.OCRSharedStorageDir,
		cfg.OCRProvider,
		internalservices.OCRClientChainOptions{
			ProviderTimeout:  time.Duration(cfg.OCRProviderTimeoutSeconds) * time.Second,
			MaxFallbackCount: cfg.OCRMaxFallbacks,
		},
	)
	worker := jobs.NewWorker(jobStore, translationService, pdfService, ocrClient, cfg.MaxPDFPages,
		jobs.WithTranslateConcurrency(cfg.TranslateConcurrency),
		jobs.WithMaxLLMConcurrency(cfg.MaxLLMConcurrency),
		jobs.WithPDFChunkWordRange(cfg.TranslateChunkMinWords, cfg.TranslateChunkMaxWords),
	)
	go worker.Run(ctx)

	// --- HTTP server ---
	app := fiber.New(fiber.Config{
		AppName: "LokLingo API",
	})

	app.Use(logger.New())
	app.Use(middleware.RequestID())
	app.Use(cors.New(cors.Config{
		AllowOriginsFunc: allowLANOrigin,
		AllowMethods:     "GET,POST,OPTIONS",
		AllowHeaders:     "Content-Type,Authorization",
	}))
	app.Use(middleware.GlobalRateLimiter(cfg.GlobalRateLimitPerMinute))

	app.Get("/health", handlers.HealthHandler)
	app.Get("/ready", handlers.NewReadinessHandler(cfg).Ready)

	translateHandler := handlers.NewTranslateHandler(translationService, jobStore)
	jobsHandler := handlers.NewJobsHandler(jobStore, cfg.MaxPDFUploadBytes)
	ocrMetricsHandler := handlers.NewOCRMetricsHandler(pgPool)
	providerMetricsHandler := handlers.NewProviderMetricsHandler(translationService)
	reliabilityMetricsHandler := handlers.NewReliabilityMetricsHandler(jobStore)
	lifecycleEventsHandler := handlers.NewLifecycleEventsHandler()
	prometheusMetricsHandler := handlers.NewPrometheusMetricsHandler(jobStore, translationService)

	api := app.Group("/api/v1")
	writeAPI := api.Group("", middleware.WriteAPIAuth(cfg.AppEnv, cfg.WriteAPIToken), middleware.WriteRateLimiter(cfg.WriteRateLimitPerMinute), middleware.UploadRateLimiter(cfg.UploadRateLimitPerMinute))
	writeAPI.Post("/translate", translateHandler.Translate)
	writeAPI.Post("/translate/image", middleware.InflightGate(cfg.SyncImageMaxInflight, "/api/v1/translate/image"), translateHandler.TranslateImage)
	writeAPI.Post("/jobs", jobsHandler.CreateJob)
	writeAPI.Post("/jobs/pdf", jobsHandler.CreatePDFJob)
	writeAPI.Post("/jobs/image", jobsHandler.CreateImageJob)
	api.Get("/jobs/dead", middleware.InternalToken(cfg.InternalToken), jobsHandler.ListDeadJobs)
	api.Post("/jobs/:id/replay", middleware.InternalToken(cfg.InternalToken), jobsHandler.ReplayDeadJob)
	api.Get("/jobs/:id", jobsHandler.GetJob)
	api.Get("/jobs/:id/events", jobsHandler.StreamJobEvents)
	api.Get("/jobs/:id/output", jobsHandler.DownloadJobOutput)
	api.Get("/metrics/ocr", middleware.InternalToken(cfg.InternalToken), ocrMetricsHandler.Summary)
	api.Get("/metrics/providers", middleware.InternalToken(cfg.InternalToken), providerMetricsHandler.Summary)
	api.Get("/metrics/providers/health", middleware.InternalToken(cfg.InternalToken), providerMetricsHandler.Health)
	api.Get("/metrics/reliability", middleware.InternalToken(cfg.InternalToken), reliabilityMetricsHandler.Summary)
	api.Get("/metrics/lifecycle/events", middleware.InternalToken(cfg.InternalToken), lifecycleEventsHandler.List)
	api.Get("/metrics/prometheus", middleware.InternalToken(cfg.InternalToken), prometheusMetricsHandler.Expose)

	bindAddr := "0.0.0.0:" + cfg.Port
	slog.Info("LokLingo backend starting", "port", cfg.Port, "bind", bindAddr)
	if err := app.Listen(bindAddr); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func allowLANOrigin(origin string) bool {
	if strings.TrimSpace(origin) == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if host == "" {
		return false
	}
	if host == "promaxgb10-6116" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback()
}
