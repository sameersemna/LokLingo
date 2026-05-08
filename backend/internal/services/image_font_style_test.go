package services

import (
	"image"
	"image/color"
	"testing"
)

// -- preferredLatinNotoEntry tests --

func TestPreferredLatinNotoEntry_SansRegular(t *testing.T) {
	entry := preferredLatinNotoEntry(BlockFontStyle{Class: sansFontClass, Bold: false})
	if entry != styledNotoSans {
		t.Fatalf("expected styledNotoSans for sans regular, got %v", entry)
	}
}

func TestPreferredLatinNotoEntry_SansBold(t *testing.T) {
	entry := preferredLatinNotoEntry(BlockFontStyle{Class: sansFontClass, Bold: true})
	if entry != styledNotoSansBold {
		t.Fatalf("expected styledNotoSansBold for sans bold, got %v", entry)
	}
}

func TestPreferredLatinNotoEntry_SerifRegular(t *testing.T) {
	entry := preferredLatinNotoEntry(BlockFontStyle{Class: serifFontClass, Bold: false})
	if entry != styledNotoSerif {
		t.Fatalf("expected styledNotoSerif for serif regular, got %v", entry)
	}
}

func TestPreferredLatinNotoEntry_SerifBold(t *testing.T) {
	entry := preferredLatinNotoEntry(BlockFontStyle{Class: serifFontClass, Bold: true})
	if entry != styledNotoSerifBold {
		t.Fatalf("expected styledNotoSerifBold for serif bold, got %v", entry)
	}
}

func TestPreferredLatinNotoEntry_Mono(t *testing.T) {
	entry := preferredLatinNotoEntry(BlockFontStyle{Class: monoFontClass})
	if entry != styledNotoSansMono {
		t.Fatalf("expected styledNotoSansMono for mono, got %v", entry)
	}
}

// -- helpers --

