package jobs

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"loklingo/backend/internal/observability"
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

type storeAcker interface {
	Ack(ctx context.Context, id string) error
}

type staleRecoverer interface {
	RecoverStale(ctx context.Context, olderThan time.Duration) (int, error)
}

type retryRequeuer interface {
	Requeue(ctx context.Context, id string, delay time.Duration) error
}

type deadLetterer interface {
	DeadLetter(ctx context.Context, id string, reason string) error
}

type chunkCheckpointStore interface {
	GetChunkCheckpoint(ctx context.Context, jobID, chunkKey string) ([]chunkUnit, bool, error)
	SetChunkCheckpoint(ctx context.Context, jobID, chunkKey string, units []chunkUnit) error
}

type chunkCheckpointLifecycleStore interface {
	ClearChunkCheckpoints(ctx context.Context, jobID string) error
}

type ocrPageConfidenceExtractor interface {
	ExtractPagesWithConfidence(filePath, lang string) ([]string, float64, error)
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

func applyLayoutModeBBoxOptions(opts *internalservices.OverlayOptions, requestedPadding int) {
	// Keep full OCR geometry in layout mode, but avoid edge collisions with
	// a small internal text padding window.
	opts.EraseBBox = true
	opts.BboxShrinkPx = 0
	opts.TextPadding = 3
	// Layout mode prefers gentler seam blending to preserve structure.
	opts.PatchFeatherPx = 1
	opts.PatchBlurRadius = 1
	if requestedPadding >= 0 {
		switch {
		case requestedPadding < 2:
			opts.TextPadding = 2
		case requestedPadding > 4:
			opts.TextPadding = 4
		default:
			opts.TextPadding = requestedPadding
		}
	}
}

func writeImagePassthroughOutput(imagePath string) (string, error) {
	ext := filepath.Ext(imagePath)
	outPath := strings.TrimSuffix(imagePath, ext) + "_translated" + ext

	in, err := os.Open(imagePath)
	if err != nil {
		return "", fmt.Errorf("open source image: %w", err)
	}
	defer in.Close()

	out, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create passthrough image: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return "", fmt.Errorf("copy passthrough image: %w", err)
	}

	return outPath, nil
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

	if recoverer, ok := w.store.(staleRecoverer); ok {
		if moved, err := recoverer.RecoverStale(ctx, 10*time.Minute); err != nil {
			slog.Warn("recover_stale_inflight_failed", "err", err)
		} else if moved > 0 {
			slog.Info("recovered_stale_inflight_jobs", "count", moved)
		}
	}

	go func() {
		recoverTicker := time.NewTicker(1 * time.Minute)
		defer recoverTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-recoverTicker.C:
				recoverer, ok := w.store.(staleRecoverer)
				if !ok {
					continue
				}
				if moved, err := recoverer.RecoverStale(ctx, 10*time.Minute); err != nil {
					slog.Warn("recover_stale_inflight_failed", "err", err)
				} else if moved > 0 {
					slog.Info("recovered_stale_inflight_jobs", "count", moved)
				}
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
		if job.Status == StatusFailed {
			w.handleFailedJob(ctx, job)
		}
		w.cleanupTerminalChunkCheckpoints(ctx, job)
		if acker, ok := w.store.(storeAcker); ok {
			if err := acker.Ack(ctx, job.ID); err != nil {
				slog.Warn("ack_inflight_job_failed", "job_id", job.ID, "err", err)
			}
		}
	}
}

func (w *Worker) cleanupTerminalChunkCheckpoints(ctx context.Context, job *Job) {
	if job == nil || job.Type != TypePDF {
		return
	}
	if job.Status != StatusCompleted && job.Status != StatusFailed {
		return
	}
	lifecycleStore, ok := w.store.(chunkCheckpointLifecycleStore)
	if !ok {
		return
	}
	if err := lifecycleStore.ClearChunkCheckpoints(ctx, job.ID); err != nil {
		observability.IncChunkCheckpointClearFailure()
		slog.Warn("chunk_checkpoint_cleanup_failed", "job_id", job.ID, "status", job.Status, "err", err)
		return
	}
	observability.IncChunkCheckpointClear()
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

// ImageUploadDir is the shared location where CreateImageJob writes uploaded files.
const ImageUploadDir = "/tmp/loklingo/images"

// pdfTranslateConcurrency is the default maximum number of page-translation goroutines
// that may be in-flight simultaneously. Override at construction via WithTranslateConcurrency.
const pdfTranslateConcurrency = 3

// translateMaxRetries is the maximum number of retry attempts after a retryable
// error (e.g. HTTP 429 rate-limit) before the page translation is failed.
const translateMaxRetries = 3

// translateChunkTimeout is the hard per-chunk deadline for a single translation
// request. If exceeded, the chunk is immediately cancelled and split into halves
// (split-before-retry). 25s bounds worst-case chunk latency while giving the LLM
// enough headroom on loaded hosts; most chunks complete in 3–12s.
// Kept as a variable so tests can temporarily override it.
var translateChunkTimeout = 25 * time.Second

// translateRetryBase is the initial back-off delay before the first retry.
// Each subsequent retry doubles the delay (capped at translateRetryBase * 2^retries).
// Kept as a variable so tests can temporarily override it.
var translateRetryBase = 500 * time.Millisecond

// slowChunkThreshold is the soft per-chunk latency threshold. Chunks that exceed
// this value are logged as slow but are not cancelled or split. Set below the
// hard translateChunkTimeout to surface latency regressions before they reach
// the deadline. Kept as a variable so tests can temporarily override it.
var slowChunkThreshold = 15 * time.Second

// pdfChunkMinWords and pdfChunkMaxWords define the target PDF translation chunk
// size in words. Chunks are built from contiguous page text units to reduce LLM
// per-chunk latency and variance while keeping inputs within a safe token budget.
// Target: 80–150 words per chunk; hard timeout + split-before-retry keep outliers bounded.
const pdfChunkMinWords = 80
const pdfChunkMaxWords = 150

const pdfChunkSep = "\n\n[[[LK_PAGE_BREAK]]]\n\n"

const defaultJobMaxAttempts = 3
const maxRetryDelay = 60 * time.Second
const retryJitterFraction = 0.25
const queueOverloadDepthThreshold = int64(200)
const retryBacklogOverloadThreshold = int64(120)
const stuckJobsOverloadThreshold = int64(20)
const severeQueueOverloadDepthThreshold = int64(400)
const severeRetryBacklogThreshold = int64(240)
const severeStuckJobsThreshold = int64(40)
const lowQueueDepthThreshold = int64(30)
const lowRetryBacklogThreshold = int64(10)
const adaptiveTranslateConcurrencyCeiling = 6
const lowOCRConfidenceThreshold = 0.72

type chunkUnit struct {
	pageIndex int
	text      string
	words     int
}

type textChunk struct {
	units []chunkUnit
	words int
}

type retryMetrics struct {
	translateRetryCount atomic.Int64
	timeoutRetryCount   atomic.Int64
	splitCount          atomic.Int64
}

func (m *retryMetrics) incTranslateRetry() {
	if m != nil {
		m.translateRetryCount.Add(1)
	}
}

func (m *retryMetrics) incTimeoutRetry() {
	if m != nil {
		m.timeoutRetryCount.Add(1)
	}
}

func (m *retryMetrics) incSplit() {
	if m != nil {
		m.splitCount.Add(1)
	}
}

func (m *retryMetrics) totalRetryEvents() int64 {
	if m == nil {
		return 0
	}
	return m.translateRetryCount.Load() + m.timeoutRetryCount.Load() + m.splitCount.Load()
}

const minAdaptiveChunkWordsFloor = 40

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

// isTransientNetworkError reports whether err is a transient network or timeout
// failure that is safe to retry.  It matches common patterns produced by the
// Go HTTP client and upstream proxies.  Importantly, it does NOT treat a
// cancelled job context as retryable — callers must check ctx.Err() separately
// before deciding to retry.
func isTransientNetworkError(err error) bool {
	if err == nil {
		return false
	}
	// net.Error with Timeout() covers http.Client deadline/timeout errors,
	// including "Client.Timeout exceeded while awaiting headers" and i/o timeouts.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "Client.Timeout exceeded") ||
		strings.Contains(msg, "net/http: request canceled") ||
		strings.Contains(msg, "connection timed out") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "timed out") ||
		strings.Contains(msg, "timeout")
}

