package services

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"loklingo/backend/internal/httputil"
	"loklingo/backend/internal/observability"
)

// OCRClient extracts text from PDF files by calling the remote OCR service.
// It is intentionally separate from PDFService: PDFService does Go-native
// text extraction; OCRClient is the HTTP fallback for image-based PDFs.
type OCRClient interface {
	// ExtractText reads the PDF at filePath, sends it to the OCR service,
	// and returns the extracted plain text. lang is an ISO 639-1 code or
	// "auto". An empty string for lang is treated as "auto".
	ExtractText(filePath, lang string) (string, error)
	// ExtractPages reads the PDF at filePath via the OCR service and returns
	// per-page plain text in document order. If the service does not return
	// per-page data, the full text is returned as a single-element slice.
	ExtractPages(filePath, lang string) ([]string, error)
	// ExtractImageBlocks reads an image file and returns OCR blocks with text
	// and axis-aligned bounding boxes.
	ExtractImageBlocks(filePath, lang string) ([]OCRTextBlock, error)
}

type ocrClient struct {
	baseURL          string
	sharedStorageDir string
	httpClient       *http.Client
}

// OCRRequestOptions carries optional request hints for OCR service features
// that should not break legacy call sites.
type OCRRequestOptions struct {
	Mode               string
	CorrectionEnabled  *bool
	CorrectionModel    string
	VisualDiffMode     bool
	CorrectionTimeoutS int
}

// NormalizeOCRServiceURL removes a trailing /ocr path segment so callers can
// configure either http://host:port or http://host:port/ocr without double-
// prefixing request paths.
func NormalizeOCRServiceURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	trimmed := strings.TrimRight(raw, "/")
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	switch parsed.Path {
	case "/ocr", "/api/v1/ocr", "/api/v1":
		parsed.Path = ""
	}
	return strings.TrimRight(parsed.String(), "/")
}

// NewOCRClient constructs an OCRClient that will call ocrBaseURL.
// A zero ocrBaseURL is valid — callers should treat that as OCR disabled.
func NewOCRClient(ocrBaseURL, sharedStorageDir string) OCRClient {
	return &ocrClient{
		baseURL:          NormalizeOCRServiceURL(ocrBaseURL),
		sharedStorageDir: filepath.Clean(sharedStorageDir),
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // OCR on large PDFs can be slow
		},
	}
}

// ocrPDFResponse mirrors the relevant subset of OCRPdfResponse.
type ocrPDFResponse struct {
	Text       string          `json:"text"`
	Pages      []ocrPageResult `json:"pages"`
	Confidence float64         `json:"confidence,omitempty"`
	Correction *ocrCorrection  `json:"correction,omitempty"`
}

type ocrCorrection struct {
	Applied           bool    `json:"applied"`
	Model             string  `json:"model,omitempty"`
	LatencyMS         float64 `json:"latency_ms,omitempty"`
	ChangedCharacters int64   `json:"changed_characters,omitempty"`
	ConfidenceDelta   float64 `json:"confidence_delta,omitempty"`
	Retries           int64   `json:"retries,omitempty"`
}

// OCRTextBlock mirrors TextBlock from the OCR service.
type OCRTextBlock struct {
	Text           string    `json:"text"`
	Bbox           []float64 `json:"bbox"` // [x1, y1, x2, y2]
	Confidence     float64   `json:"confidence,omitempty"`
	ReadingOrder   int       `json:"reading_order,omitempty"`
	RegionClass    string    `json:"region_class,omitempty"`    // "title", "heading", "body", "code", etc.
	HierarchyLevel int       `json:"hierarchy_level,omitempty"` // 0=body, 1=title, 2=heading, 3=subheading
	ColumnID       int       `json:"column_id,omitempty"`       // 0-based column index in multi-column layouts
}

type ocrPageResult struct {
	Text       string         `json:"text"`
	Blocks     []OCRTextBlock `json:"blocks"`
	Confidence float64        `json:"confidence,omitempty"`
}

