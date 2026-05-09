package pglog

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const testPostgresEnv = "LOKLINGO_TEST_POSTGRES_DSN"

func TestHandler_PersistsReliabilityEvent_Integration(t *testing.T) {
	dsn := os.Getenv(testPostgresEnv)
	if dsn == "" {
		t.Skip("integration test skipped; set LOKLINGO_TEST_POSTGRES_DSN")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect pg: %v", err)
	}
	defer pool.Close()

	ensureTablesForTest(t, ctx, pool)

	token := "it_reason_" + time.Now().Format("20060102150405.000000")
	if _, err := pool.Exec(ctx, `DELETE FROM reliability_events WHERE reason = $1`, token); err != nil {
		t.Fatalf("cleanup before test: %v", err)
	}

	logger := slog.New(New(slog.NewJSONHandler(io.Discard, nil), pool))
	logger.Warn("litellm_retry", slog.Int64("attempt", 2), slog.Int64("status", 503), slog.String("reason", token))

	waitForCount(t, ctx, pool,
		`SELECT COUNT(*)::bigint FROM reliability_events WHERE integration = 'litellm' AND event_name = 'litellm_retry' AND reason = $1`,
		1,
		token,
	)
}

func TestHandler_PersistsOCREvent_Integration(t *testing.T) {
	dsn := os.Getenv(testPostgresEnv)
	if dsn == "" {
		t.Skip("integration test skipped; set LOKLINGO_TEST_POSTGRES_DSN")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect pg: %v", err)
	}
	defer pool.Close()

	ensureTablesForTest(t, ctx, pool)

	jobID := "it_job_" + time.Now().Format("20060102150405.000000")
	if _, err := pool.Exec(ctx, `DELETE FROM ocr_events WHERE job_id = $1`, jobID); err != nil {
		t.Fatalf("cleanup before test: %v", err)
	}

	logger := slog.New(New(slog.NewJSONHandler(io.Discard, nil), pool))
	logger.Info("ocr_fallback_triggered",
		slog.String("job_id", jobID),
		slog.String("file", "integration-test.pdf"),
		slog.String("lang", "auto"),
		slog.String("target", "de"),
		slog.String("extraction_failure_reason", "test"),
		slog.String("outcome", "succeeded"),
		slog.Int64("extract_ms", 11),
		slog.Int64("ocr_ms", 22),
		slog.Int64("translate_ms", 33),
		slog.Int64("total_ms", 66),
	)

	waitForCount(t, ctx, pool,
		`SELECT COUNT(*)::bigint FROM ocr_events WHERE job_id = $1`,
		1,
		jobID,
	)
}

func ensureTablesForTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	const createOCREvents = `
		CREATE TABLE IF NOT EXISTS ocr_events (
			id BIGSERIAL PRIMARY KEY,
			recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			job_id TEXT NOT NULL,
			file TEXT NOT NULL,
			lang TEXT NOT NULL DEFAULT '',
			target TEXT NOT NULL DEFAULT '',
			extraction_failure_reason TEXT NOT NULL DEFAULT '',
			outcome TEXT NOT NULL DEFAULT 'succeeded',
			extract_ms BIGINT NOT NULL DEFAULT 0,
			ocr_ms BIGINT NOT NULL DEFAULT 0,
			translate_ms BIGINT NOT NULL DEFAULT 0,
			total_ms BIGINT NOT NULL DEFAULT 0
		);`
	if _, err := pool.Exec(ctx, createOCREvents); err != nil {
		t.Fatalf("create ocr_events: %v", err)
	}

	const createReliabilityEvents = `
		CREATE TABLE IF NOT EXISTS reliability_events (
			id BIGSERIAL PRIMARY KEY,
			recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			integration TEXT NOT NULL,
			event_name TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			endpoint TEXT NOT NULL DEFAULT '',
			attempt BIGINT NOT NULL DEFAULT 0,
			status BIGINT NOT NULL DEFAULT 0
		);`
	if _, err := pool.Exec(ctx, createReliabilityEvents); err != nil {
		t.Fatalf("create reliability_events: %v", err)
	}
}

func waitForCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, q string, want int64, arg any) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		var got int64
		err := pool.QueryRow(ctx, q, arg).Scan(&got)
		if err == nil && got >= want {
			return
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("waitForCount query failed: %v", err)
			}
			t.Fatalf("waitForCount timed out: got %d, want at least %d", got, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