// newGrayImage creates a w×h RGBA image filled with the given gray value.
func newGrayImage(w, h int, gray uint8) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	c := color.RGBA{R: gray, G: gray, B: gray, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// drawDarkHorizontalBars draws a regular pattern of 2-px-tall dark bars on a
// light background, simulating text-like stripes that produce strong horizontal
// edges.  Used to trigger the serif heuristic.
func drawDarkHorizontalBars(img *image.RGBA, box image.Rectangle) {
	dark := color.RGBA{R: 30, G: 30, B: 30, A: 255}
	for y := box.Min.Y; y < box.Max.Y; y++ {
		// 2 dark rows, 4 light rows = periodic serif-like bars
		if (y-box.Min.Y)%6 < 2 {
			for x := box.Min.X; x < box.Max.X; x++ {
				img.SetRGBA(x, y, dark)
			}
		}
	}
}

// drawDenseInk fills most of the box with dark ink to trigger bold detection.
func drawDenseInk(img *image.RGBA, box image.Rectangle, coverage float64) {
	dark := color.RGBA{R: 20, G: 20, B: 20, A: 255}
	total := box.Dx() * box.Dy()
	target := int(float64(total) * coverage)
	drawn := 0
	for y := box.Min.Y; y < box.Max.Y && drawn < target; y++ {
		for x := box.Min.X; x < box.Max.X && drawn < target; x++ {
			img.SetRGBA(x, y, dark)
			drawn++
		}
	}
}

// drawUniformColumns draws vertical stripes of equal ink width on the box,
// simulating monospace character columns.
func drawUniformColumns(img *image.RGBA, box image.Rectangle, charWidth int) {
	dark := color.RGBA{R: 20, G: 20, B: 20, A: 255}
	for x := box.Min.X; x < box.Max.X; x++ {
		// Fill ink for first half of each char-width period.
		col := (x - box.Min.X) % charWidth
		if col < charWidth/2 {
			for y := box.Min.Y; y < box.Max.Y; y++ {
				img.SetRGBA(x, y, dark)
			}
		}
	}
}

// -- boldInkDensity tests --

func TestBoldInkDensity_LightBackground(t *testing.T) {
	// Image with no ink: density must be 0.
	img := newGrayImage(60, 30, 240) // near-white
	box := image.Rect(0, 0, 60, 30)
	d := boldInkDensity(img, box, true)
	if d > 0.01 {
		t.Errorf("expected ~0 density on blank image, got %.3f", d)
	}
}

func TestBoldInkDensity_DenseInk(t *testing.T) {
	// Image almost entirely filled with dark ink should exceed bold threshold.
	img := newGrayImage(60, 30, 240)
	box := image.Rect(0, 0, 60, 30)
	drawDenseInk(img, box, 0.80)
	d := boldInkDensity(img, box, true)
	if d <= inkDensityBoldThreshold {
		t.Errorf("expected density > %.2f for bold-like ink, got %.3f", inkDensityBoldThreshold, d)
	}
}

func TestBoldInkDensity_SparseInk(t *testing.T) {
	// Light, sparse ink (≈ 5% coverage) must stay below bold threshold.
	img := newGrayImage(60, 30, 240)
	box := image.Rect(0, 0, 60, 30)
	drawDenseInk(img, box, 0.05)
	d := boldInkDensity(img, box, true)
	if d >= inkDensityBoldThreshold {
		t.Errorf("expected density < %.2f for sparse ink, got %.3f", inkDensityBoldThreshold, d)
	}
}

func TestBoldInkDensity_EmptyBox(t *testing.T) {
	img := newGrayImage(60, 30, 240)
	box := image.Rect(10, 10, 10, 20) // zero-width box
	d := boldInkDensity(img, box, true)
	if d != 0 {
		t.Errorf("expected 0 for empty box, got %.3f", d)
	}
}

// -- serifEdgeRatio tests --

func TestSerifEdgeRatio_Flat(t *testing.T) {
	// A flat gray image has no edges; ratio must be close to 1.
	img := newGrayImage(80, 40, 200)
	box := image.Rect(0, 0, 80, 40)
	r := serifEdgeRatio(img, box)
	// Flat image → sumV ≈ 0 → fallback returns 1.0
	if r != 1.0 {
		t.Errorf("expected 1.0 for flat image, got %.3f", r)
	}
}

func TestSerifEdgeRatio_HorizontalBars(t *testing.T) {
	// Horizontal dark bars on light background → strong horizontal (Gy) edges
	// → ratio > serifEdgeRatioThreshold.
	img := newGrayImage(80, 40, 240)
	box := image.Rect(2, 2, 78, 38)
	drawDarkHorizontalBars(img, box)
	r := serifEdgeRatio(img, box)
	if r <= serifEdgeRatioThreshold {
		t.Errorf("expected ratio > %.2f for horizontal bars (serif-like), got %.3f",
			serifEdgeRatioThreshold, r)
	}
}

func TestSerifEdgeRatio_SmallBox(t *testing.T) {
	// Box too small for Sobel (< 3×3) must return 1.0 (neutral / ambiguous).
	img := newGrayImage(10, 10, 200)
	box := image.Rect(4, 4, 6, 6) // 2×2
	r := serifEdgeRatio(img, box)
	if r != 1.0 {
		t.Errorf("expected 1.0 for sub-3×3 box, got %.3f", r)
	}
}

// -- columnInkCV tests --

func TestColumnInkCV_Blank(t *testing.T) {
	// Blank image: mean ink is 0 → CV defaults to 1.0 (cannot classify).
	img := newGrayImage(40, 20, 240)
	box := image.Rect(0, 0, 40, 20)
	cv := columnInkCV(img, box, true)
	if cv != 1.0 {
		t.Errorf("expected 1.0 for blank image, got %.3f", cv)
	}
}

func TestColumnInkCV_UniformColumns(t *testing.T) {
	// Uniform columns: every column has the same ink count → CV ≈ 0 (monospace-like).
	img := newGrayImage(40, 20, 240)
	box := image.Rect(0, 0, 40, 20)
	// Fill all columns identically (solid dark image).
	dark := color.RGBA{R: 20, G: 20, B: 20, A: 255}
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			img.SetRGBA(x, y, dark)
		}
	}
	cv := columnInkCV(img, box, true)
	if cv >= monoColumnCVThreshold {
		t.Errorf("expected CV < %.2f for uniform columns, got %.3f", monoColumnCVThreshold, cv)
	}
}

