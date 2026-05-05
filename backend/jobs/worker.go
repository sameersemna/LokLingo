package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	internalservices "loklingo/backend/internal/services"
	"loklingo/backend/services"
)

// Worker dequeues jobs and executes translations in the background.
type Worker struct {
	store       Store
	service     services.TranslationService
	pdfService  internalservices.PDFService
	ocrClient   internalservices.OCRClient // optional fallback for image-based PDFs
	maxPDFPages int
}

// NewWorker constructs a Worker. ocrClient may be nil to disable OCR fallback.
func NewWorker(store Store, service services.TranslationService, pdfService internalservices.PDFService, ocrClient internalservices.OCRClient, maxPDFPages int) *Worker {
	if maxPDFPages <= 0 {
		maxPDFPages = 300
	}
	return &Worker{store: store, service: service, pdfService: pdfService, ocrClient: ocrClient, maxPDFPages: maxPDFPages}
}

// Run blocks, processing jobs until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	slog.Info("translation worker started")

	// Remove stale upload files left by a previous crashed worker run.
	cleanStaleUploads(PDFUploadDir, 24*time.Hour)

	// Periodic cleanup: every hour remove uploads older than 24 h.
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanStaleUploads(PDFUploadDir, 24*time.Hour)
			}
		}
	}()

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

// normalizePages applies normalizeText to each page and discards empty results.
func normalizePages(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		if n := normalizeText(p); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// PDFUploadDir is the shared location where CreatePDFJob writes uploaded files.
// Workers read from here; both sides must agree on this path.
const PDFUploadDir = "/tmp/loklingo"

// cleanStaleUploads removes files under dir that are older than maxAge.
// Errors are logged but never propagated — this is best-effort housekeeping.
func cleanStaleUploads(dir string, maxAge time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("stale_pdf_cleanup: cannot read upload dir", "dir", dir, "err", err)
		}
		return
	}
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, e.Name())
			if !strings.HasSuffix(path, ".pdf") {
				// Only remove files we created; ignore anything else in /tmp/loklingo.
				continue
			}
			if rerr := os.Remove(path); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
				slog.Warn("stale_pdf_cleanup: remove failed", "file", path, "err", rerr)
			} else {
				removed++
			}
		}
	}
	if removed > 0 {
		slog.Info("stale_pdf_cleanup: removed stale uploads", "dir", dir, "count", removed)
	}
}