type ocrImageResponse struct {
	Text       string         `json:"text"`
	Blocks     []OCRTextBlock `json:"blocks"`
	Correction *ocrCorrection `json:"correction,omitempty"`
}

const ocrPDFPath = "/api/v1/ocr/pdf"
const ocrImagePath = "/api/v1/ocr"

var ocrMaxResponseBodyBytes int64 = 8 * 1024 * 1024

// fetchOCRResponse routes the PDF through shared-path → multipart → JSON-stream
// strategies and returns the decoded OCR response.
func (c *ocrClient) fetchOCRResponse(filePath, lang string, options OCRRequestOptions) (ocrPDFResponse, error) {
	if c.baseURL == "" {
		return ocrPDFResponse{}, fmt.Errorf("ocr: service URL is not configured")
	}
	if lang == "" {
		lang = "auto"
	}
	if _, err := os.Stat(filePath); err != nil {
		return ocrPDFResponse{}, fmt.Errorf("ocr: read file %q: %w", filePath, err)
	}

	useShared := c.canUseSharedPath(filePath)
	slog.Debug("ocr_shared_path_check",
		"file", filePath,
		"shared_dir", c.sharedStorageDir,
		"use_shared", useShared,
	)

	if useShared {
		resp, status, err := c.extractOCRSharedPath(filePath, lang, options)
		if err == nil {
			slog.Info("ocr_request_strategy", "file", filePath, "strategy", "shared_path")
			return resp, nil
		}
		if !shouldFallbackFromSharedPath(status) {
			return ocrPDFResponse{}, err
		}
		slog.Warn("ocr_shared_path_fallback",
			"file", filePath,
			"status", status,
			"err", err,
		)
	}

	resp, status, err := c.extractOCRMultipart(filePath, lang, options)
	if err == nil {
		slog.Info("ocr_request_strategy", "file", filePath, "strategy", "multipart_upload")
		return resp, nil
	}

	// The currently deployed OCR service expects JSON. Keep compatibility
	// without buffering the whole file in memory.
	if status == http.StatusBadRequest || status == http.StatusUnsupportedMediaType || status == http.StatusUnprocessableEntity {
		slog.Warn("ocr_multipart_fallback",
			"file", filePath,
			"status", status,
			"err", err,
		)
		jsonResp, jsonErr := c.extractOCRJSONStream(filePath, lang, options)
		if jsonErr == nil {
			slog.Info("ocr_request_strategy", "file", filePath, "strategy", "json_base64")
		}
		return jsonResp, jsonErr
	}

	return ocrPDFResponse{}, err
}

// ExtractText streams the PDF to the OCR endpoint and returns the full
// concatenated text. It first tries multipart/form-data, then falls back
// to streamed JSON base64 for compatibility.
func (c *ocrClient) ExtractText(filePath, lang string) (string, error) {
	resp, err := c.fetchOCRResponse(filePath, lang, OCRRequestOptions{})
	if err != nil {
		return "", err
	}
	if resp.Text == "" {
		return "", fmt.Errorf("ocr: service returned empty text")
	}
	return resp.Text, nil
}

// ExtractPages streams the PDF to the OCR endpoint and returns per-page text
// in document order. If the service response includes a pages array, each
// element's text is returned. Otherwise the full text is wrapped in a
// single-element slice for backward compatibility with older service versions.
func (c *ocrClient) ExtractPages(filePath, lang string) ([]string, error) {
	resp, err := c.fetchOCRResponse(filePath, lang, OCRRequestOptions{})
	if err != nil {
		return nil, err
	}
	if len(resp.Pages) > 0 {
		pages := make([]string, len(resp.Pages))
		for i, p := range resp.Pages {
			pages[i] = p.Text
		}
		return pages, nil
	}
	// Older service versions only populate the top-level text field.
	if resp.Text == "" {
		return nil, fmt.Errorf("ocr: service returned empty response")
	}
	return []string{resp.Text}, nil
}

