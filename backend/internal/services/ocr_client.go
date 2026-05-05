package services

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	baseURL    string
	httpClient *http.Client
}

// NewOCRClient constructs an OCRClient that will call ocrBaseURL.
// A zero ocrBaseURL is valid — callers should treat that as OCR disabled.
func NewOCRClient(ocrBaseURL string) OCRClient {
	return &ocrClient{
		baseURL: ocrBaseURL,
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

	text, status, err := c.extractTextMultipart(filePath, lang)
	if err == nil {
		return text, nil
	}

	// The currently deployed OCR service expects JSON. Keep compatibility
	// without buffering the whole file in memory.
	if status == http.StatusBadRequest || status == http.StatusUnsupportedMediaType || status == http.StatusUnprocessableEntity {
		return c.extractTextJSONStream(filePath, lang)
	}

	return "", err
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
