package config

import (
	"strings"
	"testing"
)

func TestLoadUsesEnvSpecificRedisURL(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("REDIS_URL", "redis://generic:6379")
	t.Setenv("REDIS_URL_PRODUCTION", "redis://prod:6379")

	cfg := Load()
	if cfg.RedisURL != "redis://prod:6379" {
		t.Fatalf("expected env-specific redis url, got %q", cfg.RedisURL)
	}
}

func TestLoadFallsBackToGenericRedisURL(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("REDIS_URL", "redis://generic:6379")

	cfg := Load()
	if cfg.RedisURL != "redis://generic:6379" {
		t.Fatalf("expected generic redis url fallback, got %q", cfg.RedisURL)
	}
}

func TestConfigValidateRequiresRuntimeDependencies(t *testing.T) {
	cfg := &Config{}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	msg := err.Error()
	for _, key := range []string{"LITELLM_BASE_URL", "LITELLM_MODEL", "OCR_SERVICE_URL", "REDIS_URL"} {
		if !strings.Contains(msg, key) {
			t.Fatalf("expected validation error to mention %s, got %q", key, msg)
		}
	}
}

func TestConfigValidatePassesForCompleteConfig(t *testing.T) {
	cfg := &Config{
		LiteLLMBaseURL: "http://latitude:11435",
		LiteLLMModel:   "ollama/llama3.2:latest",
		OCRServiceURL:  "http://loklingo-ocr:8000",
		RedisURL:       "redis://loklingo-redis:6379",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestLoadReadsPDFGuardrailEnvValues(t *testing.T) {
	t.Setenv("MAX_PDF_UPLOAD_BYTES", "1048576")
	t.Setenv("MAX_PDF_PAGES", "150")

	cfg := Load()
	if cfg.MaxPDFUploadBytes != 1048576 {
		t.Fatalf("expected MaxPDFUploadBytes=1048576, got %d", cfg.MaxPDFUploadBytes)
	}
	if cfg.MaxPDFPages != 150 {
		t.Fatalf("expected MaxPDFPages=150, got %d", cfg.MaxPDFPages)
	}
}
