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
		LiteLLMBaseURL:               "http://latitude:11435",
		LiteLLMModel:                 "ollama/llama3.2:latest",
		OCRServiceURL:                "http://loklingo-ocr:8000",
		RedisURL:                     "redis://loklingo-redis:6379",
		TranslateConcurrency:         3,
		MaxLLMConcurrency:            10,
		TranslateChunkMinWords:       500,
		TranslateChunkMaxWords:       1000,
		LiteLLMRequestTimeoutSeconds: 300,
		GlobalRateLimitPerMinute:     120,
		WriteRateLimitPerMinute:      40,
		UploadRateLimitPerMinute:     12,
		SyncImageMaxInflight:         8,
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestLoadReadsPDFGuardrailEnvValues(t *testing.T) {
	t.Setenv("MAX_PDF_UPLOAD_BYTES", "1048576")
	t.Setenv("MAX_PDF_PAGES", "150")
	t.Setenv("OCR_SHARED_STORAGE_DIR", "/tmp/loklingo")

	cfg := Load()
	if cfg.MaxPDFUploadBytes != 1048576 {
		t.Fatalf("expected MaxPDFUploadBytes=1048576, got %d", cfg.MaxPDFUploadBytes)
	}
	if cfg.MaxPDFPages != 150 {
		t.Fatalf("expected MaxPDFPages=150, got %d", cfg.MaxPDFPages)
	}
	if cfg.OCRSharedStorageDir != "/tmp/loklingo" {
		t.Fatalf("expected OCRSharedStorageDir=/tmp/loklingo, got %q", cfg.OCRSharedStorageDir)
	}
}

func TestLoadReadsChunkingEnvValues(t *testing.T) {
	t.Setenv("TRANSLATE_CONCURRENCY", "4")
	t.Setenv("TRANSLATE_CHUNK_MIN_WORDS", "600")
	t.Setenv("TRANSLATE_CHUNK_MAX_WORDS", "900")

	cfg := Load()
	if cfg.TranslateConcurrency != 4 {
		t.Fatalf("expected TranslateConcurrency=4, got %d", cfg.TranslateConcurrency)
	}
	if cfg.TranslateChunkMinWords != 600 {
		t.Fatalf("expected TranslateChunkMinWords=600, got %d", cfg.TranslateChunkMinWords)
	}
	if cfg.TranslateChunkMaxWords != 900 {
		t.Fatalf("expected TranslateChunkMaxWords=900, got %d", cfg.TranslateChunkMaxWords)
	}
}

func TestConfigValidateRejectsInvalidTranslationLimits(t *testing.T) {
	cfg := &Config{
		LiteLLMBaseURL:           "http://latitude:11435",
		LiteLLMModel:             "ollama/llama3.2:latest",
		OCRServiceURL:            "http://loklingo-ocr:8000",
		RedisURL:                 "redis://loklingo-redis:6379",
		TranslateConcurrency:     0,
		TranslateChunkMinWords:   500,
		TranslateChunkMaxWords:   1000,
		GlobalRateLimitPerMinute: 120,
		WriteRateLimitPerMinute:  40,
		UploadRateLimitPerMinute: 12,
		SyncImageMaxInflight:     8,
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "TRANSLATE_CONCURRENCY") {
		t.Fatalf("expected validation error to mention TRANSLATE_CONCURRENCY, got %q", err.Error())
	}
}

func TestConfigValidateRejectsInvalidChunkRange(t *testing.T) {
	cfg := &Config{
		LiteLLMBaseURL:               "http://latitude:11435",
		LiteLLMModel:                 "ollama/llama3.2:latest",
		OCRServiceURL:                "http://loklingo-ocr:8000",
		RedisURL:                     "redis://loklingo-redis:6379",
		TranslateConcurrency:         3,
		TranslateChunkMinWords:       1000,
		TranslateChunkMaxWords:       500,
		LiteLLMRequestTimeoutSeconds: 300,
		GlobalRateLimitPerMinute:     120,
		WriteRateLimitPerMinute:      40,
		UploadRateLimitPerMinute:     12,
		SyncImageMaxInflight:         8,
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "TRANSLATE_CHUNK_MAX_WORDS") {
		t.Fatalf("expected validation error to mention TRANSLATE_CHUNK_MAX_WORDS, got %q", err.Error())
	}
}

func TestLoadReadsLiteLLMRequestTimeoutSeconds(t *testing.T) {
	t.Setenv("LITELLM_REQUEST_TIMEOUT_SECONDS", "600")

	cfg := Load()
	if cfg.LiteLLMRequestTimeoutSeconds != 600 {
		t.Fatalf("expected LiteLLMRequestTimeoutSeconds=600, got %d", cfg.LiteLLMRequestTimeoutSeconds)
	}
}