// ExtractPagesWithConfidence returns per-page OCR text and an aggregate
// confidence score when available. It is intentionally an extra method (not
// part of OCRClient) so existing callers can type-assert support gradually.
func (c *ocrClient) ExtractPagesWithConfidence(filePath, lang string) ([]string, float64, error) {
	resp, err := c.fetchOCRResponse(filePath, lang, OCRRequestOptions{})
	if err != nil {
		return nil, 0, err
	}
	if len(resp.Pages) > 0 {
		pages := make([]string, len(resp.Pages))
		confidenceTotal := 0.0
		confidenceCount := 0
		for i, p := range resp.Pages {
			pages[i] = p.Text
			if p.Confidence > 0 {
				confidenceTotal += p.Confidence
				confidenceCount++
				continue
			}
			for _, b := range p.Blocks {
				if b.Confidence > 0 {
					confidenceTotal += b.Confidence
					confidenceCount++
				}
			}
		}
		avg := 0.0
		if confidenceCount > 0 {
			avg = confidenceTotal / float64(confidenceCount)
		} else if resp.Confidence > 0 {
			avg = resp.Confidence
		}
		return pages, avg, nil
	}
	if resp.Text == "" {
		return nil, 0, fmt.Errorf("ocr: service returned empty response")
	}
	return []string{resp.Text}, resp.Confidence, nil
}

func (c *ocrClient) ExtractPagesWithConfidenceAndOptions(filePath, lang string, options OCRRequestOptions) ([]string, float64, error) {
	resp, err := c.fetchOCRResponse(filePath, lang, options)
	if err != nil {
		return nil, 0, err
	}
	if len(resp.Pages) > 0 {
		pages := make([]string, len(resp.Pages))
		confidenceTotal := 0.0
		confidenceCount := 0
		for i, p := range resp.Pages {
			pages[i] = p.Text
			if p.Confidence > 0 {
				confidenceTotal += p.Confidence
				confidenceCount++
				continue
			}
			for _, b := range p.Blocks {
				if b.Confidence > 0 {
					confidenceTotal += b.Confidence
					confidenceCount++
				}
			}
		}
		avg := 0.0
		if confidenceCount > 0 {
			avg = confidenceTotal / float64(confidenceCount)
		} else if resp.Confidence > 0 {
			avg = resp.Confidence
		}
		return pages, avg, nil
	}
	if resp.Text == "" {
		return nil, 0, fmt.Errorf("ocr: service returned empty response")
	}
	return []string{resp.Text}, resp.Confidence, nil
}

func (c *ocrClient) ExtractImageBlocks(filePath, lang string) ([]OCRTextBlock, error) {
	return c.ExtractImageBlocksWithOptions(filePath, lang, OCRRequestOptions{})
}

