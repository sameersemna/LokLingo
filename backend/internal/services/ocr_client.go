package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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

// ocrPDFRequest mirrors the OCR service's OCRPdfRequest Pydantic model.
type ocrPDFRequest struct {
	PDFB64 string `json:"pdf_b64"`
	Lang   string `json:"lang"`
	DPI    int    `json:"dpi"`
}

// ocrPDFResponse mirrors the relevant subset of OCRPdfResponse.
type ocrPDFResponse struct {
	Text string `json:"text"`
}

// ExtractText encodes the PDF as base64, POSTs to <baseURL>/ocr/pdf, and
// returns the top-level "text" field from the response.
func (c *ocrClient) ExtractText(filePath, lang string) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("ocr: service URL is not configured")
	}
	if lang == "" {
		lang = "auto"
	}

	pdfBytes, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("ocr: read file %q: %w", filePath, err)
	}

	reqBody := ocrPDFRequest{
		PDFB64: base64.StdEncoding.EncodeToString(pdfBytes),
		Lang:   lang,
		DPI:    200,
	}
	encoded, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ocr: marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/ocr/pdf",
		"application/json",
		bytes.NewReader(encoded),
	)
	if err != nil {
		return "", fmt.Errorf("ocr: POST /ocr/pdf: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ocr: service returned HTTP %d", resp.StatusCode)
	}

	var result ocrPDFResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ocr: decode response: %w", err)
	}
	if result.Text == "" {
		return "", fmt.Errorf("ocr: service returned empty text")
	}
	return result.Text, nil
}
