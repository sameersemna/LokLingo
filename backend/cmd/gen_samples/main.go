package main
// gen_samples renders reference PNG images to guide/render_samples/ using the
// live image overlay pipeline with all current fixes applied.
//
// Run from the repo root:
//
//	go run ./backend/cmd/gen_samples
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"

	"loklingo/backend/internal/services"
)

func main() {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile = backend/cmd/gen_samples/main.go  → up 4 dirs to repo root
	repoRoot := filepath.Join(filepath.Dir(thisFile), "../../..")
	outDir := filepath.Join(repoRoot, "guide/render_samples")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}

	// writeCanvas creates a gradient+noise synthetic base image.
	writeCanvas := func(name string, w, h int) string {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		rng := rand.New(rand.NewSource(42))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r := uint8(120 + x*60/w + rng.Intn(6))
				g := uint8(180 - y*40/h + rng.Intn(6))
				b := uint8(200 - x*30/w + y*30/h + rng.Intn(6))
				img.SetRGBA(x, y, color.RGBA{r, g, b, 255})
			}
		}
		p := filepath.Join(os.TempDir(), name)
		f, err := os.Create(p)
		if err != nil {
			log.Fatalf("create canvas %s: %v", name, err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			log.Fatalf("encode canvas %s: %v", name, err)
		}
		return p
	}

	copyTo := func(src, dest string) {
		data, err := os.ReadFile(src)
		if err != nil {
			log.Fatalf("read %s: %v", src, err)
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			log.Fatalf("write %s: %v", dest, err)
		}
		os.Remove(src)
	}

	render := func(canvasPath string, blocks []services.ImageTextBlock, translated []string, opts services.OverlayOptions, destName string) {
		outPath, stats, err := services.DrawTextOnImageWithOptions(canvasPath, blocks, translated, opts)
		if err != nil {
			log.Fatalf("render %s: %v", destName, err)
		}
		dest := filepath.Join(outDir, destName+".png")
		copyTo(outPath, dest)
		log.Printf("✓ %-20s drawn=%d skipped=%d fontWarnings=%d → %s",
			destName, stats.BlocksDrawn, stats.BlocksSkipped, len(stats.FontWarnings), dest)
		if len(stats.FontWarnings) > 0 {
			for _, w := range stats.FontWarnings {
				log.Printf("  ⚠ font warning: %s", w)
			}
		}
	}

	opts := services.DefaultOverlayOptions()

	// ── 1. Arabic ─────────────────────────────────────────────────────────────
	{
		canvas := writeCanvas("loklingo_arabic.png", 780, 380)
		blocks := []services.ImageTextBlock{
			{Text: "The quick brown fox jumps", Bbox: []float64{55, 70, 700, 148}},
			{Text: "over the lazy dog today", Bbox: []float64{55, 160, 660, 238}},
			{Text: "Hello world greeting", Bbox: []float64{55, 265, 480, 325}},
		}
		translated := []string{
			"الثعلب البني السريع يقفز",
			"فوق الكلب الكسول اليوم",
			"مرحبا بالعالم",
		}
		render(canvas, blocks, translated, opts, "arabic_overlay")
		os.Remove(canvas)
	}

	// ── 2. CJK with narrow vertical columns ───────────────────────────────────
	{
		canvas := writeCanvas("loklingo_cjk.png", 780, 560)
		blocks := []services.ImageTextBlock{
			{Text: "Japanese horizontal text", Bbox: []float64{50, 55, 620, 125}},
			// two narrow vertical columns
			{Text: "縦書きの日本語テキスト", Bbox: []float64{710, 50, 740, 510}},
			{Text: "漢字仮名交じり文書の例", Bbox: []float64{665, 50, 700, 490}},
			{Text: "Chinese simplified text", Bbox: []float64{50, 160, 560, 230}},
			{Text: "Korean horizontal text", Bbox: []float64{50, 265, 540, 325}},
		}
		translated := []string{
			"日本語横書きテキストのサンプル",
			"縦書きの日本語テキスト",
			"漢字仮名交じり文書の例",
			"中文排版测试内容示例",
			"한국어 텍스트 샘플 예제",
		}
		render(canvas, blocks, translated, opts, "cjk_vertical")
		os.Remove(canvas)
	}

	// ── 3. overlay_result (mixed — replaces guide/render_samples/overlay_result.png) ──
	{
		canvas := writeCanvas("loklingo_mixed_ov.png", 960, 620)
		blocks := []services.ImageTextBlock{
			{Text: "Reflowed layout mode keeps spacing consistent and avoids edge collisions while preserving readability.", Bbox: []float64{60, 85, 870, 195}},
			{Text: "Arabic text block", Bbox: []float64{100, 255, 380, 365}},
			{Text: "Balanced wrap avoids trailing line at the bottom.", Bbox: []float64{475, 255, 875, 365}},
			{Text: "日本語縦書きテキスト", Bbox: []float64{706, 380, 736, 585}},
		}
		translated := []string{
			"يُعيد وضع التخطيط المُعاد تدفقه التباعد بشكل متسق ويتجنب تصادمات الحواف مع الحفاظ على إمكانية القراءة.",
			"نص عربي هنا للاختبار",
			"Balanced wrap avoids a tiny trailing line at the bottom.",
			"日本語縦書きテキスト",
		}
		render(canvas, blocks, translated, opts, "overlay_result")
		os.Remove(canvas)
	}

	// ── 4. layout_result (transparent bg — replaces guide/render_samples/layout_result.png) ──
	{
		canvas := writeCanvas("loklingo_mixed_ly.png", 960, 620)
		blocks := []services.ImageTextBlock{
			{Text: "Reflowed layout mode keeps spacing consistent and avoids edge collisions while preserving readability.", Bbox: []float64{60, 85, 870, 195}},
			{Text: "Arabic text block", Bbox: []float64{100, 255, 380, 365}},
			{Text: "Balanced wrap avoids trailing line at the bottom.", Bbox: []float64{475, 255, 875, 365}},
			{Text: "日本語縦書きテキスト", Bbox: []float64{706, 380, 736, 585}},
		}
		translated := []string{
			"يُعيد وضع التخطيط المُعاد تدفقه التباعد بشكل متسق ويتجنب تصادمات الحواف مع الحفاظ على إمكانية القراءة.",
			"نص عربي هنا للاختبار",
			"Balanced wrap avoids a tiny trailing line at the bottom.",
			"日本語縦書きテキスト",
		}
		layoutOpts := services.DefaultOverlayOptions()
		layoutOpts.BgAlpha = 0
		layoutOpts.EraseBBox = false
		render(canvas, blocks, translated, layoutOpts, "layout_result")
		os.Remove(canvas)
	}
}
