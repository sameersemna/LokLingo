package services

import (
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PDFService extracts plain text from PDF files.
type PDFService interface {
	// ExtractText reads the PDF at filePath and returns its full plain text.
	ExtractText(filePath string) (string, error)
	// PageCount returns the number of pages in the PDF.
	PageCount(filePath string) (int, error)
}

type pdfService struct{}

// NewPDFService constructs a PDFService.
func NewPDFService() PDFService {
	return &pdfService{}
}

// ExtractText opens the PDF at filePath, walks every page, and concatenates
// all text content. Pages are separated by a newline.
func (s *pdfService) ExtractText(filePath string) (string, error) {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("pdf: open %q: %w", filePath, err)
	}
	defer f.Close()

	totalPages := r.NumPage()
	if totalPages == 0 {
		return "", fmt.Errorf("pdf: %q contains no pages", filePath)
	}

	var sb strings.Builder
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("pdf: extract text from page %d of %q: %w", pageNum, filePath, err)
		}
		sb.WriteString(text)
		if pageNum < totalPages {
			sb.WriteByte('\n')
		}
	}

	result := strings.TrimSpace(sb.String())
	if result == "" {
		return "", fmt.Errorf("pdf: no text content found in %q (may be image-only; OCR required)", filePath)
	}
	return result, nil
}

func (s *pdfService) PageCount(filePath string) (int, error) {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("pdf: open %q: %w", filePath, err)
	}
	defer f.Close()

	totalPages := r.NumPage()
	if totalPages <= 0 {
		return 0, fmt.Errorf("pdf: %q contains no pages", filePath)
	}
	return totalPages, nil
}
