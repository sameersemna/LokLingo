package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	queueKey    = "loklingo:jobs:queue"
	jobKeyFmt   = "loklingo:job:%s"
	cacheKeyFmt = "loklingo:cache:%s"
	jobTTL      = 24 * time.Hour
	cacheTTL    = 30 * 24 * time.Hour // cache translations for 30 days
)

// ErrNotFound is returned by Get when the job ID does not exist in the store.
var ErrNotFound = errors.New("job not found")

// Store defines the persistence and queue operations for jobs.
type Store interface {
	// Enqueue saves the job and appends its ID to the work queue.
	Enqueue(ctx context.Context, job *Job) error
	// Get retrieves a job by ID. Returns ErrNotFound if absent.
	Get(ctx context.Context, id string) (*Job, error)
	// Update overwrites the stored job state and refreshes its TTL.
	Update(ctx context.Context, job *Job) error
	// Dequeue blocks up to ~1 s for the next job ID. Returns nil, nil on timeout.
	Dequeue(ctx context.Context) (*Job, error)
	// GetCached returns a previously cached translation, or ("", ErrNotFound).
	GetCached(ctx context.Context, text, source, target string) (string, error)
	// SetCached stores a translation result in the cache.
	SetCached(ctx context.Context, text, source, target, translated string) error
}

// CacheKey returns the deterministic Redis key for a translation triple.
// Uses SHA-256 over "source\x00target\x00text" to keep keys short and uniform.
func CacheKey(text, source, target string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s", source, target, text)
	return fmt.Sprintf(cacheKeyFmt, fmt.Sprintf("%x", h.Sum(nil)))
}

type redisStore struct {
	rdb *redis.Client
}

// NewRedisStore constructs a Store backed by the given Redis URL
// (e.g. "redis://:password@host:6379/0").
func NewRedisStore(redisURL string) (Store, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	return &redisStore{rdb: redis.NewClient(opts)}, nil
}

func (s *redisStore) Enqueue(ctx context.Context, job *Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	key := fmt.Sprintf(jobKeyFmt, job.ID)
	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, key, data, jobTTL)
	pipe.LPush(ctx, queueKey, job.ID)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *redisStore) Get(ctx context.Context, id string) (*Job, error) {
	key := fmt.Sprintf(jobKeyFmt, id)
	data, err := s.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	var job Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("unmarshal job: %w", err)
	}
	return &job, nil
}

func (s *redisStore) Update(ctx context.Context, job *Job) error {
	job.UpdatedAt = time.Now()
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	return s.rdb.Set(ctx, fmt.Sprintf(jobKeyFmt, job.ID), data, jobTTL).Err()
}

func (s *redisStore) Dequeue(ctx context.Context) (*Job, error) {
	// Use a 1-second blocking pop so the caller loop can check ctx cancellation.
	result, err := s.rdb.BRPop(ctx, 1*time.Second, queueKey).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil // timeout — no job available
	}
	if err != nil {
		return nil, fmt.Errorf("brpop: %w", err)
	}
	// result[0] = key name, result[1] = job ID
	return s.Get(ctx, result[1])
}

func (s *redisStore) GetCached(ctx context.Context, text, source, target string) (string, error) {
	key := CacheKey(text, source, target)
	val, err := s.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("redis cache get: %w", err)
	}
	return val, nil
}

func (s *redisStore) SetCached(ctx context.Context, text, source, target, translated string) error {
	key := CacheKey(text, source, target)
	return s.rdb.Set(ctx, key, translated, cacheTTL).Err()
}
