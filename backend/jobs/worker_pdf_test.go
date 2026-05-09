package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	internalservices "loklingo/backend/internal/services"
	"loklingo/backend/services"
)

// ---------- minimal mocks ------------------------------------------------

type mockWorkerStore struct {
	cachedResult string
	updateErr    error
	lastJob      *Job
	callCount    int
}

func (m *mockWorkerStore) Enqueue(_ context.Context, _ *Job) error { return nil }
func (m *mockWorkerStore) Get(_ context.Context, _ string) (*Job, error) {
	return nil, ErrNotFound
}
func (m *mockWorkerStore) Update(_ context.Context, j *Job) error {
	m.lastJob = j
	m.callCount++
	return m.updateErr
}
func (m *mockWorkerStore) Dequeue(_ context.Context) (*Job, error) { return nil, nil }
func (m *mockWorkerStore) GetCached(_ context.Context, _, _, _ string) (string, error) {
	if m.cachedResult != "" {
		return m.cachedResult, nil
	}
	return "", ErrNotFound
}
func (m *mockWorkerStore) SetCached(_ context.Context, _, _, _, _ string) error { return nil }

type mockTranslSvc struct {
	result string
	err    error
}

func (m *mockTranslSvc) Translate(_ services.TranslationInput) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

type mockPDFSvc struct {
	result       string
	err          error
	pageCount    int
	pageCountErr error
}

func (m *mockPDFSvc) ExtractText(_ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

func (m *mockPDFSvc) ExtractPages(_ string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.result == "" {
		return []string{""}, nil
	}
	return []string{m.result}, nil
}

func (m *mockPDFSvc) PageCount(_ string) (int, error) {
	if m.pageCountErr != nil {
		return 0, m.pageCountErr
	}
	if m.pageCount > 0 {
		return m.pageCount, nil
	}
	return 1, nil
}

type mockOCRSvc struct {
	result string
	err    error
	blocks []internalservices.OCRTextBlock
}

func (m *mockOCRSvc) ExtractText(_, _ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

func (m *mockOCRSvc) ExtractPages(_, _ string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.result == "" {
		return []string{""}, nil
	}
	return []string{m.result}, nil
}

func (m *mockOCRSvc) ExtractImageBlocks(_, _ string) ([]internalservices.OCRTextBlock, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.blocks) > 0 {
		return m.blocks, nil
	}
	if strings.TrimSpace(m.result) == "" {
		return nil, nil
	}
	return []internalservices.OCRTextBlock{{Text: m.result, Bbox: []float64{0, 0, 20, 20}}}, nil
}

// ---------- tests ---------------------------------------------------------

func TestWorker_PDFJob_MissingFilePath(t *testing.T) {
	store := &mockWorkerStore{}
	w := NewWorker(store, &mockTranslSvc{}, &mockPDFSvc{}, nil, 0)

	job := &Job{
		ID:     "pdf-1",
		Type:   TypePDF,
		Source: "en",
		Target: "de",
		// FilePath intentionally empty
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusFailed {
		t.Fatalf("expected status=failed, got %s", store.lastJob.Status)
	}
	if store.lastJob.ErrorMsg == "" {
		t.Fatal("expected non-empty ErrorMsg")
	}
}

func TestWorker_PDFJob_ExtractionError(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{err: errors.New("cannot open file")}
	w := NewWorker(store, &mockTranslSvc{result: "ignored"}, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-2",
		Type:     TypePDF,
		FilePath: "/tmp/no-such.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusFailed {
		t.Fatalf("expected status=failed, got %s", store.lastJob.Status)
	}
	if store.lastJob.ErrorMsg == "" {
		t.Fatal("expected non-empty ErrorMsg")
	}
}

func TestWorker_PDFJob_SuccessfulTranslation(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: "Hello PDF text"}
	transSvc := &mockTranslSvc{result: "Hallo PDF-Text"}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-3",
		Type:     TypePDF,
		FilePath: "/tmp/exists.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed, got %s", store.lastJob.Status)
	}
	if store.lastJob.TranslatedText != "Hallo PDF-Text" {
		t.Fatalf("expected 'Hallo PDF-Text', got %q", store.lastJob.TranslatedText)
	}
	if store.lastJob.ProcessingMethod != "pdf_text" {
		t.Fatalf("expected processing_method=pdf_text, got %q", store.lastJob.ProcessingMethod)
	}
}

func TestWorker_PDFJob_CacheHitSkipsExtraction(t *testing.T) {
	store := &mockWorkerStore{}
	// Pre-load extracted text into the job so cache lookup can match it.
	// We simulate a scenario where the job already has Text set (e.g. re-processed)
	// and the cache holds its translation.
	pdfSvc := &mockPDFSvc{result: "Cached PDF text"}
	store.cachedResult = "Cached Übersetzung"
	w := NewWorker(store, &mockTranslSvc{err: errors.New("should not be called")}, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-4",
		Type:     TypePDF,
		FilePath: "/tmp/exists.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed (from cache), got %s", store.lastJob.Status)
	}
	if store.lastJob.TranslatedText != "Cached Übersetzung" {
		t.Fatalf("expected cached text, got %q", store.lastJob.TranslatedText)
	}
}

func TestWorker_PDFJob_TranslationError(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: "Some extracted text"}
	transSvc := &mockTranslSvc{err: errors.New("LLM unavailable")}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-5",
		Type:     TypePDF,
		FilePath: "/tmp/exists.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusFailed {
		t.Fatalf("expected status=failed, got %s", store.lastJob.Status)
	}
}

func TestWorker_PDFJob_OCRFallback_Succeeds(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{err: errors.New("image-only PDF")}
	ocrSvc := &mockOCRSvc{result: "OCR extracted text"}
	transSvc := &mockTranslSvc{result: "OCR übersetzt"}
	w := NewWorker(store, transSvc, pdfSvc, ocrSvc, 0)

	job := &Job{
		ID:       "pdf-6",
		Type:     TypePDF,
		FilePath: "/tmp/image-only.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed after OCR fallback, got %s", store.lastJob.Status)
	}
	if store.lastJob.TranslatedText != "OCR übersetzt" {
		t.Fatalf("expected OCR-translated text, got %q", store.lastJob.TranslatedText)
	}
	if store.lastJob.ProcessingMethod != "ocr" {
		t.Fatalf("expected processing_method=ocr, got %q", store.lastJob.ProcessingMethod)
	}
}

