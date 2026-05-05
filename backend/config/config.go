package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AppEnv              string
	Port                string
	LiteLLMBaseURL      string
	LiteLLMAPIKey       string
	LiteLLMModel        string
	OCRServiceURL       string
	OCRSharedStorageDir string
	RedisURL            string
	PostgresDSN         string // optional; enables Postgres analytics sink when set
	MaxPDFUploadBytes   int64
	MaxPDFPages         int
	InternalToken       string // optional; enforces X-Internal-Token on internal routes
}

func Load() *Config {
	appEnv := getEnv("APP_ENV", "development")

	return &Config{
		AppEnv:              appEnv,
		Port:                getEnv("PORT", "8080"),
		LiteLLMBaseURL:      getEnv("LITELLM_BASE_URL", ""),
		LiteLLMAPIKey:       getEnv("LITELLM_API_KEY", ""),
		LiteLLMModel:        getEnv("LITELLM_MODEL", ""),
		OCRServiceURL:       getEnv("OCR_SERVICE_URL", ""),
		OCRSharedStorageDir: getEnv("OCR_SHARED_STORAGE_DIR", ""),
		RedisURL:            resolveRedisURL(appEnv),
		PostgresDSN:         getEnv("POSTGRES_DSN", ""),
		MaxPDFUploadBytes:   getEnvInt64("MAX_PDF_UPLOAD_BYTES", 25*1024*1024),
		MaxPDFPages:         getEnvInt("MAX_PDF_PAGES", 300),
		InternalToken:       getEnv("INTERNAL_TOKEN", ""),
	}
}

func (c *Config) Validate() error {
	missing := make([]string, 0, 4)

	if strings.TrimSpace(c.LiteLLMBaseURL) == "" {
		missing = append(missing, "LITELLM_BASE_URL")
	}
	if strings.TrimSpace(c.LiteLLMModel) == "" {
		missing = append(missing, "LITELLM_MODEL")
	}
	if strings.TrimSpace(c.OCRServiceURL) == "" {
		missing = append(missing, "OCR_SERVICE_URL")
	}
	if strings.TrimSpace(c.RedisURL) == "" {
		missing = append(missing, "REDIS_URL")
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