func (c *ocrClient) ExtractImageBlocksWithOptions(filePath, lang string, options OCRRequestOptions) ([]OCRTextBlock, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("ocr: service URL is not configured")
	}
	if lang == "" {
		lang = "auto"
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("ocr: read image %q: %w", filePath, err)
	}
	mimeType := http.DetectContentType(raw)
	body, err := json.Marshal(map[string]interface{}{
		"image_b64":        base64.StdEncoding.EncodeToString(raw),
		"mime_type":        mimeType,
		"lang":             lang,
		"mode":             options.Mode,
		"visual_diff_mode": options.VisualDiffMode,
	})
	if options.CorrectionEnabled != nil {
		var temp map[string]interface{}
		if err := json.Unmarshal(body, &temp); err != nil {
			return nil, fmt.Errorf("ocr: decode image request body for options: %w", err)
		}
		temp["ocr_correction_enabled"] = *options.CorrectionEnabled
		if strings.TrimSpace(options.CorrectionModel) != "" {
			temp["ocr_correction_model"] = strings.TrimSpace(options.CorrectionModel)
		}
		body, err = json.Marshal(temp)
		if err != nil {
			return nil, fmt.Errorf("ocr: rebuild image request body with options: %w", err)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("ocr: build image OCR request body: %w", err)
	}

	buildReq := func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, c.baseURL+ocrImagePath, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("ocr: build image request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}

	httpResp, err := c.doWithRetry(buildReq, 1)
	if err != nil {
		return nil, fmt.Errorf("ocr: POST %s: %w", ocrImagePath, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ocr: service returned HTTP %d", httpResp.StatusCode)
	}

	raw, err = httputil.ReadBodyLimited(httpResp.Body, ocrMaxResponseBodyBytes)
	if err != nil {
		observability.IncOCRResponseRejectedBody()
		slog.Warn("ocr_response_rejected", "endpoint", ocrImagePath, "reason", "body_too_large_or_unreadable", "err", err)
		return nil, fmt.Errorf("ocr: decode image response: %w", err)
	}
	var decoded ocrImageResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		observability.IncOCRResponseRejectedJSON()
		slog.Warn("ocr_response_rejected", "endpoint", ocrImagePath, "reason", "invalid_json", "err", err)
		return nil, fmt.Errorf("ocr: decode image response: %w", err)
	}
	if err := validateImageOCRResponse(decoded); err != nil {
		observability.IncOCRResponseRejectedShape()
		slog.Warn("ocr_response_rejected", "endpoint", ocrImagePath, "reason", "invalid_schema", "err", err)
		return nil, err
	}
	if len(decoded.Blocks) == 0 {
		return nil, fmt.Errorf("ocr: image response returned no blocks")
	}
	if decoded.Correction != nil {
		observability.RecordOCRCorrectionLatency(decoded.Correction.LatencyMS)
		observability.AddOCRCorrectionChangedCharacters(decoded.Correction.ChangedCharacters)
		observability.RecordOCRCorrectionConfidenceDelta(decoded.Correction.ConfidenceDelta)
		if decoded.Correction.Applied {
			observability.IncOCRCorrectionApplied()
		}
	}
	return decoded.Blocks, nil
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
		req, err := buildReq()
		if err != nil {
			return nil, err
		}
		if attempt > 0 {
			observability.IncOCRRetryAttempt()
			slog.Warn("ocr_retry", "attempt", attempt, "backoff_ms", backoff.Milliseconds(), "err", lastErr)
			timer := time.NewTimer(backoff)
			select {
			case <-req.Context().Done():
				timer.Stop()
				observability.IncOCRRetryCancelled()
				return nil, fmt.Errorf("ocr: request cancelled while retrying: %w", req.Context().Err())
			case <-timer.C:
			}
			backoff *= 2
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Network-level error — worth retrying.
			lastErr = err
			continue
		}
		if isTransientStatus(resp.StatusCode) {
			if retryAfter := parseRetryAfter(resp.Header.Get("Retry-After")); retryAfter > 0 {
				observability.IncOCRRetryAfterHonored()
				backoff = retryAfter
			}
			// Drain and close before retrying to free the connection.
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("ocr: service returned HTTP %d", resp.StatusCode)
			continue
		}
		return resp, nil
	}
	observability.IncOCRRetryExhausted()
	return nil, fmt.Errorf("ocr: all %d attempts failed: %w", maxRetries+1, lastErr)
}

func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
		return time.Duration(sec) * time.Second
	}
	if ts, err := http.ParseTime(v); err == nil {
		d := time.Until(ts)
		if d > 0 {
			return d
		}
	}
	return 0
}

