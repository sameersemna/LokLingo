package jobs

import (
	"context"
	"testing"
)

func TestRedisStore_ChunkCheckpoint_RoundTrip(t *testing.T) {
	s := newTestRedisStore(t)
	ctx := context.Background()

	jobID := "job-ckpt-1"
	chunkKey := "chunk-key-1"
	want := []chunkUnit{
		{pageIndex: 0, text: "eins", words: 1},
		{pageIndex: 1, text: "zwei", words: 1},
	}

	if err := s.SetChunkCheckpoint(ctx, jobID, chunkKey, want); err != nil {
		t.Fatalf("set chunk checkpoint: %v", err)
	}

	got, found, err := s.GetChunkCheckpoint(ctx, jobID, chunkKey)
	if err != nil {
		t.Fatalf("get chunk checkpoint: %v", err)
	}
	if !found {
		t.Fatal("expected checkpoint to exist")
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d units, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i].pageIndex != want[i].pageIndex || got[i].text != want[i].text || got[i].words != want[i].words {
			t.Fatalf("unit %d mismatch: got=%+v want=%+v", i, got[i], want[i])
		}
	}
}

func TestRedisStore_ChunkCheckpoint_Missing(t *testing.T) {
	s := newTestRedisStore(t)
	ctx := context.Background()

	got, found, err := s.GetChunkCheckpoint(ctx, "job-ckpt-missing", "missing-key")
	if err != nil {
		t.Fatalf("get missing checkpoint: %v", err)
	}
	if found {
		t.Fatal("expected checkpoint not found")
	}
	if got != nil {
		t.Fatalf("expected nil units for missing checkpoint, got %+v", got)
	}
}

func TestRedisStore_ClearChunkCheckpoints_RemovesOnlyJobKeys(t *testing.T) {
	s := newTestRedisStore(t)
	ctx := context.Background()

	if err := s.SetChunkCheckpoint(ctx, "job-a", "chunk-1", []chunkUnit{{pageIndex: 0, text: "a1", words: 1}}); err != nil {
		t.Fatalf("seed job-a checkpoint: %v", err)
	}
	if err := s.SetChunkCheckpoint(ctx, "job-a", "chunk-2", []chunkUnit{{pageIndex: 1, text: "a2", words: 1}}); err != nil {
		t.Fatalf("seed job-a checkpoint: %v", err)
	}
	if err := s.SetChunkCheckpoint(ctx, "job-b", "chunk-1", []chunkUnit{{pageIndex: 0, text: "b1", words: 1}}); err != nil {
		t.Fatalf("seed job-b checkpoint: %v", err)
	}

	if err := s.ClearChunkCheckpoints(ctx, "job-a"); err != nil {
		t.Fatalf("clear checkpoints for job-a: %v", err)
	}

	if _, found, err := s.GetChunkCheckpoint(ctx, "job-a", "chunk-1"); err != nil || found {
		t.Fatalf("expected job-a chunk-1 removed, found=%v err=%v", found, err)
	}
	if _, found, err := s.GetChunkCheckpoint(ctx, "job-a", "chunk-2"); err != nil || found {
		t.Fatalf("expected job-a chunk-2 removed, found=%v err=%v", found, err)
	}
	if _, found, err := s.GetChunkCheckpoint(ctx, "job-b", "chunk-1"); err != nil {
		t.Fatalf("expected job-b chunk-1 to remain, get err=%v", err)
	} else if !found {
		t.Fatal("expected job-b chunk-1 checkpoint to remain")
	}
}
