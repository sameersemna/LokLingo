package handlers

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"path/filepath"
	"regexp"
	"strings"

	"loklingo/backend/jobs"
)

var languageCodePattern = regexp.MustCompile(`^[a-z]{2,3}(?:-[a-z0-9]{2,8})?$`)

const maxTranslateTextChars = 10_000

func normalizeLangCode(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func isValidSourceLanguage(v string) bool {
	if v == "auto" {
		return true
	}
	return languageCodePattern.MatchString(v)
}

func isValidTargetLanguage(v string) bool {
	if v == "auto" {
		return false
	}
	return languageCodePattern.MatchString(v)
}

func normalizeAndValidateText(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("text is required")
	}
	if len(text) > maxTranslateTextChars {
		return "", fmt.Errorf("text exceeds max length (%d characters)", maxTranslateTextChars)
	}
	return text, nil
}

func normalizeAndValidateSourceTarget(sourceRaw, targetRaw string) (string, string, error) {
	source := normalizeLangCode(sourceRaw)
	target := normalizeLangCode(targetRaw)
	if target == "" {
		return "", "", fmt.Errorf("target is required")
	}
	if source == "" {
		source = "auto"
	}
	if !isValidSourceLanguage(source) {
		return "", "", fmt.Errorf("source must be auto or a valid language code")
	}
	if !isValidTargetLanguage(target) {
		return "", "", fmt.Errorf("target must be a valid language code")
	}
	return source, target, nil
}

func normalizeAndValidateMode(modeRaw string) (string, error) {
	mode := strings.TrimSpace(modeRaw)
	if mode == "" {
		mode = jobs.DefaultMode
	}
	if mode != jobs.ModeOverlay && mode != jobs.ModeLayout {
		return "", fmt.Errorf(`mode must be "overlay" or "layout"`)
	}
	return mode, nil
}

func normalizeAndValidateImageMode(modeRaw string) (string, error) {
	mode := strings.TrimSpace(modeRaw)
	if mode == "" {
		mode = jobs.DefaultMode
	}
	if mode != jobs.ModeOverlay && mode != jobs.ModeLayout && mode != jobs.ModeOCROnly {
		return "", fmt.Errorf(`mode must be "overlay", "layout", or "ocr_only"`)
	}
	return mode, nil
}

func normalizeAndValidatePDFMode(modeRaw string) (string, error) {
	mode := strings.TrimSpace(modeRaw)
	if mode == "" {
		mode = jobs.DefaultMode
	}
	if mode != jobs.ModeOverlay && mode != jobs.ModeLayout && mode != jobs.ModeOCROnly {
		return "", fmt.Errorf(`mode must be "overlay", "layout", or "ocr_only"`)
	}
	return mode, nil
}

func normalizedMediaType(contentType string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if contentType == "" {
		return ""
	}
	parsed, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return contentType
	}
	return parsed
}

func validatePDFUpload(file *multipart.FileHeader, maxBytes int64) error {
	if file.Size > maxBytes {
		return fmt.Errorf("file exceeds max size (%d bytes)", maxBytes)
	}
	contentType := normalizedMediaType(file.Header.Get("Content-Type"))
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".pdf" {
		return fmt.Errorf("file must be a PDF")
	}
	if contentType != "" && contentType != "application/octet-stream" && !strings.Contains(contentType, "pdf") {
		return fmt.Errorf("file must be a PDF")
	}
	header, err := readFileHeaderBytes(file, 16)
	if err != nil {
		return fmt.Errorf("failed to inspect file signature")
	}
	if !bytes.HasPrefix(header, []byte("%PDF-")) {
		return fmt.Errorf("file must be a PDF")
	}
	return nil
}

func validateImageUpload(file *multipart.FileHeader, maxBytes int64) (string, error) {
	if file.Size > maxBytes {
		return "", fmt.Errorf("file exceeds max size (%d bytes)", maxBytes)
	}
	contentType := normalizedMediaType(file.Header.Get("Content-Type"))
	ext := strings.ToLower(filepath.Ext(file.Filename))
	header, err := readFileHeaderBytes(file, 32)
	if err != nil {
		return "", fmt.Errorf("failed to inspect file signature")
	}
	detectedExt := detectImageExtFromHeader(header)
	if detectedExt == "" {
		detectedExt = detectImageExtFromDecode(file)
	}
	if detectedExt == "" {
		return "", fmt.Errorf("file must be a supported image")
	}
	if ext != "" {
		canonicalExt := ext
		if canonicalExt == ".jpg" {
			canonicalExt = ".jpeg"
		}
		if isAllowedImageExt(canonicalExt) && detectedExt != canonicalExt {
			slog.Warn("image extension/signature mismatch; using detected type",
				"filename", file.Filename,
				"ext", canonicalExt,
				"detected_ext", detectedExt,
			)
		}
	}
	if contentType != "" && contentType != "application/octet-stream" {
		if !strings.HasPrefix(contentType, "image/") {
			return "", fmt.Errorf("file must be a supported image")
		}
	}
	if ext != "" {
		canonicalExt := ext
		if canonicalExt == ".jpg" {
			canonicalExt = ".jpeg"
		}
		if isAllowedImageExt(canonicalExt) && canonicalExt == detectedExt {
			return ext, nil
		}
	}
	return detectedExt, nil
}

func detectImageExtFromDecode(file *multipart.FileHeader) string {
	f, err := file.Open()
	if err != nil {
		return ""
	}
	defer f.Close()

	_, format, err := image.DecodeConfig(f)
	if err != nil {
		return ""
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png":
		return ".png"
	case "jpeg", "jpg":
		return ".jpeg"
	case "gif":
		return ".gif"
	default:
		return ""
	}
}

func readFileHeaderBytes(file *multipart.FileHeader, maxBytes int64) ([]byte, error) {
	f, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, maxBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buf[:n], nil
}

func detectImageExtFromHeader(header []byte) string {
	if len(header) >= 4 && bytes.HasPrefix(header, []byte{0x89, 'P', 'N', 'G'}) {
		return ".png"
	}
	if len(header) >= 8 && bytes.HasPrefix(header, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return ".png"
	}
	if len(header) >= 3 && bytes.HasPrefix(header, []byte{0xff, 0xd8, 0xff}) {
		return ".jpeg"
	}
	if len(header) >= 6 && (bytes.HasPrefix(header, []byte("GIF87a")) || bytes.HasPrefix(header, []byte("GIF89a"))) {
		return ".gif"
	}
	if len(header) >= 12 && bytes.HasPrefix(header, []byte("RIFF")) && bytes.Equal(header[8:12], []byte("WEBP")) {
		return ".webp"
	}
	return ""
}

func isAllowedImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return true
	default:
		return false
	}
}
