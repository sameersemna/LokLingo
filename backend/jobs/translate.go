package jobs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	internalservices "loklingo/backend/internal/services"
)

func normalizeText(s string) string {
	paragraphs := strings.Split(strings.TrimSpace(s), "\n\n")
	out := paragraphs[:0]
	for _, p := range paragraphs {
		normalized := strings.Join(strings.Fields(p), " ")
		if normalized != "" {
			out = append(out, normalized)
		}
	}
	return strings.Join(out, "\n\n")
}

func normalizePages(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		if n := normalizeText(p); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func applyLayoutModeBBoxOptions(opts *internalservices.OverlayOptions, requestedPadding int) {
	opts.EraseBBox = true
	opts.BboxShrinkPx = 0
	opts.TextPadding = 3
	opts.PatchFeatherPx = 1
	opts.PatchBlurRadius = 1
	if requestedPadding >= 0 {
		switch {
		case requestedPadding < 2:
			opts.TextPadding = 2
		case requestedPadding > 4:
			opts.TextPadding = 4
		default:
			opts.TextPadding = requestedPadding
		}
	}
}

func writeImagePassthroughOutput(imagePath string) (string, error) {
	ext := filepath.Ext(imagePath)
	outPath := strings.TrimSuffix(imagePath, ext) + "_translated" + ext

	in, err := os.Open(imagePath)
	if err != nil {
		return "", fmt.Errorf("open source image: %w", err)
	}
	defer in.Close()

	out, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create passthrough image: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return "", fmt.Errorf("copy passthrough image: %w", err)
	}

	return outPath, nil
}
