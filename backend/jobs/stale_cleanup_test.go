package jobs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanStaleUploads_RemovesOldPDFs(t *testing.T) {
	dir := t.TempDir()

	// Create a stale .pdf file (mtime backdated via Chtimes).
	stalePath := filepath.Join(dir, "stale.pdf")
	if err := os.WriteFile(stalePath, []byte("fake pdf"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(stalePath, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	// Create a recent .pdf file — must NOT be deleted.
	recentPath := filepath.Join(dir, "recent.pdf")
	if err := os.WriteFile(recentPath, []byte("fake pdf"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cleanStaleUploads(dir, 24*time.Hour)

	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Errorf("expected stale file to be removed, stat err: %v", err)
	}
	if _, err := os.Stat(recentPath); os.IsNotExist(err) {
		t.Error("expected recent file to be kept, but it was removed")
	}
}

func TestCleanStaleUploads_IgnoresNonPDFFiles(t *testing.T) {
	dir := t.TempDir()

	// A non-PDF file older than the threshold must be left alone.
	txtPath := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(txtPath, []byte("some text"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(txtPath, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	cleanStaleUploads(dir, 24*time.Hour)

	if _, err := os.Stat(txtPath); os.IsNotExist(err) {
		t.Error("expected non-PDF file to be kept, but it was removed")
	}
}

func TestCleanStaleUploads_MissingDir_NoError(t *testing.T) {
	// Should not panic or return an error when the directory doesn't exist.
	cleanStaleUploads("/tmp/loklingo-nonexistent-testdir-xyz", 24*time.Hour)
}
