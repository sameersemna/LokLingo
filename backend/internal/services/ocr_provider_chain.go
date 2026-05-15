package services

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"loklingo/backend/internal/observability"
)

const (
	OCRProviderPaddle    = "paddle"
	OCRProviderTesseract = "tesseract"
	OCRProviderOllama    = "ollama"
)

// OCRRequest describes request options shared by OCR providers.
type OCRRequest struct {
	Lang     string
	DPI      int
	MimeType string
	FilePath string
}

// OCRResult is the normalized output produced by OCR providers.
type OCRResult struct {
	Text       string
	Pages      []string
	Blocks     []OCRTextBlock
	Confidence float64
	Retries    int
	Provider   string
}

// OCRProvider defines a pluggable OCR provider contract.
type OCRProvider interface {
	ExtractImageText(ctx context.Context, image []byte, opts OCRRequest) (*OCRResult, error)
	ExtractPDFText(ctx context.Context, pdf []byte, opts OCRRequest) (*OCRResult, error)
	Name() string
}

type chainedOCRClient struct {
	providers        []OCRProvider
	providerTimeout  time.Duration
	maxFallbackCount int
}

// OCRClientChainOptions configures provider-level timeout and fallback budget.
type OCRClientChainOptions struct {
	ProviderTimeout  time.Duration
	MaxFallbackCount int
}

// NewPluggableOCRClient creates an OCR client using config-driven provider
// selection with automatic fallback chain.
func NewPluggableOCRClient(ocrBaseURL, sharedStorageDir, provider string, options ...OCRClientChainOptions) OCRClient {
	primary := normalizeOCRProvider(provider)
	if primary == "" {
		primary = normalizeOCRProvider(os.Getenv("OCR_PROVIDER"))
	}
	if primary == "" {
		primary = OCRProviderPaddle
	}

	baseProviders := map[string]OCRProvider{
		OCRProviderPaddle:    NewPaddleOCRProvider(ocrBaseURL, sharedStorageDir),
		OCRProviderTesseract: NewTesseractProvider(ocrBaseURL, sharedStorageDir),
		OCRProviderOllama:    NewOllamaVisionProvider(),
	}
	order := providerOrder(primary)
	providers := make([]OCRProvider, 0, len(order))
	for _, name := range order {
		if p, ok := baseProviders[name]; ok {
			providers = append(providers, p)
		}
	}

	chainOpts := OCRClientChainOptions{
		ProviderTimeout:  120 * time.Second,
		MaxFallbackCount: len(providers) - 1,
	}
	if len(options) > 0 {
		o := options[0]
		if o.ProviderTimeout > 0 {
			chainOpts.ProviderTimeout = o.ProviderTimeout
		}
		if o.MaxFallbackCount >= 0 {
			chainOpts.MaxFallbackCount = o.MaxFallbackCount
		}
	}

	return &chainedOCRClient{
		providers:        providers,
		providerTimeout:  chainOpts.ProviderTimeout,
		maxFallbackCount: chainOpts.MaxFallbackCount,
	}
}

func normalizeOCRProvider(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case OCRProviderPaddle:
		return OCRProviderPaddle
	case OCRProviderTesseract:
		return OCRProviderTesseract
	case OCRProviderOllama:
		return OCRProviderOllama
	default:
		return ""
	}
}

func providerOrder(primary string) []string {
	switch primary {
	case OCRProviderTesseract:
		return []string{OCRProviderTesseract, OCRProviderPaddle, OCRProviderOllama}
	case OCRProviderOllama:
		return []string{OCRProviderOllama, OCRProviderPaddle, OCRProviderTesseract}
	default:
		return []string{OCRProviderPaddle, OCRProviderTesseract, OCRProviderOllama}
	}
}

func (c *chainedOCRClient) ExtractText(filePath, lang string) (string, error) {
	pages, _, err := c.ExtractPagesWithConfidence(filePath, lang)
	if err != nil {
		return "", err
	}
	if len(pages) == 0 {
		return "", fmt.Errorf("ocr: service returned empty text")
	}
	return strings.Join(pages, "\n\n"), nil
}

func (c *chainedOCRClient) ExtractPages(filePath, lang string) ([]string, error) {
	pages, _, err := c.ExtractPagesWithConfidence(filePath, lang)
	return pages, err
}

