package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"loklingo/backend/services"
)

// Worker dequeues jobs and executes translations in the background.
type Worker struct {
	store   Store
	service services.TranslationService
}

// NewWorker constructs a Worker.
func NewWorker(store Store, service services.TranslationService) *Worker {
	return &Worker{store: store, service: service}
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

func (w *Worker) process(ctx context.Context, job *Job) {
	slog.Info("processing translation job", "job_id", job.ID, "source", job.Source, "target", job.Target)

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