func TestWorker_PDFJob_OCRFallback_AlsoFails(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{err: errors.New("image-only PDF")}
	ocrSvc := &mockOCRSvc{err: errors.New("OCR service unavailable")}
	w := NewWorker(store, &mockTranslSvc{}, pdfSvc, ocrSvc, 0)

	job := &Job{
		ID:       "pdf-7",
		Type:     TypePDF,
		FilePath: "/tmp/image-only.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusFailed {
		t.Fatalf("expected status=failed when both extraction and OCR fail, got %s", store.lastJob.Status)
	}
	if store.lastJob.ErrorMsg == "" {
		t.Fatal("expected non-empty ErrorMsg mentioning both failures")
	}
}

func TestWorker_PDFJob_EmptyText_TriggersOCRFallback(t *testing.T) {
	store := &mockWorkerStore{}
	// PDF extraction succeeds but returns empty string.
	pdfSvc := &mockPDFSvc{result: ""}
	ocrSvc := &mockOCRSvc{result: "OCR recovered text"}
	transSvc := &mockTranslSvc{result: "OCR übersetzt"}
	w := NewWorker(store, transSvc, pdfSvc, ocrSvc, 0)

	job := &Job{
		ID:       "pdf-8",
		Type:     TypePDF,
		FilePath: "/tmp/empty-text.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed after OCR fallback on empty text, got %s", store.lastJob.Status)
	}
	if store.lastJob.TranslatedText != "OCR übersetzt" {
		t.Fatalf("expected OCR-translated text, got %q", store.lastJob.TranslatedText)
	}
}

// TestWorker_PDFJob_EmptyText_OCRResultUsed verifies that when PDF extraction
// returns empty text (no error), the worker falls back to the OCR service,
// stores the OCR text on the job, translates it, and marks the job completed.
func TestWorker_PDFJob_EmptyText_OCRResultUsed(t *testing.T) {
	const ocrText = "Scanned invoice line one\n\nScanned invoice line two"
	const translated = "Gescannte Rechnungszeile eins\n\nGescannte Rechnungszeile zwei"

	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: ""}      // extraction succeeds but empty
	ocrSvc := &mockOCRSvc{result: ocrText} // OCR returns real content
	transSvc := &mockTranslSvc{result: translated}
	w := NewWorker(store, transSvc, pdfSvc, ocrSvc, 0)

	job := &Job{
		ID:       "pdf-ocr-used",
		Type:     TypePDF,
		FilePath: "/tmp/scanned-invoice.pdf",
		Source:   "en",
		Target:   "de",
		Lang:     "en",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}

	// Job must complete successfully.
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed, got %s (error: %s)", store.lastJob.Status, store.lastJob.ErrorMsg)
	}

	// job.Text must carry the OCR result (after normalization) so the translation
	// service received it as input.
	wantText := normalizeText(ocrText)
	if store.lastJob.Text != wantText {
		t.Fatalf("expected job.Text=%q (OCR result), got %q", wantText, store.lastJob.Text)
	}

	// TranslatedText must be the value returned by the translation service.
	if store.lastJob.TranslatedText != translated {
		t.Fatalf("expected TranslatedText=%q, got %q", translated, store.lastJob.TranslatedText)
	}
	if store.lastJob.ProcessingMethod != "ocr" {
		t.Fatalf("expected processing_method=ocr, got %q", store.lastJob.ProcessingMethod)
	}
}

func TestWorker_PDFJob_EmptyText_NoOCR_Fails(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: ""} // returns empty, no error
	w := NewWorker(store, &mockTranslSvc{}, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-9",
		Type:     TypePDF,
		FilePath: "/tmp/empty-text.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusFailed {
		t.Fatalf("expected status=failed for empty text with no OCR, got %s", store.lastJob.Status)
	}
}

func TestWorker_PDFJob_PageGuardrailRejectsOversizedDocument(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{pageCount: 500}
	w := NewWorker(store, &mockTranslSvc{}, pdfSvc, nil, 100)

	job := &Job{
		ID:       "pdf-10",
		Type:     TypePDF,
		FilePath: "/tmp/large.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusFailed {
		t.Fatalf("expected status=failed for oversized page count, got %s", store.lastJob.Status)
	}
	if store.lastJob.ErrorMsg == "" {
		t.Fatal("expected non-empty ErrorMsg for page guardrail")
	}
}

func TestWorker_PDFJob_FileCleanedUpOnCompletion(t *testing.T) {
	// Create a real temp file to confirm os.Remove is called.
	f, err := os.CreateTemp(t.TempDir(), "loklingo-*.pdf")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := f.Name()
	f.Close()

	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: "Hello world"}
	transSvc := &mockTranslSvc{result: "Hallo Welt"}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-cleanup",
		Type:     TypePDF,
		FilePath: tmpPath,
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil || store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected completed job, got %v", store.lastJob)
	}
	if _, statErr := os.Stat(tmpPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected temp file %s to be removed after job completion", tmpPath)
	}
}

func TestWorker_PDFJob_FileCleanedUpOnFailure(t *testing.T) {
	// Even when translation fails, the file must be cleaned up.
	f, err := os.CreateTemp(t.TempDir(), "loklingo-fail-*.pdf")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := f.Name()
	f.Close()

	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: "Some text"}
	transSvc := &mockTranslSvc{err: errors.New("LLM down")}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-cleanup-fail",
		Type:     TypePDF,
		FilePath: tmpPath,
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil || store.lastJob.Status != StatusFailed {
		t.Fatalf("expected failed job, got %v", store.lastJob)
	}
	if _, statErr := os.Stat(tmpPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected temp file %s to be removed even on failure", tmpPath)
	}
}

// mockMultiPagePDFSvc returns a configurable set of pages.
type mockMultiPagePDFSvc struct {
	pages        []string
	err          error
	pageCount    int
	pageCountErr error
}

func (m *mockMultiPagePDFSvc) ExtractText(_ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return strings.Join(m.pages, "\n"), nil
}

func (m *mockMultiPagePDFSvc) ExtractPages(_ string) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.pages, nil
}

func (m *mockMultiPagePDFSvc) PageCount(_ string) (int, error) {
	if m.pageCountErr != nil {
		return 0, m.pageCountErr
	}
	if m.pageCount > 0 {
		return m.pageCount, nil
	}
	return len(m.pages), nil
}

// mockPerPageTranslSvc records call order and maps input text to translated output.
type mockPerPageTranslSvc struct {
	mu           sync.Mutex
	translations map[string]string
	callOrder    []string
}

func (m *mockPerPageTranslSvc) Translate(in services.TranslationInput) (string, error) {
	m.mu.Lock()
	m.callOrder = append(m.callOrder, in.Text)
	m.mu.Unlock()
	parts := strings.Split(in.Text, pdfChunkSep)
	if len(parts) == 1 {
		if out, ok := m.translations[in.Text]; ok {
			return out, nil
		}
		return "translated:" + in.Text, nil
	}
	translatedParts := make([]string, 0, len(parts))
	for _, p := range parts {
		if out, ok := m.translations[p]; ok {
			translatedParts = append(translatedParts, out)
			continue
		}
		translatedParts = append(translatedParts, "translated:"+p)
	}
	return strings.Join(translatedParts, pdfChunkSep), nil
}

