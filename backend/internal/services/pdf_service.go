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
	// ExtractPages reads the PDF at filePath and returns per-page plain text.
	// The slice length equals the page count; empty pages are represented as
	// empty strings rather than being omitted so callers can correlate by index.
	ExtractPages(filePath string) ([]string, error)
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
		text, err := safeExtractPageText(r, pageNum)
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

// ExtractPages opens the PDF at filePath and returns per-page plain text.
// Each entry in the returned slice corresponds to one page (1-indexed internally).
// Pages whose text is empty (e.g. image-only pages) are represented as empty strings.
func (s *pdfService) ExtractPages(filePath string) ([]string, error) {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("pdf: open %q: %w", filePath, err)
	}
	defer f.Close()

	totalPages := r.NumPage()
	if totalPages == 0 {
		return nil, fmt.Errorf("pdf: %q contains no pages", filePath)
	}

	pages := make([]string, totalPages)
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		text, err := safeExtractPageText(r, pageNum)
		if err != nil {
			return nil, fmt.Errorf("pdf: extract text from page %d of %q: %w", pageNum, filePath, err)
		}
		pages[pageNum-1] = strings.TrimSpace(text)
	}
	return pages, nil
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

// safeExtractPageText extracts text from a single page, recovering from any
// panic raised by the ledongthuc/pdf library on malformed content streams.
func safeExtractPageText(r *pdf.Reader, pageNum int) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pdf library panic on page %d: %v", pageNum, r)
		}
	}()
	page := r.Page(pageNum)
	if page.V.IsNull() {
		return "", nil
	}
	return page.GetPlainText(nil)
}
