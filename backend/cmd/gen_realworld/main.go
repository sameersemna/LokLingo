// gen_realworld renders a real downloaded JPEG with Arabic, Devanagari,
// Bengali and CJK overlays to verify no-tofu rendering on a genuine photo.
//
// Run from backend/:
//
//	go run ./cmd/gen_realworld [/path/to/source.jpg]
//
// Defaults to /tmp/realworld_src.jpg (downloaded by the font integration test).
package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"loklingo/backend/internal/services"
)

func main() {
	src := "/tmp/realworld_src.jpg"
	if len(os.Args) > 1 {
		src = os.Args[1]
	}

	_, thisFile, _, _ := runtime.Caller(0)
	outDir := filepath.Join(filepath.Dir(thisFile), "../../../guide/render_samples")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}

	blocks := []services.ImageTextBlock{
		{Text: "Ant species", Bbox: []float64{10, 8, 310, 55}},
		{Text: "Scientific name", Bbox: []float64{10, 60, 310, 108}},
		{Text: "Habitat description", Bbox: []float64{10, 300, 320, 380}},
		{Text: "Wing structure detail", Bbox: []float64{330, 60, 630, 120}},
	}
	translated := []string{
		"نوع النمل", // Arabic
		"वैज्ञानिक नाम हिंदी में",  // Devanagari
		"আবাসস্থলের বিবরণ বাংলায়", // Bengali
		"翅膀结构细节", // CJK
	}

	opts := services.DefaultOverlayOptions()
	outTmp, stats, err := services.DrawTextOnImageWithOptions(src, blocks, translated, opts)
	if err != nil {
		log.Fatalf("render: %v", err)
	}

	dest := filepath.Join(outDir, "realworld_overlay.jpg")
	data, err := os.ReadFile(outTmp)
	if err != nil {
		log.Fatalf("read tmp: %v", err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		log.Fatalf("write dest: %v", err)
	}
	os.Remove(outTmp)

	log.Printf("✓ realworld_overlay  drawn=%d skipped=%d fontWarnings=%d → %s",
		stats.BlocksDrawn, stats.BlocksSkipped, len(stats.FontWarnings), dest)
	for _, w := range stats.FontWarnings {
		log.Printf("  ⚠ %s", w)
	}
}