func TestWorker_PDFJob_MultiPage_TranslatesEachPageInOrder(t *testing.T) {
	const page1 = "First page content"
	const page2 = "Second page content"
	const page3 = "Third page content"

	store := &mockWorkerStore{}
	pdfSvc := &mockMultiPagePDFSvc{pages: []string{page1, page2, page3}}
	transSvc := &mockPerPageTranslSvc{
		translations: map[string]string{
			page1: "Erste Seite",
			page2: "Zweite Seite",
			page3: "Dritte Seite",
		},
	}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-multipage",
		Type:     TypePDF,
		FilePath: "/tmp/multipage.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed, got %s (err: %s)", store.lastJob.Status, store.lastJob.ErrorMsg)
	}

	// Translation calls are now chunked, so call count should be between 1 and
	// page count. Output order is still guaranteed.
	if len(transSvc.callOrder) < 1 || len(transSvc.callOrder) > 3 {
		t.Fatalf("expected 1..3 Translate calls with chunking, got %d", len(transSvc.callOrder))
	}

	want := "Erste Seite\n\nZweite Seite\n\nDritte Seite"
	if store.lastJob.TranslatedText != want {
		t.Fatalf("expected TranslatedText=%q, got %q", want, store.lastJob.TranslatedText)
	}

	// job.Text must be the joined extracted pages (used as cache key).
	wantText := page1 + "\n\n" + page2 + "\n\n" + page3
	if store.lastJob.Text != wantText {
		t.Fatalf("expected job.Text=%q, got %q", wantText, store.lastJob.Text)
	}
}

// concurrencyTrackingTranslSvc counts peak concurrent Translate invocations.
type concurrencyTrackingTranslSvc struct {
	mu        sync.Mutex
	active    int
	maxActive int
}

func (m *concurrencyTrackingTranslSvc) Translate(in services.TranslationInput) (string, error) {
	m.mu.Lock()
	m.active++
	if m.active > m.maxActive {
		m.maxActive = m.active
	}
	m.mu.Unlock()

	// Small delay to allow concurrent goroutines to overlap and expose peak concurrency.
	time.Sleep(5 * time.Millisecond)

	m.mu.Lock()
	m.active--
	m.mu.Unlock()

	parts := strings.Split(in.Text, pdfChunkSep)
	for i, p := range parts {
		parts[i] = "translated:" + p
	}
	return strings.Join(parts, pdfChunkSep), nil
}

// TestWorker_PDFJob_MultiPage_ConcurrencyBounded verifies that at most
// pdfTranslateConcurrency page-translation goroutines run simultaneously.
func TestWorker_PDFJob_MultiPage_ConcurrencyBounded(t *testing.T) {
	pages := []string{
		"Page one", "Page two", "Page three",
		"Page four", "Page five", "Page six",
	}

	store := &mockWorkerStore{}
	pdfSvc := &mockMultiPagePDFSvc{pages: pages}
	transSvc := &concurrencyTrackingTranslSvc{}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-concurrency",
		Type:     TypePDF,
		FilePath: "/tmp/concurrency.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil || store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected completed job, got %v", store.lastJob)
	}

	transSvc.mu.Lock()
	peak := transSvc.maxActive
	transSvc.mu.Unlock()

	if peak > pdfTranslateConcurrency {
		t.Fatalf("peak concurrent translations %d exceeded limit %d", peak, pdfTranslateConcurrency)
	}

	// Output must contain all six pages joined by double newlines.
	for _, p := range pages {
		if !strings.Contains(store.lastJob.TranslatedText, "translated:"+p) {
			t.Fatalf("missing translation for page %q in output", p)
		}
	}
}

// ---------- retry / rate-limit tests -------------------------------------

// retryCountingTranslSvc fails with a 429-like error for the first `failTimes`
// calls, then succeeds.
type retryCountingTranslSvc struct {
	mu        sync.Mutex
	calls     int
	failTimes int
	result    string
}

func (m *retryCountingTranslSvc) Translate(in services.TranslationInput) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls <= m.failTimes {
		return "", errors.New("litellm status 429: rate limit exceeded")
	}
	if m.result != "" {
		return m.result, nil
	}
	return "translated:" + in.Text, nil
}

// TestWorker_TranslateWithRetry_SucceedsAfterRateLimit verifies that a 429
// error on the first attempt is retried and ultimately succeeds.
func TestWorker_TranslateWithRetry_SucceedsAfterRateLimit(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: "Hello world"}
	// Fail twice with 429, succeed on third call.
	transSvc := &retryCountingTranslSvc{failTimes: 2, result: "Hallo Welt"}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-retry-ok",
		Type:     TypePDF,
		FilePath: "/tmp/retry.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed after retry, got %s (err: %s)", store.lastJob.Status, store.lastJob.ErrorMsg)
	}
	if store.lastJob.TranslatedText != "Hallo Welt" {
		t.Fatalf("expected 'Hallo Welt', got %q", store.lastJob.TranslatedText)
	}

	transSvc.mu.Lock()
	totalCalls := transSvc.calls
	transSvc.mu.Unlock()
	if totalCalls != 3 {
		t.Fatalf("expected 3 Translate calls (2 failures + 1 success), got %d", totalCalls)
	}
}

func TestWorker_TranslateWithRetry_MetricsCountTranslateRetries(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: "Hello world"}
	transSvc := &retryCountingTranslSvc{failTimes: 2, result: "Hallo Welt"}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	metrics := &retryMetrics{}
	out, err := w.translateWithRetry(context.Background(), "hello", "en", "de", metrics)
	if err != nil {
		t.Fatalf("expected translateWithRetry to succeed, got err: %v", err)
	}
	if out != "Hallo Welt" {
		t.Fatalf("expected translated output, got %q", out)
	}
	if got := metrics.translateRetryCount.Load(); got != 2 {
		t.Fatalf("expected translate retries=2, got %d", got)
	}
	if got := metrics.timeoutRetryCount.Load(); got != 0 {
		t.Fatalf("expected timeout retries=0, got %d", got)
	}
	if got := metrics.splitCount.Load(); got != 0 {
		t.Fatalf("expected split count=0, got %d", got)
	}
	if got := metrics.totalRetryEvents(); got != 2 {
		t.Fatalf("expected total retry events=2, got %d", got)
	}
}

