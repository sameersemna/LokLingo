package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	queueKey           = "loklingo:jobs:queue"
	inflightQueueKey   = "loklingo:jobs:inflight"
	inflightZSetKey    = "loklingo:jobs:inflight:zset"
	retryZSetKey       = "loklingo:jobs:retry:zset"
	deadLetterQueueKey = "loklingo:jobs:dead:queue"
	deadLetterMetaKey  = "loklingo:jobs:dead:meta"
	jobKeyFmt          = "loklingo:job:%s"
	jobChunkKeyFmt     = "loklingo:job:%s:chunk:%s"
	cacheKeyFmt        = "loklingo:cache:%s"
	jobTTL             = 24 * time.Hour
	cacheTTL           = 30 * 24 * time.Hour // cache translations for 30 days
	defaultRecoverTTL  = 10 * time.Minute
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

// QueueStats returns current queue health counters from Redis.
func (s *redisStore) QueueStats(ctx context.Context) (QueueStats, error) {
	queueDepth, err := s.rdb.LLen(ctx, queueKey).Result()
	if err != nil {
		return QueueStats{}, fmt.Errorf("queue depth: %w", err)
	}
	inflightDepth, err := s.rdb.LLen(ctx, inflightQueueKey).Result()
	if err != nil {
		return QueueStats{}, fmt.Errorf("inflight depth: %w", err)
	}
	retryBacklog, err := s.rdb.ZCard(ctx, retryZSetKey).Result()
	if err != nil {
		return QueueStats{}, fmt.Errorf("retry backlog: %w", err)
	}
	deadLetterCount, err := s.rdb.LLen(ctx, deadLetterQueueKey).Result()
	if err != nil {
		return QueueStats{}, fmt.Errorf("dead-letter volume: %w", err)
	}

	threshold := time.Now().Add(-defaultRecoverTTL).Unix()
	stuckJobs, err := s.rdb.ZCount(ctx, inflightZSetKey, "-inf", strconv.FormatInt(threshold, 10)).Result()
	if err != nil {
		return QueueStats{}, fmt.Errorf("stuck jobs: %w", err)
	}

	return QueueStats{
		QueueDepth:      queueDepth,
		InflightDepth:   inflightDepth,
		StuckJobs:       stuckJobs,
		RetryBacklog:    retryBacklog,
		DeadLetterCount: deadLetterCount,
	}, nil
}

type chunkCheckpointUnit struct {
	PageIndex int    `json:"page_index"`
	Text      string `json:"text"`
	Words     int    `json:"words"`
}

type chunkCheckpointRecord struct {
	Units []chunkCheckpointUnit `json:"units"`
}

func chunkUnitsToCheckpointUnits(units []chunkUnit) []chunkCheckpointUnit {
	out := make([]chunkCheckpointUnit, 0, len(units))
	for _, u := range units {
		out = append(out, chunkCheckpointUnit{PageIndex: u.pageIndex, Text: u.text, Words: u.words})
	}
	return out
}

func checkpointUnitsToChunkUnits(units []chunkCheckpointUnit) []chunkUnit {
	out := make([]chunkUnit, 0, len(units))
	for _, u := range units {
		out = append(out, chunkUnit{pageIndex: u.PageIndex, text: u.Text, words: u.Words})
	}
	return out
}

// DeadJobEntry represents one dead-letter queue item with optional job payload.
type DeadJobEntry struct {
	ID     string `json:"id"`
	Reason string `json:"reason,omitempty"`
	Job    *Job   `json:"job,omitempty"`
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
	if _, err := s.RecoverStale(ctx, defaultRecoverTTL); err != nil {
		return nil, fmt.Errorf("recover stale inflight: %w", err)
	}
	if _, err := s.promoteDueRetries(ctx, time.Now()); err != nil {
		return nil, fmt.Errorf("promote due retries: %w", err)
	}

	// Use a 1-second blocking pop+move so the caller loop can check ctx cancellation
	// while preserving job IDs in an inflight queue until Ack.
	jobID, err := s.rdb.BRPopLPush(ctx, queueKey, inflightQueueKey, 1*time.Second).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil // timeout — no job available
	}
	if err != nil {
		return nil, fmt.Errorf("brpoplpush: %w", err)
	}
	if zErr := s.rdb.ZAdd(ctx, inflightZSetKey, redis.Z{Score: float64(time.Now().Unix()), Member: jobID}).Err(); zErr != nil {
		return nil, fmt.Errorf("mark inflight: %w", zErr)
	}

	job, getErr := s.Get(ctx, jobID)
	if errors.Is(getErr, ErrNotFound) {
		// Job payload disappeared; ack queue entry so workers don't repeatedly pick it up.
		_ = s.Ack(ctx, jobID)
		return nil, nil
	}
	if getErr != nil {
		return nil, getErr
	}
	return job, nil
}

