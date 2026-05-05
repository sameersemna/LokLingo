package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	internalservices "loklingo/backend/internal/services"
	"loklingo/backend/services"
)

// Worker dequeues jobs and executes translations in the background.
type Worker struct {
	store                Store
	service              services.TranslationService
	pdfService           internalservices.PDFService
	ocrClient            internalservices.OCRClient // optional fallback for image-based PDFs
	maxPDFPages          int
	translateConcurrency int           // semaphore width for chunk-level LLM calls per job
	globalLLMSem         chan struct{} // shared semaphore: max total concurrent LLM calls across all jobs
	chunkMinWords        int           // preferred minimum words for a PDF translation chunk
	chunkMaxWords        int           // hard cap words for a PDF translation chunk
}

// WorkerOption is a functional option for NewWorker.
type WorkerOption func(*Worker)

// WithTranslateConcurrency sets the maximum number of concurrent chunk-translation
// goroutines. Must be ≥ 1; values ≤ 0 are ignored (default of 3 is kept).
func WithTranslateConcurrency(n int) WorkerOption {
	return func(w *Worker) {
		if n > 0 {
			w.translateConcurrency = n
		}
	}
}

// WithPDFChunkWordRange configures chunk size for PDF translation.
// Values <= 0 are ignored. If maxWords < minWords, maxWords is clamped up to minWords.
func WithPDFChunkWordRange(minWords, maxWords int) WorkerOption {
	return func(w *Worker) {
		if minWords > 0 {
			w.chunkMinWords = minWords
		}
		if maxWords > 0 {
			w.chunkMaxWords = maxWords
		}
		if w.chunkMaxWords < w.chunkMinWords {
			w.chunkMaxWords = w.chunkMinWords
		}
	}
}

// WithMaxLLMConcurrency sets the global maximum number of concurrent LLM calls
// across all jobs. Must be ≥ 1; values ≤ 0 are ignored.
func WithMaxLLMConcurrency(n int) WorkerOption {
	return func(w *Worker) {
		if n > 0 && n != cap(w.globalLLMSem) {
			// Create a new semaphore with the specified capacity
			w.globalLLMSem = make(chan struct{}, n)
		}
	}
}

// NewWorker constructs a Worker. ocrClient may be nil to disable OCR fallback.
// Additional behaviour can be tuned via WorkerOption values.
func NewWorker(store Store, service services.TranslationService, pdfService internalservices.PDFService, ocrClient internalservices.OCRClient, maxPDFPages int, opts ...WorkerOption) *Worker {
	if maxPDFPages <= 0 {
		maxPDFPages = 300
	}
	w := &Worker{
		store:                store,
		service:              service,
		pdfService:           pdfService,
		ocrClient:            ocrClient,
		maxPDFPages:          maxPDFPages,
		translateConcurrency: pdfTranslateConcurrency,
		globalLLMSem:         make(chan struct{}, 10), // default global concurrency = 10
		chunkMinWords:        pdfChunkMinWords,
		chunkMaxWords:        pdfChunkMaxWords,
	}
	for _, o := range opts {
		o(w)
	}
	return w
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

// pdfTranslateConcurrency is the default maximum number of page-translation goroutines
// that may be in-flight simultaneously. Override at construction via WithTranslateConcurrency.
const pdfTranslateConcurrency = 3

// translateMaxRetries is the maximum number of retry attempts after a retryable
// error (e.g. HTTP 429 rate-limit) before the page translation is failed.
const translateMaxRetries = 3

// translateRetryBase is the initial back-off delay before the first retry.
// Each subsequent retry doubles the delay (capped at translateRetryBase * 2^retries).
const translateRetryBase = 500 * time.Millisecond

// pdfChunkMinWords and pdfChunkMaxWords define the target PDF translation chunk
// size in words. Chunks are built from contiguous page text units to reduce LLM
// call count while keeping inputs within a safer token budget.
const pdfChunkMinWords = 500
const pdfChunkMaxWords = 1000

const pdfChunkSep = "\n\n[[[LK_PAGE_BREAK]]]\n\n"

type chunkUnit struct {
	pageIndex int
	text      string
	words     int
}

type textChunk struct {
	units []chunkUnit
	words int
}

// isRateLimitError reports whether err looks like an HTTP 429 / rate-limit
// response from LiteLLM.  The service wraps the status as a formatted string
// so we match on substrings rather than a sentinel error type.
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "litellm status 429") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "rate_limit")
}