func (c *chainedOCRClient) ExtractPagesWithConfidence(filePath, lang string) ([]string, float64, error) {
	pdf, err := os.ReadFile(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("ocr: read file %q: %w", filePath, err)
	}
	res, err := c.extractPDFWithFallback(context.Background(), pdf, OCRRequest{Lang: lang, DPI: 200, FilePath: filePath})
	if err != nil {
		return nil, 0, err
	}
	if len(res.Pages) > 0 {
		return res.Pages, res.Confidence, nil
	}
	if strings.TrimSpace(res.Text) == "" {
		return nil, 0, fmt.Errorf("ocr: service returned empty response")
	}
	return []string{res.Text}, res.Confidence, nil
}

func (c *chainedOCRClient) ExtractImageBlocks(filePath, lang string) ([]OCRTextBlock, error) {
	image, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("ocr: read image %q: %w", filePath, err)
	}
	res, err := c.extractImageWithFallback(context.Background(), image, OCRRequest{Lang: lang, FilePath: filePath})
	if err != nil {
		return nil, err
	}
	if len(res.Blocks) == 0 {
		return nil, fmt.Errorf("ocr: image response returned no blocks")
	}
	return res.Blocks, nil
}

func (c *chainedOCRClient) extractPDFWithFallback(ctx context.Context, pdf []byte, opts OCRRequest) (*OCRResult, error) {
	if len(c.providers) == 0 {
		return nil, fmt.Errorf("ocr: no providers configured")
	}
	if opts.Lang == "" {
		opts.Lang = "auto"
	}

	var lastErr error
	retries := 0
	fallbacks := 0
	for idx, provider := range c.providers {
		if c.maxFallbackCount >= 0 && fallbacks > c.maxFallbackCount {
			return nil, fmt.Errorf("ocr: fallback budget exhausted after %d fallback(s): %w", c.maxFallbackCount, lastErr)
		}
		slog.Info("ocr_provider_selected", "provider", provider.Name(), "position", idx)
		providerCtx := ctx
		cancel := func() {}
		if c.providerTimeout > 0 {
			providerCtx, cancel = context.WithTimeout(ctx, c.providerTimeout)
		}
		start := time.Now()
		res, err := provider.ExtractPDFText(providerCtx, pdf, opts)
		cancel()
		latency := time.Since(start).Milliseconds()
		if err != nil {
			lastErr = err
			slog.Warn("ocr_provider_failed", "provider", provider.Name(), "latency_ms", latency, "err", err)
			if idx < len(c.providers)-1 && (c.maxFallbackCount < 0 || fallbacks < c.maxFallbackCount) {
				fallbacks++
				retries++
				observability.IncOCRProviderFallback()
				slog.Warn("ocr_provider_fallback", "from_provider", provider.Name(), "to_provider", c.providers[idx+1].Name(), "fallback_count", fallbacks)
			} else if idx < len(c.providers)-1 {
				slog.Warn("ocr_provider_fallback_budget_exhausted", "provider", provider.Name(), "fallback_count", fallbacks, "max_fallback_count", c.maxFallbackCount)
				return nil, fmt.Errorf("ocr: fallback budget exhausted after %d fallback(s): %w", c.maxFallbackCount, err)
			}
			continue
		}
		if err := validateProviderResult(res, true); err != nil {
			lastErr = err
			slog.Warn("ocr_provider_failed", "provider", provider.Name(), "latency_ms", latency, "err", err)
			if idx < len(c.providers)-1 && (c.maxFallbackCount < 0 || fallbacks < c.maxFallbackCount) {
				fallbacks++
				retries++
				observability.IncOCRProviderFallback()
				slog.Warn("ocr_provider_fallback", "from_provider", provider.Name(), "to_provider", c.providers[idx+1].Name(), "fallback_count", fallbacks)
			} else if idx < len(c.providers)-1 {
				slog.Warn("ocr_provider_fallback_budget_exhausted", "provider", provider.Name(), "fallback_count", fallbacks, "max_fallback_count", c.maxFallbackCount)
				return nil, fmt.Errorf("ocr: fallback budget exhausted after %d fallback(s): %w", c.maxFallbackCount, err)
			}
			continue
		}
		res.Provider = provider.Name()
		res.Retries += retries
		observability.IncOCRProviderUsed(provider.Name())
		observability.RecordOCRProviderLatency(latency)
		observability.RecordOCRProviderConfidence(res.Confidence)
		if res.Retries > 0 {
			observability.IncOCRProviderRetries(int64(res.Retries))
		}
		slog.Info("ocr_completed", "provider", provider.Name(), "latency_ms", latency, "confidence", res.Confidence, "retries", res.Retries, "fallback_count", fallbacks)
		return res, nil
	}
	return nil, fmt.Errorf("ocr: all providers exhausted: %w", lastErr)
}

