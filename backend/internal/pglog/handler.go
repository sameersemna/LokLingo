// Package pglog provides a slog.Handler that intercepts structured log events
// and persists analytics records to Postgres.
//
// Only events with the message "ocr_fallback_triggered" are written to the
// ocr_events table; every other event is forwarded to the wrapped handler
// unchanged.  This keeps the Postgres writes narrow and cheap.
package pglog

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const ocrFallbackMsg = "ocr_fallback_triggered"

// Handler wraps another slog.Handler and additionally writes
// ocr_fallback_triggered events to the ocr_events Postgres table.
type Handler struct {
	inner slog.Handler
	pool  *pgxpool.Pool
}

// New returns a Handler.  inner must not be nil; pool may be nil (handler
// becomes a transparent pass-through — useful in tests / development).
func New(inner slog.Handler, pool *pgxpool.Pool) *Handler {
	return &Handler{inner: inner, pool: pool}
}

// Enabled delegates to the wrapped handler.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// WithAttrs delegates to the wrapped handler.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{inner: h.inner.WithAttrs(attrs), pool: h.pool}
}

// WithGroup delegates to the wrapped handler.
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{inner: h.inner.WithGroup(name), pool: h.pool}
}

// Handle forwards the record to the wrapped handler, then — if the message is
// "ocr_fallback_triggered" and a pool is configured — inserts a row into
// ocr_events.  The Postgres write is best-effort: failures are logged at Warn
// level through the wrapped handler rather than being returned.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	// Always write to the underlying handler first (stdout / JSON).
	if err := h.inner.Handle(ctx, r); err != nil {
		return err
	}

	if r.Message != ocrFallbackMsg || h.pool == nil {
		return nil
	}

	// Collect the structured fields from the record.
	fields := make(map[string]slog.Value, 10)
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value
		return true
	})

	go h.insertOCREvent(fields, r.Time)
	return nil
}

// insertOCREvent writes one row to ocr_events in the background.
func (h *Handler) insertOCREvent(fields map[string]slog.Value, recordedAt time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const q = `
		INSERT INTO ocr_events
			(recorded_at, job_id, file, lang, target, extraction_failure_reason, outcome, extract_ms, ocr_ms, translate_ms, total_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	// Best-effort: ignore Postgres errors — stdout already has the event.
	_, _ = h.pool.Exec(ctx, q,
		recordedAt,
		fieldString(fields, "job_id"),
		fieldString(fields, "file"),
		fieldString(fields, "lang"),
		fieldString(fields, "target"),
		fieldString(fields, "extraction_failure_reason"),
		fieldString(fields, "outcome"),
		fieldInt64(fields, "extract_ms"),
		fieldInt64(fields, "ocr_ms"),
		fieldInt64(fields, "translate_ms"),
		fieldInt64(fields, "total_ms"),
	)
}

func fieldString(fields map[string]slog.Value, key string) string {
	v, ok := fields[key]
	if !ok {
		return ""
	}
	if v.Kind() == slog.KindString {
		return v.String()
	}
	return fmt.Sprint(v.Any())
}

func fieldInt64(fields map[string]slog.Value, key string) int64 {
	v, ok := fields[key]
	if !ok {
		return 0
	}
	switch v.Kind() {
	case slog.KindInt64:
		return v.Int64()
	case slog.KindUint64:
		return int64(v.Uint64())
	case slog.KindFloat64:
		return int64(v.Float64())
	case slog.KindString:
		i, err := strconv.ParseInt(v.String(), 10, 64)
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}