func (s *redisStore) promoteDueRetries(ctx context.Context, now time.Time) (int, error) {
	nowUnix := now.Unix()

	const promoteRetriesLua = `
local retry = KEYS[1]
local queue = KEYS[2]
local now = tonumber(ARGV[1])

local ids = redis.call('ZRANGEBYSCORE', retry, '-inf', now)
local moved = 0
for _, id in ipairs(ids) do
  redis.call('LPUSH', queue, id)
  redis.call('ZREM', retry, id)
  moved = moved + 1
end
return moved
`

	res, err := s.rdb.Eval(ctx, promoteRetriesLua, []string{retryZSetKey, queueKey}, nowUnix).Result()
	if err != nil {
		return 0, fmt.Errorf("promote retries eval: %w", err)
	}
	switch v := res.(type) {
	case int64:
		return int(v), nil
	case string:
		n, convErr := strconv.Atoi(v)
		if convErr != nil {
			return 0, fmt.Errorf("promote retries parse result: %w", convErr)
		}
		return n, nil
	default:
		return 0, nil
	}
}

// Ack marks a dequeued job as completed from queue bookkeeping.
func (s *redisStore) Ack(ctx context.Context, id string) error {
	pipe := s.rdb.Pipeline()
	pipe.LRem(ctx, inflightQueueKey, 1, id)
	pipe.ZRem(ctx, inflightZSetKey, id)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("ack inflight job: %w", err)
	}
	return nil
}

// RecoverStale moves stale inflight job IDs back to the processing queue.
// It returns the number of recovered job IDs.
func (s *redisStore) RecoverStale(ctx context.Context, olderThan time.Duration) (int, error) {
	if olderThan <= 0 {
		olderThan = defaultRecoverTTL
	}
	threshold := time.Now().Add(-olderThan).Unix()

	const recoverStaleLua = `
local zset = KEYS[1]
local inflight = KEYS[2]
local queue = KEYS[3]
local threshold = tonumber(ARGV[1])

local ids = redis.call('ZRANGEBYSCORE', zset, '-inf', threshold)
local moved = 0
for _, id in ipairs(ids) do
  local removed = redis.call('LREM', inflight, 1, id)
  if removed > 0 then
    redis.call('LPUSH', queue, id)
    moved = moved + 1
  end
  redis.call('ZREM', zset, id)
end
return moved
`

	res, err := s.rdb.Eval(ctx, recoverStaleLua, []string{inflightZSetKey, inflightQueueKey, queueKey}, threshold).Result()
	if err != nil {
		return 0, fmt.Errorf("recover stale eval: %w", err)
	}
	switch v := res.(type) {
	case int64:
		return int(v), nil
	case string:
		n, convErr := strconv.Atoi(v)
		if convErr != nil {
			return 0, fmt.Errorf("recover stale parse result: %w", convErr)
		}
		return n, nil
	default:
		return 0, nil
	}
}

// Requeue schedules a failed job ID to be retried after delay.
// delay <= 0 causes immediate enqueue.
func (s *redisStore) Requeue(ctx context.Context, id string, delay time.Duration) error {
	if delay <= 0 {
		if err := s.rdb.LPush(ctx, queueKey, id).Err(); err != nil {
			return fmt.Errorf("requeue immediate: %w", err)
		}
		return nil
	}
	dueAt := time.Now().Add(delay).Unix()
	if err := s.rdb.ZAdd(ctx, retryZSetKey, redis.Z{Score: float64(dueAt), Member: id}).Err(); err != nil {
		return fmt.Errorf("requeue delayed: %w", err)
	}
	return nil
}

// DeadLetter records terminally-failed jobs in a dedicated queue and metadata hash.
func (s *redisStore) DeadLetter(ctx context.Context, id string, reason string) error {
	pipe := s.rdb.Pipeline()
	pipe.LPush(ctx, deadLetterQueueKey, id)
	pipe.HSet(ctx, deadLetterMetaKey, id, reason)
	pipe.ZRem(ctx, retryZSetKey, id)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("dead letter job: %w", err)
	}
	return nil
}