func (c *chainedOCRClient) extractImageWithFallback(ctx context.Context, image []byte, opts OCRRequest) (*OCRResult, error) {
	if len(c.providers) == 0 {
		return nil, fmt.Errorf("ocr: no providers configured")
	}
	if opts.Lang == "" {
		opts.Lang = "auto"
	}

	var lastErr error
	retries := 0
	fallbacks := 0
	for idx, provider := range c.providers {
		if c.maxFallbackCount >= 0 && fallbacks > c.maxFallbackCount {
			return nil, fmt.Errorf("ocr: fallback budget exhausted after %d fallback(s): %w", c.maxFallbackCount, lastErr)
		}
		slog.Info("ocr_provider_selected", "provider", provider.Name(), "position", idx)
		providerCtx := ctx
		cancel := func() {}
		if c.providerTimeout > 0 {
			providerCtx, cancel = context.WithTimeout(ctx, c.providerTimeout)
		}
		start := time.Now()
		res, err := provider.ExtractImageText(providerCtx, image, opts)
		cancel()
		latency := time.Since(start).Milliseconds()
		if err != nil {
			lastErr = err
			slog.Warn("ocr_provider_failed", "provider", provider.Name(), "latency_ms", latency, "err", err)
			if idx < len(c.providers)-1 && (c.maxFallbackCount < 0 || fallbacks < c.maxFallbackCount) {
				fallbacks++
				retries++
				observability.IncOCRProviderFallback()
				slog.Warn("ocr_provider_fallback", "from_provider", provider.Name(), "to_provider", c.providers[idx+1].Name(), "fallback_count", fallbacks)
			} else if idx < len(c.providers)-1 {
				slog.Warn("ocr_provider_fallback_budget_exhausted", "provider", provider.Name(), "fallback_count", fallbacks, "max_fallback_count", c.maxFallbackCount)
				return nil, fmt.Errorf("ocr: fallback budget exhausted after %d fallback(s): %w", c.maxFallbackCount, err)
			}
			continue
		}
		if err := validateProviderResult(res, false); err != nil {
			lastErr = err
			slog.Warn("ocr_provider_failed", "provider", provider.Name(), "latency_ms", latency, "err", err)
			if idx < len(c.providers)-1 && (c.maxFallbackCount < 0 || fallbacks < c.maxFallbackCount) {
				fallbacks++
				retries++
				observability.IncOCRProviderFallback()
				slog.Warn("ocr_provider_fallback", "from_provider", provider.Name(), "to_provider", c.providers[idx+1].Name(), "fallback_count", fallbacks)
			} else if idx < len(c.providers)-1 {
				slog.Warn("ocr_provider_fallback_budget_exhausted", "provider", provider.Name(), "fallback_count", fallbacks, "max_fallback_count", c.maxFallbackCount)
				return nil, fmt.Errorf("ocr: fallback budget exhausted after %d fallback(s): %w", c.maxFallbackCount, err)
			}
			continue
		}
		res.Provider = provider.Name()
		res.Retries += retries
		observability.IncOCRProviderUsed(provider.Name())
		observability.RecordOCRProviderLatency(latency)
		observability.RecordOCRProviderConfidence(res.Confidence)
		if res.Retries > 0 {
			observability.IncOCRProviderRetries(int64(res.Retries))
		}
		slog.Info("ocr_completed", "provider", provider.Name(), "latency_ms", latency, "confidence", res.Confidence, "retries", res.Retries, "fallback_count", fallbacks)
		return res, nil
	}
	return nil, fmt.Errorf("ocr: all providers exhausted: %w", lastErr)
}

func validateProviderResult(result *OCRResult, isPDF bool) error {
	if result == nil {
		return fmt.Errorf("ocr: provider returned nil result")
	}
	if isPDF {
		if strings.TrimSpace(result.Text) == "" && len(result.Pages) == 0 {
			return fmt.Errorf("ocr: service returned empty response")
		}
		return nil
	}
	if len(result.Blocks) == 0 {
		return fmt.Errorf("ocr: image response returned no blocks")
	}
	return nil
}

// Name identifies Paddle-backed provider.
type PaddleOCRProvider struct {
	client OCRClient
}

