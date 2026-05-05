package services

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// OCRClient extracts text from PDF files by calling the remote OCR service.
// It is intentionally separate from PDFService: PDFService does Go-native
// text extraction; OCRClient is the HTTP fallback for image-based PDFs.
type OCRClient interface {
	// ExtractText reads the PDF at filePath, sends it to the OCR service,
	// and returns the extracted plain text. lang is an ISO 639-1 code or
	// "auto". An empty string for lang is treated as "auto".
	ExtractText(filePath, lang string) (string, error)
}

type ocrClient struct {
	baseURL          string
	sharedStorageDir string
	httpClient       *http.Client
}

// NewOCRClient constructs an OCRClient that will call ocrBaseURL.
// A zero ocrBaseURL is valid — callers should treat that as OCR disabled.
func NewOCRClient(ocrBaseURL, sharedStorageDir string) OCRClient {
	return &ocrClient{
		baseURL:          ocrBaseURL,
		sharedStorageDir: filepath.Clean(sharedStorageDir),
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // OCR on large PDFs can be slow
		},
	}
}

// ocrPDFResponse mirrors the relevant subset of OCRPdfResponse.
type ocrPDFResponse struct {
	Text string `json:"text"`
}

const ocrPDFPath = "/ocr/pdf"

// ExtractText streams the PDF to the OCR endpoint. It first tries
// multipart/form-data (future contract), then falls back to streamed JSON
// base64 (current contract) for compatibility.
func (c *ocrClient) ExtractText(filePath, lang string) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("ocr: service URL is not configured")
	}
	if lang == "" {
		lang = "auto"
	}
	if _, err := os.Stat(filePath); err != nil {
		return "", fmt.Errorf("ocr: read file %q: %w", filePath, err)
	}

	useShared := c.canUseSharedPath(filePath)
	slog.Debug("ocr_shared_path_check",
		"file", filePath,
		"shared_dir", c.sharedStorageDir,
		"use_shared", useShared,
	)

	if useShared {
		text, status, err := c.extractTextSharedPath(filePath, lang)
		if err == nil {
			slog.Info("ocr_request_strategy", "file", filePath, "strategy", "shared_path")
			return text, nil
		}
		if !shouldFallbackFromSharedPath(status) {
			return "", err
		}
		slog.Warn("ocr_shared_path_fallback",
			"file", filePath,
			"status", status,
			"err", err,
		)
	}

	text, status, err := c.extractTextMultipart(filePath, lang)
	if err == nil {
		slog.Info("ocr_request_strategy", "file", filePath, "strategy", "multipart_upload")
		return text, nil
	}

	// The currently deployed OCR service expects JSON. Keep compatibility
	// without buffering the whole file in memory.
	if status == http.StatusBadRequest || status == http.StatusUnsupportedMediaType || status == http.StatusUnprocessableEntity {
		slog.Warn("ocr_multipart_fallback",
			"file", filePath,
			"status", status,
			"err", err,
		)
		text, err = c.extractTextJSONStream(filePath, lang)
		if err == nil {
			slog.Info("ocr_request_strategy", "file", filePath, "strategy", "json_base64")
		}
		return text, err
	}

	return "", err
}

func (c *ocrClient) canUseSharedPath(filePath string) bool {
	if strings.TrimSpace(c.sharedStorageDir) == "" {
		return false
	}
	root, err := filepath.Abs(c.sharedStorageDir)
	if err != nil {
		return false
	}
	path, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func shouldFallbackFromSharedPath(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUnsupportedMediaType:
		return true
	default:
		return false
	}
}

// isTransientStatus returns true for HTTP statuses that warrant a retry.
func isTransientStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable
}

// doWithRetry executes buildReq() and retries up to maxRetries times with
// exponential backoff for transient errors (network failures, 429, 503).
// The caller owns the response body and must close it on success.
func (c *ocrClient) doWithRetry(buildReq func() (*http.Request, error), maxRetries int) (*http.Response, error) {
	backoff := 200 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			slog.Warn("ocr_retry", "attempt", attempt, "backoff_ms", backoff.Milliseconds(), "err", lastErr)
			time.Sleep(backoff)
			backoff *= 2
		}
		req, err := buildReq()
		if err != nil {
			return nil, err
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Network-level error — worth retrying.
			lastErr = err
			continue
		}
		if isTransientStatus(resp.StatusCode) {
			// Drain and close before retrying to free the connection.
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("ocr: service returned HTTP %d", resp.StatusCode)
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("ocr: all %d attempts failed: %w", maxRetries+1, lastErr)
}

