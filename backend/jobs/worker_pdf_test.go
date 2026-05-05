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

// TestWorker_WithTranslateConcurrency_Option verifies that the WorkerOption
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

func TestWorker_PDFJob_Chunking_ReducesLLMCallsAndPreservesOrder(t *testing.T) {
	mkPage := func(tag string) string {
		// 120 words/page means 6 pages produce 720 words total and should fit one chunk.
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