// TestWorker_TranslateWithRetry_ExhaustsRetries verifies that after
// translateMaxRetries+1 consecutive 429s the job is marked failed.
func TestWorker_TranslateWithRetry_ExhaustsRetries(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: "Hello world"}
	// Always fail with 429 — more times than allowed retries.
	transSvc := &retryCountingTranslSvc{failTimes: translateMaxRetries + 10}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-retry-exhausted",
		Type:     TypePDF,
		FilePath: "/tmp/retry-fail.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusFailed {
		t.Fatalf("expected status=failed after exhausted retries, got %s", store.lastJob.Status)
	}
	if store.lastJob.ErrorMsg == "" {
		t.Fatal("expected non-empty ErrorMsg")
	}

	transSvc.mu.Lock()
	totalCalls := transSvc.calls
	transSvc.mu.Unlock()
	// Exactly translateMaxRetries+1 attempts (initial + retries).
	wantCalls := translateMaxRetries + 1
	if totalCalls != wantCalls {
		t.Fatalf("expected %d Translate calls, got %d", wantCalls, totalCalls)
	}
}

// TestWorker_TranslateWithRetry_NonRetryableErrorIsImmediate verifies that a
// non-429 error is NOT retried and the job fails immediately.
func TestWorker_TranslateWithRetry_NonRetryableErrorIsImmediate(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: "Hello world"}
	transSvc := &retryCountingTranslSvc{failTimes: 99, result: "never"}
	// Override the error to be a non-rate-limit one.
	nonRLSvc := &mockTranslSvc{err: errors.New("internal server error")}
	w := NewWorker(store, nonRLSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-nonrl-error",
		Type:     TypePDF,
		FilePath: "/tmp/nonrl.pdf",
		Source:   "en",
		Target:   "de",
	}
	_ = transSvc // unused in this sub-test
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusFailed {
		t.Fatalf("expected status=failed, got %s", store.lastJob.Status)
	}
}

// ---------- transient network error retry tests --------------------------

// transientFailTranslSvc fails with a configurable error for the first
// `failTimes` calls, then succeeds with `result`.
type transientFailTranslSvc struct {
	mu        sync.Mutex
	calls     int
	failTimes int
	failErr   string
	result    string
}

func (m *transientFailTranslSvc) Translate(in services.TranslationInput) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls <= m.failTimes {
		return "", errors.New(m.failErr)
	}
	if m.result != "" {
		return m.result, nil
	}
	return "translated:" + in.Text, nil
}

// TestWorker_TranslateWithRetry_RetriesOnNetworkTimeout verifies that a
// network timeout error is retried and ultimately succeeds.
func TestWorker_TranslateWithRetry_RetriesOnNetworkTimeout(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: "Hello world"}
	transSvc := &transientFailTranslSvc{
		failTimes: 2,
		failErr:   "net/http: request canceled (Client.Timeout exceeded while awaiting headers): i/o timeout",
		result:    "Hallo Welt",
	}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-retry-timeout",
		Type:     TypePDF,
		FilePath: "/tmp/timeout.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed after timeout retries, got %s (err: %s)", store.lastJob.Status, store.lastJob.ErrorMsg)
	}
	if store.lastJob.TranslatedText != "Hallo Welt" {
		t.Fatalf("expected 'Hallo Welt', got %q", store.lastJob.TranslatedText)
	}
	transSvc.mu.Lock()
	totalCalls := transSvc.calls
	transSvc.mu.Unlock()
	if totalCalls != 3 {
		t.Fatalf("expected 3 Translate calls (2 timeouts + 1 success), got %d", totalCalls)
	}
}

// TestWorker_TranslateWithRetry_RetriesOnDeadlineExceeded verifies that a
// wrapped "context deadline exceeded" error (e.g. an internal HTTP request
// timeout, not the job context) is retried and ultimately succeeds.
func TestWorker_TranslateWithRetry_RetriesOnDeadlineExceeded(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: "Hello world"}
	transSvc := &transientFailTranslSvc{
		failTimes: 1,
		failErr:   "Post \"http://litellm:4000/chat/completions\": context deadline exceeded",
		result:    "Hallo Welt",
	}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-retry-deadline",
		Type:     TypePDF,
		FilePath: "/tmp/deadline.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed after deadline retry, got %s (err: %s)", store.lastJob.Status, store.lastJob.ErrorMsg)
	}
	if store.lastJob.TranslatedText != "Hallo Welt" {
		t.Fatalf("expected 'Hallo Welt', got %q", store.lastJob.TranslatedText)
	}
	transSvc.mu.Lock()
	totalCalls := transSvc.calls
	transSvc.mu.Unlock()
	if totalCalls != 2 {
		t.Fatalf("expected 2 Translate calls (1 deadline + 1 success), got %d", totalCalls)
	}
}

// TestWorker_TranslateWithRetry_NoRetryOnCancelledJobContext verifies that
// when the job's own context is cancelled, a transient error is NOT retried.
func TestWorker_TranslateWithRetry_NoRetryOnCancelledJobContext(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: "Hello world"}

	// Always fail with a transient error — but we'll cancel the context first.
	transSvc := &transientFailTranslSvc{
		failTimes: 99,
		failErr:   "i/o timeout",
	}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the job even starts

	job := &Job{
		ID:       "pdf-retry-ctx-cancelled",
		Type:     TypePDF,
		FilePath: "/tmp/cancelled.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(ctx, job)

	// Job must be failed (context cancelled → no successful translation).
	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusFailed {
		t.Fatalf("expected status=failed for cancelled context, got %s", store.lastJob.Status)
	}
	// Must not have retried after the context was already done.
	transSvc.mu.Lock()
	totalCalls := transSvc.calls
	transSvc.mu.Unlock()
	if totalCalls > 1 {
		t.Fatalf("expected at most 1 Translate call for cancelled context, got %d", totalCalls)
	}
}

// correctly overrides the default concurrency.
func TestWorker_WithTranslateConcurrency_Option(t *testing.T) {
	const customConcurrency = 5
	pages := make([]string, 10)
	for i := range pages {
		pages[i] = fmt.Sprintf("Page %d content", i+1)
	}

	store := &mockWorkerStore{}
	pdfSvc := &mockMultiPagePDFSvc{pages: pages}
	transSvc := &concurrencyTrackingTranslSvc{}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0, WithTranslateConcurrency(customConcurrency))

	job := &Job{
		ID:       "pdf-custom-concurrency",
		Type:     TypePDF,
		FilePath: "/tmp/custom.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil || store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected completed job, got %v", store.lastJob)
	}

	transSvc.mu.Lock()
	peak := transSvc.maxActive
	transSvc.mu.Unlock()

	if peak > customConcurrency {
		t.Fatalf("peak concurrent translations %d exceeded custom limit %d", peak, customConcurrency)
	}
}

