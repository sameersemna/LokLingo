package jobs

import (
	"context"
	"errors"
	"os"
	"testing"

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
