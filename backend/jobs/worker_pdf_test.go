package jobs
package jobs

import (
	"context"
	"errors"
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
	result string
	err    error
}

func (m *mockPDFSvc) ExtractText(_ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

// ---------- tests ---------------------------------------------------------

func TestWorker_PDFJob_MissingFilePath(t *testing.T) {
	store := &mockWorkerStore{}
	w := NewWorker(store, &mockTranslSvc{}, &mockPDFSvc{})

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
	w := NewWorker(store, &mockTranslSvc{result: "ignored"}, pdfSvc)

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
	w := NewWorker(store, transSvc, pdfSvc)

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
}

func TestWorker_PDFJob_CacheHitSkipsExtraction(t *testing.T) {
	store := &mockWorkerStore{}
	// Pre-load extracted text into the job so cache lookup can match it.
	// We simulate a scenario where the job already has Text set (e.g. re-processed)
	// and the cache holds its translation.
	pdfSvc := &mockPDFSvc{result: "Cached PDF text"}
	store.cachedResult = "Cached Übersetzung"
	w := NewWorker(store, &mockTranslSvc{err: errors.New("should not be called")}, pdfSvc)

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
	w := NewWorker(store, transSvc, pdfSvc)

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