func TestColumnInkCV_VariableColumns(t *testing.T) {
	// Highly variable columns: alternating full and empty → CV >> threshold.
	img := newGrayImage(40, 20, 240)
	box := image.Rect(0, 0, 40, 20)
	dark := color.RGBA{R: 20, G: 20, B: 20, A: 255}
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x += 2 { // only even columns have ink
			img.SetRGBA(x, y, dark)
		}
	}
	cv := columnInkCV(img, box, true)
	if cv < monoColumnCVThreshold {
		t.Errorf("expected CV >= %.2f for alternating columns, got %.3f", monoColumnCVThreshold, cv)
	}
}

// -- isMonospaceBlock tests --

func TestIsMonospaceBlock_TooFewChars(t *testing.T) {
	img := newGrayImage(40, 20, 240)
	box := image.Rect(0, 0, 40, 20)
	// Text with fewer than monoMinChars runes must never classify as mono.
	if isMonospaceBlock(img, box, true, "ab") {
		t.Error("expected false for < monoMinChars characters")
	}
}

func TestIsMonospaceBlock_WrongWidthRatio(t *testing.T) {
	// Very wide relative to height (charWidth/height >> monoCharWidthRatioMax):
	// 200-wide box, 20-high, 3 chars → charWidth = 66, ratio = 3.3 → reject.
	img := newGrayImage(200, 20, 240)
	box := image.Rect(0, 0, 200, 20)
	drawUniformColumns(img, box, 8)
	if isMonospaceBlock(img, box, true, "abc") {
		t.Error("expected false: width ratio too high for 3 chars in 200×20 box")
	}
}

func TestIsMonospaceBlock_UniformInRange(t *testing.T) {
	// 60-wide, 20-high, 5 chars → charWidth=12, ratio=0.6 ∈ [0.44,0.82].
	// Identical ink per column (bottom 30% dark) → CV=0 < 0.50 → monospace.
	img := newGrayImage(60, 20, 240)
	box := image.Rect(0, 0, 60, 20)
	dark := color.RGBA{R: 20, G: 20, B: 20, A: 255}
	inkRows := box.Dy() * 30 / 100
	for x := box.Min.X; x < box.Max.X; x++ {
		for y := box.Max.Y - inkRows; y < box.Max.Y; y++ {
			img.SetRGBA(x, y, dark)
		}
	}
	if !isMonospaceBlock(img, box, true, "abcde") {
		t.Error("expected monospace: uniform columns, width ratio in range")
	}
}

// -- detectBlockFontStyle integration tests --

func TestDetectBlockFontStyle_EmptyBox(t *testing.T) {
	img := newGrayImage(40, 20, 240)
	box := image.Rect(10, 10, 10, 20) // empty
	style := detectBlockFontStyle(img, box, "hello")
	// Should return zero-value style (sans, not bold) without panicking.
	if style.Bold {
		t.Error("expected not bold for empty box")
	}
	if style.Class != sansFontClass {
		t.Errorf("expected sansFontClass for empty box, got %d", style.Class)
	}
}

func TestDetectBlockFontStyle_BlankImage_IsSans(t *testing.T) {
	// No ink → density = 0 → not bold; flat edges → serif ratio = 1.0 → sans.
	img := newGrayImage(60, 30, 240)
	box := image.Rect(0, 0, 60, 30)
	style := detectBlockFontStyle(img, box, "hello world")
	if style.Bold {
		t.Error("expected not bold on blank image")
	}
	if style.Class != sansFontClass {
		t.Errorf("expected sans on blank image, got %d", style.Class)
	}
}

func TestDetectBlockFontStyle_DenseInk_IsBold(t *testing.T) {
	// 25% dark ink on a white background: avg lum ~0.72 → inkIsDark=true;
	// density 0.25 > inkDensityBoldThreshold (0.20) → bold.
	img := newGrayImage(60, 30, 240)
	box := image.Rect(0, 0, 60, 30)
	drawDenseInk(img, box, 0.25)
	style := detectBlockFontStyle(img, box, "hello")
	if !style.Bold {
		t.Error("expected bold for dense-ink image")
	}
}

