package services

import (
	"os"
	"testing"
)

func TestExtractText_NonexistentFile(t *testing.T) {
	svc := NewPDFService()
	_, err := svc.ExtractText("/tmp/does-not-exist-loklingo-8675309.pdf")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestExtractText_InvalidFile(t *testing.T) {
	// Write garbage bytes to a temp file — not a valid PDF.
	f, err := os.CreateTemp("", "loklingo-test-invalid-*.pdf")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString("this is not a pdf"); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()

	svc := NewPDFService()
	_, err = svc.ExtractText(f.Name())
	if err == nil {
		t.Fatal("expected error for invalid PDF content, got nil")
	}
}

func TestExtractText_EmptyFile(t *testing.T) {
	// An empty file should fail PDF parsing.
	f, err := os.CreateTemp("", "loklingo-test-empty-*.pdf")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.Close()

	svc := NewPDFService()
	_, err = svc.ExtractText(f.Name())
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
}
