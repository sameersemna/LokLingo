package handlers

import (
	"context"
	"fmt"
	"strings"

	"loklingo/backend/internal/observability"
	"loklingo/backend/jobs"

	"github.com/gofiber/fiber/v2"
)

// PrometheusMetricsHandler exposes low-overhead process metrics in Prometheus format.
type PrometheusMetricsHandler struct {
	store             jobs.Store
	healthSnapshotter providerHealthSnapshotter
}

func NewPrometheusMetricsHandler(store jobs.Store, healthSnapshotter ...providerHealthSnapshotter) *PrometheusMetricsHandler {
	h := &PrometheusMetricsHandler{store: store}
	if len(healthSnapshotter) > 0 {
		h.healthSnapshotter = healthSnapshotter[0]
	}
	return h
}

// Expose handles GET /api/v1/metrics/prometheus.
func (h *PrometheusMetricsHandler) Expose(c *fiber.Ctx) error {
	pipeline := observability.SnapshotPipelineMetrics()
	providers := observability.SnapshotProviderMetrics()
	timeouts := observability.SnapshotTimeoutReasonMetrics()
	checkpoints := observability.SnapshotChunkCheckpointMetrics()
	degraded := observability.SnapshotDegradedModeMetrics()
	policyScores := []struct {
		Provider string
		Score    float64
	}{}
	if h.healthSnapshotter != nil {
		for _, p := range h.healthSnapshotter.SnapshotProviderHealth() {
			policyScores = append(policyScores, struct {
				Provider string
				Score    float64
			}{Provider: p.Provider, Score: p.Score})
		}
	}
	queue := jobs.QueueStats{}

	if provider, ok := h.store.(queueMetricsProvider); ok {
		stats, err := provider.QueueStats(context.Background())
		if err == nil {
			queue = stats
		}
	}

	var b strings.Builder
	b.WriteString("# TYPE loklingo_pipeline_retries_total counter\n")
	fmt.Fprintf(&b, "loklingo_pipeline_retries_total %d\n", pipeline.RetriesTotal)
	b.WriteString("# TYPE loklingo_pipeline_failovers_total counter\n")
	fmt.Fprintf(&b, "loklingo_pipeline_failovers_total %d\n", pipeline.FailoversTotal)
	b.WriteString("# TYPE loklingo_pipeline_timeouts_total counter\n")
	fmt.Fprintf(&b, "loklingo_pipeline_timeouts_total %d\n", pipeline.TimeoutsTotal)
	b.WriteString("# TYPE loklingo_pipeline_chunk_success_ratio gauge\n")
	fmt.Fprintf(&b, "loklingo_pipeline_chunk_success_ratio %g\n", pipeline.ChunkSuccessRate)
	b.WriteString("# TYPE loklingo_pipeline_export_success_ratio gauge\n")
	fmt.Fprintf(&b, "loklingo_pipeline_export_success_ratio %g\n", pipeline.ExportSuccessRate)

	b.WriteString("# TYPE loklingo_pipeline_ocr_latency_avg_ms gauge\n")
	fmt.Fprintf(&b, "loklingo_pipeline_ocr_latency_avg_ms %g\n", pipeline.OCRLatencyMS.AvgMS)
	b.WriteString("# TYPE loklingo_pipeline_translation_latency_avg_ms gauge\n")
	fmt.Fprintf(&b, "loklingo_pipeline_translation_latency_avg_ms %g\n", pipeline.TranslationLatencyMS.AvgMS)
	b.WriteString("# TYPE loklingo_pipeline_render_latency_avg_ms gauge\n")
	fmt.Fprintf(&b, "loklingo_pipeline_render_latency_avg_ms %g\n", pipeline.RenderLatencyMS.AvgMS)
	b.WriteString("# TYPE loklingo_pipeline_queue_wait_avg_ms gauge\n")
	fmt.Fprintf(&b, "loklingo_pipeline_queue_wait_avg_ms %g\n", pipeline.QueueWaitMS.AvgMS)

	b.WriteString("# TYPE loklingo_queue_depth gauge\n")
	fmt.Fprintf(&b, "loklingo_queue_depth %d\n", queue.QueueDepth)
	b.WriteString("# TYPE loklingo_queue_processing_concurrency gauge\n")
	fmt.Fprintf(&b, "loklingo_queue_processing_concurrency %d\n", queue.InflightDepth)
	b.WriteString("# TYPE loklingo_queue_stuck_jobs gauge\n")
	fmt.Fprintf(&b, "loklingo_queue_stuck_jobs %d\n", queue.StuckJobs)
	b.WriteString("# TYPE loklingo_queue_retry_backlog gauge\n")
	fmt.Fprintf(&b, "loklingo_queue_retry_backlog %d\n", queue.RetryBacklog)
	b.WriteString("# TYPE loklingo_queue_dead_letter_volume gauge\n")
	fmt.Fprintf(&b, "loklingo_queue_dead_letter_volume %d\n", queue.DeadLetterCount)

	b.WriteString("# TYPE loklingo_provider_success_total counter\n")
	b.WriteString("# TYPE loklingo_provider_failure_total counter\n")
	b.WriteString("# TYPE loklingo_provider_retry_total counter\n")
	b.WriteString("# TYPE loklingo_provider_failover_total counter\n")
	b.WriteString("# TYPE loklingo_provider_timeout_total counter\n")
	b.WriteString("# TYPE loklingo_provider_avg_latency_ms gauge\n")
	for _, p := range providers {
		label := sanitizePromLabel(p.Provider)
		fmt.Fprintf(&b, "loklingo_provider_success_total{provider=\"%s\"} %d\n", label, p.SuccessTotal)
		fmt.Fprintf(&b, "loklingo_provider_failure_total{provider=\"%s\"} %d\n", label, p.FailureTotal)
		fmt.Fprintf(&b, "loklingo_provider_retry_total{provider=\"%s\"} %d\n", label, p.RetryTotal)
		fmt.Fprintf(&b, "loklingo_provider_failover_total{provider=\"%s\"} %d\n", label, p.FailoverTotal)
		fmt.Fprintf(&b, "loklingo_provider_timeout_total{provider=\"%s\"} %d\n", label, p.TimeoutTotal)
		fmt.Fprintf(&b, "loklingo_provider_avg_latency_ms{provider=\"%s\"} %g\n", label, p.AvgLatencyMS)
	}

	b.WriteString("# TYPE loklingo_provider_timeout_reason_total counter\n")
	for _, tr := range timeouts {
		reason := sanitizePromLabel(tr.Reason)
		fmt.Fprintf(&b, "loklingo_provider_timeout_reason_total{reason=\"%s\"} %d\n", reason, tr.Total)
	}

	b.WriteString("# TYPE loklingo_provider_policy_score gauge\n")
	for _, p := range policyScores {
		label := sanitizePromLabel(p.Provider)
		fmt.Fprintf(&b, "loklingo_provider_policy_score{provider=\"%s\"} %g\n", label, p.Score)
	}

	b.WriteString("# TYPE loklingo_chunk_checkpoint_hit_total counter\n")
	fmt.Fprintf(&b, "loklingo_chunk_checkpoint_hit_total %d\n", checkpoints.HitTotal)
	b.WriteString("# TYPE loklingo_chunk_checkpoint_miss_total counter\n")
	fmt.Fprintf(&b, "loklingo_chunk_checkpoint_miss_total %d\n", checkpoints.MissTotal)
	b.WriteString("# TYPE loklingo_chunk_checkpoint_persist_failure_total counter\n")
	fmt.Fprintf(&b, "loklingo_chunk_checkpoint_persist_failure_total %d\n", checkpoints.PersistFailureTotal)
	b.WriteString("# TYPE loklingo_chunk_checkpoint_clear_total counter\n")
	fmt.Fprintf(&b, "loklingo_chunk_checkpoint_clear_total %d\n", checkpoints.ClearTotal)
	b.WriteString("# TYPE loklingo_chunk_checkpoint_clear_failure_total counter\n")
	fmt.Fprintf(&b, "loklingo_chunk_checkpoint_clear_failure_total %d\n", checkpoints.ClearFailureTotal)

	b.WriteString("# TYPE loklingo_degraded_mode_total counter\n")
	fmt.Fprintf(&b, "loklingo_degraded_mode_total %d\n", degraded.TotalEvents)
	b.WriteString("# TYPE loklingo_degraded_render_fallback_total counter\n")
	fmt.Fprintf(&b, "loklingo_degraded_render_fallback_total %d\n", degraded.RenderFallbackEvents)
	b.WriteString("# TYPE loklingo_degraded_ocr_low_confidence_total counter\n")
	fmt.Fprintf(&b, "loklingo_degraded_ocr_low_confidence_total %d\n", degraded.OCRLowConfidenceEvents)
	b.WriteString("# TYPE loklingo_adaptive_concurrency_reduce_total counter\n")
	fmt.Fprintf(&b, "loklingo_adaptive_concurrency_reduce_total %d\n", degraded.AdaptiveReduceEvents)
	b.WriteString("# TYPE loklingo_adaptive_concurrency_boost_total counter\n")
	fmt.Fprintf(&b, "loklingo_adaptive_concurrency_boost_total %d\n", degraded.AdaptiveBoostEvents)
	b.WriteString("# TYPE loklingo_adaptive_concurrency_clamp_total counter\n")
	fmt.Fprintf(&b, "loklingo_adaptive_concurrency_clamp_total %d\n", degraded.AdaptiveClampEvents)

	c.Set(fiber.HeaderContentType, "text/plain; version=0.0.4; charset=utf-8")
	return c.SendString(b.String())
}

func sanitizePromLabel(v string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"\n", "\\n",
	)
	return replacer.Replace(v)
}