type callCountingChunkSvc struct {
	mu    sync.Mutex
	calls int
}

func (m *callCountingChunkSvc) Translate(in services.TranslationInput) (string, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	parts := strings.Split(in.Text, pdfChunkSep)
	for i, p := range parts {
		parts[i] = "translated:" + p
	}
	return strings.Join(parts, pdfChunkSep), nil
}

type adaptiveChunkSvc struct {
	mu            sync.Mutex
	calls         int
	wordsPerCall  []int
	firstCallSlow time.Duration
}

func (m *adaptiveChunkSvc) Translate(in services.TranslationInput) (string, error) {
	m.mu.Lock()
	m.calls++
	callNum := m.calls
	m.wordsPerCall = append(m.wordsPerCall, len(strings.Fields(in.Text)))
	m.mu.Unlock()

	if callNum == 1 && m.firstCallSlow > 0 {
		time.Sleep(m.firstCallSlow)
	}

	parts := strings.Split(in.Text, pdfChunkSep)
	for i, p := range parts {
		parts[i] = "translated:" + p
	}
	return strings.Join(parts, pdfChunkSep), nil
}

type timingProfileTranslSvc struct {
	mu            sync.Mutex
	callDurations []time.Duration
	fixedLatency  time.Duration
}

func (m *timingProfileTranslSvc) Translate(in services.TranslationInput) (string, error) {
	start := time.Now()
	if m.fixedLatency > 0 {
		time.Sleep(m.fixedLatency)
	}
	parts := strings.Split(in.Text, pdfChunkSep)
	for i, p := range parts {
		parts[i] = "translated:" + p
	}
	dur := time.Since(start)
	m.mu.Lock()
	m.callDurations = append(m.callDurations, dur)
	m.mu.Unlock()
	return strings.Join(parts, pdfChunkSep), nil
}

func (m *timingProfileTranslSvc) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callDurations = nil
}

func (m *timingProfileTranslSvc) maxCallDuration() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	var max time.Duration
	for _, d := range m.callDurations {
		if d > max {
			max = d
		}
	}
	return max
}

func (m *timingProfileTranslSvc) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.callDurations)
}

func TestWorker_PDFJob_Chunking_ReducesLLMCallsAndPreservesOrder(t *testing.T) {
	mkPage := func(tag string) string {
		// 40 words/page with default 80-150 range should merge pages into fewer chunks.
		return strings.TrimSpace(strings.Repeat(tag+" ", 40))
	}
	pages := []string{
		mkPage("one"),
		mkPage("two"),
		mkPage("three"),
		mkPage("four"),
		mkPage("five"),
		mkPage("six"),
	}

	store := &mockWorkerStore{}
	pdfSvc := &mockMultiPagePDFSvc{pages: pages}
	transSvc := &callCountingChunkSvc{}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-chunking-reduced-calls",
		Type:     TypePDF,
		FilePath: "/tmp/chunked.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil || store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected completed job, got %v", store.lastJob)
	}

	transSvc.mu.Lock()
	calls := transSvc.calls
	transSvc.mu.Unlock()
	if calls >= len(pages) {
		t.Fatalf("expected fewer than %d LLM calls with chunking, got %d", len(pages), calls)
	}

	translatedPages := strings.Split(store.lastJob.TranslatedText, "\n\n")
	if len(translatedPages) != len(pages) {
		t.Fatalf("expected %d translated pages, got %d", len(pages), len(translatedPages))
	}
	for i, tp := range translatedPages {
		wantPrefix := "translated:" + strings.SplitN(pages[i], " ", 2)[0]
		if !strings.HasPrefix(tp, wantPrefix) {
			t.Fatalf("page %d ordering mismatch: got prefix %q, want %q", i, tp[:min(20, len(tp))], wantPrefix)
		}
	}
}

func TestWorker_PDFJob_CustomChunkWordRange_ChangesCallCount(t *testing.T) {
	mkPage := func(tag string) string {
		return strings.TrimSpace(strings.Repeat(tag+" ", 120))
	}
	pages := []string{
		mkPage("one"),
		mkPage("two"),
		mkPage("three"),
		mkPage("four"),
		mkPage("five"),
		mkPage("six"),
	}

	store := &mockWorkerStore{}
	pdfSvc := &mockMultiPagePDFSvc{pages: pages}
	transSvc := &callCountingChunkSvc{}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0, WithPDFChunkWordRange(200, 300))

	job := &Job{
		ID:       "pdf-chunking-custom-range",
		Type:     TypePDF,
		FilePath: "/tmp/chunked-custom.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil || store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected completed job, got %v", store.lastJob)
	}

	transSvc.mu.Lock()
	calls := transSvc.calls
	transSvc.mu.Unlock()
	if calls < 2 {
		t.Fatalf("expected at least 2 calls with tighter chunk cap, got %d", calls)
	}
}

func TestWorker_PDFJob_AdaptiveChunkSizing_ReducesAfterSlowChunk(t *testing.T) {
	origSlowThreshold := slowChunkThreshold
	slowChunkThreshold = 1 * time.Millisecond
	defer func() { slowChunkThreshold = origSlowThreshold }()

	mkPage := func(tag string) string {
		// 50 words/page exposes rechunking differences between 60/120 and 48/96.
		return strings.TrimSpace(strings.Repeat(tag+" ", 50))
	}
	pages := make([]string, 32)
	for i := range pages {
		pages[i] = mkPage(fmt.Sprintf("p%d", i+1))
	}

	store := &mockWorkerStore{}
	pdfSvc := &mockMultiPagePDFSvc{pages: pages}
	transSvc := &adaptiveChunkSvc{firstCallSlow: 5 * time.Millisecond}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-adaptive-chunking",
		Type:     TypePDF,
		FilePath: "/tmp/adaptive.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil || store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected completed job, got %v", store.lastJob)
	}

	transSvc.mu.Lock()
	calls := transSvc.calls
	wordsPerCall := append([]int(nil), transSvc.wordsPerCall...)
	transSvc.mu.Unlock()

	// With min=80/max=150 and 50 words/page: initial chunks are 3 pages (150 words,
	// since 100w < max and 150+50=200 > max). First batch (concurrency=3) consumes
	// 9 pages; after a slow chunk we shrink to min=64/max=120 (2 pages/chunk at 100w),
	// giving 12 more calls for the remaining 23 pages. Total: 3 + 12 = 15.
	if calls != 15 {
		t.Fatalf("expected 15 Translate calls with adaptive chunk shrinking, got %d", calls)
	}
	if len(wordsPerCall) != calls {
		t.Fatalf("expected wordsPerCall length=%d, got %d", calls, len(wordsPerCall))
	}
	// First chunk should be around the default 150-word size (3 × 50).
	if wordsPerCall[0] < 130 {
		t.Fatalf("expected first chunk near default size (~150 words), got %d", wordsPerCall[0])
	}
	// At least one later call should be smaller than the default chunk size due to adaptation.
	sawReduced := false
	for _, w := range wordsPerCall[1:] {
		if w < 150 {
			sawReduced = true
			break
		}
	}
	if !sawReduced {
		t.Fatalf("expected adaptive sizing to produce at least one reduced chunk; words/call=%v", wordsPerCall)
	}
}