func TestLoadReadsOptionalProviderEnvValues(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "http://ollama:11434/v1")
	t.Setenv("OLLAMA_MODEL", "llama3.2")
	t.Setenv("OPENAI_COMPAT_BASE_URL", "https://example.test/v1")
	t.Setenv("OPENAI_COMPAT_API_KEY", "secret")
	t.Setenv("OPENAI_COMPAT_MODEL", "gpt-4o-mini")

	cfg := Load()
	if cfg.OllamaBaseURL != "http://ollama:11434/v1" {
		t.Fatalf("expected OllamaBaseURL to be loaded, got %q", cfg.OllamaBaseURL)
	}
	if cfg.OllamaModel != "llama3.2" {
		t.Fatalf("expected OllamaModel to be loaded, got %q", cfg.OllamaModel)
	}
	if cfg.OpenAICompatBaseURL != "https://example.test/v1" {
		t.Fatalf("expected OpenAICompatBaseURL to be loaded, got %q", cfg.OpenAICompatBaseURL)
	}
	if cfg.OpenAICompatAPIKey != "secret" {
		t.Fatalf("expected OpenAICompatAPIKey to be loaded, got %q", cfg.OpenAICompatAPIKey)
	}
	if cfg.OpenAICompatModel != "gpt-4o-mini" {
		t.Fatalf("expected OpenAICompatModel to be loaded, got %q", cfg.OpenAICompatModel)
	}
}

func TestConfigValidateRejectsZeroRequestTimeout(t *testing.T) {
	cfg := &Config{
		LiteLLMBaseURL:               "http://latitude:11435",
		LiteLLMModel:                 "ollama/llama3.2:latest",
		OCRServiceURL:                "http://loklingo-ocr:8000",
		RedisURL:                     "redis://loklingo-redis:6379",
		TranslateConcurrency:         3,
		MaxLLMConcurrency:            10,
		TranslateChunkMinWords:       250,
		TranslateChunkMaxWords:       500,
		LiteLLMRequestTimeoutSeconds: 0,
		GlobalRateLimitPerMinute:     120,
		WriteRateLimitPerMinute:      40,
		UploadRateLimitPerMinute:     12,
		SyncImageMaxInflight:         8,
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for zero timeout, got nil")
	}
	if !strings.Contains(err.Error(), "LITELLM_REQUEST_TIMEOUT_SECONDS") {
		t.Fatalf("expected validation error to mention LITELLM_REQUEST_TIMEOUT_SECONDS, got %q", err.Error())
	}
}

func TestConfigValidate_ProductionRequiresWriteAPIToken(t *testing.T) {
	cfg := &Config{
		AppEnv:                       "production",
		LiteLLMBaseURL:               "http://latitude:11435",
		LiteLLMModel:                 "gpt-4o-mini",
		OCRServiceURL:                "http://loklingo-ocr:8000",
		RedisURL:                     "redis://loklingo-redis:6379",
		TranslateConcurrency:         3,
		MaxLLMConcurrency:            10,
		TranslateChunkMinWords:       80,
		TranslateChunkMaxWords:       150,
		LiteLLMRequestTimeoutSeconds: 300,
		GlobalRateLimitPerMinute:     120,
		WriteRateLimitPerMinute:      40,
		UploadRateLimitPerMinute:     12,
		SyncImageMaxInflight:         8,
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for missing WRITE_API_TOKEN in production")
	}
	if !strings.Contains(err.Error(), "WRITE_API_TOKEN") {
		t.Fatalf("expected validation error to mention WRITE_API_TOKEN, got %q", err.Error())
	}
}

func TestConfigValidate_DevelopmentAllowsMissingWriteAPIToken(t *testing.T) {
	cfg := &Config{
		AppEnv:                       "development",
		LiteLLMBaseURL:               "http://latitude:11435",
		LiteLLMModel:                 "gpt-4o-mini",
		OCRServiceURL:                "http://loklingo-ocr:8000",
		RedisURL:                     "redis://loklingo-redis:6379",
		TranslateConcurrency:         3,
		MaxLLMConcurrency:            10,
		TranslateChunkMinWords:       80,
		TranslateChunkMaxWords:       150,
		LiteLLMRequestTimeoutSeconds: 300,
		GlobalRateLimitPerMinute:     120,
		WriteRateLimitPerMinute:      40,
		UploadRateLimitPerMinute:     12,
		SyncImageMaxInflight:         8,
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid development config without WRITE_API_TOKEN, got %v", err)
	}
}

func TestConfigValidateRejectsInvalidTrafficLimits(t *testing.T) {
	cfg := &Config{
		AppEnv:                       "development",
		LiteLLMBaseURL:               "http://latitude:11435",
		LiteLLMModel:                 "gpt-4o-mini",
		OCRServiceURL:                "http://loklingo-ocr:8000",
		RedisURL:                     "redis://loklingo-redis:6379",
		TranslateConcurrency:         3,
		MaxLLMConcurrency:            10,
		TranslateChunkMinWords:       80,
		TranslateChunkMaxWords:       150,
		LiteLLMRequestTimeoutSeconds: 300,
		GlobalRateLimitPerMinute:     0,
		WriteRateLimitPerMinute:      0,
		UploadRateLimitPerMinute:     0,
		SyncImageMaxInflight:         0,
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid traffic limits")
	}
	msg := err.Error()
	for _, key := range []string{
		"GLOBAL_RATE_LIMIT_PER_MINUTE",
		"WRITE_RATE_LIMIT_PER_MINUTE",
		"UPLOAD_RATE_LIMIT_PER_MINUTE",
		"SYNC_IMAGE_MAX_INFLIGHT",
	} {
		if !strings.Contains(msg, key) {
			t.Fatalf("expected validation error to mention %s, got %q", key, msg)
		}
	}
}
