package config

import (
	"os"
)

type Config struct {
	Port           string
	LiteLLMBaseURL string
	LiteLLMAPIKey  string
	LiteLLMModel   string
	OCRServiceURL  string
}

func Load() *Config {
	return &Config{
		Port:           getEnv("PORT", "8080"),
		LiteLLMBaseURL: getEnv("LITELLM_BASE_URL", "http://localhost:4000"),
		LiteLLMAPIKey:  getEnv("LITELLM_API_KEY", "sk-loklingo"),
		LiteLLMModel:   getEnv("LITELLM_MODEL", "gpt-4o-mini"),
		OCRServiceURL:  getEnv("OCR_SERVICE_URL", "http://ocr-service:8000"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
