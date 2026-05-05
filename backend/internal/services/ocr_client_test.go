package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestOCRClient_NoURL(t *testing.T) {
	client := NewOCRClient("")
	_, err := client.ExtractText("/tmp/any.pdf", "auto")
	if err == nil {
		t.Fatal("expected error when OCR URL is empty")
	}
}

func TestOCRClient_NonexistentFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("HTTP call should not be made for a missing file")
	}))
	defer srv.Close()

	client := NewOCRClient(srv.URL)
	_, err := client.ExtractText("/tmp/does-not-exist-ocr-test-99.pdf", "auto")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestOCRClient_ServiceReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	// Write a tiny temp file so the client can read it.
	f, err := os.CreateTemp("", "loklingo-ocr-test-*.pdf")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.WriteString("%PDF-1.4")
	f.Close()

	client := NewOCRClient(srv.URL)
	_, err = client.ExtractText(f.Name(), "auto")
	if err == nil {
		t.Fatal("expected error when OCR service returns 500")
	}
}

func TestOCRClient_ServiceReturnsEmptyText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"text":       "",
			"confidence": 0.0,
			"pages":      []interface{}{},
		})
	}))
	defer srv.Close()

	f, err := os.CreateTemp("", "loklingo-ocr-test-*.pdf")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.WriteString("%PDF-1.4")
	f.Close()

	client := NewOCRClient(srv.URL)
	_, err = client.ExtractText(f.Name(), "auto")
	if err == nil {
		t.Fatal("expected error when OCR service returns empty text")
	}
}

func TestOCRClient_Success(t *testing.T) {
	const expectedText = "Hello from OCR"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/ocr/pdf" {
			t.Errorf("expected path /ocr/pdf, got %s", r.URL.Path)
		}
		// Verify request body has required fields.
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if _, ok := body["pdf_b64"]; !ok {
			t.Error("expected pdf_b64 field in request body")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"text":       expectedText,
			"confidence": 0.95,
			"pages":      []interface{}{},
		})
	}))
	defer srv.Close()

	f, err := os.CreateTemp("", "loklingo-ocr-test-*.pdf")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.WriteString("%PDF-1.4 test content")
	f.Close()

	client := NewOCRClient(srv.URL)
	got, err := client.ExtractText(f.Name(), "de")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expectedText {
		t.Fatalf("expected %q, got %q", expectedText, got)
	}
}

func TestOCRClient_LangDefaultsToAuto(t *testing.T) {
	var receivedLang string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if l, ok := body["lang"].(string); ok {
			receivedLang = l
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"text":       "some text",
			"confidence": 0.9,
			"pages":      []interface{}{},
		})
	}))
	defer srv.Close()

	f, err := os.CreateTemp("", "loklingo-ocr-test-*.pdf")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.WriteString("%PDF-1.4")
	f.Close()

	client := NewOCRClient(srv.URL)
	// Pass empty lang — should default to "auto".
	if _, err := client.ExtractText(f.Name(), ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLang != "auto" {
		t.Fatalf("expected lang=auto, got %q", receivedLang)
	}
}