func TestDetectBlockFontStyle_HorizontalBars_IsSerif(t *testing.T) {
	// Horizontal dark bars on a white background produce strong Sobel-Gy
	// (horizontal edge) responses and near-zero Sobel-Gx (vertical edges).
	// The smoothed serif ratio (sumH+1)/(sumV+1) must exceed the threshold.
	img := newGrayImage(80, 40, 240)
	// Draw bars over the full image, then analyse the interior box.
	fullBox := image.Rect(0, 0, 80, 40)
	drawDarkHorizontalBars(img, fullBox)
	box := image.Rect(2, 2, 78, 38) // interior box for analysis
	style := detectBlockFontStyle(img, box, "Hello World Wide")
	if style.Class != serifFontClass {
		t.Errorf("expected serif for horizontal-bar image, got %d", style.Class)
	}
}

func TestDetectBlockFontStyle_UniformColumns_IsMono(t *testing.T) {
	// 60×20 box, 5 chars → charWidth=12, heightRatio=0.6 ∈ [0.44,0.82].
	// All columns have identical ink (bottom 30% of each column is dark):
	// avg lum ~0.77 → inkIsDark=true; column CV = 0 < 0.50 → monospace.
	img := newGrayImage(60, 20, 240)
	box := image.Rect(0, 0, 60, 20)
	dark := color.RGBA{R: 20, G: 20, B: 20, A: 255}
	inkRows := box.Dy() * 30 / 100 // 6 dark rows at the bottom
	for x := box.Min.X; x < box.Max.X; x++ {
		for y := box.Max.Y - inkRows; y < box.Max.Y; y++ {
			img.SetRGBA(x, y, dark)
		}
	}
	if !isMonospaceBlock(img, box, true, "abcde") {
		t.Error("expected monospace: uniform columns, width ratio in range")
	}
	style := detectBlockFontStyle(img, box, "abcde")
	if style.Class != monoFontClass {
		t.Errorf("expected mono, got %d", style.Class)
	}
}

// -- segFaceStyled smoke tests --

func TestSegFaceStyled_LatinSans(t *testing.T) {
	seg := scriptSegment{text: "Hello", useSystem: false}
	style := BlockFontStyle{Class: sansFontClass, Bold: false}
	face, err := segFaceStyled(seg, 14, style)
	if err != nil {
		t.Fatalf("segFaceStyled latin sans: %v", err)
	}
	if face == nil {
		t.Fatal("expected non-nil face")
	}
}

func TestSegFaceStyled_LatinBold_FallsBackToGoregular(t *testing.T) {
	// NotoSans-Bold may not be installed; the call must not error and must
	// return a usable face (goregular fallback).
	seg := scriptSegment{text: "Bold", useSystem: false}
	style := BlockFontStyle{Class: sansFontClass, Bold: true}
	face, err := segFaceStyled(seg, 14, style)
	if err != nil {
		t.Fatalf("segFaceStyled bold fallback: %v", err)
	}
	if face == nil {
		t.Fatal("expected non-nil face")
	}
}

func TestSegFaceStyled_Mono_FallsBackToGoregular(t *testing.T) {
	seg := scriptSegment{text: "code", useSystem: false}
	style := BlockFontStyle{Class: monoFontClass}
	face, err := segFaceStyled(seg, 12, style)
	if err != nil {
		t.Fatalf("segFaceStyled mono fallback: %v", err)
	}
	if face == nil {
		t.Fatal("expected non-nil face")
	}
}

func TestSegFaceStyled_Serif_FallsBackToGoregular(t *testing.T) {
	seg := scriptSegment{text: "serif text", useSystem: false}
	style := BlockFontStyle{Class: serifFontClass, Bold: false}
	face, err := segFaceStyled(seg, 16, style)
	if err != nil {
		t.Fatalf("segFaceStyled serif fallback: %v", err)
	}
	if face == nil {
		t.Fatal("expected non-nil face")
	}
}