// translateWithRetry calls w.service.Translate with exponential back-off on
// rate-limit and transient network errors.  Non-retryable errors are returned
// immediately.  If the job context is already cancelled or expired the call
// returns ctx.Err() without retrying.
// Acquires a slot from the global LLM semaphore before calling the service.
// metrics is incremented (atomically) each time a retry/split event is issued; it may be nil.
func (w *Worker) translateWithRetry(ctx context.Context, text, source, target string, metrics *retryMetrics) (string, error) {
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
		isRateLimit := isRateLimitError(err)
		isNetTimeout := isTransientNetworkError(err)
		isRetryable := isRateLimit || isNetTimeout
		// Never retry if the job context is already done, or we've exhausted attempts.
		if !isRetryable || attempt >= translateMaxRetries || ctx.Err() != nil {
			return "", err
		}
		metrics.incTranslateRetry()
		observability.EmitLifecycleEventFromContext(ctx, "chunk_retry", observability.LifecycleEvent{Provider: "translation", RetryCount: int64(attempt + 1)})
		errKind := "rate_limit"
		if isNetTimeout {
			errKind = "network_timeout"
		}
		slog.Warn("translate transient error, backing off",
			"attempt", attempt+1,
			"max_attempts", translateMaxRetries+1,
			"backoff_ms", delay.Milliseconds(),
			"err_kind", errKind,
			"err", err,
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

func chunkCheckpointKey(chunk textChunk, source, target string) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\x00%s", source, target)
	for _, u := range chunk.units {
		_, _ = fmt.Fprintf(h, "\x00%d\x00%s", u.pageIndex, u.text)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
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

func reduceChunkWordRange(minWords, maxWords int) (int, int) {
	if minWords <= 0 {
		minWords = minAdaptiveChunkWordsFloor
	}
	if maxWords < minWords {
		maxWords = minWords
	}
	newMin := int(float64(minWords) * 0.8)
	newMax := int(float64(maxWords) * 0.8)
	if newMin < minAdaptiveChunkWordsFloor {
		newMin = minAdaptiveChunkWordsFloor
	}
	if newMin > minWords {
		newMin = minWords
	}
	if newMax < newMin {
		newMax = newMin
	}
	return newMin, newMax
}

func isChunkTimeoutError(parentCtx context.Context, chunkCtx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if parentCtx != nil && parentCtx.Err() != nil {
		return false
	}
	if chunkCtx != nil && errors.Is(chunkCtx.Err(), context.DeadlineExceeded) {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context deadline exceeded")
}

func isUnrecoverableChunkError(parentCtx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if parentCtx != nil && parentCtx.Err() != nil {
		return true
	}
	return errors.Is(err, context.Canceled)
}

func sumUnitWords(units []chunkUnit) int {
	total := 0
	for _, u := range units {
		total += u.words
	}
	return total
}

func (w *Worker) translateWithChunkTimeout(ctx context.Context, text, source, target string, metrics *retryMetrics) (string, error) {
	timeout := translateChunkTimeout
	if timeout <= 0 {
		return w.translateWithRetry(ctx, text, source, target, metrics)
	}
	chunkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return w.translateWithRetry(chunkCtx, text, source, target, metrics)
}

func (w *Worker) translateChunk(ctx context.Context, chunk textChunk, source, target string, metrics *retryMetrics) ([]chunkUnit, error) {
	if len(chunk.units) == 1 {
		// Single-unit chunks: cannot split further, so fail immediately on timeout
		// (do not retry the same slow chunk).
		translated, err := w.translateWithChunkTimeout(ctx, chunk.units[0].text, source, target, metrics)
		if err == nil {
			return []chunkUnit{{
				pageIndex: chunk.units[0].pageIndex,
				text:      translated,
				words:     len(strings.Fields(translated)),
			}}, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Log timeout failure
		if isChunkTimeoutError(ctx, nil, err) {
			slog.Warn("single_unit_chunk_timeout_fail",
				"chunk_words", chunk.words,
				"timeout_s", int(translateChunkTimeout.Seconds()),
				"err", err,
			)
		}
		return nil, err
	}

	// Multi-unit chunks: on timeout, immediately split (do not retry the same slow chunk).
	parts := make([]string, 0, len(chunk.units))
	for _, u := range chunk.units {
		parts = append(parts, u.text)
	}
	payload := strings.Join(parts, pdfChunkSep)

	chunkCtx, cancel := context.WithTimeout(ctx, translateChunkTimeout)
	translated, err := w.translateWithRetry(chunkCtx, payload, source, target, metrics)
	cancel()

	if err == nil {
		split := strings.Split(translated, pdfChunkSep)
		if len(split) != len(chunk.units) {
			// Fallback: if the model modified the delimiter, translate units one-by-one.
			out := make([]chunkUnit, 0, len(chunk.units))
			for _, u := range chunk.units {
				part, perr := w.translateWithRetry(ctx, u.text, source, target, metrics)
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

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// split-before-retry: for multi-unit chunks, always split on failure instead
	// of propagating or retrying the same combined payload. Timeout errors are
	// distinguished for metrics; all other errors also trigger a split so that
	// single-unit sub-chunks can succeed independently with full retry logic.
	mid := len(chunk.units) / 2
	if mid == 0 {
		mid = 1
	}
	leftUnits := chunk.units[:mid]
	rightUnits := chunk.units[mid:]
	if isChunkTimeoutError(ctx, nil, err) {
		metrics.incTimeoutRetry()
		metrics.incSplit()
		slog.Warn("chunk_timeout_split",
			"chunk_words", chunk.words,
			"chunk_units", len(chunk.units),
			"left_words", sumUnitWords(leftUnits),
			"left_units", len(leftUnits),
			"right_words", sumUnitWords(rightUnits),
			"right_units", len(rightUnits),
			"timeout_s", int(translateChunkTimeout.Seconds()),
			"err", err,
		)
	} else {
		metrics.incSplit()
		slog.Warn("chunk_error_split",
			"chunk_words", chunk.words,
			"chunk_units", len(chunk.units),
			"left_units", len(leftUnits),
			"right_units", len(rightUnits),
			"err", err,
		)
	}
	left, lerr := w.translateChunk(ctx, textChunk{units: leftUnits, words: sumUnitWords(leftUnits)}, source, target, metrics)
	if lerr != nil {
		return nil, lerr
	}
	right, rerr := w.translateChunk(ctx, textChunk{units: rightUnits, words: sumUnitWords(rightUnits)}, source, target, metrics)
	if rerr != nil {
		return nil, rerr
	}
	return append(left, right...), nil
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

func isRetryableJobError(msg string) bool {
	if msg == "" {
		return false
	}
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "litellm status 429") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "timed out") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "service unavailable") ||
		strings.Contains(lower, "temporarily unavailable")
}

func isPermanentJobError(msg string) bool {
	if msg == "" {
		return false
	}
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "invalid api key") ||
		strings.Contains(lower, "authentication failed") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "unsupported language") ||
		strings.Contains(lower, "invalid request") ||
		strings.Contains(lower, "malformed")
}

func classifyJobError(msg string) (kind string, retryable bool) {
	if isPermanentJobError(msg) {
		return "permanent", false
	}
	if isRetryableJobError(msg) {
		return "transient", true
	}
	return "unknown", false
}

func retryDelayForAttempt(attempt int) time.Duration {
	if attempt <= 1 {
		base := 2 * time.Second
		jitter := time.Duration(rand.Float64() * float64(base) * retryJitterFraction)
		return base + jitter
	}
	delay := 2 * time.Second
	for i := 1; i < attempt && delay < maxRetryDelay; i++ {
		delay *= 2
	}
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
	jitter := time.Duration(rand.Float64() * float64(delay) * retryJitterFraction)
	return delay + jitter
}

func (w *Worker) isQueueOverloaded(ctx context.Context) bool {
	statsProvider, ok := w.store.(interface {
		QueueStats(context.Context) (QueueStats, error)
	})
	if !ok {
		return false
	}
	stats, err := statsProvider.QueueStats(ctx)
	if err != nil {
		return false
	}
	return stats.QueueDepth >= queueOverloadDepthThreshold ||
		stats.RetryBacklog >= retryBacklogOverloadThreshold ||
		stats.StuckJobs >= stuckJobsOverloadThreshold
}

func (w *Worker) effectiveTranslateConcurrency(ctx context.Context) int {
	base := w.translateConcurrency
	if base <= 0 {
		base = 1
	}
	statsProvider, ok := w.store.(interface {
		QueueStats(context.Context) (QueueStats, error)
	})
	if !ok {
		return base
	}
	stats, err := statsProvider.QueueStats(ctx)
	if err != nil {
		return base
	}
	if stats.QueueDepth >= severeQueueOverloadDepthThreshold ||
		stats.RetryBacklog >= severeRetryBacklogThreshold ||
		stats.StuckJobs >= severeStuckJobsThreshold {
		observability.IncAdaptiveConcurrencyClamp()
		return 1
	}
	if stats.QueueDepth >= queueOverloadDepthThreshold ||
		stats.RetryBacklog >= retryBacklogOverloadThreshold ||
		stats.StuckJobs >= stuckJobsOverloadThreshold {
		observability.IncAdaptiveConcurrencyReduce()
		half := base / 2
		if half < 1 {
			return 1
		}
		return half
	}
	if stats.QueueDepth <= lowQueueDepthThreshold &&
		stats.RetryBacklog <= lowRetryBacklogThreshold &&
		stats.StuckJobs == 0 &&
		base < adaptiveTranslateConcurrencyCeiling {
		observability.IncAdaptiveConcurrencyBoost()
		return base + 1
	}
	return base
}

func averageOCRBlockConfidence(blocks []internalservices.OCRTextBlock) float64 {
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

type ocrPageConfidenceExtractorWithOptions interface {
	ExtractPagesWithConfidenceAndOptions(filePath, lang string, options internalservices.OCRRequestOptions) ([]string, float64, error)
}

type ocrImageBlockExtractorWithOptions interface {
	ExtractImageBlocksWithOptions(filePath, lang string, options internalservices.OCRRequestOptions) ([]internalservices.OCRTextBlock, error)
}

func shouldEnableOCRCorrection(mode Mode) bool {
	// Keep Fast (overlay) mode low-latency; correction is aimed at Studio (layout)
	// and OCR-only extraction quality.
	return mode == ModeLayout || mode == ModeOCROnly
}

func extractOCRPagesAndConfidence(client internalservices.OCRClient, filePath, lang string, mode Mode) ([]string, float64, error) {
	options := internalservices.OCRRequestOptions{Mode: string(mode)}
	enabled := shouldEnableOCRCorrection(mode)
	options.CorrectionEnabled = &enabled

	if withOptions, ok := client.(ocrPageConfidenceExtractorWithOptions); ok {
		return withOptions.ExtractPagesWithConfidenceAndOptions(filePath, lang, options)
	}
	if withConfidence, ok := client.(ocrPageConfidenceExtractor); ok {
		return withConfidence.ExtractPagesWithConfidence(filePath, lang)
	}
	pages, err := client.ExtractPages(filePath, lang)
	return pages, 0, err
}

func (w *Worker) handleFailedJob(ctx context.Context, job *Job) {
	reason := strings.TrimSpace(job.ErrorMsg)
	if reason == "" {
		reason = "job failed without an error message"
	}

	if job.MaxAttempts <= 0 {
		job.MaxAttempts = defaultJobMaxAttempts
	}

	errorKind, retryable := classifyJobError(reason)
	overloaded := w.isQueueOverloaded(ctx)
	effectiveMaxAttempts := job.MaxAttempts
	if overloaded && effectiveMaxAttempts > 1 {
		effectiveMaxAttempts--
	}
	job.Attempt++
	job.LastErrorMsg = reason

	if retryable && job.Attempt < effectiveMaxAttempts {
		delay := retryDelayForAttempt(job.Attempt)
		if overloaded {
			delay += 3 * time.Second
		}
		nextRetry := time.Now().Add(delay)
		job.Status = StatusPending
		job.ErrorMsg = ""
		job.NextRetryAt = nextRetry
		if err := w.store.Update(ctx, job); err != nil {
			slog.Error("update job for retry failed", "job_id", job.ID, "attempt", job.Attempt, "max_attempts", job.MaxAttempts, "err", err)
			return
		}
		requeuer, ok := w.store.(retryRequeuer)
		if !ok {
			slog.Warn("store does not support delayed requeue; falling back to terminal failure", "job_id", job.ID)
			job.Status = StatusFailed
			job.ErrorMsg = reason
			if uerr := w.store.Update(ctx, job); uerr != nil {
				slog.Error("update job after retry fallback failed", "job_id", job.ID, "err", uerr)
			}
			return
		}
		if err := requeuer.Requeue(ctx, job.ID, delay); err != nil {
			slog.Error("requeue job failed", "job_id", job.ID, "attempt", job.Attempt, "delay_ms", delay.Milliseconds(), "err", err)
			job.Status = StatusFailed
			job.ErrorMsg = reason
			if uerr := w.store.Update(ctx, job); uerr != nil {
				slog.Error("update job after requeue failure failed", "job_id", job.ID, "err", uerr)
			}
			return
		}
		slog.Warn("job scheduled for retry",
			"job_id", job.ID,
			"attempt", job.Attempt,
			"max_attempts", job.MaxAttempts,
			"effective_max_attempts", effectiveMaxAttempts,
			"error_kind", errorKind,
			"queue_overloaded", overloaded,
			"next_retry_at", nextRetry.UTC().Format(time.RFC3339),
			"reason", reason,
		)
		return
	}

	job.Status = StatusFailed
	job.ErrorMsg = reason
	job.DeadLetteredAt = time.Now().UTC()
	if err := w.store.Update(ctx, job); err != nil {
		slog.Error("update job terminal failure failed", "job_id", job.ID, "attempt", job.Attempt, "max_attempts", job.MaxAttempts, "err", err)
		return
	}
	dl, ok := w.store.(deadLetterer)
	if ok {
		if err := dl.DeadLetter(ctx, job.ID, reason); err != nil {
			slog.Warn("dead letter enqueue failed", "job_id", job.ID, "err", err)
		}
	}
	slog.Error("job moved to dead letter",
		"job_id", job.ID,
		"attempt", job.Attempt,
		"max_attempts", job.MaxAttempts,
		"effective_max_attempts", effectiveMaxAttempts,
		"error_kind", errorKind,
		"queue_overloaded", overloaded,
		"retryable", retryable,
		"reason", reason,
	)
}

func (w *Worker) process(ctx context.Context, job *Job) {
	job.Warnings = nil
	job.OCRConfidence = 0

	// Best-effort: remove uploaded source files once the job reaches a terminal state.
	defer func() {
		shouldCleanup := false
		switch job.Type {
		case TypePDF:
			// PDF uploads are no longer needed after a completion/failure attempt.
			shouldCleanup = job.Status == StatusCompleted || job.Status == StatusFailed
		case TypeImage:
			// Keep image source files for retryable failures; remove only once terminal.
			shouldCleanup = job.Status == StatusCompleted || (job.Status == StatusFailed && !job.DeadLetteredAt.IsZero())
		}
		if shouldCleanup && job.FilePath != "" {
			if err := os.Remove(job.FilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				slog.Warn("source_file_cleanup_failed", "job_id", job.ID, "file", job.FilePath, "err", err)
			}
		}
	}()

	startedAt := time.Now()
	queueDepth := int64(0)
	if statsProvider, ok := w.store.(interface {
		QueueStats(context.Context) (QueueStats, error)
	}); ok {
		if stats, err := statsProvider.QueueStats(ctx); err == nil {
			queueDepth = stats.QueueDepth
		}
	}
	queueWaitMS := startedAt.Sub(job.CreatedAt).Milliseconds()
	observability.RecordQueueWaitLatency(queueWaitMS)
	effectiveTranslateConcurrency := w.effectiveTranslateConcurrency(ctx)
	if job.CorrelationID == "" {
		job.CorrelationID = job.ID
	}
	ctx = observability.WithLifecycleContext(ctx, job.ID, job.CorrelationID)
	var extractMS int64
	var ocrMS int64
	var translateMS int64
	var ocrTriggered bool
	var ocrReason string
	var ocrOutcome string
	metrics := &retryMetrics{}
	var chunkCount int
	defer func() {
		durationMS := time.Since(startedAt).Milliseconds()
		switch job.Status {
		case StatusFailed:
			observability.EmitLifecycleEventFromContext(ctx, "job_failed", observability.LifecycleEvent{Provider: "pipeline", DurationMS: durationMS, RetryCount: metrics.totalRetryEvents(), QueueDepth: queueDepth})
		case StatusCompleted:
			observability.EmitLifecycleEventFromContext(ctx, "job_completed", observability.LifecycleEvent{Provider: "pipeline", DurationMS: durationMS, RetryCount: metrics.totalRetryEvents(), QueueDepth: queueDepth})
		}
	}()

	slog.Info("processing translation job", "job_id", job.ID, "type", job.Type, "mode", job.Mode, "source", job.Source, "target", job.Target)
	slog.Info("adaptive_translation_concurrency_selected",
		"job_id", job.ID,
		"job_type", job.Type,
		"base_concurrency", w.translateConcurrency,
		"effective_concurrency", effectiveTranslateConcurrency,
		"queue_depth", queueDepth,
	)

	// pages holds per-page normalized text used for translation.
	// For PDF jobs this is populated during extraction; for text jobs it wraps job.Text.
	var pages []string

	// effectiveMode normalises a missing mode value (legacy jobs) to ModeOverlay.
	effectiveJobMode := effectiveMode(job.Mode)

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
		if effectiveJobMode == ModeOCROnly {
			if w.ocrClient == nil {
				slog.Error("pdf OCR-only requested, no OCR client configured", "job_id", job.ID, "file", job.FilePath)
				job.Status = StatusFailed
				job.ErrorMsg = "ocr client is required for translate_pdf jobs in ocr_only mode"
				if uerr := w.store.Update(ctx, job); uerr != nil {
					slog.Error("update job result (missing ocr client in ocr_only mode)", "job_id", job.ID, "err", uerr)
				}
				return
			}
			observability.EmitLifecycleEventFromContext(ctx, "ocr_started", observability.LifecycleEvent{Provider: "ocr", QueueDepth: queueDepth})
			ocrStart := time.Now()
			ocrPages, ocrConfidence, ocrErr := extractOCRPagesAndConfidence(w.ocrClient, job.FilePath, job.Lang, effectiveJobMode)
			ocrMS = time.Since(ocrStart).Milliseconds()
			observability.EmitLifecycleEventFromContext(ctx, "ocr_completed", observability.LifecycleEvent{Provider: "ocr", DurationMS: ocrMS, QueueDepth: queueDepth})
			if ocrErr != nil {
				slog.Error("pdf OCR-only extraction failed", "job_id", job.ID, "file", job.FilePath, "err", ocrErr)
				job.Status = StatusFailed
				job.ErrorMsg = fmt.Sprintf("pdf OCR extraction failed: %v", ocrErr)
				if uerr := w.store.Update(ctx, job); uerr != nil {
					slog.Error("update job result (ocr_only extraction failed)", "job_id", job.ID, "err", uerr)
				}
				return
			}
			pages = normalizePages(ocrPages)
			job.OCRConfidence = ocrConfidence
			if job.OCRConfidence > 0 && job.OCRConfidence < lowOCRConfidenceThreshold {
				job.Warnings = append(job.Warnings, fmt.Sprintf("OCR confidence is low (%.2f). Continue with caution and verify critical text.", job.OCRConfidence))
				observability.IncOCRLowConfidence()
			}
			if len(pages) == 0 {
				job.Status = StatusFailed
				job.ErrorMsg = "pdf OCR extraction returned no text"
				if uerr := w.store.Update(ctx, job); uerr != nil {
					slog.Error("update job result (ocr_only empty text)", "job_id", job.ID, "err", uerr)
				}
				return
			}
			job.ProcessingMethod = "ocr"
			job.Text = strings.Join(pages, "\n\n")
			job.TranslatedText = job.Text
			job.TotalPages = len(pages)
			job.ProcessedPages = len(pages)
			job.Status = StatusCompleted
			if err := w.store.Update(ctx, job); err != nil {
				slog.Error("update job result (ocr_only completed)", "job_id", job.ID, "err", err)
			}
			slog.Info("pdf_ocr_only_job_finished",
				"job_id", job.ID,
				"status", job.Status,
				"pages", len(pages),
				"ocr_ms", ocrMS,
			)
			return
		}

		w.setJobStage(ctx, job, StageDetectingText, "Detecting text in document...", 0.05)
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
			observability.EmitLifecycleEventFromContext(ctx, "ocr_started", observability.LifecycleEvent{Provider: "ocr", QueueDepth: queueDepth})
			ocrStart := time.Now()
			ocrPages, ocrConfidence, ocrErr := extractOCRPagesAndConfidence(w.ocrClient, job.FilePath, job.Lang, effectiveJobMode)
			ocrMS = time.Since(ocrStart).Milliseconds()
			observability.EmitLifecycleEventFromContext(ctx, "ocr_completed", observability.LifecycleEvent{Provider: "ocr", DurationMS: ocrMS, QueueDepth: queueDepth})
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
			job.OCRConfidence = ocrConfidence
			if job.OCRConfidence > 0 && job.OCRConfidence < lowOCRConfidenceThreshold {
				job.Warnings = append(job.Warnings, fmt.Sprintf("OCR confidence is low (%.2f). Continue with caution and verify critical text.", job.OCRConfidence))
				observability.IncOCRLowConfidence()
			}
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

	// Branch on job.Mode to select the rendering/translation pipeline.
	slog.Info("translation_pipeline_selected",
		"job_id", job.ID,
		"mode", effectiveJobMode,
		"job_type", job.Type,
	)
	switch effectiveJobMode {
	case ModeLayout:
		if job.Type != TypeImage {
			w.processLayoutMode(ctx, job)
			return
		}
	case ModeOCROnly:
		if job.Type != TypeImage {
			job.Status = StatusFailed
			job.ErrorMsg = "ocr_only mode is currently supported for image and pdf jobs only"
			if err := w.store.Update(ctx, job); err != nil {
				slog.Error("update job result (ocr_only unsupported)", "job_id", job.ID, "err", err)
			}
			return
		}
	default: // ModeOverlay — continue with the overlay translation pipeline below.
	}

	if job.Type == TypeImage {
		if job.FilePath == "" {
			job.Status = StatusFailed
			job.ErrorMsg = "file_path is required for translate_image jobs"
			if err := w.store.Update(ctx, job); err != nil {
				slog.Error("update image job result (missing file_path)", "job_id", job.ID, "err", err)
			}
			return
		}
		if w.ocrClient == nil {
			job.Status = StatusFailed
			job.ErrorMsg = "ocr client is required for translate_image jobs"
			if err := w.store.Update(ctx, job); err != nil {
				slog.Error("update image job result (missing ocr client)", "job_id", job.ID, "err", err)
			}
			return
		}

		job.Status = StatusProcessing
		w.setJobStage(ctx, job, StageDetectingText, "Detecting text in image…", 0.05)
		observability.EmitLifecycleEventFromContext(ctx, "ocr_started", observability.LifecycleEvent{Provider: "ocr", QueueDepth: queueDepth})

		ocrStart := time.Now()
		ocrOptions := internalservices.OCRRequestOptions{Mode: string(effectiveJobMode)}
		enabled := shouldEnableOCRCorrection(effectiveJobMode)
		ocrOptions.CorrectionEnabled = &enabled
		var blocks []internalservices.OCRTextBlock
		var err error
		if withOptions, ok := w.ocrClient.(ocrImageBlockExtractorWithOptions); ok {
			blocks, err = withOptions.ExtractImageBlocksWithOptions(job.FilePath, job.Lang, ocrOptions)
		} else {
			blocks, err = w.ocrClient.ExtractImageBlocks(job.FilePath, job.Lang)
		}
		ocrMS = time.Since(ocrStart).Milliseconds()
		observability.EmitLifecycleEventFromContext(ctx, "ocr_completed", observability.LifecycleEvent{Provider: "ocr", DurationMS: ocrMS, QueueDepth: queueDepth})
		if err != nil {
			job.Status = StatusFailed
			job.ErrorMsg = fmt.Sprintf("image OCR failed: %v", err)
			if uerr := w.store.Update(ctx, job); uerr != nil {
				slog.Error("update image job result (ocr failed)", "job_id", job.ID, "err", uerr)
			}
			return
		}
		job.OCRConfidence = averageOCRBlockConfidence(blocks)
		if job.OCRConfidence > 0 && job.OCRConfidence < lowOCRConfidenceThreshold {
			warning := fmt.Sprintf("OCR confidence is low (%.2f). Translation continued in graceful-degradation mode; verify critical text manually.", job.OCRConfidence)
			job.Warnings = append(job.Warnings, warning)
			observability.IncOCRLowConfidence()
			slog.Warn("ocr_low_confidence_detected",
				"job_id", job.ID,
				"ocr_confidence", job.OCRConfidence,
				"threshold", lowOCRConfidenceThreshold,
			)
		}

		w.setJobStage(ctx, job, StageUnderstandingLayout, "Understanding layout geometry…", 0.14)

		translatedTexts := make([]string, len(blocks))
		segmentText := make([]string, 0, len(blocks))
		segmentBlockIdx := make([]int, 0, len(blocks))
		for i, b := range blocks {
			sourceText := strings.TrimSpace(b.Text)
			if sourceText == "" {
				continue
			}
			segmentText = append(segmentText, sourceText)
			segmentBlockIdx = append(segmentBlockIdx, i)
		}

		if len(segmentText) == 0 {
			job.Text = ""
			job.TranslatedText = ""
			job.ErrorMsg = ""
			if effectiveJobMode == ModeOCROnly {
				job.OutputFilePath = ""
			} else {
				outPath, copyErr := writeImagePassthroughOutput(job.FilePath)
				if copyErr != nil {
					job.Status = StatusFailed
					job.ErrorMsg = fmt.Sprintf("image passthrough failed: %v", copyErr)
					if uerr := w.store.Update(ctx, job); uerr != nil {
						slog.Error("update image job result (passthrough failed)", "job_id", job.ID, "err", uerr)
					}
					return
				}
				job.OutputFilePath = outPath
			}
			job.Status = StatusCompleted
			if uerr := w.store.Update(ctx, job); uerr != nil {
				slog.Error("update image job result (no text passthrough)", "job_id", job.ID, "err", uerr)
			}
			slog.Warn("image_no_translatable_text_passthrough", "job_id", job.ID, "mode", effectiveJobMode)
			return
		}

		w.setJobStage(ctx, job, StageDetectingLanguages, "Detecting languages and script direction…", 0.24)

		if effectiveJobMode == ModeOCROnly {
			job.Text = strings.Join(segmentText, "\n")
			job.TranslatedText = job.Text
			job.OutputFilePath = ""
			job.Status = StatusCompleted
			if err := w.store.Update(ctx, job); err != nil {
				slog.Error("update image job result (ocr_only completed)", "job_id", job.ID, "err", err)
			}
			slog.Info("image_ocr_only_job_finished",
				"job_id", job.ID,
				"blocks", len(segmentText),
				"status", job.Status,
			)
			return
		}

		units := buildChunkUnits(segmentText, w.chunkMaxWords)
		chunks := buildTextChunks(units, w.chunkMinWords, w.chunkMaxWords)
		if len(chunks) == 0 {
			job.Text = strings.Join(segmentText, "\n")
			job.TranslatedText = job.Text
			job.ErrorMsg = ""
			outPath, copyErr := writeImagePassthroughOutput(job.FilePath)
			if copyErr != nil {
				job.Status = StatusFailed
				job.ErrorMsg = fmt.Sprintf("image passthrough failed: %v", copyErr)
				if uerr := w.store.Update(ctx, job); uerr != nil {
					slog.Error("update image job result (no chunks passthrough failed)", "job_id", job.ID, "err", uerr)
				}
				return
			}
			job.OutputFilePath = outPath
			job.Status = StatusCompleted
			if uerr := w.store.Update(ctx, job); uerr != nil {
				slog.Error("update image job result (no chunks passthrough)", "job_id", job.ID, "err", uerr)
			}
			slog.Warn("image_no_chunks_passthrough", "job_id", job.ID, "mode", effectiveJobMode)
			return
		}

		results := make([][]chunkUnit, len(chunks))
		errByChunk := make([]error, len(chunks))
		maxParallel := effectiveTranslateConcurrency
		if maxParallel <= 0 {
			maxParallel = 1
		}
		// Attach stage notifier so orchestrator retry/failover events surface in real-time.
		var imgStageMu sync.Mutex
		imgNotifyCtx := services.WithStageNotifier(ctx, w.stageNotifierFor(ctx, job, &imgStageMu))
		w.setJobStage(ctx, job, StageTranslating, "Translating image text…", 0.3)
		observability.EmitLifecycleEventFromContext(ctx, "translation_started", observability.LifecycleEvent{Provider: "pipeline", QueueDepth: queueDepth})

		sem := make(chan struct{}, maxParallel)
		var wg sync.WaitGroup
		for i, chunk := range chunks {
			wg.Add(1)
			go func(idx int, c textChunk) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
				case <-imgNotifyCtx.Done():
					errByChunk[idx] = imgNotifyCtx.Err()
					return
				}
				defer func() { <-sem }()
				observability.EmitLifecycleEventFromContext(imgNotifyCtx, "chunk_started", observability.LifecycleEvent{Provider: "translation", QueueDepth: queueDepth})
				chunkStarted := time.Now()
				translatedUnits, terr := w.translateChunk(imgNotifyCtx, c, job.Source, job.Target, metrics)
				observability.EmitLifecycleEventFromContext(imgNotifyCtx, "chunk_completed", observability.LifecycleEvent{Provider: "translation", DurationMS: time.Since(chunkStarted).Milliseconds(), RetryCount: metrics.totalRetryEvents(), QueueDepth: queueDepth})
				if terr != nil {
					errByChunk[idx] = terr
					return
				}
				results[idx] = translatedUnits
			}(i, chunk)
		}
		wg.Wait()

		translatedPartsBySegment := make([][]string, len(segmentText))
		for i, res := range results {
			if errByChunk[i] != nil {
				if isUnrecoverableChunkError(ctx, errByChunk[i]) {
					job.Status = StatusFailed
					job.ErrorMsg = errByChunk[i].Error()
					if uerr := w.store.Update(ctx, job); uerr != nil {
						slog.Error("update image job result (translation failed)", "job_id", job.ID, "err", uerr)
					}
					return
				}
				slog.Warn("image_chunk_translation_degraded",
					"job_id", job.ID,
					"chunk_idx", i,
					"chunk_words", chunks[i].words,
					"degrade_reason", "chunk_retry_exhausted",
					"err", errByChunk[i],
				)
				observability.IncDegradedMode()
				results[i] = chunks[i].units
				res = results[i]
			}
			for _, u := range res {
				if u.pageIndex >= 0 && u.pageIndex < len(translatedPartsBySegment) {
					translatedPartsBySegment[u.pageIndex] = append(translatedPartsBySegment[u.pageIndex], strings.TrimSpace(u.text))
				}
			}
		}

		fullSource := make([]string, 0, len(segmentText))
		fullTarget := make([]string, 0, len(segmentText))
		for segIdx, blockIdx := range segmentBlockIdx {
			translated := strings.TrimSpace(strings.Join(translatedPartsBySegment[segIdx], " "))
			if translated == "" {
				continue
			}
			translatedTexts[blockIdx] = translated
			fullSource = append(fullSource, segmentText[segIdx])
			fullTarget = append(fullTarget, translated)
		}

		if len(fullTarget) == 0 {
			job.Text = strings.Join(segmentText, "\n")
			job.TranslatedText = job.Text
			job.ErrorMsg = ""
			outPath, copyErr := writeImagePassthroughOutput(job.FilePath)
			if copyErr != nil {
				job.Status = StatusFailed
				job.ErrorMsg = fmt.Sprintf("image passthrough failed: %v", copyErr)
				if uerr := w.store.Update(ctx, job); uerr != nil {
					slog.Error("update image job result (empty translation passthrough failed)", "job_id", job.ID, "err", uerr)
				}
				return
			}
			job.OutputFilePath = outPath
			job.Status = StatusCompleted
			if uerr := w.store.Update(ctx, job); uerr != nil {
				slog.Error("update image job result (empty translation passthrough)", "job_id", job.ID, "err", uerr)
			}
			slog.Warn("image_empty_translation_passthrough", "job_id", job.ID, "mode", effectiveJobMode)
			return
		}

		renderBlocks := make([]internalservices.ImageTextBlock, len(blocks))
		for i, b := range blocks {
			renderBlocks[i] = internalservices.ImageTextBlock{Text: b.Text, Bbox: b.Bbox, RegionClass: b.RegionClass}
		}
		renderOpts := internalservices.DefaultOverlayOptions()
		if effectiveJobMode == ModeLayout {
			// Studio mode: use high-quality options as the base. Individual job
			// fields (BgAlpha, TextPadding, JPEGQuality) can still override.
			renderOpts = internalservices.DefaultStudioOptions()
		}
		if effectiveJobMode == ModeLayout {
			// Layout mode keeps original OCR geometry and uses minimal internal
			// padding so text does not touch bbox edges.
			applyLayoutModeBBoxOptions(&renderOpts, job.TextPadding)
		}
		if job.JPEGQuality > 0 {
			renderOpts.JPEGQuality = job.JPEGQuality
		}
		if job.BgAlpha >= 0 {
			renderOpts.BgAlpha = uint8(job.BgAlpha)
		}
		if job.TextPadding >= 0 && effectiveJobMode != ModeLayout {
			renderOpts.TextPadding = job.TextPadding
		}
		if effectiveJobMode == ModeLayout {
			w.setJobStage(ctx, job, StageRebuildingLayout, "Rebuilding image layout…", 0.85)
		} else {
			w.setJobStage(ctx, job, StageRendering, "Rendering translated image…", 0.85)
		}
		observability.EmitLifecycleEventFromContext(ctx, "render_started", observability.LifecycleEvent{Provider: "render", QueueDepth: queueDepth})
		renderStart := time.Now()
		outPath, renderStats, err := internalservices.DrawTextOnImageWithOptions(job.FilePath, renderBlocks, translatedTexts, renderOpts)
		if err != nil && effectiveJobMode == ModeLayout {
			// If layout rendering fails, fall back to the regular overlay render path.
			observability.IncRenderFailure("layout")
			slog.Warn("layout_render_failed_falling_back_to_overlay",
				"job_id", job.ID,
				"failover_reason", "layout_render_error",
				"err", err,
			)
			observability.IncRenderFallback()
			overlayOpts := internalservices.DefaultOverlayOptions()
			if job.JPEGQuality > 0 {
				overlayOpts.JPEGQuality = job.JPEGQuality
			}
			if job.BgAlpha >= 0 {
				overlayOpts.BgAlpha = uint8(job.BgAlpha)
			}
			if job.TextPadding >= 0 {
				overlayOpts.TextPadding = job.TextPadding
			}
			outPath, renderStats, err = internalservices.DrawTextOnImageWithOptions(job.FilePath, renderBlocks, translatedTexts, overlayOpts)
			renderOpts = overlayOpts
		}
		if err != nil {
			observability.IncRenderFailure("fast")
			slog.Warn("image_render_failed_falling_back_to_passthrough",
				"job_id", job.ID,
				"mode", effectiveJobMode,
				"failover_reason", "rendering_failed_all_modes",
				"err", err,
			)
			observability.IncRenderFallback()
			w.setJobStage(ctx, job, StageRendering, "Rendering fallback active (Fast mode)…", 0.92)
			passthroughPath, copyErr := writeImagePassthroughOutput(job.FilePath)
			if copyErr != nil {
				job.Status = StatusFailed
				job.ErrorMsg = fmt.Sprintf("image rendering failed: %v; passthrough fallback failed: %v", err, copyErr)
				if uerr := w.store.Update(ctx, job); uerr != nil {
					slog.Error("update image job result (render+fallback failed)", "job_id", job.ID, "err", uerr)
				}
				return
			}
			outPath = passthroughPath
		}
		observability.EmitLifecycleEventFromContext(ctx, "render_completed", observability.LifecycleEvent{Provider: "render", DurationMS: time.Since(renderStart).Milliseconds(), QueueDepth: queueDepth})

		renderEvent := "image_overlay_rendered"
		if effectiveJobMode == ModeLayout {
			renderEvent = "image_layout_rendered"
		}
		slog.Info(renderEvent,
			"job_id", job.ID,
			"blocks_drawn", renderStats.BlocksDrawn,
			"blocks_skipped", renderStats.BlocksSkipped,
			"jpeg_quality", renderOpts.JPEGQuality,
			"bg_alpha", renderOpts.BgAlpha,
			"text_padding", renderOpts.TextPadding,
			"bbox_shrink_px", renderOpts.BboxShrinkPx,
			"erase_bbox", renderOpts.EraseBBox,
		)

		job.Text = strings.Join(fullSource, "\n")
		job.TranslatedText = strings.Join(fullTarget, "\n")
		job.OutputFilePath = outPath
		observability.EmitLifecycleEventFromContext(ctx, "export_started", observability.LifecycleEvent{Provider: "render", QueueDepth: queueDepth})
		observability.EmitLifecycleEventFromContext(ctx, "export_completed", observability.LifecycleEvent{Provider: "render", QueueDepth: queueDepth})
		job.Status = StatusCompleted
		if err := w.store.SetCached(ctx, job.Text, job.Source, job.Target, job.TranslatedText); err != nil {
			slog.Warn("failed to cache image translation", "job_id", job.ID, "err", err)
		}
		job.Stage = StageCompleted
		job.StageMessage = "Translation complete"
		job.StageProgress = 1.0
		if err := w.store.Update(ctx, job); err != nil {
			slog.Error("update image job result", "job_id", job.ID, "err", err)
		}
		slog.Info("image_job_finished",
			"job_id", job.ID,
			"output_file_path", job.OutputFilePath,
			"blocks", len(blocks),
			"status", job.Status,
		)
		return
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
		slog.Info("job finished (cached)", "job_id", job.ID, "mode", effectiveJobMode, "job_type", job.Type)
		return
	}

	job.Status = StatusProcessing
	observability.EmitLifecycleEventFromContext(ctx, "translation_started", observability.LifecycleEvent{Provider: "pipeline", QueueDepth: queueDepth})
	var pdfStageMu sync.Mutex
	pdfNotifyCtx := services.WithStageNotifier(ctx, w.stageNotifierFor(ctx, job, &pdfStageMu))
	w.setJobStage(ctx, job, StageTranslating, "Translating content...", 0.2)

	translateStart := time.Now()
	type pageResult struct {
		units []chunkUnit
		err   error
		dur   time.Duration
	}
	units := buildChunkUnits(pages, w.chunkMaxWords)
	unitTargetsByPage := make([]int, len(pages))
	for _, u := range units {
		if u.pageIndex >= 0 && u.pageIndex < len(unitTargetsByPage) {
			unitTargetsByPage[u.pageIndex]++
		}
	}
	unitDoneByPage := make([]int, len(pages))
	allResults := make([]pageResult, 0, len(units))
	var progressMu sync.Mutex
	chunkIdxOffset := 0
	remainingUnits := units
	adaptiveMinWords := w.chunkMinWords
	adaptiveMaxWords := w.chunkMaxWords
	checkpointStore, hasCheckpointStore := w.store.(chunkCheckpointStore)
	for len(remainingUnits) > 0 {
		chunks := buildTextChunks(remainingUnits, adaptiveMinWords, adaptiveMaxWords)
		if len(chunks) == 0 {
			break
		}
		batchSize := effectiveTranslateConcurrency
		if batchSize <= 0 {
			batchSize = 1
		}
		if batchSize > len(chunks) {
			batchSize = len(chunks)
		}
		batch := chunks[:batchSize]
		results := make([]pageResult, len(batch))
		sem := make(chan struct{}, batchSize)
		var wg sync.WaitGroup
		for i, chunk := range batch {
			wg.Add(1)
			go func(idx int, c textChunk) {
				defer wg.Done()

				checkpointKey := chunkCheckpointKey(c, job.Source, job.Target)
				if hasCheckpointStore {
					cachedUnits, found, cerr := checkpointStore.GetChunkCheckpoint(pdfNotifyCtx, job.ID, checkpointKey)
					if cerr != nil {
						slog.Warn("chunk_checkpoint_lookup_failed", "job_id", job.ID, "chunk_idx", chunkIdxOffset+idx, "err", cerr)
					} else if found {
						observability.IncChunkCheckpointHit()
						results[idx] = pageResult{units: cachedUnits, err: nil, dur: 0}
						slog.Info("chunk_checkpoint_hit", "job_id", job.ID, "chunk_idx", chunkIdxOffset+idx, "units", len(cachedUnits))
						return
					} else {
						observability.IncChunkCheckpointMiss()
					}
				}

				select {
				case sem <- struct{}{}:
				case <-pdfNotifyCtx.Done():
					results[idx] = pageResult{units: c.units, err: pdfNotifyCtx.Err()}
					return
				}
				defer func() { <-sem }()
				chunkStart := time.Now()
				observability.EmitLifecycleEventFromContext(pdfNotifyCtx, "chunk_started", observability.LifecycleEvent{Provider: "translation", QueueDepth: queueDepth})
				translatedUnits, err := w.translateChunk(pdfNotifyCtx, c, job.Source, job.Target, metrics)
				chunkEnd := time.Now()
				chunkDur := chunkEnd.Sub(chunkStart)
				chunkDurMs := chunkDur.Milliseconds()
				chunkStartTS := chunkStart.UTC().Format(time.RFC3339Nano)
				chunkEndTS := chunkEnd.UTC().Format(time.RFC3339Nano)
				chunkIdx := chunkIdxOffset + idx
				observability.EmitLifecycleEventFromContext(pdfNotifyCtx, "chunk_completed", observability.LifecycleEvent{Provider: "translation", DurationMS: chunkDurMs, RetryCount: metrics.totalRetryEvents(), QueueDepth: queueDepth})
				if chunkDur > slowChunkThreshold {
					slog.Warn("slow_chunk_translation",
						"job_id", job.ID,
						"job_type", job.Type,
						"chunk_idx", chunkIdx,
						"chunk_words", c.words,
						"start_time", chunkStartTS,
						"end_time", chunkEndTS,
						"duration_ms", chunkDurMs,
					)
				}
				if err != nil {
					slog.Error("chunk_translation_failed",
						"job_id", job.ID,
						"job_type", job.Type,
						"chunk_idx", chunkIdx,
						"chunk_words", c.words,
						"start_time", chunkStartTS,
						"end_time", chunkEndTS,
						"duration_ms", chunkDurMs,
						"err", err,
					)
				} else {
					slog.Info("chunk_translated",
						"job_id", job.ID,
						"job_type", job.Type,
						"chunk_idx", chunkIdx,
						"chunk_words", c.words,
						"start_time", chunkStartTS,
						"end_time", chunkEndTS,
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
							if uerr := w.store.Update(pdfNotifyCtx, job); uerr != nil {
								slog.Error("update pdf progress", "job_id", job.ID, "processed_pages", job.ProcessedPages, "err", uerr)
							}
						}
					}
					progressMu.Unlock()
				}
				if err != nil {
					results[idx] = pageResult{units: c.units, err: err, dur: chunkDur}
					return
				}
				if hasCheckpointStore {
					if cerr := checkpointStore.SetChunkCheckpoint(pdfNotifyCtx, job.ID, checkpointKey, translatedUnits); cerr != nil {
						observability.IncChunkCheckpointPersistFailure()
						slog.Warn("chunk_checkpoint_persist_failed", "job_id", job.ID, "chunk_idx", chunkIdxOffset+idx, "err", cerr)
					}
				}
				results[idx] = pageResult{units: translatedUnits, err: nil, dur: chunkDur}
			}(i, chunk)
		}
		wg.Wait()

		slowInBatch := false
		consumedUnits := 0
		for i, r := range results {
			allResults = append(allResults, r)
			// Adaptive reduction fires at the soft threshold so future chunks
			// are pre-shrunk before they can reach the hard deadline.
			if r.dur > slowChunkThreshold {
				slowInBatch = true
			}
			consumedUnits += len(batch[i].units)
		}
		chunkCount += len(batch)
		chunkIdxOffset += len(batch)
		if consumedUnits > len(remainingUnits) {
			consumedUnits = len(remainingUnits)
		}
		remainingUnits = remainingUnits[consumedUnits:]

		if slowInBatch && len(remainingUnits) > 0 {
			nextMin, nextMax := reduceChunkWordRange(adaptiveMinWords, adaptiveMaxWords)
			if nextMin != adaptiveMinWords || nextMax != adaptiveMaxWords {
				slog.Warn("adaptive_chunk_sizing_reduced",
					"job_id", job.ID,
					"job_type", job.Type,
					"previous_min_words", adaptiveMinWords,
					"previous_max_words", adaptiveMaxWords,
					"next_min_words", nextMin,
					"next_max_words", nextMax,
				)
				adaptiveMinWords = nextMin
				adaptiveMaxWords = nextMax
			}
		}
	}
	translatedPageParts := make([][]string, len(pages))
	var translateErr error
	degradedChunks := 0
	for _, r := range allResults {
		if r.err != nil {
			if isUnrecoverableChunkError(ctx, r.err) {
				translateErr = r.err
				break
			}
			degradedChunks++
			slog.Warn("pdf_chunk_translation_degraded",
				"job_id", job.ID,
				"degrade_reason", "chunk_retry_exhausted",
				"err", r.err,
			)
			observability.IncDegradedMode()
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
	observability.RecordTranslationLatency(translateMS)
	if translateErr != nil {
		slog.Error("translation failed", "job_id", job.ID, "err", translateErr)
		job.Status = StatusFailed
		job.ErrorMsg = translateErr.Error()
		if ocrTriggered {
			ocrOutcome = "translation_failed"
		}
	} else {
		job.Stage = StageCompleted
		if degradedChunks > 0 {
			job.StageMessage = fmt.Sprintf("Translation complete with graceful fallback on %d chunk(s)", degradedChunks)
		} else {
			job.StageMessage = "Translation complete"
		}
		job.StageProgress = 1.0
		job.Status = StatusCompleted
		observability.EmitLifecycleEventFromContext(ctx, "export_started", observability.LifecycleEvent{Provider: "pipeline", QueueDepth: queueDepth})
		job.TranslatedText = translated
		observability.EmitLifecycleEventFromContext(ctx, "export_completed", observability.LifecycleEvent{Provider: "pipeline", QueueDepth: queueDepth})
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
		"job_mode", job.Mode,
		"status", job.Status,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"chunk_count", chunkCount,
		"retry_count", metrics.totalRetryEvents(),
		"total_retry_events", metrics.totalRetryEvents(),
		"timeout_retry_count", metrics.timeoutRetryCount.Load(),
		"split_count", metrics.splitCount.Load(),
	)
}

// effectiveMode returns job.Mode, defaulting to ModeOverlay for empty values.
// setJobStage updates the job's Stage, StageMessage, and StageProgress and
// persists the change to the store. Errors are logged but not propagated since
// stage updates are best-effort and must not abort a job.
func (w *Worker) setJobStage(ctx context.Context, job *Job, stage, message string, progress float64) {
	job.Stage = stage
	job.StageMessage = message
	job.StageProgress = progress
	if err := w.store.Update(ctx, job); err != nil {
		slog.Warn("set_job_stage_update_failed", "job_id", job.ID, "stage", stage, "err", err)
	}
}

// stageNotifierFor returns a services.StageNotifyFn that updates job stage in
// memory and persists it. A mutex prevents concurrent goroutines from racing on
// the job struct fields. The persist is best-effort; errors are silently dropped.
func (w *Worker) stageNotifierFor(ctx context.Context, job *Job, mu *sync.Mutex) services.StageNotifyFn {
	return func(stage, message string, progress float64) {
		mu.Lock()
		job.Stage = stage
		job.StageMessage = message
		if progress > 0 {
			job.StageProgress = progress
		}
		mu.Unlock()
		// Best-effort persist so the UI sees "retrying" / "fallback_provider" in real-time.
		_ = w.store.Update(ctx, job)
	}
}

// This covers legacy jobs that were enqueued before the mode field was introduced.
func effectiveMode(m Mode) Mode {
	if m == "" {
		return ModeOverlay
	}
	return m
}

// processLayoutMode handles job types that do not yet support layout mode.
// Image jobs are handled in the main process path using layout render options.
func (w *Worker) processLayoutMode(ctx context.Context, job *Job) {
	slog.Info("layout_pipeline_invoked",
		"job_id", job.ID,
		"mode", effectiveMode(job.Mode),
		"job_type", job.Type,
		"source", job.Source,
		"target", job.Target,
	)
	job.Status = StatusFailed
	job.ErrorMsg = "layout mode is currently supported for image jobs only"
	if err := w.store.Update(ctx, job); err != nil {
		slog.Error("update job result (layout stub)", "job_id", job.ID, "err", err)
	}
}