func TestWorker_PDFJob_TimingProfiles_SmallMediumLarge(t *testing.T) {
	mkPages := func(pageCount int, wordsPerPage int) []string {
		pages := make([]string, pageCount)
		for i := 0; i < pageCount; i++ {
			tag := fmt.Sprintf("p%d", i+1)
			pages[i] = strings.TrimSpace(strings.Repeat(tag+" ", wordsPerPage))
		}
		return pages
	}

	runCase := func(name string, pageCount int) (time.Duration, time.Duration, int, Status) {
		store := &mockWorkerStore{}
		pdfSvc := &mockMultiPagePDFSvc{pages: mkPages(pageCount, 60)}
		transSvc := &timingProfileTranslSvc{fixedLatency: 35 * time.Millisecond}
		w := NewWorker(store, transSvc, pdfSvc, nil, 0)

		job := &Job{
			ID:       "pdf-timing-" + name,
			Type:     TypePDF,
			FilePath: "/tmp/" + name + ".pdf",
			Source:   "en",
			Target:   "de",
		}

		start := time.Now()
		w.process(context.Background(), job)
		total := time.Since(start)
		maxChunk := transSvc.maxCallDuration()
		calls := transSvc.callCount()
		if store.lastJob == nil {
			t.Fatalf("%s: expected updated job", name)
		}
		if store.lastJob.Status != StatusCompleted {
			t.Fatalf("%s: expected completed job, got %s (err: %s)", name, store.lastJob.Status, store.lastJob.ErrorMsg)
		}
		if store.lastJob.ProcessedPages != pageCount {
			t.Fatalf("%s: expected ProcessedPages=%d, got %d", name, pageCount, store.lastJob.ProcessedPages)
		}
		t.Logf("TIMING_PROFILE name=%s pages=%d total_ms=%d max_chunk_ms=%d calls=%d", name, pageCount, total.Milliseconds(), maxChunk.Milliseconds(), calls)
		return total, maxChunk, calls, store.lastJob.Status
	}

	smallTotal, smallMaxChunk, _, _ := runCase("small", 5)
	mediumTotal, mediumMaxChunk, _, _ := runCase("medium", 20)
	largeTotal, largeMaxChunk, _, largeStatus := runCase("large", 50)

	if !(smallTotal <= mediumTotal && mediumTotal <= largeTotal) {
		t.Fatalf("expected non-decreasing timing by size; small=%v medium=%v large=%v", smallTotal, mediumTotal, largeTotal)
	}

	globalMaxChunk := smallMaxChunk
	if mediumMaxChunk > globalMaxChunk {
		globalMaxChunk = mediumMaxChunk
	}
	if largeMaxChunk > globalMaxChunk {
		globalMaxChunk = largeMaxChunk
	}
	t.Logf("TIMING_SUMMARY small_ms=%d medium_ms=%d large_ms=%d max_chunk_ms=%d large_completed=%t", smallTotal.Milliseconds(), mediumTotal.Milliseconds(), largeTotal.Milliseconds(), globalMaxChunk.Milliseconds(), largeStatus == StatusCompleted)
}

func TestWorker_WithPDFChunkWordRange_ClampsInvalidRange(t *testing.T) {
	w := NewWorker(&mockWorkerStore{}, &mockTranslSvc{}, &mockPDFSvc{}, nil, 0, WithPDFChunkWordRange(900, 300))

	if w.chunkMinWords != 900 {
		t.Fatalf("expected chunkMinWords=900, got %d", w.chunkMinWords)
	}
	if w.chunkMaxWords != 900 {
		t.Fatalf("expected chunkMaxWords to clamp to 900, got %d", w.chunkMaxWords)
	}
}

