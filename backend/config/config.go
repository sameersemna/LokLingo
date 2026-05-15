package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AppEnv                       string
	Port                         string
	LiteLLMBaseURL               string
	LiteLLMAPIKey                string
	LiteLLMModel                 string
	OllamaBaseURL                string
	OllamaModel                  string
	OpenAICompatBaseURL          string
	OpenAICompatAPIKey           string
	OpenAICompatModel            string
	OCRServiceURL                string
	OCRProvider                  string
	OCRProviderTimeoutSeconds    int
	OCRMaxFallbacks              int
	OCRSharedStorageDir          string
	RedisURL                     string
	PostgresDSN                  string // optional; enables Postgres analytics sink when set
	MaxPDFUploadBytes            int64
	MaxPDFPages                  int
	TranslateConcurrency         int    // max concurrent LLM page-translation requests per job; default 3
	MaxLLMConcurrency            int    // max total concurrent LLM calls across all jobs (global); default 10
	TranslateChunkMinWords       int    // preferred minimum words per translation chunk; default 80
	TranslateChunkMaxWords       int    // hard cap words per translation chunk; default 150
	LiteLLMRequestTimeoutSeconds int    // HTTP client timeout for LLM calls in seconds; default 300 (5 min)
	InternalToken                string // optional; enforces X-Internal-Token on internal routes
	WriteAPIToken                string // optional in development; required in production for write routes
	GlobalRateLimitPerMinute     int    // max requests per minute per IP for non-health routes
	WriteRateLimitPerMinute      int    // max write requests per minute per IP
	UploadRateLimitPerMinute     int    // max upload-heavy write requests per minute per IP
	SyncImageMaxInflight         int    // max concurrent in-flight sync image requests
}

func Load() *Config {
	appEnv := getEnv("APP_ENV", "development")

	return &Config{
		AppEnv:                       appEnv,
		Port:                         getEnv("PORT", "8080"),
		LiteLLMBaseURL:               getEnv("LITELLM_BASE_URL", ""),
		LiteLLMAPIKey:                getEnv("LITELLM_API_KEY", ""),
		LiteLLMModel:                 getEnv("LITELLM_MODEL", ""),
		OllamaBaseURL:                getEnv("OLLAMA_BASE_URL", ""),
		OllamaModel:                  getEnv("OLLAMA_MODEL", ""),
		OpenAICompatBaseURL:          getEnv("OPENAI_COMPAT_BASE_URL", ""),
		OpenAICompatAPIKey:           getEnv("OPENAI_COMPAT_API_KEY", ""),
		OpenAICompatModel:            getEnv("OPENAI_COMPAT_MODEL", ""),
		OCRServiceURL:                getEnv("OCR_SERVICE_URL", ""),
		OCRProvider:                  getEnv("OCR_PROVIDER", "paddle"),
		OCRProviderTimeoutSeconds:    getEnvInt("OCR_PROVIDER_TIMEOUT_SECONDS", 120),
		OCRMaxFallbacks:              getEnvInt("OCR_MAX_FALLBACKS", 2),
		OCRSharedStorageDir:          getEnv("OCR_SHARED_STORAGE_DIR", ""),
		RedisURL:                     resolveRedisURL(appEnv),
		PostgresDSN:                  getEnv("POSTGRES_DSN", ""),
		MaxPDFUploadBytes:            getEnvInt64("MAX_PDF_UPLOAD_BYTES", 25*1024*1024),
		MaxPDFPages:                  getEnvInt("MAX_PDF_PAGES", 300),
		TranslateConcurrency:         getEnvInt("TRANSLATE_CONCURRENCY", 3),
		MaxLLMConcurrency:            getEnvInt("MAX_LLM_CONCURRENCY", 10),
		TranslateChunkMinWords:       getEnvInt("TRANSLATE_CHUNK_MIN_WORDS", 80),
		TranslateChunkMaxWords:       getEnvInt("TRANSLATE_CHUNK_MAX_WORDS", 150),
		LiteLLMRequestTimeoutSeconds: getEnvInt("LITELLM_REQUEST_TIMEOUT_SECONDS", 300),
		InternalToken:                getEnv("INTERNAL_TOKEN", ""),
		WriteAPIToken:                getEnv("WRITE_API_TOKEN", ""),
		GlobalRateLimitPerMinute:     getEnvInt("GLOBAL_RATE_LIMIT_PER_MINUTE", 120),
		WriteRateLimitPerMinute:      getEnvInt("WRITE_RATE_LIMIT_PER_MINUTE", 40),
		UploadRateLimitPerMinute:     getEnvInt("UPLOAD_RATE_LIMIT_PER_MINUTE", 12),
		SyncImageMaxInflight:         getEnvInt("SYNC_IMAGE_MAX_INFLIGHT", 8),
	}
}

