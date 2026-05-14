package observability

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// LifecycleEvent stores the structured payload emitted for pipeline lifecycle events.
type LifecycleEvent struct {
	JobID         string `json:"job_id"`
	CorrelationID string `json:"correlation_id"`
	Provider      string `json:"provider"`
	DurationMS    int64  `json:"duration_ms"`
	RetryCount    int64  `json:"retry_count"`
	QueueDepth    int64  `json:"queue_depth"`
	Timestamp     string `json:"timestamp"`
}

// LifecycleEventRecord stores a lifecycle event name and payload for short-term tracing.
type LifecycleEventRecord struct {
	Event string `json:"event"`
	LifecycleEvent
}

type lifecycleContextKey struct{}

// LifecycleContext stores request lineage identifiers propagated across stages.
type LifecycleContext struct {
	JobID         string
	CorrelationID string
}

// WithLifecycleContext attaches job/correlation identifiers to a context.
func WithLifecycleContext(ctx context.Context, jobID, correlationID string) context.Context {
	return context.WithValue(ctx, lifecycleContextKey{}, LifecycleContext{
		JobID:         jobID,
		CorrelationID: correlationID,
	})
}

// LifecycleContextFrom extracts lifecycle context values from a context.
func LifecycleContextFrom(ctx context.Context) (LifecycleContext, bool) {
	v := ctx.Value(lifecycleContextKey{})
	if v == nil {
		return LifecycleContext{}, false
	}
	data, ok := v.(LifecycleContext)
	return data, ok
}

// PipelineMetricsSnapshot is a lightweight in-memory reliability metrics snapshot.
type PipelineMetricsSnapshot struct {
	OCRLatencyMS         LatencyStats `json:"ocr_latency_ms"`
	TranslationLatencyMS LatencyStats `json:"translation_latency_ms"`
	RenderLatencyMS      LatencyStats `json:"render_latency_ms"`
	QueueWaitMS          LatencyStats `json:"queue_wait_ms"`
	RetriesTotal         int64        `json:"retries_total"`
	FailoversTotal       int64        `json:"failovers_total"`
	TimeoutsTotal        int64        `json:"timeouts_total"`
	ChunkStartedTotal    int64        `json:"chunk_started_total"`
	ChunkCompletedTotal  int64        `json:"chunk_completed_total"`
	ExportStartedTotal   int64        `json:"export_started_total"`
	ExportCompletedTotal int64        `json:"export_completed_total"`
	JobFailedTotal       int64        `json:"job_failed_total"`
	JobCompletedTotal    int64        `json:"job_completed_total"`
	ChunkSuccessRate     float64      `json:"chunk_success_rate"`
	ExportSuccessRate    float64      `json:"export_success_rate"`
}

// LatencyStats is a low-overhead aggregate suitable for Prometheus migration.
type LatencyStats struct {
	TotalMS int64   `json:"total_ms"`
	Count   int64   `json:"count"`
	AvgMS   float64 `json:"avg_ms"`
}

var (
	ocrLatencyTotalMS         atomic.Int64
	ocrLatencyCount           atomic.Int64
	translationLatencyTotalMS atomic.Int64
	translationLatencyCount   atomic.Int64
	renderLatencyTotalMS      atomic.Int64
	renderLatencyCount        atomic.Int64
	queueWaitTotalMS          atomic.Int64
	queueWaitCount            atomic.Int64

	retriesTotal         atomic.Int64
	failoversTotal       atomic.Int64
	timeoutsTotal        atomic.Int64
	chunkStartedTotal    atomic.Int64
	chunkCompletedTotal  atomic.Int64
	exportStartedTotal   atomic.Int64
	exportCompletedTotal atomic.Int64
	jobFailedTotal       atomic.Int64
	jobCompletedTotal    atomic.Int64

	lifecycleEventsMu     sync.RWMutex
	lifecycleEvents       []LifecycleEventRecord
	lifecycleEventsMaxLen = 2000
)

// SnapshotLifecycleEvents returns lifecycle events filtered by optional fields.
func SnapshotLifecycleEvents(correlationID, jobID, event string, limit int) []LifecycleEventRecord {
	if limit <= 0 {
		limit = 100
	}

	lifecycleEventsMu.RLock()
	defer lifecycleEventsMu.RUnlock()

	out := make([]LifecycleEventRecord, 0, limit)
	for i := len(lifecycleEvents) - 1; i >= 0 && len(out) < limit; i-- {
		record := lifecycleEvents[i]
		if correlationID != "" && record.CorrelationID != correlationID {
			continue
		}
		if jobID != "" && record.JobID != jobID {
			continue
		}
		if event != "" && record.Event != event {
			continue
		}
		out = append(out, record)
	}

	return out
}

func appendLifecycleEvent(record LifecycleEventRecord) {
	lifecycleEventsMu.Lock()
	defer lifecycleEventsMu.Unlock()

	lifecycleEvents = append(lifecycleEvents, record)
	if len(lifecycleEvents) > lifecycleEventsMaxLen {
		drop := len(lifecycleEvents) - lifecycleEventsMaxLen
		lifecycleEvents = append([]LifecycleEventRecord(nil), lifecycleEvents[drop:]...)
	}
}

