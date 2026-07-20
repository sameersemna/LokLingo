package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"loklingo/backend/internal/services"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	src := "/tmp/realworld_src.jpg"
	if len(os.Args) > 1 {
		src = os.Args[1]
	}

	_, thisFile, _, _ := runtime.Caller(0)
	outDir := filepath.Join(filepath.Dir(thisFile), "../../../guide/render_samples")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		slog.Error("mkdir failed", "err", err)
		os.Exit(1)
	}

	blocks := []services.ImageTextBlock{
		{Text: "Ant species", Bbox: []float64{10, 8, 310, 55}},
		{Text: "Scientific name", Bbox: []float64{10, 60, 310, 108}},
		{Text: "Habitat description", Bbox: []float64{10, 300, 320, 380}},
		{Text: "Wing structure detail", Bbox: []float64{330, 60, 630, 120}},
	}
	translated := []string{
		"نوع النمل",
		"वैज्ञानिक नाम हिंदी में",
		"আবাসস্থলের বিবরণ বাংলায়",
		"翅膀结构细节",
	}

	opts := services.DefaultOverlayOptions()
	outTmp, stats, err := services.DrawTextOnImageWithOptions(src, blocks, translated, opts)
	if err != nil {
		slog.Error("render failed", "err", err)
		os.Exit(1)
	}

	dest := filepath.Join(outDir, "realworld_overlay.jpg")
	data, err := os.ReadFile(outTmp)
	if err != nil {
		slog.Error("read tmp failed", "err", err)
		os.Exit(1)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		slog.Error("write dest failed", "err", err)
		os.Exit(1)
	}
	os.Remove(outTmp)

	slog.Info("realworld_overlay generated",
		"drawn", stats.BlocksDrawn,
		"skipped", stats.BlocksSkipped,
		"fontWarnings", len(stats.FontWarnings),
		"dest", dest,
	)
	for _, w := range stats.FontWarnings {
		slog.Warn("font warning", "warning", w)
	}
}
