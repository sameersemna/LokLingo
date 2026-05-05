package services

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestOCRClient_MultipartSuccess(t *testing.T) {
	const expectedText = "Hello from OCR multipart"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/ocr/pdf" {
			t.Errorf("expected path /ocr/pdf, got %s", r.URL.Path)
		}

		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("parse content-type: %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("expected multipart/form-data, got %s", mediaType)
		}

		mr := multipart.NewReader(r.Body, params["boundary"])
		foundFile := false
		foundLang := false
		foundDPI := false
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("read multipart: %v", err)
			}
			b, _ := io.ReadAll(part)
			switch part.FormName() {
			case "file":
				if len(b) == 0 {
					t.Fatal("expected non-empty file part")
				}
				foundFile = true
			case "lang":
				if string(b) != "de" {
					t.Fatalf("expected lang=de, got %q", string(b))
				}
				foundLang = true
			case "dpi":
				if string(b) != "200" {
					t.Fatalf("expected dpi=200, got %q", string(b))
				}
				foundDPI = true
			}
		}
		if !foundFile || !foundLang || !foundDPI {
			t.Fatal("expected multipart fields: file, lang, dpi")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
	_, _ = f.WriteString("%PDF-1.4 test content")
	_ = f.Close()

	client := NewOCRClient(srv.URL)
	got, err := client.ExtractText(f.Name(), "de")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expectedText {
		t.Fatalf("expected %q, got %q", expectedText, got)
	}
}

func TestOCRClient_FallsBackToJSONCompatibility(t *testing.T) {
	const expectedText = "Hello from OCR JSON fallback"
	requestCount := 0
	seenMultipart := false
	seenJSON := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		ct := r.Header.Get("Content-Type")

		if strings.HasPrefix(ct, "multipart/form-data") {
			seenMultipart = true
			// Simulate current OCR service contract rejecting multipart.
			http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
			return
		}

		if strings.HasPrefix(ct, "application/json") {
			seenJSON = true
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode json fallback body: %v", err)
			}
			if _, ok := body["pdf_b64"]; !ok {
				t.Fatal("expected pdf_b64 field in fallback JSON body")
			}
			if encoded, ok := body["pdf_b64"].(string); !ok || encoded == "" {
				t.Fatal("expected non-empty pdf_b64 string")
			} else {
				if _, err := base64.StdEncoding.DecodeString(encoded); err != nil {
					t.Fatalf("expected valid base64 payload: %v", err)
				}
			}
			if l, ok := body["lang"].(string); !ok || l != "auto" {
				t.Fatalf("expected lang=auto in fallback JSON, got %v", body["lang"])
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"text":       expectedText,
				"confidence": 0.93,
				"pages":      []interface{}{},
			})
			return
		}

		t.Fatalf("unexpected content-type: %s", ct)
	}))
	defer srv.Close()

	f, err := os.CreateTemp("", "loklingo-ocr-test-*.pdf")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("%PDF-1.4 streamed")
	_ = f.Close()

	client := NewOCRClient(srv.URL)
	got, err := client.ExtractText(f.Name(), "") // defaults to auto
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expectedText {
		t.Fatalf("expected %q, got %q", expectedText, got)
	}
	if !seenMultipart || !seenJSON {
		t.Fatalf("expected multipart attempt and json fallback; multipart=%v json=%v", seenMultipart, seenJSON)
	}
	if requestCount != 2 {
		t.Fatalf("expected exactly 2 requests (multipart + fallback), got %d", requestCount)
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
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("expected multipart/form-data request, got %s", r.Header.Get("Content-Type"))
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
	var requestCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			// Force fallback path so we can verify current-contract behavior.
			http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
			return
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if l, ok := body["lang"].(string); ok {
			receivedLang = l
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
	if requestCount != 2 {
		t.Fatalf("expected 2 requests (multipart + json fallback), got %d", requestCount)
	}
	if receivedLang != "auto" {
		t.Fatalf("expected lang=auto, got %q", receivedLang)
	}
}