// translateWithRetry calls w.service.Translate with exponential back-off on
// rate-limit errors.  Non-retryable errors are returned immediately.
// Acquires a slot from the global LLM semaphore before calling the service.
// retries is incremented (atomically) each time a retry is issued; it may be nil.
func (w *Worker) translateWithRetry(ctx context.Context, text, source, target string, retries *atomic.Int64) (string, error) {
	// Acquire global LLM concurrency slot; respect context cancellation.
	select {
	case w.globalLLMSem <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-w.globalLLMSem }()

	input := services.TranslationInput{
		Text:   text,
		Source: source,
		Target: target,
		Ctx:    ctx,
	}
	delay := translateRetryBase
	for attempt := 0; ; attempt++ {
		result, err := w.service.Translate(input)
		if err == nil {
			return result, nil
		}
		if !isRateLimitError(err) || attempt >= translateMaxRetries {
			return "", err
		}
		if retries != nil {
			retries.Add(1)
		}
		slog.Warn("translate rate-limited, backing off",
			"attempt", attempt+1,
			"backoff_ms", delay.Milliseconds(),
		)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
		delay *= 2
	}
}

func splitByWordBudget(text string, maxWords int) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	words := strings.Fields(text)
	if len(words) <= maxWords {
		return []string{strings.Join(words, " ")}
	}
	parts := make([]string, 0, (len(words)+maxWords-1)/maxWords)
	for i := 0; i < len(words); i += maxWords {
		end := i + maxWords
		if end > len(words) {
			end = len(words)
		}
		parts = append(parts, strings.Join(words[i:end], " "))
	}
	return parts
}

func buildChunkUnits(pages []string, maxWords int) []chunkUnit {
	units := make([]chunkUnit, 0, len(pages))
	for pageIdx, pageText := range pages {
		parts := splitByWordBudget(pageText, maxWords)
		for _, part := range parts {
			units = append(units, chunkUnit{
				pageIndex: pageIdx,
				text:      part,
				words:     len(strings.Fields(part)),
			})
		}
	}
	return units
}