func (w *Worker) process(ctx context.Context, job *Job) {
	// Best-effort: remove the uploaded PDF once the job reaches a terminal state.
	defer func() {
		if job.Type == TypePDF && job.FilePath != "" {
			if err := os.Remove(job.FilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				slog.Warn("pdf_cleanup_failed", "job_id", job.ID, "file", job.FilePath, "err", err)
			}
		}
	}()

	startedAt := time.Now()
	var extractMS int64
	var ocrMS int64
	var translateMS int64
	var ocrTriggered bool
	var ocrReason string
	var ocrOutcome string

	slog.Info("processing translation job", "job_id", job.ID, "type", job.Type, "source", job.Source, "target", job.Target)

	// pages holds per-page normalized text used for translation.
	// For PDF jobs this is populated during extraction; for text jobs it wraps job.Text.
	var pages []string

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
		if w.maxPDFPages > 0 {
			pageCount, pageErr := w.pdfService.PageCount(job.FilePath)
			if pageErr != nil {
				slog.Warn("pdf page count unavailable", "job_id", job.ID, "file", job.FilePath, "err", pageErr)
			} else if pageCount > w.maxPDFPages {
				slog.Warn("pdf_guardrail_rejected", "job_id", job.ID, "file", job.FilePath, "page_count", pageCount, "max_pdf_pages", w.maxPDFPages)
				job.Status = StatusFailed
				job.ErrorMsg = fmt.Sprintf("pdf has %d pages; max allowed is %d", pageCount, w.maxPDFPages)
				if uerr := w.store.Update(ctx, job); uerr != nil {
					slog.Error("update job result (pdf guardrail rejected)", "job_id", job.ID, "err", uerr)
				}
				return
			}
		}
		extractStart := time.Now()
		pdfPages, err := w.pdfService.ExtractPages(job.FilePath)
		extractMS = time.Since(extractStart).Milliseconds()
		pages = normalizePages(pdfPages)
		if err != nil || len(pages) == 0 {
			reason := "empty text"
			if err != nil {
				reason = err.Error()
			}
			ocrReason = reason
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
			ocrTriggered = true
			ocrStart := time.Now()
			ocrPages, ocrErr := w.ocrClient.ExtractPages(job.FilePath, job.Lang)
			ocrMS = time.Since(ocrStart).Milliseconds()
			if ocrErr != nil {
				slog.Error("OCR fallback also failed", "job_id", job.ID, "file", job.FilePath, "err", ocrErr)
				job.Status = StatusFailed
				job.ErrorMsg = fmt.Sprintf("pdf extraction failed: %s; OCR fallback failed: %v", reason, ocrErr)
				ocrOutcome = "also_failed"
				slog.Info("ocr_fallback_triggered",
					"job_id", job.ID,
					"file", job.FilePath,
					"lang", job.Lang,
					"target", job.Target,
					"extraction_failure_reason", ocrReason,
					"outcome", ocrOutcome,
					"extract_ms", extractMS,
					"ocr_ms", ocrMS,
					"translate_ms", translateMS,
					"total_ms", time.Since(startedAt).Milliseconds(),
				)
				if uerr := w.store.Update(ctx, job); uerr != nil {
					slog.Error("update job result (OCR fallback failed)", "job_id", job.ID, "err", uerr)
				}
				return
			}
			pages = normalizePages(ocrPages)
			job.ProcessingMethod = "ocr"
		} else {
			job.ProcessingMethod = "pdf_text"
		}
		job.Text = strings.Join(pages, "\n\n")
	}

	// For non-PDF jobs job.Text is already set; wrap it as a single page so the
	// translation loop below is uniform across all job types.
	if job.Type != TypePDF {
		pages = []string{job.Text}
	}

	// Check translation cache before calling the LLM.
	if cached, err := w.store.GetCached(ctx, job.Text, job.Source, job.Target); err == nil {
		slog.Info("cache hit", "job_id", job.ID)
		job.Status = StatusCompleted
		job.TranslatedText = cached
		if ocrTriggered {
			ocrOutcome = "cached"
			slog.Info("ocr_fallback_triggered",
				"job_id", job.ID,
				"file", job.FilePath,
				"lang", job.Lang,
				"target", job.Target,
				"extraction_failure_reason", ocrReason,
				"outcome", ocrOutcome,
				"extract_ms", extractMS,
				"ocr_ms", ocrMS,
				"translate_ms", translateMS,
				"total_ms", time.Since(startedAt).Milliseconds(),
			)
		}
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

	translateStart := time.Now()
	translatedPages := make([]string, 0, len(pages))
	var translateErr error
	for _, pageText := range pages {
		var part string
		part, translateErr = w.service.Translate(services.TranslationInput{
			Text:   pageText,
			Source: job.Source,
			Target: job.Target,
			Ctx:    ctx,
		})
		if translateErr != nil {
			break
		}
		translatedPages = append(translatedPages, part)
	}
	translated := strings.Join(translatedPages, "\n\n")
	translateMS = time.Since(translateStart).Milliseconds()
	if translateErr != nil {
		slog.Error("translation failed", "job_id", job.ID, "err", translateErr)
		job.Status = StatusFailed
		job.ErrorMsg = translateErr.Error()
		if ocrTriggered {
			ocrOutcome = "translation_failed"
		}
	} else {
		job.Status = StatusCompleted
		job.TranslatedText = translated
		if ocrTriggered {
			ocrOutcome = "succeeded"
		}
		// Populate cache so future identical requests skip the LLM.
		if cerr := w.store.SetCached(ctx, job.Text, job.Source, job.Target, translated); cerr != nil {
			slog.Warn("failed to cache translation", "job_id", job.ID, "err", cerr)
		}
	}

	if err := w.store.Update(ctx, job); err != nil {
		slog.Error("update job result", "job_id", job.ID, "err", err)
	}
	if ocrTriggered {
		slog.Info("ocr_fallback_triggered",
			"job_id", job.ID,
			"file", job.FilePath,
			"lang", job.Lang,
			"target", job.Target,
			"extraction_failure_reason", ocrReason,
			"outcome", ocrOutcome,
			"extract_ms", extractMS,
			"ocr_ms", ocrMS,
			"translate_ms", translateMS,
			"total_ms", time.Since(startedAt).Milliseconds(),
		)
	}
	slog.Info("job finished", "job_id", job.ID, "status", job.Status)
}