func TestWorker_PDFJob_ProgressCounters_ReachTotalPages(t *testing.T) {
	store := &mockWorkerStore{}
	page1 := strings.TrimSpace(strings.Repeat("one ", 120))
	page2 := strings.TrimSpace(strings.Repeat("two ", 120))
	page3 := strings.TrimSpace(strings.Repeat("three ", 120))

	pdfSvc := &mockMultiPagePDFSvc{pages: []string{page1, page2, page3}}
	transSvc := &mockPerPageTranslSvc{translations: map[string]string{
		page1: "eins",
		page2: "zwei",
		page3: "drei",
	}}

	w := NewWorker(store, transSvc, pdfSvc, nil, 0)
	job := &Job{
		ID:       "pdf-progress",
		Type:     TypePDF,
		FilePath: "/tmp/progress.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed, got %s", store.lastJob.Status)
	}
	if store.lastJob.TotalPages != 3 {
		t.Fatalf("expected total_pages=3, got %d", store.lastJob.TotalPages)
	}
	if store.lastJob.ProcessedPages != 3 {
		t.Fatalf("expected processed_pages=3, got %d", store.lastJob.ProcessedPages)
	}
	if store.callCount < 5 {
		t.Fatalf("expected multiple updates including progress updates, got callCount=%d", store.callCount)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type chunkTimeoutSplitSvc struct {
	mu       sync.Mutex
	calls    int
	payloads []string
}

func (m *chunkTimeoutSplitSvc) Translate(in services.TranslationInput) (string, error) {
	m.mu.Lock()
	m.calls++
	m.payloads = append(m.payloads, in.Text)
	m.mu.Unlock()

	if strings.Contains(in.Text, pdfChunkSep) {
		// Simulate a hung large request that only returns when the per-chunk
		// context deadline is hit.
		<-in.Ctx.Done()
		return "", in.Ctx.Err()
	}
	return "translated:" + in.Text, nil
}

func TestWorker_TranslateChunk_TimeoutSplitsAndCompletes(t *testing.T) {
	origTimeout := translateChunkTimeout
	translateChunkTimeout = 10 * time.Millisecond
	defer func() { translateChunkTimeout = origTimeout }()

	store := &mockWorkerStore{}
	svc := &chunkTimeoutSplitSvc{}
	w := NewWorker(store, svc, &mockPDFSvc{}, nil, 0)

	chunk := textChunk{
		units: []chunkUnit{
			{pageIndex: 0, text: "alpha beta", words: 2},
			{pageIndex: 1, text: "gamma delta", words: 2},
		},
		words: 4,
	}

	metrics := &retryMetrics{}
	out, err := w.translateChunk(context.Background(), chunk, "en", "de", metrics)
	if err != nil {
		t.Fatalf("expected split fallback to succeed, got err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 translated units, got %d", len(out))
	}
	if out[0].pageIndex != 0 || out[1].pageIndex != 1 {
		t.Fatalf("expected page order to be preserved, got page indexes %d,%d", out[0].pageIndex, out[1].pageIndex)
	}
	if out[0].text != "translated:alpha beta" || out[1].text != "translated:gamma delta" {
		t.Fatalf("unexpected split translation output: %#v", out)
	}

	svc.mu.Lock()
	calls := svc.calls
	payloads := append([]string(nil), svc.payloads...)
	svc.mu.Unlock()
	// New behavior: 1 timed-out call on full chunk + 2 split unit calls = 3 total
	if calls != 3 {
		t.Fatalf("expected 3 calls (1 timed-out full chunk + 2 split units), got %d", calls)
	}
	// First call should be the combined payload (which times out)
	if len(payloads) < 1 || !strings.Contains(payloads[0], pdfChunkSep) {
		t.Fatal("expected first call to be the combined payload before split")
	}
	if got := metrics.timeoutRetryCount.Load(); got != 1 {
		t.Fatalf("expected timeout_retry_count=1, got %d", got)
	}
	if got := metrics.splitCount.Load(); got != 1 {
		t.Fatalf("expected split_count=1, got %d", got)
	}
	if got := metrics.totalRetryEvents(); got != 2 {
		t.Fatalf("expected total_retry_events=2 for one timeout retry and one split, got %d", got)
	}
}

type chunkTimeoutAlwaysSvc struct {
	mu    sync.Mutex
	calls int
}

func (m *chunkTimeoutAlwaysSvc) Translate(in services.TranslationInput) (string, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	<-in.Ctx.Done()
	return "", in.Ctx.Err()
}

func TestWorker_TranslateChunk_SingleUnitTimeoutHasBoundedRetries(t *testing.T) {
	origTimeout := translateChunkTimeout
	translateChunkTimeout = 10 * time.Millisecond
	defer func() { translateChunkTimeout = origTimeout }()

	store := &mockWorkerStore{}
	svc := &chunkTimeoutAlwaysSvc{}
	w := NewWorker(store, svc, &mockPDFSvc{}, nil, 0)

	chunk := textChunk{
		units: []chunkUnit{{pageIndex: 0, text: "alpha beta", words: 2}},
		words: 2,
	}

	_, err := w.translateChunk(context.Background(), chunk, "en", "de", nil)
	if err == nil {
		t.Fatal("expected timeout error for single-unit chunk")
	}

	svc.mu.Lock()
	calls := svc.calls
	svc.mu.Unlock()
	// New behavior: fail immediately on timeout for single-unit chunks (no retries)
	if calls != 1 {
		t.Fatalf("expected 1 attempt (fail immediately on timeout) for single-unit chunk, got %d", calls)
	}
}

// ---------- isTransientNetworkError unit tests ---------------------------

// mockNetError implements net.Error with a configurable Timeout() value.
type mockNetError struct {
	msg     string
	timeout bool
}

func (e *mockNetError) Error() string   { return e.msg }
func (e *mockNetError) Timeout() bool   { return e.timeout }
func (e *mockNetError) Temporary() bool { return false }

func TestIsTransientNetworkError_NetErrorTimeout(t *testing.T) {
	err := &mockNetError{msg: "read tcp: i/o timeout", timeout: true}
	if !isTransientNetworkError(err) {
		t.Errorf("expected net.Error with Timeout()=true to be retryable")
	}
}

func TestIsTransientNetworkError_NetErrorNonTimeout(t *testing.T) {
	err := &mockNetError{msg: "connection refused", timeout: false}
	// Falls through to string matching: "connection refused" is still retryable.
	if !isTransientNetworkError(err) {
		t.Errorf("expected 'connection refused' net.Error to be retryable via string match")
	}
}

func TestIsTransientNetworkError_ClientTimeoutExceeded(t *testing.T) {
	err := errors.New("Post \"http://litellm:4000/chat/completions\": Client.Timeout exceeded while awaiting headers")
	if !isTransientNetworkError(err) {
		t.Errorf("expected 'Client.Timeout exceeded' to be retryable")
	}
}

func TestIsTransientNetworkError_ContextDeadlineExceeded(t *testing.T) {
	err := errors.New("Post \"http://litellm:4000\": context deadline exceeded")
	if !isTransientNetworkError(err) {
		t.Errorf("expected 'context deadline exceeded' to be retryable")
	}
}

func TestIsTransientNetworkError_NonRetryable(t *testing.T) {
	cases := []string{
		"internal server error",
		"invalid JSON response",
		"status 500",
	}
	for _, msg := range cases {
		err := errors.New(msg)
		if isTransientNetworkError(err) {
			t.Errorf("expected %q to NOT be retryable", msg)
		}
	}
}

func TestIsTransientNetworkError_Nil(t *testing.T) {
	if isTransientNetworkError(nil) {
		t.Error("expected nil error to return false")
	}
}

// TestWorker_TranslateWithRetry_RetriesOnNetError verifies that a net.Error
// with Timeout()=true (the type-assertion path, not the string-match path)
// triggers retry and ultimately succeeds.
type netTimeoutTranslSvc struct {
	mu        sync.Mutex
	calls     int
	failTimes int
	result    string
}

func (m *netTimeoutTranslSvc) Translate(in services.TranslationInput) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls <= m.failTimes {
		return "", &mockNetError{msg: "dial tcp: i/o timeout", timeout: true}
	}
	return m.result, nil
}

func TestWorker_TranslateWithRetry_RetriesOnNetError(t *testing.T) {
	// Override backoff to zero so the test doesn't sleep.
	orig := translateRetryBase
	translateRetryBase = 0
	defer func() { translateRetryBase = orig }()

	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: "Hello world"}
	transSvc := &netTimeoutTranslSvc{failTimes: 2, result: "Hallo Welt"}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-retry-neterr",
		Type:     TypePDF,
		FilePath: "/tmp/neterr.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed after net.Error retries, got %s (err: %s)", store.lastJob.Status, store.lastJob.ErrorMsg)
	}
	if store.lastJob.TranslatedText != "Hallo Welt" {
		t.Fatalf("expected 'Hallo Welt', got %q", store.lastJob.TranslatedText)
	}
	transSvc.mu.Lock()
	totalCalls := transSvc.calls
	transSvc.mu.Unlock()
	if totalCalls != 3 {
		t.Fatalf("expected 3 Translate calls (2 net.Error + 1 success), got %d", totalCalls)
	}
}