// ListDead returns up to limit dead-letter entries in newest-first order.
func (s *redisStore) ListDead(ctx context.Context, limit int) ([]DeadJobEntry, error) {
	if limit <= 0 {
		limit = 25
	}
	ids, err := s.rdb.LRange(ctx, deadLetterQueueKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("list dead jobs: %w", err)
	}
	entries := make([]DeadJobEntry, 0, len(ids))
	for _, id := range ids {
		reason, rErr := s.rdb.HGet(ctx, deadLetterMetaKey, id).Result()
		if errors.Is(rErr, redis.Nil) {
			reason = ""
		} else if rErr != nil {
			return nil, fmt.Errorf("get dead-letter reason for %s: %w", id, rErr)
		}
		job, jErr := s.Get(ctx, id)
		if errors.Is(jErr, ErrNotFound) {
			entries = append(entries, DeadJobEntry{ID: id, Reason: reason})
			continue
		}
		if jErr != nil {
			return nil, fmt.Errorf("get dead-letter job %s: %w", id, jErr)
		}
		entries = append(entries, DeadJobEntry{ID: id, Reason: reason, Job: job})
	}
	return entries, nil
}

// ReplayDead removes a dead-letter job from dead-letter structures and re-enqueues it.
func (s *redisStore) ReplayDead(ctx context.Context, id string) (*Job, error) {
	job, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	job.Status = StatusPending
	job.ErrorMsg = ""
	job.LastErrorMsg = ""
	job.NextRetryAt = time.Time{}
	job.DeadLetteredAt = time.Time{}
	job.Attempt = 0
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = 3
	}
	job.UpdatedAt = now

	data, err := json.Marshal(job)
	if err != nil {
		return nil, fmt.Errorf("marshal replay job: %w", err)
	}

	const replayDeadLua = `
local deadQueue = KEYS[1]
local deadMeta = KEYS[2]
local retrySet = KEYS[3]
local queue = KEYS[4]
local jobKey = KEYS[5]
local id = ARGV[1]
local payload = ARGV[2]
local ttl = tonumber(ARGV[3])

local removed = redis.call('LREM', deadQueue, 0, id)
if removed == 0 then
  return 0
end
redis.call('HDEL', deadMeta, id)
redis.call('ZREM', retrySet, id)
redis.call('SET', jobKey, payload, 'EX', ttl)
redis.call('LPUSH', queue, id)
return removed
`

	res, err := s.rdb.Eval(
		ctx,
		replayDeadLua,
		[]string{deadLetterQueueKey, deadLetterMetaKey, retryZSetKey, queueKey, fmt.Sprintf(jobKeyFmt, id)},
		id,
		string(data),
		int(jobTTL.Seconds()),
	).Result()
	if err != nil {
		return nil, fmt.Errorf("replay dead job: %w", err)
	}

	removedCount := int64(0)
	switch v := res.(type) {
	case int64:
		removedCount = v
	case string:
		n, convErr := strconv.ParseInt(v, 10, 64)
		if convErr != nil {
			return nil, fmt.Errorf("replay dead job parse result: %w", convErr)
		}
		removedCount = n
	default:
		return nil, fmt.Errorf("replay dead job unexpected result type %T", res)
	}
	if removedCount == 0 {
		return nil, ErrNotFound
	}

	return job, nil
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

// GetChunkCheckpoint returns previously persisted chunk output units for jobID
// and chunkKey. found=false indicates there is no persisted checkpoint.
func (s *redisStore) GetChunkCheckpoint(ctx context.Context, jobID, chunkKey string) ([]chunkUnit, bool, error) {
	key := fmt.Sprintf(jobChunkKeyFmt, jobID, chunkKey)
	data, err := s.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("redis get chunk checkpoint: %w", err)
	}
	var rec chunkCheckpointRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, false, fmt.Errorf("unmarshal chunk checkpoint: %w", err)
	}
	return checkpointUnitsToChunkUnits(rec.Units), true, nil
}

// SetChunkCheckpoint persists chunk output units for later resume/retry reuse.
func (s *redisStore) SetChunkCheckpoint(ctx context.Context, jobID, chunkKey string, units []chunkUnit) error {
	key := fmt.Sprintf(jobChunkKeyFmt, jobID, chunkKey)
	data, err := json.Marshal(chunkCheckpointRecord{Units: chunkUnitsToCheckpointUnits(units)})
	if err != nil {
		return fmt.Errorf("marshal chunk checkpoint: %w", err)
	}
	if err := s.rdb.Set(ctx, key, data, jobTTL).Err(); err != nil {
		return fmt.Errorf("redis set chunk checkpoint: %w", err)
	}
	return nil
}

// ClearChunkCheckpoints removes all persisted chunk checkpoints for jobID.
func (s *redisStore) ClearChunkCheckpoints(ctx context.Context, jobID string) error {
	if strings.TrimSpace(jobID) == "" {
		return fmt.Errorf("job id is required")
	}
	pattern := fmt.Sprintf(jobChunkKeyFmt, jobID, "*")
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("scan chunk checkpoints: %w", err)
		}
		if len(keys) > 0 {
			if err := s.rdb.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("delete chunk checkpoints: %w", err)
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}
