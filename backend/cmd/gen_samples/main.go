package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"

	"loklingo/backend/internal/services"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "../../..")
	outDir := filepath.Join(repoRoot, "guide/render_samples")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		slog.Error("mkdir failed", "err", err)
		os.Exit(1)
	}

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
			slog.Error("create canvas failed", "name", name, "err", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			slog.Error("encode canvas failed", "name", name, "err", err)
			os.Exit(1)
		}
		return p
	}

	copyTo := func(src, dest string) {
		data, err := os.ReadFile(src)
		if err != nil {
			slog.Error("read failed", "src", src, "err", err)
			os.Exit(1)
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			slog.Error("write failed", "dest", dest, "err", err)
			os.Exit(1)
		}
		os.Remove(src)
	}

	bboxToRect := func(bbox []float64, bounds image.Rectangle) image.Rectangle {
		if len(bbox) < 4 {
			return image.Rectangle{}
		}
		x0 := int(bbox[0])
		y0 := int(bbox[1])
		x1 := int(bbox[2])
		y1 := int(bbox[3])
		if x0 > x1 {
			x0, x1 = x1, x0
		}
		if y0 > y1 {
			y0, y1 = y1, y0
		}
		return image.Rect(x0, y0, x1, y1).Intersect(bounds)
	}

	writeCanvasWithText := func(name string, w, h int, blocks []services.ImageTextBlock, texts []string, colors []color.RGBA, bold []bool) string {
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

		n := len(blocks)
		if len(texts) < n {
			n = len(texts)
		}
		for i := 0; i < n; i++ {
			box := bboxToRect(blocks[i].Bbox, img.Bounds())
			if box.Empty() {
				continue
			}
			ink := color.RGBA{R: 24, G: 24, B: 24, A: 255}
			if i < len(colors) {
				ink = colors[i]
			}
			isBold := i < len(bold) && bold[i]

			padX := 6
			if box.Dx() < 24 {
				padX = 2
			}
			padY := 8
			if box.Dy() < 20 {
				padY = 2
			}
			dot := fixed.P(box.Min.X+padX, box.Min.Y+padY+basicfont.Face7x13.Metrics().Ascent.Ceil())
			d := &font.Drawer{Dst: img, Src: image.NewUniform(ink), Face: basicfont.Face7x13, Dot: dot}
			d.DrawString(texts[i])
			if isBold {
				d2 := &font.Drawer{Dst: img, Src: image.NewUniform(ink), Face: basicfont.Face7x13, Dot: fixed.P(dot.X.Ceil()+1, dot.Y.Ceil())}
				d2.DrawString(texts[i])
			}

			barW := min(14, max(6, box.Dx()/8))
			barH := min(6, max(3, box.Dy()/12))
			bar := image.Rect(box.Min.X+padX, box.Min.Y+2, box.Min.X+padX+barW, box.Min.Y+2+barH).Intersect(img.Bounds())
			draw.Draw(img, bar, image.NewUniform(ink), image.Point{}, draw.Src)

			strokeW := max(10, box.Dx()/3)
			strokeH := max(2, box.Dy()/14)
			for s := 0; s < 3; s++ {
				sy := box.Min.Y + padY + s*(strokeH+2)
				sx := box.Min.X + padX + s*4
				st := image.Rect(sx, sy, sx+strokeW, sy+strokeH).Intersect(img.Bounds())
				draw.Draw(img, st, image.NewUniform(ink), image.Point{}, draw.Src)
			}
		}

		p := filepath.Join(os.TempDir(), name)
		f, err := os.Create(p)
		if err != nil {
			slog.Error("create canvas failed", "name", name, "err", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			slog.Error("encode canvas failed", "name", name, "err", err)
			os.Exit(1)
		}
		return p
	}

	render := func(canvasPath string, blocks []services.ImageTextBlock, translated []string, opts services.OverlayOptions, destName string) {
		outPath, stats, err := services.DrawTextOnImageWithOptions(canvasPath, blocks, translated, opts)
		if err != nil {
			slog.Error("render failed", "dest", destName, "err", err)
			os.Exit(1)
		}
		dest := filepath.Join(outDir, destName+".png")
		copyTo(outPath, dest)
		slog.Info("render complete",
			"dest", destName,
			"drawn", stats.BlocksDrawn,
			"skipped", stats.BlocksSkipped,
			"fontWarnings", len(stats.FontWarnings),
		)
		if len(stats.FontWarnings) > 0 {
			for _, w := range stats.FontWarnings {
				slog.Warn("font warning", "warning", w)
			}
		}
	}

	opts := services.DefaultOverlayOptions()
	before := services.DefaultOverlayOptions()
	before.DisableFontStyleDetection = true
	before.DisableColorSampling = true
	after := services.DefaultOverlayOptions()

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

	// ── 3. overlay_result ──
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

	// ── 4. layout_result ──
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

	// ── 5. before/after: bold text style detection ──────────────────────────────
	{
		blocks := []services.ImageTextBlock{
			{Text: "BOLD HEADING", Bbox: []float64{50, 40, 900, 150}},
			{Text: "regular line", Bbox: []float64{50, 180, 640, 250}},
			{Text: "secondary regular", Bbox: []float64{50, 265, 640, 335}},
		}
		canvas := writeCanvasWithText(
			"loklingo_before_after_bold.png", 980, 370,
			blocks,
			[]string{"BOLD HEADING", "regular line", "secondary regular"},
			[]color.RGBA{{18, 18, 18, 255}, {32, 32, 32, 255}, {40, 40, 40, 255}},
			[]bool{true, false, false},
		)
		translated := []string{
			"BOLD HEADING",
			"regular line",
			"secondary regular",
		}
		render(canvas, blocks, translated, before, "before_bold_style")
		render(canvas, blocks, translated, after, "after_bold_style")
		os.Remove(canvas)
	}

	// ── 6. before/after: colored text preservation ──────────────────────────────
	{
		blocks := []services.ImageTextBlock{
			{Text: "Status: WARNING", Bbox: []float64{60, 60, 860, 135}},
			{Text: "Status: OK", Bbox: []float64{60, 150, 860, 225}},
			{Text: "Status: ERROR", Bbox: []float64{60, 240, 860, 305}},
		}
		canvas := writeCanvasWithText(
			"loklingo_before_after_colored.png", 920, 320,
			blocks,
			[]string{"Status: WARNING", "Status: OK", "Status: ERROR"},
			[]color.RGBA{{168, 142, 110, 255}, {108, 146, 126, 255}, {156, 112, 118, 255}},
			[]bool{false, false, true},
		)
		translated := []string{
			"Status: WARNING",
			"Status: OK",
			"Status: ERROR",
		}
		render(canvas, blocks, translated, before, "before_colored_text")
		render(canvas, blocks, translated, after, "after_colored_text")
		os.Remove(canvas)
	}

	// ── 7. before/after: mixed styles ──
	{
		blocks := []services.ImageTextBlock{
			{Text: "FEATURE ANNOUNCEMENT", Bbox: []float64{60, 45, 930, 145}},
			{Text: "CODE: SELECT * FROM users WHERE id = 42;", Bbox: []float64{70, 175, 940, 255}},
			{Text: "Localized status chip", Bbox: []float64{70, 280, 450, 355}},
			{Text: "Mixed script subtitle", Bbox: []float64{490, 280, 930, 355}},
			{Text: "日本語縦書きテキスト", Bbox: []float64{860, 385, 900, 615}},
			{Text: "Arabic section title", Bbox: []float64{70, 405, 800, 485}},
			{Text: "Body copy with normal style", Bbox: []float64{70, 515, 860, 640}},
		}
		canvas := writeCanvasWithText(
			"loklingo_before_after_mixed.png", 1020, 680,
			blocks,
			[]string{
				"FEATURE ANNOUNCEMENT",
				"CODE: SELECT * FROM users WHERE id = 42;",
				"Localized status chip",
				"नमस्ते / Hello / مرحبا",
				"日本語縦書きテキスト",
				"عنوان عربي للاختبار",
				"Body copy with normal style",
			},
			[]color.RGBA{
				{18, 18, 18, 255},
				{26, 26, 26, 255},
				{92, 132, 174, 255},
				{170, 124, 164, 255},
				{40, 40, 40, 255},
				{98, 140, 136, 255},
				{45, 45, 45, 255},
			},
			[]bool{true, false, false, false, false, true, false},
		)
		translated := []string{
			"FEATURE ANNOUNCEMENT",
			"CODE: SELECT * FROM users WHERE id = 42;",
			"Localized status chip",
			"नमस्ते / Hello / مرحبا",
			"日本語縦書きテキスト",
			"عنوان عربي للاختبار",
			"Body copy with normal style for mixed-style validation.",
		}
		render(canvas, blocks, translated, before, "before_mixed_styles")
		render(canvas, blocks, translated, after, "after_mixed_styles")
		os.Remove(canvas)
	}
}