func (c *Config) Validate() error {
	missing := make([]string, 0, 4)
	invalid := make([]string, 0, 3)

	liteBase := strings.TrimSpace(c.LiteLLMBaseURL)
	liteModel := strings.TrimSpace(c.LiteLLMModel)
	ollamaBase := strings.TrimSpace(c.OllamaBaseURL)
	ollamaModel := strings.TrimSpace(c.OllamaModel)
	compatBase := strings.TrimSpace(c.OpenAICompatBaseURL)
	compatModel := strings.TrimSpace(c.OpenAICompatModel)

	hasLite := liteBase != "" && liteModel != ""
	hasOllama := ollamaBase != "" && ollamaModel != ""
	hasCompat := compatBase != "" && compatModel != ""

	if !hasLite && !hasOllama && !hasCompat {
		missing = append(missing, "translation provider configuration (LITELLM_*, OLLAMA_*, or OPENAI_COMPAT_*)")
	}
	if (liteBase == "") != (liteModel == "") {
		invalid = append(invalid, "LITELLM_BASE_URL and LITELLM_MODEL must be set together")
	}
	if (ollamaBase == "") != (ollamaModel == "") {
		invalid = append(invalid, "OLLAMA_BASE_URL and OLLAMA_MODEL must be set together")
	}
	if (compatBase == "") != (compatModel == "") {
		invalid = append(invalid, "OPENAI_COMPAT_BASE_URL and OPENAI_COMPAT_MODEL must be set together")
	}
	if strings.TrimSpace(c.OCRServiceURL) == "" {
		missing = append(missing, "OCR_SERVICE_URL")
	}
	switch strings.ToLower(strings.TrimSpace(c.OCRProvider)) {
	case "", "paddle", "tesseract", "ollama":
		// allowed
	default:
		invalid = append(invalid, "OCR_PROVIDER must be one of: paddle, tesseract, ollama")
	}
	if c.OCRProviderTimeoutSeconds < 0 {
		invalid = append(invalid, "OCR_PROVIDER_TIMEOUT_SECONDS must be >= 0")
	}
	if c.OCRProviderTimeoutSeconds == 0 {
		c.OCRProviderTimeoutSeconds = 120
	}
	if c.OCRMaxFallbacks < 0 {
		invalid = append(invalid, "OCR_MAX_FALLBACKS must be >= 0")
	}
	if strings.TrimSpace(c.RedisURL) == "" {
		missing = append(missing, "REDIS_URL")
	}
	if strings.EqualFold(strings.TrimSpace(c.AppEnv), "production") && strings.TrimSpace(c.WriteAPIToken) == "" {
		missing = append(missing, "WRITE_API_TOKEN")
	}

	if c.TranslateConcurrency <= 0 {
		invalid = append(invalid, "TRANSLATE_CONCURRENCY must be >= 1")
	}
	if c.MaxLLMConcurrency <= 0 {
		invalid = append(invalid, "MAX_LLM_CONCURRENCY must be >= 1")
	}
	if c.TranslateChunkMinWords <= 0 {
		invalid = append(invalid, "TRANSLATE_CHUNK_MIN_WORDS must be >= 1")
	}
	if c.TranslateChunkMaxWords < c.TranslateChunkMinWords {
		invalid = append(invalid, "TRANSLATE_CHUNK_MAX_WORDS must be >= TRANSLATE_CHUNK_MIN_WORDS")
	}
	if c.LiteLLMRequestTimeoutSeconds <= 0 {
		invalid = append(invalid, "LITELLM_REQUEST_TIMEOUT_SECONDS must be >= 1")
	}
	if c.GlobalRateLimitPerMinute <= 0 {
		invalid = append(invalid, "GLOBAL_RATE_LIMIT_PER_MINUTE must be >= 1")
	}
	if c.WriteRateLimitPerMinute <= 0 {
		invalid = append(invalid, "WRITE_RATE_LIMIT_PER_MINUTE must be >= 1")
	}
	if c.UploadRateLimitPerMinute <= 0 {
		invalid = append(invalid, "UPLOAD_RATE_LIMIT_PER_MINUTE must be >= 1")
	}
	if c.SyncImageMaxInflight <= 0 {
		invalid = append(invalid, "SYNC_IMAGE_MAX_INFLIGHT must be >= 1")
	}

	if len(invalid) > 0 {
		return fmt.Errorf("invalid configuration: %s", strings.Join(invalid, "; "))
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return nil
}

// resolveRedisURL selects the Redis URL for the active environment.
// Resolution order:
//  1. REDIS_URL_<UPPER_APP_ENV>  (e.g. REDIS_URL_DEVELOPMENT, REDIS_URL_PRODUCTION)
//  2. REDIS_URL                  (generic fallback, preserves backward compatibility)
func resolveRedisURL(appEnv string) string {
	envKey := "REDIS_URL_" + strings.ToUpper(appEnv)
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return os.Getenv("REDIS_URL")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