func (c *ocrClient) extractOCRSharedPath(filePath, lang string, options OCRRequestOptions) (ocrPDFResponse, int, error) {
	body, err := json.Marshal(map[string]interface{}{
		"file_path":        filePath,
		"lang":             lang,
		"dpi":              200,
		"mode":             options.Mode,
		"visual_diff_mode": options.VisualDiffMode,
	})
	if options.CorrectionEnabled != nil {
		var temp map[string]interface{}
		if err := json.Unmarshal(body, &temp); err != nil {
			return ocrPDFResponse{}, 0, fmt.Errorf("ocr: decode shared-path body for options: %w", err)
		}
		temp["ocr_correction_enabled"] = *options.CorrectionEnabled
		if strings.TrimSpace(options.CorrectionModel) != "" {
			temp["ocr_correction_model"] = strings.TrimSpace(options.CorrectionModel)
		}
		body, err = json.Marshal(temp)
		if err != nil {
			return ocrPDFResponse{}, 0, fmt.Errorf("ocr: rebuild shared-path body with options: %w", err)
		}
	}
	if err != nil {
		return ocrPDFResponse{}, 0, fmt.Errorf("ocr: build shared-path request body: %w", err)
	}

	buildReq := func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, c.baseURL+ocrPDFPath, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("ocr: build shared-path request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}

	httpResp, err := c.doWithRetry(buildReq, 2)
	if err != nil {
		return ocrPDFResponse{}, 0, fmt.Errorf("ocr: POST %s (shared path): %w", ocrPDFPath, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return ocrPDFResponse{}, httpResp.StatusCode, fmt.Errorf("ocr: service returned HTTP %d", httpResp.StatusCode)
	}

	ocrResp, err := decodeOCRResponse(httpResp.Body)
	if err != nil {
		return ocrPDFResponse{}, httpResp.StatusCode, err
	}
	return ocrResp, httpResp.StatusCode, nil
}

func (c *ocrClient) extractOCRMultipart(filePath, lang string, options OCRRequestOptions) (ocrPDFResponse, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return ocrPDFResponse{}, 0, fmt.Errorf("ocr: read file %q: %w", filePath, err)
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
		if strings.TrimSpace(options.Mode) != "" {
			if err := mw.WriteField("mode", strings.TrimSpace(options.Mode)); err != nil {
				_ = pw.CloseWithError(fmt.Errorf("ocr: write multipart mode field: %w", err))
				return
			}
		}
		if options.VisualDiffMode {
			if err := mw.WriteField("visual_diff_mode", "true"); err != nil {
				_ = pw.CloseWithError(fmt.Errorf("ocr: write multipart visual_diff_mode field: %w", err))
				return
			}
		}
		if options.CorrectionEnabled != nil {
			if err := mw.WriteField("ocr_correction_enabled", strconv.FormatBool(*options.CorrectionEnabled)); err != nil {
				_ = pw.CloseWithError(fmt.Errorf("ocr: write multipart correction flag: %w", err))
				return
			}
			if strings.TrimSpace(options.CorrectionModel) != "" {
				if err := mw.WriteField("ocr_correction_model", strings.TrimSpace(options.CorrectionModel)); err != nil {
					_ = pw.CloseWithError(fmt.Errorf("ocr: write multipart correction model: %w", err))
					return
				}
			}
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
		return ocrPDFResponse{}, 0, fmt.Errorf("ocr: build multipart request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return ocrPDFResponse{}, 0, fmt.Errorf("ocr: POST %s (multipart): %w", ocrPDFPath, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return ocrPDFResponse{}, httpResp.StatusCode, fmt.Errorf("ocr: service returned HTTP %d", httpResp.StatusCode)
	}

	ocrResp, err := decodeOCRResponse(httpResp.Body)
	if err != nil {
		return ocrPDFResponse{}, httpResp.StatusCode, err
	}
	return ocrResp, httpResp.StatusCode, nil
}

func (c *ocrClient) extractOCRJSONStream(filePath, lang string, options OCRRequestOptions) (ocrPDFResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return ocrPDFResponse{}, fmt.Errorf("ocr: read file %q: %w", filePath, err)
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

		tail := `","lang":` + strconv.Quote(lang) + `,"dpi":200`
		if strings.TrimSpace(options.Mode) != "" {
			tail += `,"mode":` + strconv.Quote(strings.TrimSpace(options.Mode))
		}
		if options.VisualDiffMode {
			tail += `,"visual_diff_mode":true`
		}
		if options.CorrectionEnabled != nil {
			tail += `,"ocr_correction_enabled":` + strconv.FormatBool(*options.CorrectionEnabled)
			if strings.TrimSpace(options.CorrectionModel) != "" {
				tail += `,"ocr_correction_model":` + strconv.Quote(strings.TrimSpace(options.CorrectionModel))
			}
		}
		tail += `}`
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
		return ocrPDFResponse{}, fmt.Errorf("ocr: build JSON request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return ocrPDFResponse{}, fmt.Errorf("ocr: POST %s (json): %w", ocrPDFPath, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return ocrPDFResponse{}, fmt.Errorf("ocr: service returned HTTP %d", httpResp.StatusCode)
	}

	return decodeOCRResponse(httpResp.Body)
}

func decodeOCRResponse(r io.Reader) (ocrPDFResponse, error) {
	raw, err := httputil.ReadBodyLimited(r, ocrMaxResponseBodyBytes)
	if err != nil {
		observability.IncOCRResponseRejectedBody()
		slog.Warn("ocr_response_rejected", "endpoint", ocrPDFPath, "reason", "body_too_large_or_unreadable", "err", err)
		return ocrPDFResponse{}, fmt.Errorf("ocr: decode response: %w", err)
	}
	var result ocrPDFResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		observability.IncOCRResponseRejectedJSON()
		slog.Warn("ocr_response_rejected", "endpoint", ocrPDFPath, "reason", "invalid_json", "err", err)
		return ocrPDFResponse{}, fmt.Errorf("ocr: decode response: %w", err)
	}
	if err := validatePDFOCRResponse(result); err != nil {
		observability.IncOCRResponseRejectedShape()
		slog.Warn("ocr_response_rejected", "endpoint", ocrPDFPath, "reason", "invalid_schema", "err", err)
		return ocrPDFResponse{}, err
	}
	if result.Text == "" && len(result.Pages) == 0 {
		return ocrPDFResponse{}, fmt.Errorf("ocr: service returned empty response")
	}
	if result.Correction != nil {
		observability.RecordOCRCorrectionLatency(result.Correction.LatencyMS)
		observability.AddOCRCorrectionChangedCharacters(result.Correction.ChangedCharacters)
		observability.RecordOCRCorrectionConfidenceDelta(result.Correction.ConfidenceDelta)
		if result.Correction.Applied {
			observability.IncOCRCorrectionApplied()
		}
	}
	return result, nil
}

func validatePDFOCRResponse(resp ocrPDFResponse) error {
	if len(resp.Pages) == 0 {
		return nil
	}
	for i, page := range resp.Pages {
		for j, block := range page.Blocks {
			if err := validateOCRTextBlock(block); err != nil {
				return fmt.Errorf("ocr: invalid page block %d/%d: %w", i, j, err)
			}
		}
	}
	return nil
}

func validateImageOCRResponse(resp ocrImageResponse) error {
	for i, block := range resp.Blocks {
		if err := validateOCRTextBlock(block); err != nil {
			return fmt.Errorf("ocr: invalid image block %d: %w", i, err)
		}
	}
	return nil
}

func validateOCRTextBlock(block OCRTextBlock) error {
	if len(block.Bbox) != 4 {
		return fmt.Errorf("bbox must have 4 values")
	}
	for _, v := range block.Bbox {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("bbox contains non-finite value")
		}
	}
	if block.Bbox[2] <= block.Bbox[0] || block.Bbox[3] <= block.Bbox[1] {
		return fmt.Errorf("bbox has invalid geometry")
	}
	return nil
}
