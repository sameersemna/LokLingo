package handlers

import (
	"fmt"
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
	contentType := normalizedMediaType(file.Header.Get("Content-Type"))
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".pdf" {
		return fmt.Errorf("file must be a PDF")
	}
	if contentType != "" && contentType != "application/octet-stream" && !strings.Contains(contentType, "pdf") {
		return fmt.Errorf("file must be a PDF")
	}
	if file.Size > maxBytes {
		return fmt.Errorf("file exceeds max size (%d bytes)", maxBytes)
	}
	return nil
}

func validateImageUpload(file *multipart.FileHeader, maxBytes int64) (string, error) {
	contentType := normalizedMediaType(file.Header.Get("Content-Type"))
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !isAllowedImageUpload(contentType, ext) {
		return "", fmt.Errorf("file must be a supported image")
	}
	if file.Size > maxBytes {
		return "", fmt.Errorf("file exceeds max size (%d bytes)", maxBytes)
	}
	return ext, nil
}

func isAllowedImageUpload(contentType, ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		if contentType == "" || contentType == "application/octet-stream" {
			return true
		}
		return strings.HasPrefix(contentType, "image/")
	default:
		return false
	}
}