// TestWorker_TranslateWithRetry_ClientTimeoutString verifies that a plain
// "Client.Timeout exceeded" string error (as wrapped by the Go HTTP client
// when it does not produce a net.Error) is retried.
func TestWorker_TranslateWithRetry_ClientTimeoutString(t *testing.T) {
	orig := translateRetryBase
	translateRetryBase = 0
	defer func() { translateRetryBase = orig }()

	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: "Hello world"}
	transSvc := &transientFailTranslSvc{
		failTimes: 1,
		failErr:   "Post \"http://litellm:4000/chat/completions\": Client.Timeout exceeded while awaiting headers",
		result:    "Hallo Welt",
	}
	w := NewWorker(store, transSvc, pdfSvc, nil, 0)

	job := &Job{
		ID:       "pdf-retry-client-timeout",
		Type:     TypePDF,
		FilePath: "/tmp/client-timeout.pdf",
		Source:   "en",
		Target:   "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed after Client.Timeout retry, got %s (err: %s)", store.lastJob.Status, store.lastJob.ErrorMsg)
	}
	transSvc.mu.Lock()
	totalCalls := transSvc.calls
	transSvc.mu.Unlock()
	if totalCalls != 2 {
		t.Fatalf("expected 2 Translate calls (1 Client.Timeout + 1 success), got %d", totalCalls)
	}
}

// ---------- mode branching -----------------------------------------------

// TestWorker_EffectiveMode verifies that effectiveMode normalises "" → overlay
// and passes through valid values unchanged.
func TestWorker_EffectiveMode(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ModeOverlay},
		{ModeOverlay, ModeOverlay},
		{ModeLayout, ModeLayout},
		{ModeOCROnly, ModeOCROnly},
	}
	for _, tc := range cases {
		got := effectiveMode(tc.input)
		if got != tc.want {
			t.Errorf("effectiveMode(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestWorker_LayoutMode_MarksJobFailed verifies that the layout pipeline stub
// marks the job as failed with a descriptive error and calls store.Update.
func TestWorker_LayoutMode_MarksJobFailed(t *testing.T) {
	store := &mockWorkerStore{}
	w := NewWorker(store, &mockTranslSvc{result: "ignored"}, &mockPDFSvc{}, nil, 0)

	job := &Job{
		ID:     "text-layout-1",
		Type:   TypeText,
		Mode:   ModeLayout,
		Text:   "Hello world",
		Source: "en",
		Target: "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected store.Update to be called")
	}
	if store.lastJob.Status != StatusFailed {
		t.Fatalf("expected status=failed for layout mode stub, got %s", store.lastJob.Status)
	}
	if store.lastJob.ErrorMsg == "" {
		t.Fatal("expected non-empty ErrorMsg for layout mode stub")
	}
}

// TestWorker_LayoutMode_TranslationServiceNotCalled verifies the overlay
// translation service is NOT invoked when mode is "layout".
func TestWorker_LayoutMode_TranslationServiceNotCalled(t *testing.T) {
	store := &mockWorkerStore{}
	callCount := 0
	svc := &mockTranslSvcCounting{fn: func() { callCount++ }, result: "Hallo"}
	w := NewWorker(store, svc, &mockPDFSvc{}, nil, 0)

	job := &Job{
		ID:     "text-layout-2",
		Type:   TypeText,
		Mode:   ModeLayout,
		Text:   "Hello",
		Source: "en",
		Target: "de",
	}
	w.process(context.Background(), job)

	if callCount != 0 {
		t.Fatalf("expected 0 Translate calls for layout mode stub, got %d", callCount)
	}
}

// TestWorker_OverlayMode_TranslatesNormally verifies that mode=overlay runs
// the existing translation pipeline and completes successfully.
func TestWorker_OverlayMode_TranslatesNormally(t *testing.T) {
	store := &mockWorkerStore{}
	w := NewWorker(store, &mockTranslSvc{result: "Hallo Welt"}, &mockPDFSvc{}, nil, 0)

	job := &Job{
		ID:     "text-overlay-1",
		Type:   TypeText,
		Mode:   ModeOverlay,
		Text:   "Hello world",
		Source: "en",
		Target: "de",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected store.Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed for overlay mode, got %s (err: %s)", store.lastJob.Status, store.lastJob.ErrorMsg)
	}
	if store.lastJob.TranslatedText == "" {
		t.Fatal("expected non-empty TranslatedText for overlay mode")
	}
}

// TestWorker_EmptyMode_DefaultsToOverlay verifies that a job with no mode set
// (legacy job) is processed as overlay, not routed to the layout stub.
func TestWorker_EmptyMode_DefaultsToOverlay(t *testing.T) {
	store := &mockWorkerStore{}
	w := NewWorker(store, &mockTranslSvc{result: "Bonjour"}, &mockPDFSvc{}, nil, 0)

	job := &Job{
		ID:     "text-legacy-1",
		Type:   TypeText,
		Mode:   "", // legacy: no mode field
		Text:   "Hello",
		Source: "en",
		Target: "fr",
	}
	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected store.Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected legacy job to complete as overlay, got %s (err: %s)", store.lastJob.Status, store.lastJob.ErrorMsg)
	}
}

func TestWorker_PDFJob_OCROnly_CompletesWithoutTranslation(t *testing.T) {
	store := &mockWorkerStore{}
	pdfSvc := &mockPDFSvc{result: "ignored native text"}
	ocrSvc := &mockOCRSvc{result: "Hello from OCR"}
	transSvc := &mockTranslSvc{result: "SHOULD_NOT_BE_USED"}
	w := NewWorker(store, transSvc, pdfSvc, ocrSvc, 0)

	job := &Job{
		ID:       "pdf-ocr-only-1",
		Type:     TypePDF,
		Mode:     ModeOCROnly,
		FilePath: "/tmp/input.pdf",
		Source:   "en",
		Target:   "de",
	}

	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected store.Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed, got %s (err: %s)", store.lastJob.Status, store.lastJob.ErrorMsg)
	}
	want := "Hello from OCR"
	if store.lastJob.TranslatedText != want {
		t.Fatalf("expected OCR text in translated_text, got %q", store.lastJob.TranslatedText)
	}
	if store.lastJob.ProcessingMethod != "ocr" {
		t.Fatalf("expected processing_method=ocr, got %q", store.lastJob.ProcessingMethod)
	}
}

type mockTranslSvcCounting struct {
	fn     func()
	result string
	err    error
}

func (m *mockTranslSvcCounting) Translate(_ services.TranslationInput) (string, error) {
	m.fn()
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}