func NewPaddleOCRProvider(baseURL, sharedStorageDir string) OCRProvider {
	return &PaddleOCRProvider{client: NewOCRClient(baseURL, sharedStorageDir)}
}

func (p *PaddleOCRProvider) Name() string { return OCRProviderPaddle }

func (p *PaddleOCRProvider) ExtractPDFText(ctx context.Context, pdf []byte, opts OCRRequest) (*OCRResult, error) {
	return runLegacyPDFProvider(ctx, p.client, pdf, opts)
}

func (p *PaddleOCRProvider) ExtractImageText(ctx context.Context, image []byte, opts OCRRequest) (*OCRResult, error) {
	return runLegacyImageProvider(ctx, p.client, image, opts)
}

// Name identifies Tesseract-backed provider.
type TesseractProvider struct {
	client OCRClient
}

func NewTesseractProvider(baseURL, sharedStorageDir string) OCRProvider {
	return &TesseractProvider{client: NewOCRClient(baseURL, sharedStorageDir)}
}

func (p *TesseractProvider) Name() string { return OCRProviderTesseract }

func (p *TesseractProvider) ExtractPDFText(ctx context.Context, pdf []byte, opts OCRRequest) (*OCRResult, error) {
	return runLegacyPDFProvider(ctx, p.client, pdf, opts)
}

func (p *TesseractProvider) ExtractImageText(ctx context.Context, image []byte, opts OCRRequest) (*OCRResult, error) {
	return runLegacyImageProvider(ctx, p.client, image, opts)
}

// OllamaVisionProvider is a placeholder for upcoming multimodal OCR support.
type OllamaVisionProvider struct{}

func NewOllamaVisionProvider() OCRProvider { return &OllamaVisionProvider{} }

func (p *OllamaVisionProvider) Name() string { return OCRProviderOllama }

func (p *OllamaVisionProvider) ExtractPDFText(context.Context, []byte, OCRRequest) (*OCRResult, error) {
	return nil, fmt.Errorf("ocr provider %s is not implemented yet", p.Name())
}

func (p *OllamaVisionProvider) ExtractImageText(context.Context, []byte, OCRRequest) (*OCRResult, error) {
	return nil, fmt.Errorf("ocr provider %s is not implemented yet", p.Name())
}

type pageConfidenceExtractor interface {
	ExtractPagesWithConfidence(filePath, lang string) ([]string, float64, error)
}

func runLegacyPDFProvider(ctx context.Context, client OCRClient, pdf []byte, opts OCRRequest) (*OCRResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp("", "loklingo-ocr-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("ocr: create temp PDF: %w", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(pdf); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("ocr: write temp PDF: %w", err)
	}
	_ = f.Close()

	type res struct {
		pages      []string
		confidence float64
		err        error
	}
	ch := make(chan res, 1)
	go func() {
		if withConfidence, ok := client.(pageConfidenceExtractor); ok {
			pages, confidence, err := withConfidence.ExtractPagesWithConfidence(f.Name(), opts.Lang)
			ch <- res{pages: pages, confidence: confidence, err: err}
			return
		}
		pages, err := client.ExtractPages(f.Name(), opts.Lang)
		ch <- res{pages: pages, confidence: 0, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case out := <-ch:
		if out.err != nil {
			return nil, out.err
		}
		return &OCRResult{Pages: out.pages, Text: strings.Join(out.pages, "\n\n"), Confidence: out.confidence}, nil
	}
}

func runLegacyImageProvider(ctx context.Context, client OCRClient, image []byte, opts OCRRequest) (*OCRResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp("", "loklingo-ocr-image-*.img")
	if err != nil {
		return nil, fmt.Errorf("ocr: create temp image: %w", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(image); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("ocr: write temp image: %w", err)
	}
	_ = f.Close()

	type res struct {
		blocks []OCRTextBlock
		err    error
	}
	ch := make(chan res, 1)
	go func() {
		blocks, err := client.ExtractImageBlocks(f.Name(), opts.Lang)
		ch <- res{blocks: blocks, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case out := <-ch:
		if out.err != nil {
			return nil, out.err
		}
		return &OCRResult{Blocks: out.blocks, Confidence: averageBlockConfidence(out.blocks)}, nil
	}
}

func averageBlockConfidence(blocks []OCRTextBlock) float64 {
	if len(blocks) == 0 {
		return 0
	}
	total := 0.0
	count := 0
	for _, b := range blocks {
		if b.Confidence > 0 {
			total += b.Confidence
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}
