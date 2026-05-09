package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedisStore(t *testing.T) *redisStore {
	t.Helper()
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mini.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	return &redisStore{rdb: rdb}
}

func TestRedisStore_ReplayDead_RequiresDeadLetterMembership(t *testing.T) {
	s := newTestRedisStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	job := &Job{
		ID:          "job-not-dead",
		Status:      StatusFailed,
		Type:        TypePDF,
		Mode:        ModeOverlay,
		Source:      "en",
		Target:      "de",
		CreatedAt:   now,
		UpdatedAt:   now,
		Attempt:     3,
		MaxAttempts: 3,
		ErrorMsg:    "rate limit",
	}
	if err := s.Update(ctx, job); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	_, err := s.ReplayDead(ctx, job.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-dead job replay, got %v", err)
	}

	got, err := s.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job after failed replay: %v", err)
	}
	if got.Attempt != 3 {
		t.Fatalf("expected attempt unchanged=3, got %d", got.Attempt)
	}
	if got.Status != StatusFailed {
		t.Fatalf("expected status failed to remain unchanged, got %s", got.Status)
	}
}

func TestRedisStore_ReplayDead_RequeuesDeadLetteredJob(t *testing.T) {
	s := newTestRedisStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	job := &Job{
		ID:             "job-dead",
		Status:         StatusFailed,
		Type:           TypeImage,
		Mode:           ModeOverlay,
		Source:         "en",
		Target:         "de",
		CreatedAt:      now,
		UpdatedAt:      now,
		Attempt:        3,
		MaxAttempts:    3,
		ErrorMsg:       "rate limit",
		LastErrorMsg:   "rate limit",
		DeadLetteredAt: now,
	}
	if err := s.Update(ctx, job); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	if err := s.DeadLetter(ctx, job.ID, "rate limit"); err != nil {
		t.Fatalf("seed dead letter: %v", err)
	}

	replayed, err := s.ReplayDead(ctx, job.ID)
	if err != nil {
		t.Fatalf("replay dead job: %v", err)
	}
	if replayed.Status != StatusPending {
		t.Fatalf("expected replayed status pending, got %s", replayed.Status)
	}
	if replayed.Attempt != 0 {
		t.Fatalf("expected replayed attempt reset to 0, got %d", replayed.Attempt)
	}
	if !replayed.DeadLetteredAt.IsZero() {
		t.Fatalf("expected dead_lettered_at cleared, got %s", replayed.DeadLetteredAt)
	}

	entries, err := s.ListDead(ctx, 10)
	if err != nil {
		t.Fatalf("list dead after replay: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected dead-letter queue to be empty after replay, got %d", len(entries))
	}

	got, err := s.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("get replayed job: %v", err)
	}
	if got.Status != StatusPending {
		t.Fatalf("expected stored job status pending after replay, got %s", got.Status)
	}

	next, err := s.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue replayed job: %v", err)
	}
	if next == nil || next.ID != job.ID {
		t.Fatalf("expected replayed job %s in active queue, got %+v", job.ID, next)
	}
}