func buildTextChunks(units []chunkUnit, minWords, maxWords int) []textChunk {
	if len(units) == 0 {
		return nil
	}
	chunks := make([]textChunk, 0, len(units))
	current := textChunk{units: make([]chunkUnit, 0, 8)}
	for _, unit := range units {
		if len(current.units) > 0 && current.words >= minWords && current.words+unit.words > maxWords {
			chunks = append(chunks, current)
			current = textChunk{units: make([]chunkUnit, 0, 8)}
		}
		current.units = append(current.units, unit)
		current.words += unit.words
	}
	if len(current.units) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

func (w *Worker) translateChunk(ctx context.Context, chunk textChunk, source, target string, retries *atomic.Int64) ([]chunkUnit, error) {
	if len(chunk.units) == 1 {
		translated, err := w.translateWithRetry(ctx, chunk.units[0].text, source, target, retries)
		if err != nil {
			return nil, err
		}
		return []chunkUnit{{
			pageIndex: chunk.units[0].pageIndex,
			text:      translated,
			words:     len(strings.Fields(translated)),
		}}, nil
	}

	parts := make([]string, 0, len(chunk.units))
	for _, u := range chunk.units {
		parts = append(parts, u.text)
	}
	payload := strings.Join(parts, pdfChunkSep)
	translated, err := w.translateWithRetry(ctx, payload, source, target, retries)
	if err != nil {
		return nil, err
	}

	split := strings.Split(translated, pdfChunkSep)
	if len(split) != len(chunk.units) {
		// Fallback: if the model modified the delimiter, translate units one-by-one.
		out := make([]chunkUnit, 0, len(chunk.units))
		for _, u := range chunk.units {
			part, perr := w.translateWithRetry(ctx, u.text, source, target, retries)
			if perr != nil {
				return nil, perr
			}
			out = append(out, chunkUnit{pageIndex: u.pageIndex, text: part, words: len(strings.Fields(part))})
		}
		return out, nil
	}

	out := make([]chunkUnit, 0, len(split))
	for i, s := range split {
		out = append(out, chunkUnit{
			pageIndex: chunk.units[i].pageIndex,
			text:      strings.TrimSpace(s),
			words:     len(strings.Fields(s)),
		})
	}
	return out, nil
}

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
	var retryCount atomic.Int64
	var chunkCount int

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
		job.TotalPages = len(pages)
		job.ProcessedPages = 0
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
		if job.Type == TypePDF {
			job.ProcessedPages = job.TotalPages
		}
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
	type pageResult struct {
		units []chunkUnit
		err   error
	}
	units := buildChunkUnits(pages, w.chunkMaxWords)
	chunks := buildTextChunks(units, w.chunkMinWords, w.chunkMaxWords)
	chunkCount = len(chunks)
	unitTargetsByPage := make([]int, len(pages))
	for _, u := range units {
		if u.pageIndex >= 0 && u.pageIndex < len(unitTargetsByPage) {
			unitTargetsByPage[u.pageIndex]++
		}
	}
	unitDoneByPage := make([]int, len(pages))
	results := make([]pageResult, len(chunks))
	sem := make(chan struct{}, w.translateConcurrency)
	var wg sync.WaitGroup
	var progressMu sync.Mutex
	for i, chunk := range chunks {
		wg.Add(1)
		go func(idx int, c textChunk) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[idx] = pageResult{err: ctx.Err()}
				return
			}
			defer func() { <-sem }()
			chunkStart := time.Now()
			translatedUnits, err := w.translateChunk(ctx, c, job.Source, job.Target, &retryCount)
			chunkDurMs := time.Since(chunkStart).Milliseconds()
			if err != nil {
				slog.Error("chunk_translation_failed",
					"job_id", job.ID,
					"job_type", job.Type,
					"chunk_idx", idx,
					"duration_ms", chunkDurMs,
					"err", err,
				)
			} else {
				slog.Info("chunk_translated",
					"job_id", job.ID,
					"job_type", job.Type,
					"chunk_idx", idx,
					"chunk_words", c.words,
					"duration_ms", chunkDurMs,
				)
			}
			if err == nil && job.Type == TypePDF {
				progressMu.Lock()
				for _, u := range translatedUnits {
					if u.pageIndex < 0 || u.pageIndex >= len(unitDoneByPage) {
						continue
					}
					unitDoneByPage[u.pageIndex]++
					if unitDoneByPage[u.pageIndex] == unitTargetsByPage[u.pageIndex] {
						job.ProcessedPages++
						if uerr := w.store.Update(ctx, job); uerr != nil {
							slog.Error("update pdf progress", "job_id", job.ID, "processed_pages", job.ProcessedPages, "err", uerr)
						}
					}
				}
				progressMu.Unlock()
			}
			results[idx] = pageResult{units: translatedUnits, err: err}
		}(i, chunk)
	}
	wg.Wait()
	translatedPageParts := make([][]string, len(pages))
	var translateErr error
	for _, r := range results {
		if r.err != nil {
			translateErr = r.err
			break
		}
		for _, u := range r.units {
			if u.pageIndex >= 0 && u.pageIndex < len(translatedPageParts) {
				translatedPageParts[u.pageIndex] = append(translatedPageParts[u.pageIndex], u.text)
			}
		}
	}
	translatedPages := make([]string, 0, len(pages))
	if translateErr == nil {
		for _, parts := range translatedPageParts {
			translatedPages = append(translatedPages, strings.TrimSpace(strings.Join(parts, " ")))
		}
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
	slog.Info("job_finished",
		"job_id", job.ID,
		"job_type", job.Type,
		"status", job.Status,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"chunk_count", chunkCount,
		"retry_count", retryCount.Load(),
	)
}