func (c *ocrClient) extractTextSharedPath(filePath, lang string) (string, int, error) {
	body, err := json.Marshal(map[string]interface{}{
		"file_path": filePath,
		"lang":      lang,
		"dpi":       200,
	})
	if err != nil {
		return "", 0, fmt.Errorf("ocr: build shared-path request body: %w", err)
	}

	buildReq := func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, c.baseURL+ocrPDFPath, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("ocr: build shared-path request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}

	resp, err := c.doWithRetry(buildReq, 2)
	if err != nil {
		return "", 0, fmt.Errorf("ocr: POST %s (shared path): %w", ocrPDFPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, fmt.Errorf("ocr: service returned HTTP %d", resp.StatusCode)
	}

	text, err := decodeOCRText(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	return text, resp.StatusCode, nil
}

func (c *ocrClient) extractTextMultipart(filePath, lang string) (string, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("ocr: read file %q: %w", filePath, err)
	}
	defer file.Close()

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()

		if err := mw.WriteField("lang", lang); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("ocr: write multipart lang field: %w", err))
			return
		}
		if err := mw.WriteField("dpi", "200"); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("ocr: write multipart dpi field: %w", err))
			return
		}

		part, err := mw.CreateFormFile("file", filepath.Base(filePath))
		if err != nil {
			_ = pw.CloseWithError(fmt.Errorf("ocr: create multipart file part: %w", err))
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("ocr: stream multipart file: %w", err))
			return
		}
		if err := mw.Close(); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("ocr: finalize multipart body: %w", err))
			return
		}
	}()

	req, err := http.NewRequest(http.MethodPost, c.baseURL+ocrPDFPath, pr)
	if err != nil {
		return "", 0, fmt.Errorf("ocr: build multipart request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("ocr: POST %s (multipart): %w", ocrPDFPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, fmt.Errorf("ocr: service returned HTTP %d", resp.StatusCode)
	}

	text, err := decodeOCRText(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	return text, resp.StatusCode, nil
}

func (c *ocrClient) extractTextJSONStream(filePath, lang string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("ocr: read file %q: %w", filePath, err)
	}
	defer file.Close()

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		bw := bufio.NewWriter(pw)
		if _, err := bw.WriteString(`{"pdf_b64":"`); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("ocr: write json prefix: %w", err))
			return
		}

		enc := base64.NewEncoder(base64.StdEncoding, bw)
		if _, err := io.Copy(enc, file); err != nil {
			_ = enc.Close()
			_ = pw.CloseWithError(fmt.Errorf("ocr: stream base64 payload: %w", err))
			return
		}
		if err := enc.Close(); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("ocr: close base64 encoder: %w", err))
			return
		}

		tail := `","lang":` + strconv.Quote(lang) + `,"dpi":200}`
		if _, err := bw.WriteString(tail); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("ocr: write json tail: %w", err))
			return
		}
		if err := bw.Flush(); err != nil {
			_ = pw.CloseWithError(fmt.Errorf("ocr: flush json stream: %w", err))
			return
		}
	}()

	req, err := http.NewRequest(http.MethodPost, c.baseURL+ocrPDFPath, pr)
	if err != nil {
		return "", fmt.Errorf("ocr: build JSON request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ocr: POST %s (json): %w", ocrPDFPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ocr: service returned HTTP %d", resp.StatusCode)
	}

	return decodeOCRText(resp.Body)
}

func decodeOCRText(r io.Reader) (string, error) {
	var result ocrPDFResponse
	if err := json.NewDecoder(r).Decode(&result); err != nil {
		return "", fmt.Errorf("ocr: decode response: %w", err)
	}
	if result.Text == "" {
		return "", fmt.Errorf("ocr: service returned empty text")
	}
	return result.Text, nil
}