func recordLatency(total *atomic.Int64, count *atomic.Int64, durationMS int64) {
	if durationMS < 0 {
		durationMS = 0
	}
	total.Add(durationMS)
	count.Add(1)
}

func IncRetriesTotal()         { retriesTotal.Add(1) }
func IncFailoversTotal()       { failoversTotal.Add(1) }
func IncTimeoutsTotal()        { timeoutsTotal.Add(1) }
func IncChunkStartedTotal()    { chunkStartedTotal.Add(1) }
func IncChunkCompletedTotal()  { chunkCompletedTotal.Add(1) }
func IncExportStartedTotal()   { exportStartedTotal.Add(1) }
func IncExportCompletedTotal() { exportCompletedTotal.Add(1) }
func IncJobFailedTotal()       { jobFailedTotal.Add(1) }
func IncJobCompletedTotal()    { jobCompletedTotal.Add(1) }

func RecordOCRLatency(ms int64) { recordLatency(&ocrLatencyTotalMS, &ocrLatencyCount, ms) }
func RecordTranslationLatency(ms int64) {
	recordLatency(&translationLatencyTotalMS, &translationLatencyCount, ms)
}
func RecordRenderLatency(ms int64)    { recordLatency(&renderLatencyTotalMS, &renderLatencyCount, ms) }
func RecordQueueWaitLatency(ms int64) { recordLatency(&queueWaitTotalMS, &queueWaitCount, ms) }

func buildLatencyStats(total, count int64) LatencyStats {
	avg := 0.0
	if count > 0 {
		avg = float64(total) / float64(count)
	}
	return LatencyStats{TotalMS: total, Count: count, AvgMS: avg}
}

func ratio(numerator, denominator int64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

// SnapshotPipelineMetrics returns the current in-memory reliability snapshot.
func SnapshotPipelineMetrics() PipelineMetricsSnapshot {
	chunkStarted := chunkStartedTotal.Load()
	chunkCompleted := chunkCompletedTotal.Load()
	exportsStarted := exportStartedTotal.Load()
	exportsCompleted := exportCompletedTotal.Load()

	return PipelineMetricsSnapshot{
		OCRLatencyMS:         buildLatencyStats(ocrLatencyTotalMS.Load(), ocrLatencyCount.Load()),
		TranslationLatencyMS: buildLatencyStats(translationLatencyTotalMS.Load(), translationLatencyCount.Load()),
		RenderLatencyMS:      buildLatencyStats(renderLatencyTotalMS.Load(), renderLatencyCount.Load()),
		QueueWaitMS:          buildLatencyStats(queueWaitTotalMS.Load(), queueWaitCount.Load()),
		RetriesTotal:         retriesTotal.Load(),
		FailoversTotal:       failoversTotal.Load(),
		TimeoutsTotal:        timeoutsTotal.Load(),
		ChunkStartedTotal:    chunkStarted,
		ChunkCompletedTotal:  chunkCompleted,
		ExportStartedTotal:   exportsStarted,
		ExportCompletedTotal: exportsCompleted,
		JobFailedTotal:       jobFailedTotal.Load(),
		JobCompletedTotal:    jobCompletedTotal.Load(),
		ChunkSuccessRate:     ratio(chunkCompleted, chunkStarted),
		ExportSuccessRate:    ratio(exportsCompleted, exportsStarted),
	}
}

// EmitLifecycleEvent writes a structured lifecycle event to logs and updates counters.
func EmitLifecycleEvent(event string, payload LifecycleEvent) {
	if payload.Timestamp == "" {
		payload.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	appendLifecycleEvent(LifecycleEventRecord{Event: event, LifecycleEvent: payload})

	slog.Info("lifecycle_event",
		"event", event,
		"job_id", payload.JobID,
		"correlation_id", payload.CorrelationID,
		"provider", payload.Provider,
		"duration_ms", payload.DurationMS,
		"retry_count", payload.RetryCount,
		"queue_depth", payload.QueueDepth,
		"timestamp", payload.Timestamp,
	)

	switch event {
	case "chunk_started":
		IncChunkStartedTotal()
	case "chunk_completed":
		IncChunkCompletedTotal()
	case "chunk_retry":
		IncRetriesTotal()
	case "provider_failover":
		IncFailoversTotal()
	case "render_completed":
		RecordRenderLatency(payload.DurationMS)
	case "translation_started":
		if payload.DurationMS > 0 {
			RecordTranslationLatency(payload.DurationMS)
		}
	case "ocr_completed":
		RecordOCRLatency(payload.DurationMS)
	case "export_started":
		IncExportStartedTotal()
	case "export_completed":
		IncExportCompletedTotal()
	case "job_failed":
		IncJobFailedTotal()
	case "job_completed":
		IncJobCompletedTotal()
	}
}

// EmitLifecycleEventFromContext emits an event using lineage values from ctx.
func EmitLifecycleEventFromContext(ctx context.Context, event string, payload LifecycleEvent) {
	if data, ok := LifecycleContextFrom(ctx); ok {
		if payload.JobID == "" {
			payload.JobID = data.JobID
		}
		if payload.CorrelationID == "" {
			payload.CorrelationID = data.CorrelationID
		}
	}
	EmitLifecycleEvent(event, payload)
}
