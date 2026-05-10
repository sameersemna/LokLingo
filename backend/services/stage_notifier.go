package services

import "context"

// StageNotifyFn is called by the Orchestrator to report real-time state
// changes (e.g. "retrying", "fallback_provider") back to the caller.
// The caller is responsible for persisting the state change if desired.
//
// stage    — one of the Stage* constants in the jobs package (imported by
//
//	the caller; the orchestrator uses bare strings to avoid a circular dep).
//
// message  — human-readable description suitable for display in the UI.
// progress — [0, 1] completion estimate; may be 0 when unknown.
type StageNotifyFn func(stage, message string, progress float64)

type stageNotifierKey struct{}

// WithStageNotifier attaches fn to ctx so that the Orchestrator can report
// progress events without changing the TranslationService interface.
func WithStageNotifier(ctx context.Context, fn StageNotifyFn) context.Context {
	return context.WithValue(ctx, stageNotifierKey{}, fn)
}

// stageNotifierFrom extracts the notifier from ctx, or returns nil.
func stageNotifierFrom(ctx context.Context) StageNotifyFn {
	fn, _ := ctx.Value(stageNotifierKey{}).(StageNotifyFn)
	return fn
}

// notify is a convenience helper: calls fn if non-nil.
func notify(ctx context.Context, stage, message string, progress float64) {
	if fn := stageNotifierFrom(ctx); fn != nil {
		fn(stage, message, progress)
	}
}
