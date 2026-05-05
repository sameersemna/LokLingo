package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	internalservices "loklingo/backend/internal/services"
	"loklingo/backend/services"
)

// Worker dequeues jobs and executes translations in the background.
type Worker struct {
	store      Store
	service    services.TranslationService
	pdfService internalservices.PDFService
	ocrClient  internalservices.OCRClient // optional fallback for image-based PDFs
}

// NewWorker constructs a Worker. ocrClient may be nil to disable OCR fallback.
func NewWorker(store Store, service services.TranslationService, pdfService internalservices.PDFService, ocrClient internalservices.OCRClient) *Worker {
	return &Worker{store: store, service: service, pdfService: pdfService, ocrClient: ocrClient}
}

// Run blocks, processing jobs until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	slog.Info("translation worker started")
	for {
		select {
		case <-ctx.Done():
			slog.Info("translation worker stopped")
			return
		default:
		}

		job, err := w.store.Dequeue(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			slog.Error("dequeue error", "err", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if job == nil {
			// BRPop timed out — loop and check ctx again.
			continue
		}

		w.process(ctx, job)
	}
}

// normalizeText collapses runs of whitespace within each paragraph while
// preserving paragraph breaks (blank lines between sections).
func normalizeText(s string) string {
	paragraphs := strings.Split(strings.TrimSpace(s), "\n\n")
	out := paragraphs[:0]
	for _, p := range paragraphs {
		normalized := strings.Join(strings.Fields(p), " ")
		if normalized != "" {
			out = append(out, normalized)
		}
	}
	return strings.Join(out, "\n\n")
}

func (w *Worker) process(ctx context.Context, job *Job) {
	slog.Info("processing translation job", "job_id", job.ID, "type", job.Type, "source", job.Source, "target", job.Target)

	// For PDF jobs: extract text from the file before translating.
	if job.Type == TypePDF {
		if job.FilePath == "" {
			slog.Error("pdf job missing file_path", "job_id", job.ID)
			job.Status = StatusFailed
			job.ErrorMsg = "file_path is required for translate_pdf jobs"
			if uerr := w.store.Update(ctx, job); uerr != nil {
				slog.Error("update job result (missing file_path)", "job_id", job.ID, "err", uerr)
			}
			return
		}
		extracted, err := w.pdfService.ExtractText(job.FilePath)
		if err != nil || extracted == "" {
			reason := "empty text"
			if err != nil {
				reason = err.Error()
			}
			if w.ocrClient == nil {
				slog.Error("pdf text extraction failed, no OCR fallback configured", "job_id", job.ID, "file", job.FilePath, "reason", reason)
				job.Status = StatusFailed
				job.ErrorMsg = fmt.Sprintf("pdf extraction failed: %s", reason)
				if uerr := w.store.Update(ctx, job); uerr != nil {
					slog.Error("update job result (pdf extraction failed)", "job_id", job.ID, "err", uerr)
				}
				return
			}
			slog.Warn("pdf text extraction failed, falling back to OCR", "job_id", job.ID, "file", job.FilePath, "reason", reason)
			ocred, ocrErr := w.ocrClient.ExtractText(job.FilePath, job.Lang)
			if ocrErr != nil {
				slog.Error("OCR fallback also failed", "job_id", job.ID, "file", job.FilePath, "err", ocrErr)
				job.Status = StatusFailed
				job.ErrorMsg = fmt.Sprintf("pdf extraction failed: %s; OCR fallback failed: %v", reason, ocrErr)
				if uerr := w.store.Update(ctx, job); uerr != nil {
					slog.Error("update job result (OCR fallback failed)", "job_id", job.ID, "err", uerr)
				}
				return
			}
			slog.Info("ocr_fallback_triggered",
				"job_id", job.ID,
				"file", job.FilePath,
				"lang", job.Lang,
				"target", job.Target,
				"extraction_failure_reason", reason,
			)
			extracted = ocred
			job.ProcessingMethod = "ocr"
		} else {
			job.ProcessingMethod = "pdf_text"
		}
		job.Text = normalizeText(extracted)
	}

	// Check translation cache before calling the LLM.
	if cached, err := w.store.GetCached(ctx, job.Text, job.Source, job.Target); err == nil {
		slog.Info("cache hit", "job_id", job.ID)
		job.Status = StatusCompleted
		job.TranslatedText = cached
		if err := w.store.Update(ctx, job); err != nil {
			slog.Error("update job result (cache hit)", "job_id", job.ID, "err", err)
		}
		slog.Info("job finished (cached)", "job_id", job.ID)
		return
	}

	job.Status = StatusProcessing
	if err := w.store.Update(ctx, job); err != nil {
		slog.Error("update job to processing", "job_id", job.ID, "err", err)
	}

	translated, err := w.service.Translate(services.TranslationInput{
		Text:   job.Text,
		Source: job.Source,
		Target: job.Target,
		Ctx:    ctx,
	})
	if err != nil {
		slog.Error("translation failed", "job_id", job.ID, "err", err)
		job.Status = StatusFailed
		job.ErrorMsg = err.Error()
	} else {
		job.Status = StatusCompleted
		job.TranslatedText = translated
		// Populate cache so future identical requests skip the LLM.
		if cerr := w.store.SetCached(ctx, job.Text, job.Source, job.Target, translated); cerr != nil {
			slog.Warn("failed to cache translation", "job_id", job.ID, "err", cerr)
		}
	}

	if err := w.store.Update(ctx, job); err != nil {
		slog.Error("update job result", "job_id", job.ID, "err", err)
	}
	slog.Info("job finished", "job_id", job.ID, "status", job.Status)
}
