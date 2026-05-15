package services

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeOCRProvider struct {
	name      string
	pdfResult *OCRResult
	pdfErr    error
	imageErr  error
	delay     time.Duration
}

func (f *fakeOCRProvider) Name() string { return f.name }

func (f *fakeOCRProvider) ExtractPDFText(ctx context.Context, _ []byte, _ OCRRequest) (*OCRResult, error) {
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(f.delay):
		}
	}
	if f.pdfErr != nil {
		return nil, f.pdfErr
	}
	return f.pdfResult, nil
}

func (f *fakeOCRProvider) ExtractImageText(ctx context.Context, _ []byte, _ OCRRequest) (*OCRResult, error) {
	if f.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(f.delay):
		}
	}
	if f.imageErr != nil {
		return nil, f.imageErr
	}
	return &OCRResult{Blocks: []OCRTextBlock{{Text: "ok", Bbox: []float64{0, 0, 10, 10}}}}, nil
}

func TestProviderOrderSelection(t *testing.T) {
	if got := providerOrder(OCRProviderPaddle); got[0] != OCRProviderPaddle {
		t.Fatalf("expected paddle first, got %v", got)
	}
	if got := providerOrder(OCRProviderTesseract); got[0] != OCRProviderTesseract {
		t.Fatalf("expected tesseract first, got %v", got)
	}
	if got := providerOrder(OCRProviderOllama); got[0] != OCRProviderOllama {
		t.Fatalf("expected ollama first, got %v", got)
	}
}

func TestChainedOCRClient_FallbackOnProviderFailure(t *testing.T) {
	chain := &chainedOCRClient{providers: []OCRProvider{
		&fakeOCRProvider{name: OCRProviderPaddle, pdfErr: errors.New("paddle down")},
		&fakeOCRProvider{name: OCRProviderTesseract, pdfResult: &OCRResult{Pages: []string{"page 1"}, Confidence: 0.91}},
	}}

	res, err := chain.extractPDFWithFallback(context.Background(), []byte("pdf"), OCRRequest{Lang: "en"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Provider != OCRProviderTesseract {
		t.Fatalf("expected fallback provider tesseract, got %s", res.Provider)
	}
	if res.Retries != 1 {
		t.Fatalf("expected retries=1 after one fallback, got %d", res.Retries)
	}
}

func TestChainedOCRClient_TimeoutHandling(t *testing.T) {
	chain := &chainedOCRClient{providers: []OCRProvider{
		&fakeOCRProvider{name: OCRProviderPaddle, delay: 100 * time.Millisecond, pdfResult: &OCRResult{Pages: []string{"late"}}},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := chain.extractPDFWithFallback(ctx, []byte("pdf"), OCRRequest{Lang: "en"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestChainedOCRClient_InvalidOCRResult(t *testing.T) {
	chain := &chainedOCRClient{providers: []OCRProvider{
		&fakeOCRProvider{name: OCRProviderPaddle, pdfResult: &OCRResult{}},
	}}

	_, err := chain.extractPDFWithFallback(context.Background(), []byte("pdf"), OCRRequest{Lang: "en"})
	if err == nil {
		t.Fatal("expected invalid OCR output error")
	}
}
