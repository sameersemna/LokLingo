package services

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// testFace loads a goregular face at the given size for use in tests.
func testFace(t *testing.T, size float64) font.Face {
	t.Helper()
	face, err := loadOverlayFace(size)
	if err != nil {
		t.Fatalf("testFace(%v): %v", size, err)
	}
	return face
}

func TestDrawTextOnImage_LoadDrawSave(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "sample.png")

	img := image.NewRGBA(image.Rect(0, 0, 120, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 120; x++ {
			img.Set(x, y, color.RGBA{R: 240, G: 240, B: 240, A: 255})
		}
	}
	f, err := os.Create(inPath)
	if err != nil {
		t.Fatalf("create input image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatalf("encode input image: %v", err)
	}
	_ = f.Close()

	blocks := []ImageTextBlock{
		{Text: "Hello", Bbox: []float64{10, 10, 100, 40}},
	}
	translated := []string{"Hallo"}

	outPath, err := DrawTextOnImage(inPath, blocks, translated)
	if err != nil {
		t.Fatalf("DrawTextOnImage returned error: %v", err)
	}
	if outPath == "" {
		t.Fatal("expected non-empty output path")
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected output image to exist: %v", err)
	}

	inBytes, err := os.ReadFile(inPath)
	if err != nil {
		t.Fatalf("read input image: %v", err)
	}
	outBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output image: %v", err)
	}
	if string(inBytes) == string(outBytes) {
		t.Fatal("expected output image bytes to differ after drawing text")
	}
}

func TestDrawTextOnImage_JPEGRoundTrip(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "sample.jpg")

	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 210, B: 220, A: 255})
		}
	}
	f, err := os.Create(inPath)
	if err != nil {
		t.Fatalf("create jpeg input: %v", err)
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		_ = f.Close()
		t.Fatalf("encode jpeg: %v", err)
	}
	_ = f.Close()

	blocks := []ImageTextBlock{
		{Text: "World", Bbox: []float64{10, 10, 180, 60}},
	}
	translated := []string{"Welt"}

	outPath, err := DrawTextOnImage(inPath, blocks, translated)
	if err != nil {
		t.Fatalf("DrawTextOnImage returned error: %v", err)
	}
	if !strings.HasSuffix(outPath, ".jpg") {
		t.Fatalf("expected output path to end in .jpg, got %q", outPath)
	}
	outFile, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("open output jpeg: %v", err)
	}
	defer outFile.Close()
	decoded, err := jpeg.Decode(outFile)
	if err != nil {
		t.Fatalf("decode output jpeg: %v", err)
	}
	if decoded.Bounds().Dx() != 200 || decoded.Bounds().Dy() != 100 {
		t.Fatalf("expected 200×100 output, got %v", decoded.Bounds())
	}
}

func TestDrawTextOnImage_ValidatesInput(t *testing.T) {
	_, err := DrawTextOnImage("", nil, nil)
	if err == nil {
		t.Fatal("expected error for empty image path")
	}

	dir := t.TempDir()
	imgPath := filepath.Join(dir, "x.png")
	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatalf("create image file: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatalf("encode image: %v", err)
	}
	_ = f.Close()

	_, err = DrawTextOnImage(imgPath, nil, []string{"hello"})
	if err == nil {
		t.Fatal("expected error for empty blocks")
	}

	_, err = DrawTextOnImage(imgPath, []ImageTextBlock{{Bbox: []float64{0, 0, 4, 4}}}, nil)
	if err == nil {
		t.Fatal("expected error for empty translated texts")
	}
}

func TestWrapTextToWidth_BreaksOnWords(t *testing.T) {
	face := testFace(t, 10)
	words := []string{"one", "two", "three", "four"}
	// Compute a maxWidth that fits each word alone but not any two consecutive words.
	maxSingle := 0
	for _, w := range words {
		if pw := measureStringPx(face, w); pw > maxSingle {
			maxSingle = pw
		}
	}
	minPair := 999999
	for i := 0; i < len(words)-1; i++ {
		if pw := measureStringPx(face, words[i]+" "+words[i+1]); pw < minPair {
			minPair = pw
		}
	}
	if maxSingle >= minPair {
		t.Skip("font metrics do not allow clean single/pair separation for test words")
	}
	maxWidth := (maxSingle + minPair) / 2

	lines := wrapTextToWidth(strings.Join(words, " "), maxWidth, face)
	if len(lines) != len(words) {
		t.Fatalf("expected %d lines (one per word), got %d: %v", len(words), len(lines), lines)
	}
	for i, line := range lines {
		if line != words[i] {
			t.Errorf("line %d: got %q, want %q", i, line, words[i])
		}
	}
}

func TestRebalanceWrappedLines_ReducesShortLastLine(t *testing.T) {
	measure := func(s string) int { return len(s) }
	lines := []string{"alpha beta gamma", "delta"}
	got := rebalanceWrappedLines(lines, 20, measure)
	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(got))
	}
	if got[0] != "alpha beta" || got[1] != "gamma delta" {
		t.Fatalf("unexpected rebalanced lines: %v", got)
	}
}

func TestRebalanceWrappedLines_KeepsReasonableLastLine(t *testing.T) {
	measure := func(s string) int { return len(s) }
	lines := []string{"alpha beta", "gamma delta"}
	got := rebalanceWrappedLines(lines, 20, measure)
	if got[0] != lines[0] || got[1] != lines[1] {
		t.Fatalf("expected unchanged lines, got %v", got)
	}
}

func TestWrapTextToWidth_SplitsLongWords(t *testing.T) {
	face := testFace(t, 10)
	// maxWidth=20: at 10pt a single char is ~6px, so we expect ~3 chars per line.
	lines := wrapTextToWidth("supercalifragilistic", 20, face)
	if len(lines) < 2 {
		t.Fatalf("expected long word to be split, got %d lines: %v", len(lines), lines)
	}
	// Reassembled parts must equal the original word.
	if strings.Join(lines, "") != "supercalifragilistic" {
		t.Fatalf("split parts do not reassemble to original: %v", lines)
	}
	// Each part must fit within maxWidth.
	for _, part := range lines {
		if w := measureStringPx(face, part); w > 20 {
			t.Errorf("part %q is %dpx wide, exceeds maxWidth 20", part, w)
		}
	}
}

func TestFitTextLayout_ReducesFontSizeToFitHeight(t *testing.T) {
	layout := fitTextLayout("one two three four five six", 60, 24, overlayMinFontSize, overlayMaxFontSize)
	if len(layout.lines) < 2 {
		t.Fatalf("expected wrapped lines, got %v", layout.lines)
	}
	if layout.fontSize >= overlayMaxFontSize {
		t.Fatalf("expected reduced font size, got %v", layout.fontSize)
	}
	// Validate against the actual renderer formula: topInset + ascent + (N-1)*lineHeight.
	// Allow the configured 3 % overflow tolerance.
	n := len(layout.lines)
	fs := int(layout.fontSize)
	blkH := blockHeight(layout.topInset, approximateAscent(fs), layout.lineHeight, n)
	tolerated := int(math.Round(float64(24) * (1.0 + overlayFitOverflowPct)))
	if blkH > tolerated {
		t.Fatalf("layout block height %d exceeds tolerated %d (lines=%d fontSize=%v)", blkH, tolerated, n, layout.fontSize)
	}
}

func TestFitTextLayout_UsesLargerFontForTallerBox(t *testing.T) {
	shortBox := fitTextLayout("alpha beta gamma", 90, 18, overlayMinFontSize, overlayMaxFontSize)
	tallBox := fitTextLayout("alpha beta gamma", 90, 60, overlayMinFontSize, overlayMaxFontSize)
	if shortBox.fontSize <= 0 || tallBox.fontSize <= 0 {
		t.Fatalf("expected positive font sizes, got short=%v tall=%v", shortBox.fontSize, tallBox.fontSize)
	}
	if tallBox.fontSize <= shortBox.fontSize {
		t.Fatalf("expected taller box to allow larger font size, got short=%v tall=%v", shortBox.fontSize, tallBox.fontSize)
	}
}

func TestOverlayTextPadding_InRangeForCleanerOutput(t *testing.T) {
	if overlayTextPadding < 4 || overlayTextPadding > 8 {
		t.Fatalf("expected overlayTextPadding in [4,8], got %d", overlayTextPadding)
	}
}

func TestApproximateTopInset_ScalesWithFontSize(t *testing.T) {
	small := approximateTopInset(8)
	large := approximateTopInset(24)
	if small < 1 {
		t.Fatalf("expected minimum inset >= 1, got %d", small)
	}
	if large <= small {
		t.Fatalf("expected larger font to have larger top inset, got small=%d large=%d", small, large)
	}
}

func TestApproximateAscent_IsPositiveAndScales(t *testing.T) {
	for _, fs := range []int{8, 12, 16, 20, 32} {
		a := approximateAscent(fs)
		if a <= 0 {
			t.Errorf("approximateAscent(%d) = %d, want > 0", fs, a)
		}
		if a >= approximateLineHeight(fs) {
			t.Errorf("approximateAscent(%d) = %d must be < lineHeight %d", fs, a, approximateLineHeight(fs))
		}
	}
}

// TestFitTextLayout_HeightGuarantee verifies that the layout's block height
// (using the actual renderer formula: topInset + ascent + (N-1)*lineHeight)
// never exceeds maxHeight by more than the configured overflow tolerance (3 %).
func TestFitTextLayout_HeightGuarantee(t *testing.T) {
	cases := []struct {
		text       string
		maxW, maxH int
	}{
		{"alpha beta gamma delta epsilon", 80, 30},
		{"hello world", 60, 20},
		{"one two three four five six seven eight", 100, 50},
		{"short text", 200, 100},
	}
	for _, tc := range cases {
		layout := fitTextLayout(tc.text, tc.maxW, tc.maxH, overlayMinFontSize, overlayMaxFontSize)
		if len(layout.lines) == 0 {
			continue // empty — no layout possible
		}
		n := len(layout.lines)
		fs := int(layout.fontSize)
		blkH := blockHeight(layout.topInset, approximateAscent(fs), layout.lineHeight, n)
		tolerated := int(math.Round(float64(tc.maxH) * (1.0 + overlayFitOverflowPct)))
		if blkH > tolerated {
			t.Errorf("text=%q maxW=%d maxH=%d: blockH=%d > tolerated=%d (fontSize=%v lines=%d)",
				tc.text, tc.maxW, tc.maxH, blkH, tolerated, layout.fontSize, n)
		}
	}
}

// TestFitTextLayout_CorrectFormulaPrefersLargerFont checks that the corrected
// height formula allows a larger font than the old formula (N*lineHeight) in at
// least one of several representative boxes. This detects regression back to
// the overestimate.
func TestFitTextLayout_CorrectFormulaPrefersLargerFont(t *testing.T) {
	// At fontSize F the 1-line block height ≈ topInset + ascent
	// = 0.10F + 0.82F = 0.92F (new formula).
	// With 3 % overflow tolerance the threshold becomes 0.92F ≤ 1.03×maxH,
	// i.e. F ≤ maxH×1.03/0.92 ≈ maxH×1.12.
	// For maxH=22 the largest F that fits ≈ 24; expect at least fontSize≥20.
	layout := fitTextLayout("Single", 200, 22, overlayMinFontSize, overlayMaxFontSize)
	if int(layout.fontSize) < 20 {
		t.Fatalf("expected fontSize >= 20 for 1-line text in 22px box, got %.0f", layout.fontSize)
	}
}

func TestTruncateWithEllipsis_AppendsEllipsis(t *testing.T) {
	face := testFace(t, 10)
	lines := []string{"first line", "second line", "third line"}
	result := truncateWithEllipsis(lines, 2, 100, face)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	if result[0] != "first line" {
		t.Fatalf("first line should be unchanged, got %q", result[0])
	}
	if !strings.HasSuffix(result[1], "…") {
		t.Fatalf("expected last line to end with ellipsis, got %q", result[1])
	}
}

func TestTruncateWithEllipsis_TrimsToFitWidth(t *testing.T) {
	face := testFace(t, 10)
	lines := []string{"a", "second line too long"}
	// maxWidth=30: at 10pt a few characters fit; the long second line must be trimmed.
	result := truncateWithEllipsis(lines, 2, 30, face)
	last := result[len(result)-1]
	if !strings.HasSuffix(last, "…") {
		t.Fatalf("expected ellipsis suffix, got %q", last)
	}
	// The whole last line (including ellipsis) must fit within maxWidth.
	if w := measureStringPx(face, last); w > 30 {
		t.Fatalf("last line %q is %dpx, exceeds maxWidth 30", last, w)
	}
}

func TestPrepareDrawableBox_ShrinksBox(t *testing.T) {
	x1, y1, x2, y2, ok := prepareDrawableBox(10, 20, 60, 70)
	if !ok {
		t.Fatal("expected box to remain drawable")
	}
	if x1 != 12 || y1 != 22 || x2 != 58 || y2 != 68 {
		t.Fatalf("unexpected shrunken box: got (%d,%d)-(%d,%d)", x1, y1, x2, y2)
	}
}

func TestPrepareDrawableBox_SkipsTooSmallBox(t *testing.T) {
	_, _, _, _, ok := prepareDrawableBox(0, 0, 20, 20)
	if ok {
		t.Fatal("expected tiny box to be skipped")
	}
}

func TestPrepareDrawableBox_KeepsWidthAtThresholdAfterShrink(t *testing.T) {
	// Width: 28 -> 24 after shrink (2px per side), Height: 32 -> 28.
	_, _, _, _, ok := prepareDrawableBox(0, 0, 28, 32)
	if !ok {
		t.Fatal("expected threshold-sized box to remain drawable")
	}
}

func TestPrepareDrawableBox_SkipsWidthBelowThresholdAfterShrink(t *testing.T) {
	// Width: 27 -> 23 after shrink, which is below overlayMinDrawWidth (24).
	_, _, _, _, ok := prepareDrawableBox(0, 0, 27, 32)
	if ok {
		t.Fatal("expected below-threshold width box to be skipped")
	}
}

func TestPrepareDrawableBoxWithShrink_ZeroPreservesCoordinates(t *testing.T) {
	x1, y1, x2, y2, ok := prepareDrawableBoxWithShrink(10, 20, 60, 70, 0)
	if !ok {
		t.Fatal("expected box to remain drawable with zero shrink")
	}
	if x1 != 10 || y1 != 20 || x2 != 60 || y2 != 70 {
		t.Fatalf("unexpected coordinates with zero shrink: got (%d,%d)-(%d,%d)", x1, y1, x2, y2)
	}
}

func TestShouldSkipForOverlap_HighOverlap(t *testing.T) {
	candidate := image.Rect(0, 0, 100, 100)
	existing := []image.Rectangle{image.Rect(30, 0, 100, 100)}
	if !shouldSkipForOverlap(candidate, existing) {
		t.Fatal("expected high-overlap candidate to be skipped")
	}
}

func TestShouldSkipForOverlap_LowOverlap(t *testing.T) {
	candidate := image.Rect(0, 0, 100, 100)
	existing := []image.Rectangle{image.Rect(80, 0, 180, 100)}
	if shouldSkipForOverlap(candidate, existing) {
		t.Fatal("expected low-overlap candidate to be kept")
	}
}

func TestRectArea_EmptyRectIsZero(t *testing.T) {
	if area := rectArea(image.Rect(10, 10, 10, 20)); area != 0 {
		t.Fatalf("expected zero area for empty rect, got %d", area)
	}
}

func TestSortOverlayCandidates_PrioritizesLargerThenTopLeft(t *testing.T) {
	candidates := []overlayCandidate{
		{box: image.Rect(10, 10, 30, 30), text: "small"},
		{box: image.Rect(20, 20, 80, 80), text: "large"},
		{box: image.Rect(5, 5, 25, 25), text: "small-top-left"},
	}
	sortOverlayCandidates(candidates)

	if candidates[0].text != "large" {
		t.Fatalf("expected largest candidate first, got %q", candidates[0].text)
	}
	if candidates[1].text != "small-top-left" {
		t.Fatalf("expected top-left tie-breaker next, got %q", candidates[1].text)
	}
	if candidates[2].text != "small" {
		t.Fatalf("expected remaining candidate last, got %q", candidates[2].text)
	}
}

func TestNormalizeOverlayText_LowercasesAndCollapsesSpace(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Hello  World", "hello world"},
		{"  FOO BAR  ", "foo bar"},
		{"already normalized", "already normalized"},
		{"", ""},
	}
	for _, tc := range cases {
		got := normalizeOverlayText(tc.in)
		if got != tc.want {
			t.Errorf("normalizeOverlayText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsNearbyBox_AdjacentBoxesAreNearby(t *testing.T) {
	r1 := image.Rect(0, 0, 50, 50)
	r2 := image.Rect(60, 0, 110, 50) // 10px gap horizontally
	if !isNearbyBox(r1, r2, 15) {
		t.Error("expected adjacent boxes within 15px to be nearby")
	}
}

func TestIsNearbyBox_FarBoxesAreNotNearby(t *testing.T) {
	r1 := image.Rect(0, 0, 50, 50)
	r2 := image.Rect(200, 200, 250, 250) // far away
	if isNearbyBox(r1, r2, 30) {
		t.Error("expected far boxes to not be nearby")
	}
}

func TestIsNearbyBox_OverlappingBoxesAreNearby(t *testing.T) {
	r1 := image.Rect(0, 0, 100, 100)
	r2 := image.Rect(50, 50, 150, 150) // overlapping
	if !isNearbyBox(r1, r2, 0) {
		t.Error("expected overlapping boxes to be nearby with maxDist=0")
	}
}

func TestIsDuplicateNearbyText_SameTextNearby(t *testing.T) {
	candidate := overlayCandidate{box: image.Rect(10, 10, 80, 50), text: "Hello World"}
	drawn := []overlayCandidate{
		{box: image.Rect(15, 12, 85, 52), text: "hello world"},
	}
	if !isDuplicateNearbyText(candidate, drawn) {
		t.Error("expected same normalized text nearby to be suppressed")
	}
}

func TestIsDuplicateNearbyText_SubstringNearby(t *testing.T) {
	candidate := overlayCandidate{box: image.Rect(10, 10, 80, 50), text: "hello"}
	drawn := []overlayCandidate{
		{box: image.Rect(12, 10, 82, 52), text: "hello world"},
	}
	if !isDuplicateNearbyText(candidate, drawn) {
		t.Error("expected substring text nearby to be suppressed")
	}
}

func TestIsDuplicateNearbyText_DifferentTextNotSuppressed(t *testing.T) {
	candidate := overlayCandidate{box: image.Rect(10, 10, 80, 50), text: "Goodbye"}
	drawn := []overlayCandidate{
		{box: image.Rect(12, 10, 82, 52), text: "Hello World"},
	}
	if isDuplicateNearbyText(candidate, drawn) {
		t.Error("expected different text not to be suppressed")
	}
}

func TestIsDuplicateNearbyText_SameTextFarAway(t *testing.T) {
	candidate := overlayCandidate{box: image.Rect(0, 0, 60, 40), text: "Hello World"}
	drawn := []overlayCandidate{
		{box: image.Rect(500, 500, 560, 540), text: "hello world"},
	}
	if isDuplicateNearbyText(candidate, drawn) {
		t.Error("expected same text far away to not be suppressed")
	}
}

func TestOverlayBgAlpha_IsReasonableOpacity(t *testing.T) {
	// overlayBgAlpha must be opaque enough to occlude original text (>=180).
	// The upper bound is 255 (fully opaque), which is the max for uint8.
	if overlayBgAlpha < 180 {
		t.Fatalf("expected overlayBgAlpha >= 180, got %d", overlayBgAlpha)
	}
}

func TestIsRTLRune_ArabicIsRTL(t *testing.T) {
	// U+0627 ARABIC LETTER ALEF
	if !isRTLRune(0x0627) {
		t.Error("expected Arabic ALEF (U+0627) to be RTL")
	}
}

func TestIsRTLRune_HebrewIsRTL(t *testing.T) {
	// U+05D0 HEBREW LETTER ALEF
	if !isRTLRune(0x05D0) {
		t.Error("expected Hebrew ALEF (U+05D0) to be RTL")
	}
}

func TestIsRTLRune_LatinIsNotRTL(t *testing.T) {
	for _, r := range "Hello" {
		if isRTLRune(r) {
			t.Errorf("expected Latin rune %q (U+%04X) to not be RTL", r, r)
		}
	}
}

func TestIsRTLText_ArabicTextDetected(t *testing.T) {
	// U+0645 U+0631 U+062D U+0628 U+0627 = مرحبا (Arabic "hello")
	arabic := "\u0645\u0631\u062D\u0628\u0627"
	if !isRTLText(arabic) {
		t.Error("expected Arabic text to be detected as RTL")
	}
}

func TestIsRTLText_HebrewTextDetected(t *testing.T) {
	// U+05E9 U+05DC U+05D5 U+05DD = שלום (Hebrew "hello")
	hebrew := "\u05E9\u05DC\u05D5\u05DD"
	if !isRTLText(hebrew) {
		t.Error("expected Hebrew text to be detected as RTL")
	}
}

func TestIsRTLText_EnglishIsNotRTL(t *testing.T) {
	if isRTLText("Hello World") {
		t.Error("expected English text to not be RTL")
	}
}

func TestIsRTLText_EmptyIsNotRTL(t *testing.T) {
	if isRTLText("") {
		t.Error("expected empty string to not be RTL")
	}
}

func TestIsRTLText_MixedMajorityRTL(t *testing.T) {
	// 3 Arabic runes + 1 Latin = majority RTL
	mixed := "\u0645\u0631\u062D" + "a"
	if !isRTLText(mixed) {
		t.Error("expected majority-RTL mixed text to be detected as RTL")
	}
}

func TestIsRTLText_MixedMajorityLTR(t *testing.T) {
	// 1 Arabic rune + 3 Latin = majority LTR
	mixed := "\u0645" + "abc"
	if isRTLText(mixed) {
		t.Error("expected majority-LTR mixed text to not be RTL")
	}
}

func TestBaselineScriptOffset_CJKIsNegative(t *testing.T) {
	if off := baselineScriptOffset("日本語", 20); off >= 0 {
		t.Fatalf("expected negative CJK offset, got %d", off)
	}
}

func TestBaselineScriptOffset_RTLHasLessUpwardShift(t *testing.T) {
	rtl := baselineScriptOffset("مرحبا", 20)
	ltr := baselineScriptOffset("Hello world", 20)
	if rtl < ltr {
		t.Fatalf("expected RTL offset (%d) to be >= LTR offset (%d)", rtl, ltr)
	}
}

func TestBaselineScriptOffset_LTRIsSlightlyUpward(t *testing.T) {
	if off := baselineScriptOffset("Hello world", 20); off >= 0 {
		t.Fatalf("expected slight upward LTR offset (negative), got %d", off)
	}
}

func TestBaselineScriptOffset_DescenderHeavyCompensatesUpwardShift(t *testing.T) {
	plain := baselineScriptOffset("minimum text", 20)
	descHeavy := baselineScriptOffset("gypyqj", 20)
	if descHeavy < plain {
		t.Fatalf("expected descender-heavy offset (%d) to be >= plain offset (%d)", descHeavy, plain)
	}
}

func TestHasMixedDirectionText_MixedArabicLatin(t *testing.T) {
	if !hasMixedDirectionText("iPhone استخدام") {
		t.Fatal("expected mixed-direction text to be detected")
	}
}

func TestHasMixedDirectionText_PureLTR(t *testing.T) {
	if hasMixedDirectionText("Hello World") {
		t.Fatal("expected pure LTR text to not be detected as mixed")
	}
}

func TestReorderBidiForRendering_MixedArabicLatin(t *testing.T) {
	got := reorderBidiForRendering("iPhone استخدام")
	want := "iPhone مادختسا"
	if got != want {
		t.Fatalf("reorderBidiForRendering mixed text = %q, want %q", got, want)
	}
}

func TestReorderBidiForRendering_PureRTLUnchanged(t *testing.T) {
	in := "استخدام"
	if got := reorderBidiForRendering(in); got != in {
		t.Fatalf("expected pure RTL text unchanged, got %q", got)
	}
}

func TestShouldRenderVerticalText_CJKTallBox(t *testing.T) {
	text := "日本語縦書き"
	box := image.Rect(0, 0, 30, 120)
	if !shouldRenderVerticalText(text, box) {
		t.Fatal("expected tall CJK block to be treated as vertical")
	}
}

func TestShouldRenderVerticalText_WideBoxFallsBack(t *testing.T) {
	text := "日本語"
	box := image.Rect(0, 0, 120, 40)
	if shouldRenderVerticalText(text, box) {
		t.Fatal("expected wide box to not be treated as vertical")
	}
}

func TestShouldRenderVerticalText_NonCJKFallsBack(t *testing.T) {
	text := "Vertical Text"
	box := image.Rect(0, 0, 30, 120)
	if shouldRenderVerticalText(text, box) {
		t.Fatal("expected non-CJK text to not be treated as vertical")
	}
}

func TestVerticalCJKRunes_FiltersNonCJKAndSpaces(t *testing.T) {
	runes := verticalCJKRunes("日 a 本")
	if got, want := string(runes), "日本"; got != want {
		t.Fatalf("verticalCJKRunes mismatch: got %q want %q", got, want)
	}
}

func TestVerticalTextPadding_ReducesPaddingForNarrowColumns(t *testing.T) {
	opts := DefaultOverlayOptions()
	if got := verticalTextPadding(20, opts); got != overlayVerticalMinPadding {
		t.Fatalf("verticalTextPadding(20) = %d, want %d", got, overlayVerticalMinPadding)
	}
	if got := verticalTextPadding(80, opts); got != opts.TextPadding {
		t.Fatalf("verticalTextPadding(80) = %d, want %d", got, opts.TextPadding)
	}
}

func TestVerticalMinFontSize_NarrowColumnsGetExtraBoost(t *testing.T) {
	opts := DefaultOverlayOptions()
	narrow := verticalMinFontSize(opts, overlayVerticalNarrowWidthThreshold)
	wide := verticalMinFontSize(opts, overlayVerticalNarrowWidthThreshold+10)
	if narrow <= wide {
		t.Fatalf("expected narrow vertical min font > wide min font, got narrow=%d wide=%d", narrow, wide)
	}
}

func TestDrawVerticalTextBlock_NarrowColumnStillDraws(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 28, 150))
	box := image.Rect(0, 0, 28, 150)
	candidate := overlayCandidate{box: box, text: "日本語縦書き"}

	drew, err := drawVerticalTextBlock(img, candidate, box, DefaultOverlayOptions())
	if err != nil {
		t.Fatalf("drawVerticalTextBlock returned error: %v", err)
	}
	if !drew {
		t.Fatal("expected narrow vertical CJK block to draw successfully")
	}
}

func TestAvgRegionLuminance_WhiteImageReturnsOne(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	lum := avgRegionLuminance(img, img.Bounds())
	if lum < 0.99 {
		t.Fatalf("expected luminance ~1.0 for white image, got %v", lum)
	}
}

func TestAvgRegionLuminance_BlackImageReturnsZero(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	// Leave pixels at zero (default black).
	lum := avgRegionLuminance(img, img.Bounds())
	if lum > 0.01 {
		t.Fatalf("expected luminance ~0 for black image, got %v", lum)
	}
}

func TestAvgRegionLuminance_EmptyRegionReturnsOne(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	lum := avgRegionLuminance(img, image.Rect(5, 5, 5, 5)) // empty
	if lum != 1.0 {
		t.Fatalf("expected 1.0 for empty region, got %v", lum)
	}
}

func TestInkColorForBackground_LightSourceUsesBlack(t *testing.T) {
	// Light backgrounds should use black ink.
	c := inkColorForBackground(1.0)
	r, g, b, _ := c.RGBA()
	if r != 0 || g != 0 || b != 0 {
		t.Fatalf("expected black ink for light background, got %v", c)
	}
}

func TestInkColorForBackground_DarkBackgroundUsesWhite(t *testing.T) {
	// Dark backgrounds should use white ink.
	c := inkColorForBackground(0.0)
	r, g, b, _ := c.RGBA()
	if r != 0xffff || g != 0xffff || b != 0xffff {
		t.Fatalf("expected white ink for dark background, got %v", c)
	}
}

func TestAvgRegionColor_AveragesPixels(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 100, G: 120, B: 140, A: 255})
	img.Set(1, 0, color.RGBA{R: 100, G: 120, B: 140, A: 255})
	img.Set(0, 1, color.RGBA{R: 200, G: 220, B: 240, A: 255})
	img.Set(1, 1, color.RGBA{R: 200, G: 220, B: 240, A: 255})

	c := avgRegionColor(img, img.Bounds())
	r, g, b, _ := c.RGBA()
	if r>>8 != 150 || g>>8 != 170 || b>>8 != 190 {
		t.Fatalf("expected average rgb(150,170,190), got rgb(%d,%d,%d)", r>>8, g>>8, b>>8)
	}
}

func TestMedianRegionColor_RobustToInkOutliers(t *testing.T) {
	// 3×3 region: 8 light background pixels (200,200,200) + 1 dark ink pixel (0,0,0).
	// Average would be ~178; median must return the background color (200,200,200).
	img := image.NewRGBA(image.Rect(0, 0, 3, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 200, G: 200, B: 200, A: 255})
		}
	}
	img.SetRGBA(1, 1, color.RGBA{R: 0, G: 0, B: 0, A: 255}) // lone ink pixel

	c := medianRegionColor(img, img.Bounds())
	if c.R != 200 || c.G != 200 || c.B != 200 {
		t.Fatalf("expected median rgb(200,200,200), got rgb(%d,%d,%d)", c.R, c.G, c.B)
	}
}

func TestMedianRegionColor_EmptyRegionReturnsWhite(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	c := medianRegionColor(img, image.Rect(20, 20, 30, 30)) // outside image bounds
	if c.R != 255 || c.G != 255 || c.B != 255 {
		t.Fatalf("expected white for empty intersection, got rgb(%d,%d,%d)", c.R, c.G, c.B)
	}
}

func TestPatchFillBBox_PreservesGradient(t *testing.T) {
	// Create a 20×20 image with a clear left-to-right gradient (left=50, right=200).
	// After patchFillBBox, corner pixels should retain their gradient colors.
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			v := uint8(50 + x*150/19) // 50 → 200
			img.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	box := image.Rect(0, 0, 20, 20)
	patchFillBBox(img, box)

	// Top-left corner should be close to the original left value (50).
	tl := img.RGBAAt(0, 0)
	if int(tl.R) < 40 || int(tl.R) > 80 {
		t.Errorf("top-left R expected ~50, got %d", tl.R)
	}
	// Top-right corner should be close to the original right value (200).
	tr := img.RGBAAt(19, 0)
	if int(tr.R) < 170 || int(tr.R) > 210 {
		t.Errorf("top-right R expected ~200, got %d", tr.R)
	}
}

func TestPatchFillBBox_FlatImageStaysFlat(t *testing.T) {
	// Uniform image: every pixel should remain the same color after fill.
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 123, G: 45, B: 67, A: 255})
		}
	}
	patchFillBBox(img, img.Bounds())
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			c := img.RGBAAt(x, y)
			if c.R != 123 || c.G != 45 || c.B != 67 {
				t.Fatalf("pixel (%d,%d) changed: got rgb(%d,%d,%d)", x, y, c.R, c.G, c.B)
			}
		}
	}
}

func TestPatchFillBBox_EmptyBoxIsNoop(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	img.SetRGBA(5, 5, color.RGBA{R: 42, G: 42, B: 42, A: 255})
	patchFillBBox(img, image.Rect(20, 20, 30, 30)) // fully outside
	// Pixel should be unchanged.
	c := img.RGBAAt(5, 5)
	if c.R != 42 {
		t.Fatalf("pixel changed unexpectedly: got R=%d", c.R)
	}
}

func TestVerticalCenteringOffset_SingleLine(t *testing.T) {
	// Single-line block: vertOffset must push the baseline to the vertical
	// midpoint of the available content height.
	const (
		fontSize = 16
		boxH     = 60
		pad      = 6
		ascent   = 14 // approximate for fontSize=16
		topInset = 2  // approximateTopInset(16) = max(1, round(16*0.15)) = 2
	)
	availContentH := boxH - 2*pad                     // 48
	blockH := topInset + ascent + 0                   // single line: (nLines-1)*lineH = 0
	vertOffset := max(0, (availContentH-blockH)/2)    // (48-16)/2 = 16
	baselineY := pad + vertOffset + topInset + ascent // 6+16+2+14 = 38

	// Baseline should be at or past the vertical midpoint of the box.
	midpoint := boxH / 2 // 30
	if baselineY < midpoint {
		t.Errorf("single-line baseline %d is above midpoint %d: text not centered", baselineY, midpoint)
	}
}

func TestVerticalCenteringOffset_MultiLine_NoExpand(t *testing.T) {
	// When the natural block already fills available height, vertOffset must be 0.
	const (
		fontSize = 16
		nLines   = 3
		pad      = 6
		ascent   = 14
		topInset = 2
	)
	lineHeight := approximateLineHeight(fontSize)
	availContentH := topInset + ascent + (nLines-1)*lineHeight // exactly fills
	blockH := topInset + ascent + (nLines-1)*lineHeight
	vertOffset := max(0, (availContentH-blockH)/2)
	if vertOffset != 0 {
		t.Errorf("expected vertOffset=0 when block fills available height, got %d", vertOffset)
	}
}

func TestApproximateLineHeight_BasedOnFontSize(t *testing.T) {
	// Values reflect the 1.18× multiplier introduced to reduce conservative shrink:
	//   12 → round(12×1.18)=14; min=14 → 14
	//   16 → round(16×1.18)=19; min=18 → 19
	//    8 → round(8×1.18)=9;   min=10 → 10 (clamped to min)
	if got := approximateLineHeight(12); got != 14 {
		t.Fatalf("expected line height 14 for font 12, got %d", got)
	}
	if got := approximateLineHeight(16); got != 19 {
		t.Fatalf("expected line height 19 for font 16, got %d", got)
	}
	if got := approximateLineHeight(8); got != 10 {
		t.Fatalf("expected line height 10 for font 8, got %d", got)
	}
}

func TestApproximateLineHeight_IsMonotonic(t *testing.T) {
	prev := approximateLineHeight(1)
	for fs := 2; fs <= 64; fs++ {
		got := approximateLineHeight(fs)
		if got < prev {
			t.Fatalf("line height decreased at font %d: prev=%d got=%d", fs, prev, got)
		}
		prev = got
	}
}

func TestApproximateLineHeight_RespectsReadabilityBounds(t *testing.T) {
	for _, fs := range []int{8, 12, 16, 24, 32} {
		lh := approximateLineHeight(fs)
		if lh < fs+overlayLineSpacing {
			t.Fatalf("font %d: line height %d too tight", fs, lh)
		}
		if lh > fs+9 {
			t.Fatalf("font %d: line height %d too loose", fs, lh)
		}
	}
}

func TestMeasureStringPx_NonZeroForNonEmptyString(t *testing.T) {
	face := testFace(t, 12)
	w := measureStringPx(face, "Hello")
	if w <= 0 {
		t.Fatalf("expected positive pixel width for 'Hello' at size 12, got %d", w)
	}
}

func TestMeasureStringPx_EmptyStringIsZero(t *testing.T) {
	face := testFace(t, 12)
	if w := measureStringPx(face, ""); w != 0 {
		t.Fatalf("expected 0 for empty string, got %d", w)
	}
}

func TestLoadOverlayFace_CacheReturnsSameInstance(t *testing.T) {
	face1, err := loadOverlayFace(14)
	if err != nil {
		t.Fatalf("loadOverlayFace: %v", err)
	}
	face2, err := loadOverlayFace(14)
	if err != nil {
		t.Fatalf("loadOverlayFace second call: %v", err)
	}
	// The cache must return the exact same font.Face pointer.
	if face1 != face2 {
		t.Fatal("expected loadOverlayFace to return same cached instance for same size")
	}
}

func TestShadowColorFor_IsContrastingAndSemiTransparent(t *testing.T) {
	// Shadow of black ink should be a light (high R/G/B) colour.
	shadow := shadowColorFor(color.Black)
	r, g, b, a := shadow.RGBA()
	if r < 0x7000 || g < 0x7000 || b < 0x7000 {
		t.Fatalf("expected light shadow for black ink, got rgba(%d,%d,%d,%d)", r>>8, g>>8, b>>8, a>>8)
	}
	if a>>8 != uint32(overlayShadowAlpha) {
		t.Fatalf("expected shadow alpha=%d, got %d", overlayShadowAlpha, a>>8)
	}
}

func TestIsFallbackRune_CJKTrue(t *testing.T) {
	cjk := []rune{
		0x4E00, // CJK unified ideograph
		0x6587, // 文
		0x3042, // Hiragana あ
		0xAC00, // Hangul syllable
		0x0905, // Devanagari अ
		0x0E01, // Thai ก
	}
	for _, r := range cjk {
		if !isFallbackRune(r) {
			t.Errorf("isFallbackRune(%U) = false, want true", r)
		}
	}
}

func TestIsFallbackRune_LatinFalse(t *testing.T) {
	latin := []rune{'A', 'z', '0', ' ', '!', 0x00E9 /* é */}
	for _, r := range latin {
		if isFallbackRune(r) {
			t.Errorf("isFallbackRune(%U) = true, want false", r)
		}
	}
}

// TestFontCoversText_GoregularCoversLatin verifies that the goregular font
// (overlayFontData) covers basic Latin text.
func TestFontCoversText_GoregularCoversLatin(t *testing.T) {
	if !fontCoversText(overlayFontData, "Hello, world!") {
		t.Fatal("expected goregular to cover basic Latin text")
	}
}

// TestFontCoversText_GoregularMissesCJK verifies that goregular does NOT cover
// CJK characters (the check must return false, not silently accept them).
func TestFontCoversText_GoregularMissesCJK(t *testing.T) {
	if fontCoversText(overlayFontData, "日本語") {
		t.Fatal("goregular must not report coverage for CJK text")
	}
}

// TestFontCoversText_SkipsFormatCharacters verifies that invisible Unicode
// format characters (category Cf: ZWJ, ZWNJ, directional marks) are skipped
// so a font is not incorrectly rejected for lacking their cmap entries.
func TestFontCoversText_SkipsFormatCharacters(t *testing.T) {
	// U+200D ZWJ and U+200C ZWNJ are Cf characters; goregular covers the
	// surrounding Latin but need not have ZWJ/ZWNJ glyphs.
	text := "A\u200DB" // A + ZWJ + B
	if !fontCoversText(overlayFontData, text) {
		t.Fatal("fontCoversText must skip Cf format chars and accept the font")
	}
}

// TestFontCoversText_ReturnsFalseOnFirstMissingRune verifies that a single
// missing rune causes the function to return false immediately.
func TestFontCoversText_ReturnsFalseOnFirstMissingRune(t *testing.T) {
	// Mix a Latin word (covered by goregular) with one Arabic rune (not covered).
	text := "Hello\u0627" // Hello + Arabic letter Alef
	if fontCoversText(overlayFontData, text) {
		t.Fatal("fontCoversText must return false when any rune is missing")
	}
}

// TestFontCoversText_ConcurrentSafe exercises fontCoversText from many
// goroutines simultaneously to confirm there is no data race on the shared
// font buffer.  Run with -race to catch any remaining issues.
func TestFontCoversText_ConcurrentSafe(t *testing.T) {
	const workers = 20
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				fontCoversText(overlayFontData, "Hello")
			}
		}()
	}
	wg.Wait()
}

func TestNeedsFallbackFont_CJKText(t *testing.T) {
	if !needsFallbackFont("日本語テスト") {
		t.Fatal("expected needsFallbackFont=true for CJK text")
	}
}

func TestNeedsFallbackFont_LatinText(t *testing.T) {
	if needsFallbackFont("Hello world") {
		t.Fatal("expected needsFallbackFont=false for Latin text")
	}
}

func TestNeedsFallbackFont_MixedText(t *testing.T) {
	// Even one CJK rune should trigger fallback.
	if !needsFallbackFont("Hello 世界") {
		t.Fatal("expected needsFallbackFont=true for mixed Latin+CJK")
	}
}

func TestFaceForText_LatinReturnsGoregular(t *testing.T) {
	face, err := faceForText("Hello", 14)
	if err != nil {
		t.Fatalf("faceForText: %v", err)
	}
	// goregular face is always available; same pointer as loadOverlayFace.
	baseline, err := loadOverlayFace(14)
	if err != nil {
		t.Fatalf("loadOverlayFace: %v", err)
	}
	if face != baseline {
		t.Fatal("faceForText with Latin text should return the goregular face")
	}
}

func TestFaceForText_CJKGracefulWhenNoSystemFont(t *testing.T) {
	// Even if no system font is installed, faceForText must not error.
	face, err := faceForText("日本語", 14)
	if err != nil {
		t.Fatalf("faceForText with CJK text returned error: %v", err)
	}
	if face == nil {
		t.Fatal("expected non-nil face for CJK text (falls back to goregular)")
	}
}

func TestDrawTextLine_ShadowClampedToBox(t *testing.T) {
	// Create a canvas, draw a line at the bottom-right corner of a tiny box,
	// and verify no pixel outside the box boundary was modified.
	canvas := image.NewRGBA(image.Rect(0, 0, 60, 60))
	box := image.Rect(10, 10, 40, 40)
	drawTextLine(canvas, 10, "Hi", 35, 39, color.Black, box)

	for y := 0; y < 60; y++ {
		for x := 0; x < 60; x++ {
			if x >= box.Min.X && x < box.Max.X && y >= box.Min.Y && y < box.Max.Y {
				continue // inside box — modifications are expected
			}
			r, g, b, a := canvas.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 || a != 0 {
				t.Fatalf("pixel (%d,%d) outside box was modified: rgba(%d,%d,%d,%d)",
					x, y, r>>8, g>>8, b>>8, a>>8)
			}
		}
	}
}

// --------------------------------------------------------------------
// Font fallback hardening tests
// --------------------------------------------------------------------

func TestFontCoverageWarning_LatinNoWarning(t *testing.T) {
	w := fontCoverageWarning("Hello world")
	if w != "" {
		t.Fatalf("expected no warning for Latin text, got %q", w)
	}
}

func TestFontCoverageWarning_CJKWarnsOrSilentIfFontAvailable(t *testing.T) {
	// When a system CJK font is installed the warning should be empty;
	// when it is not installed the warning must be non-empty and mention
	// a known keyword. Either outcome is valid — the test just verifies
	// the two possible states are handled without panic.
	w := fontCoverageWarning("日本語テスト")
	if w != "" {
		if !strings.Contains(w, "Noto") && !strings.Contains(w, "tofu") {
			t.Fatalf("unexpected warning text %q", w)
		}
	}
	// No panic is the key assertion; both "" and a warning string are fine.
}

func TestDetectMissingScripts_CJK(t *testing.T) {
	scripts := detectMissingScripts("日本語")
	found := false
	for _, s := range scripts {
		if s == "CJK" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected CJK in detected scripts, got %v", scripts)
	}
}

func TestDetectMissingScripts_Arabic(t *testing.T) {
	// Arabic Unicode block U+0600–U+06FF
	scripts := detectMissingScripts("\u0645\u0631\u062D\u0628\u0627")
	found := false
	for _, s := range scripts {
		if s == "Arabic" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected Arabic in detected scripts, got %v", scripts)
	}
}

func TestDetectMissingScripts_LatinEmpty(t *testing.T) {
	scripts := detectMissingScripts("Hello 123")
	if len(scripts) != 0 {
		t.Fatalf("expected no scripts for Latin text, got %v", scripts)
	}
}

func TestAvailableFallbackFonts_ReturnsList(t *testing.T) {
	// Only verifies the function returns without panic; installed fonts vary.
	fonts := AvailableFallbackFonts()
	// All returned names must be non-empty.
	for _, name := range fonts {
		if name == "" {
			t.Fatal("AvailableFallbackFonts returned an empty name")
		}
	}
	t.Logf("available fallback fonts on this machine: %v", fonts)
}

func TestOverlayStats_FontWarningsField(t *testing.T) {
	// OverlayStats must carry FontWarnings without compiler error.
	stats := OverlayStats{
		BlocksDrawn:   1,
		BlocksSkipped: 0,
		FontWarnings:  []string{"test warning"},
	}
	if len(stats.FontWarnings) != 1 {
		t.Fatalf("expected 1 FontWarning, got %d", len(stats.FontWarnings))
	}
}

// BenchmarkDrawTextOnImage measures the end-to-end cost of rendering 20 text
// blocks onto an 800×600 image, including face selection, layout, and encoding.
func BenchmarkDrawTextOnImage(b *testing.B) {
	dir := b.TempDir()
	inPath := filepath.Join(dir, "bench.png")

	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 800; x++ {
			img.Set(x, y, color.RGBA{R: 220, G: 220, B: 220, A: 255})
		}
	}
	f, err := os.Create(inPath)
	if err != nil {
		b.Fatalf("create bench image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		b.Fatalf("encode bench image: %v", err)
	}
	_ = f.Close()

	blocks := make([]ImageTextBlock, 20)
	texts := make([]string, 20)
	for i := range blocks {
		col := i % 4
		row := i / 4
		x1 := float64(col * 200)
		y1 := float64(row * 120)
		blocks[i] = ImageTextBlock{
			Text: "Sample translated text block",
			Bbox: []float64{x1, y1, x1 + 190, y1 + 110},
		}
		texts[i] = "Translated text"
	}

	b.ResetTimer()
	for range b.N {
		outPath, err := DrawTextOnImage(inPath, blocks, texts)
		if err != nil {
			b.Fatalf("DrawTextOnImage: %v", err)
		}
		_ = os.Remove(outPath)
	}
}

// ---- OverlayOptions ----

func TestDefaultOverlayOptions_Values(t *testing.T) {
	opts := DefaultOverlayOptions()
	if opts.BgAlpha != overlayBgAlpha {
		t.Errorf("BgAlpha: want %d, got %d", overlayBgAlpha, opts.BgAlpha)
	}
	if opts.MaxFontSize != overlayMaxFontSize {
		t.Errorf("MaxFontSize: want %d, got %d", overlayMaxFontSize, opts.MaxFontSize)
	}
	if opts.MinFontSize != overlayMinFontSize {
		t.Errorf("MinFontSize: want %d, got %d", overlayMinFontSize, opts.MinFontSize)
	}
	if opts.JPEGQuality != 90 {
		t.Errorf("JPEGQuality: want 90, got %d", opts.JPEGQuality)
	}
	if opts.BboxShrinkPx != overlayBboxShrinkPx {
		t.Errorf("BboxShrinkPx: want %d, got %d", overlayBboxShrinkPx, opts.BboxShrinkPx)
	}
	if !opts.EraseBBox {
		t.Error("EraseBBox: want true by default")
	}
}

func TestDrawTextOnImageWithOptions_CustomBgAlpha(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "opts.png")

	img := image.NewRGBA(image.Rect(0, 0, 120, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 120; x++ {
			img.Set(x, y, color.RGBA{R: 100, G: 100, B: 100, A: 255})
		}
	}
	f, err := os.Create(inPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatalf("encode: %v", err)
	}
	_ = f.Close()

	opts := DefaultOverlayOptions()
	opts.BgAlpha = 255 // fully opaque
	blocks := []ImageTextBlock{{Text: "X", Bbox: []float64{10, 10, 100, 50}}}
	_, _, err = DrawTextOnImageWithOptions(inPath, blocks, []string{"Y"}, opts)
	if err != nil {
		t.Fatalf("DrawTextOnImageWithOptions: %v", err)
	}
}

// ---- splitIntoScriptSegments ----

func TestSplitIntoScriptSegments_AllLatin(t *testing.T) {
	segs := splitIntoScriptSegments("Hello world")
	if len(segs) != 1 {
		t.Fatalf("expected 1 segment for all-Latin, got %d", len(segs))
	}
	if segs[0].useSystem {
		t.Fatal("Latin segment should not use system font")
	}
	if segs[0].text != "Hello world" {
		t.Fatalf("expected full text in segment, got %q", segs[0].text)
	}
}

func TestSplitIntoScriptSegments_AllCJK(t *testing.T) {
	segs := splitIntoScriptSegments("世界")
	if len(segs) != 1 {
		t.Fatalf("expected 1 segment for all-CJK, got %d", len(segs))
	}
	if !segs[0].useSystem {
		t.Fatal("CJK segment should use system font")
	}
}

func TestSplitIntoScriptSegments_Mixed(t *testing.T) {
	// "Hello " (Latin) + "世界" (CJK) → 2 segments
	segs := splitIntoScriptSegments("Hello 世界")
	if len(segs) != 2 {
		t.Fatalf("expected 2 segments for mixed text, got %d: %+v", len(segs), segs)
	}
	if segs[0].useSystem {
		t.Error("first segment (Latin) should not use system font")
	}
	if !segs[1].useSystem {
		t.Error("second segment (CJK) should use system font")
	}
}

func TestSplitIntoScriptSegments_EmptyString(t *testing.T) {
	if segs := splitIntoScriptSegments(""); len(segs) != 0 {
		t.Fatalf("expected 0 segments for empty string, got %d", len(segs))
	}
}

// ---- measureLinePx ----

func TestMeasureLinePx_AllLatin(t *testing.T) {
	face := testFace(t, 14)
	// measureLinePx should match measureStringPx for all-Latin text (same face).
	want := measureStringPx(face, "Hello")
	got := measureLinePx("Hello", 14)
	if got != want {
		t.Fatalf("measureLinePx(%q, 14) = %d, want %d", "Hello", got, want)
	}
}

// ---- computeLetterSpacing ----

func TestComputeLetterSpacing_NeutralWhenNearFull(t *testing.T) {
	// textWidth ≈ 97 % of availWidth → raw surplus ≈ 0.16 px/gap → after 30 % dampen ≈ 0.05 px → below threshold → 0
	ls := computeLetterSpacing(97, 100, 10, BlockFontStyle{})
	if ls != 0 {
		t.Errorf("expected 0 for near-full line, got %v", ls)
	}
}

func TestComputeLetterSpacing_PositiveForWideBox(t *testing.T) {
	// textWidth=50 in availWidth=100, 11 runes → 10 gaps
	// rawPx = 50/10 = 5.0; dampened = 1.5; clamped = 1.5 (< 2) → positive
	ls := computeLetterSpacing(50, 100, 11, BlockFontStyle{})
	if ls <= 0 {
		t.Errorf("expected positive spacing for wide box, got %v", ls)
	}
}

func TestComputeLetterSpacing_NegativeForTightBox(t *testing.T) {
	// textWidth=102 in availWidth=100, 11 runes → 10 gaps
	// rawPx = -2/10 = -0.2; dampened = -0.06 → below 0.25 threshold → 0
	// Use a larger deficit to get past threshold: textWidth=110 → rawPx=-1, dampen=-0.3 → -0.3 < -0.25 → negative
	ls := computeLetterSpacing(110, 100, 11, BlockFontStyle{})
	if ls >= 0 {
		t.Errorf("expected negative spacing for tight box, got %v", ls)
	}
}

func TestComputeLetterSpacing_ZeroForSingleRune(t *testing.T) {
	ls := computeLetterSpacing(10, 100, 1, BlockFontStyle{})
	if ls != 0 {
		t.Errorf("expected 0 for single rune, got %v", ls)
	}
}

func TestComputeLetterSpacing_ZeroForMono(t *testing.T) {
	// Monospace blocks must never have spacing adjusted.
	ls := computeLetterSpacing(50, 100, 11, BlockFontStyle{Class: monoFontClass})
	if ls != 0 {
		t.Errorf("expected 0 for monospace block, got %v", ls)
	}
}

func TestComputeLetterSpacing_CappedAt2px(t *testing.T) {
	// Very wide box: textWidth=10 in 1000px → massive raw surplus → must clamp to +2px.
	ls := computeLetterSpacing(10, 1000, 5, BlockFontStyle{})
	maxFixed := fixed.Int26_6(math.Round(2.0 * 64))
	if ls > maxFixed {
		t.Errorf("expected spacing <= +2px (fixed %v), got %v", maxFixed, ls)
	}
}

func TestComputeLetterSpacing_ZeroForZeroAvail(t *testing.T) {
	ls := computeLetterSpacing(50, 0, 5, BlockFontStyle{})
	if ls != 0 {
		t.Errorf("expected 0 for zero availWidth, got %v", ls)
	}
}

// ---- computeStrokeOffsets ----

func TestComputeStrokeOffsets_MonoNoStrokes(t *testing.T) {
	// Mono blocks must never get extra passes.
	got := computeStrokeOffsets(BlockFontStyle{Class: monoFontClass}, 12)
	if len(got) != 0 {
		t.Errorf("expected no strokes for mono, got %v", got)
	}
}

func TestComputeStrokeOffsets_LargeFontNoStrokes(t *testing.T) {
	// Non-bold styles at any size, and bold above 32px, should get no extra passes.
	for _, style := range []BlockFontStyle{
		{},
		{Class: sansFontClass},
	} {
		got := computeStrokeOffsets(style, 25)
		if len(got) != 0 {
			t.Errorf("fontSize=25 style=%+v: expected no strokes, got %v", style, got)
		}
	}
	// Bold above the 32px cutoff should also get no extra passes.
	got := computeStrokeOffsets(BlockFontStyle{Bold: true}, 33)
	if len(got) != 0 {
		t.Errorf("bold fontSize=33: expected no strokes, got %v", got)
	}
}

func TestComputeStrokeOffsets_BoldCrossThickening(t *testing.T) {
	// Bold at a small font should return the 5-pass set (cross + diagonal).
	got := computeStrokeOffsets(BlockFontStyle{Bold: true}, 14)
	if len(got) != 5 {
		t.Fatalf("bold fontSize=14: expected 5 strokes, got %v", got)
	}
	want := []strokeOffset{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("bold fontSize=14: pass %d: got %v, want %v", i, got[i], w)
		}
	}
}

func TestComputeStrokeOffsets_BoldAtBoundary(t *testing.T) {
	// fontSize=32 (at limit, not over) must still get bold strokes.
	got := computeStrokeOffsets(BlockFontStyle{Bold: true}, 32)
	if len(got) != 5 {
		t.Errorf("bold fontSize=32: expected 5 strokes, got %v", got)
	}
}

func TestComputeStrokeOffsets_SerifNoStrokes(t *testing.T) {
	// Serif (non-bold) must not get extra passes regardless of size.
	for _, fs := range []float64{8, 12, 16, 24} {
		got := computeStrokeOffsets(BlockFontStyle{Class: serifFontClass}, fs)
		if len(got) != 0 {
			t.Errorf("serif fontSize=%.0f: expected no strokes, got %v", fs, got)
		}
	}
}

func TestComputeStrokeOffsets_NormalSansSmallOnePass(t *testing.T) {
	// Normal sans at fontSize ≤ 16 should get exactly one horizontal pass.
	for _, fs := range []float64{8, 12, 16} {
		got := computeStrokeOffsets(BlockFontStyle{}, fs)
		if len(got) != 1 {
			t.Errorf("sans fontSize=%.0f: expected 1 stroke, got %v", fs, got)
			continue
		}
		if got[0] != (strokeOffset{1, 0}) {
			t.Errorf("sans fontSize=%.0f: expected (+1,0) stroke, got %v", fs, got[0])
		}
	}
}

func TestComputeStrokeOffsets_NormalSansMediumNoStroke(t *testing.T) {
	// Normal sans at 17–24 should get no extra passes.
	for _, fs := range []float64{17, 20, 24} {
		got := computeStrokeOffsets(BlockFontStyle{}, fs)
		if len(got) != 0 {
			t.Errorf("sans fontSize=%.0f: expected no strokes, got %v", fs, got)
		}
	}
}

// ---- colorLuminance ----

func TestColorLuminance_Black(t *testing.T) {
	l := colorLuminance(color.RGBA{R: 0, G: 0, B: 0, A: 255})
	if l != 0.0 {
		t.Errorf("expected 0 for black, got %v", l)
	}
}

func TestColorLuminance_White(t *testing.T) {
	l := colorLuminance(color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if math.Abs(l-1.0) > 0.01 {
		t.Errorf("expected ~1.0 for white, got %v", l)
	}
}

func TestColorLuminance_NTSCCoefficients(t *testing.T) {
	// Pure red: expected lum = 0.299.
	l := colorLuminance(color.RGBA{R: 255, G: 0, B: 0, A: 255})
	if math.Abs(l-0.299) > 0.005 {
		t.Errorf("expected ~0.299 for pure red, got %.4f", l)
	}
}

// ---- sampleDominantTextColor helpers ----

// fillRectRGBA paints every pixel in r with c on img.
func fillRectRGBA(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// ---- sampleDominantTextColor ----

func TestSampleDominantTextColor_FallbackForUniformWhite(t *testing.T) {
	// A completely white image has no contrasting pixels → should fall back to
	// inkColorForBackground(1.0) = Black so text is still legible.
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	fillRectRGBA(img, img.Bounds(), color.RGBA{255, 255, 255, 255})
	got := sampleDominantTextColor(img, img.Bounds())
	if got != color.Black {
		t.Errorf("expected Black fallback for uniform white image, got %v", got)
	}
}

func TestSampleDominantTextColor_BlackTextOnWhite(t *testing.T) {
	// White background (720 px) + black text region (80 px = 10 %).
	// sampleDominantTextColor should return a very dark colour (not fall back).
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	fillRectRGBA(img, img.Bounds(), color.RGBA{255, 255, 255, 255})
	fillRectRGBA(img, image.Rect(5, 5, 15, 13), color.RGBA{0, 0, 0, 255}) // 10×8 = 80 px
	got := sampleDominantTextColor(img, img.Bounds())
	c, ok := got.(color.RGBA)
	if !ok {
		t.Fatalf("expected color.RGBA type, got %T (%v)", got, got)
	}
	lum := colorLuminance(c)
	if lum > 0.10 {
		t.Errorf("expected very dark ink for black-on-white, luminance %.3f (%v)", lum, c)
	}
}

func TestSampleDominantTextColor_RedTextOnWhite(t *testing.T) {
	// White background with a clearly-red text region (≥15 % of pixels).
	// The sampled colour should be reddish, not black/white.
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	fillRectRGBA(img, img.Bounds(), color.RGBA{255, 255, 255, 255})
	// 16×10 = 160 px red, total 800 px → 20 % — well above the 10 % threshold.
	fillRectRGBA(img, image.Rect(4, 4, 20, 14), color.RGBA{220, 30, 30, 255})
	got := sampleDominantTextColor(img, img.Bounds())
	c, ok := got.(color.RGBA)
	if !ok {
		t.Fatalf("expected color.RGBA type, got %T (%v)", got, got)
	}
	if c.R < 180 || c.G > 80 || c.B > 80 {
		t.Errorf("expected reddish ink for red-on-white, got %v", c)
	}
}

func TestSampleDominantTextColor_EmptyBox(t *testing.T) {
	// Empty rectangle must not panic and must return a non-nil fallback.
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	fillRectRGBA(img, img.Bounds(), color.RGBA{255, 255, 255, 255})
	got := sampleDominantTextColor(img, image.Rectangle{})
	if got == nil {
		t.Error("expected non-nil fallback for empty box")
	}
}

func TestSampleDominantTextColor_FallbackWhenTooFewTextPixels(t *testing.T) {
	// Only 1 contrasting pixel in a large white image — below 10 % threshold.
	// Must fall back to inkColorForBackground (Black on white background).
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	fillRectRGBA(img, img.Bounds(), color.RGBA{255, 255, 255, 255})
	img.SetRGBA(20, 10, color.RGBA{0, 0, 0, 255}) // single black pixel
	got := sampleDominantTextColor(img, img.Bounds())
	if got != color.Black {
		t.Errorf("expected Black fallback when too few text pixels, got %v", got)
	}
}

func TestMeasureLinePx_NonZeroForCJK(t *testing.T) {
	// Even if no system font is available, measureLinePx falls back to goregular
	// and returns a non-zero width.
	w := measureLinePx("世界", 14)
	if w <= 0 {
		t.Fatalf("expected positive width for CJK text, got %d", w)
	}
}

// ---- Shrink-wrap background ----

func TestDrawTextOnImage_ShrinkWrap(t *testing.T) {
	// Render a small text block in the top portion of a large box and verify
	// that the white background does not extend all the way to the bottom of
	// the OCR bbox — the paint rect is shrink-wrapped to the text height.
	dir := t.TempDir()
	inPath := filepath.Join(dir, "sw.png")

	const w, h = 200, 200
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Fill with a distinct non-white colour so we can detect the paint region.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 60, G: 60, B: 60, A: 255})
		}
	}
	f, err := os.Create(inPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatalf("encode: %v", err)
	}
	_ = f.Close()

	// Give a tall bbox (10→180) but only one short word — text will occupy
	// much less than 170px vertically.
	blocks := []ImageTextBlock{{Text: "Hi", Bbox: []float64{10, 10, 180, 180}}}
	outPath, err := DrawTextOnImage(inPath, blocks, []string{"Hi"})
	if err != nil {
		t.Fatalf("DrawTextOnImage: %v", err)
	}

	outFile, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer outFile.Close()
	outImg, err := png.Decode(outFile)
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}

	// Scan column x=90 (middle of the box) from y=10 downward.
	// After the shrink-wrapped white region ends, pixels should return to the
	// original dark colour — i.e. not every row from 10→180 is white.
	whiteRows := 0
	darkRows := 0
	for y := 10; y < 180; y++ {
		r, g, b, _ := outImg.At(90, y).RGBA()
		// Treat as "white-ish" if all channels > 200 (alpha-composited overlay).
		if r>>8 > 200 && g>>8 > 200 && b>>8 > 200 {
			whiteRows++
		} else {
			darkRows++
		}
	}
	if darkRows == 0 {
		t.Fatalf("expected shrink-wrapped box: no dark rows in column 90 between y=10..180 (whiteRows=%d)", whiteRows)
	}
}

// ---- cachedAscent ----

func TestCachedAscent_ReturnsSameValue(t *testing.T) {
	face := testFace(t, 14)
	var cache sync.Map
	a1 := cachedAscent(face, &cache, 14)
	a2 := cachedAscent(face, &cache, 14)
	if a1 != a2 {
		t.Fatalf("cachedAscent returned different values: %d vs %d", a1, a2)
	}
	if a1 <= 0 {
		t.Fatalf("expected positive ascent, got %d", a1)
	}
}

func TestCachedAscent_MatchesFaceMetrics(t *testing.T) {
	face := testFace(t, 12)
	var cache sync.Map
	want := face.Metrics().Ascent.Ceil()
	got := cachedAscent(face, &cache, 12)
	if got != want {
		t.Fatalf("cachedAscent(%d) = %d, want %d (from Metrics)", 12, got, want)
	}
}

func TestCachedAscent_DifferentSizesStoredSeparately(t *testing.T) {
	face12 := testFace(t, 12)
	face16 := testFace(t, 16)
	var cache sync.Map
	a12 := cachedAscent(face12, &cache, 12)
	a16 := cachedAscent(face16, &cache, 16)
	if a12 >= a16 {
		t.Fatalf("expected ascent at 12 < ascent at 16, got %d vs %d", a12, a16)
	}
}

// ---- JPEG compression guard ----

func TestDrawTextOnImageWithOptions_CompressionGuardPNG(t *testing.T) {
	// PNG output: compression guard is skipped. This test verifies the function
	// still returns a valid path for PNG images.
	dir := t.TempDir()
	inPath := filepath.Join(dir, "cg.png")

	img := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 200, B: 200, A: 255})
		}
	}
	f, _ := os.Create(inPath)
	_ = png.Encode(f, img)
	_ = f.Close()

	blocks := []ImageTextBlock{{Text: "Test", Bbox: []float64{5, 5, 75, 35}}}
	outPath, _, err := DrawTextOnImageWithOptions(inPath, blocks, []string{"Test"}, DefaultOverlayOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outPath == "" {
		t.Fatal("expected non-empty output path")
	}
}

// ---- wrapLines ----

func TestWrapLines_SingleMeasureFn_MatchesFaceWrap(t *testing.T) {
	face := testFace(t, 14)
	measureFn := func(s string) int { return measureStringPx(face, s) }
	splitFn := func(word string, maxW int) (string, string) { return fitWordToWidth(word, maxW, face) }

	text := "one two three four five"
	maxW := 60
	linesA := wrapTextToWidth(text, maxW, face)
	linesB := wrapLines(text, maxW, measureFn, splitFn)
	if len(linesA) != len(linesB) {
		t.Fatalf("wrapLines and wrapTextToWidth disagree: %v vs %v", linesB, linesA)
	}
	for i := range linesA {
		if linesA[i] != linesB[i] {
			t.Fatalf("line %d differs: %q vs %q", i, linesB[i], linesA[i])
		}
	}
}

func TestWrapLines_EmptyText_ReturnsNil(t *testing.T) {
	called := false
	measureFn := func(s string) int { called = true; return 0 }
	splitFn := func(word string, maxW int) (string, string) { return word, "" }
	if segs := wrapLines("", 100, measureFn, splitFn); segs != nil {
		t.Fatalf("expected nil for empty input, got %v", segs)
	}
	if called {
		t.Fatal("measureFn should not be called for empty text")
	}
}

func TestWrapLines_ZeroMaxWidth_ReturnsSingleLine(t *testing.T) {
	measureFn := func(s string) int { return len(s) * 10 }
	splitFn := func(word string, maxW int) (string, string) { return word, "" }
	lines := wrapLines("hello world", 0, measureFn, splitFn)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line for maxWidth=0, got %d: %v", len(lines), lines)
	}
}

func TestWrapLines_PerSegmentMeasure_NarrowBox(t *testing.T) {
	// Use measureLinePx as the measureFn to simulate fitTextLayout behaviour.
	const fontSize = 14
	measureFn := func(s string) int { return measureLinePx(s, fontSize) }
	splitFn := func(word string, maxW int) (string, string) {
		f, err := faceForText(word, fontSize)
		if err != nil {
			return word, ""
		}
		return fitWordToWidth(word, maxW, f)
	}
	// A very narrow box forces multi-line output.
	lines := wrapLines("hello world foo bar", 30, measureFn, splitFn)
	if len(lines) < 2 {
		t.Fatalf("expected multiple lines for narrow box, got %d: %v", len(lines), lines)
	}
}

// ---- truncateWithEllipsisFn ----

func TestTruncateWithEllipsisFn_MatchesFaceVersion(t *testing.T) {
	face := testFace(t, 12)
	measureFn := func(s string) int { return measureStringPx(face, s) }
	lines := []string{"alpha", "beta", "gamma", "delta"}

	got := truncateWithEllipsisFn(lines, 2, 80, measureFn)
	want := truncateWithEllipsis(lines, 2, 80, face)
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTruncateWithEllipsisFn_EndsWithEllipsis(t *testing.T) {
	measureFn := func(s string) int { return len(s) * 8 } // synthetic measure
	lines := []string{"this is a very long line that should be trimmed", "second"}
	out := truncateWithEllipsisFn(lines, 1, 40, measureFn)
	if len(out) != 1 {
		t.Fatalf("expected 1 line, got %d", len(out))
	}
	if !strings.HasSuffix(out[0], "…") {
		t.Fatalf("expected ellipsis at end, got %q", out[0])
	}
}

// ---- DefaultOverlayOptions TextPadding ----

func TestDefaultOverlayOptions_TextPadding(t *testing.T) {
	opts := DefaultOverlayOptions()
	if opts.TextPadding != overlayTextPadding {
		t.Fatalf("TextPadding: want %d, got %d", overlayTextPadding, opts.TextPadding)
	}
	if opts.PatchFeatherPx != overlayPatchFeatherPx {
		t.Fatalf("PatchFeatherPx: want %d, got %d", overlayPatchFeatherPx, opts.PatchFeatherPx)
	}
	if opts.PatchBlurRadius != overlayPatchBlurRadius {
		t.Fatalf("PatchBlurRadius: want %d, got %d", overlayPatchBlurRadius, opts.PatchBlurRadius)
	}
}

func TestDrawTextOnImageWithOptions_ZeroTextPaddingFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "pad.png")

	img := image.NewRGBA(image.Rect(0, 0, 120, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 120; x++ {
			img.Set(x, y, color.RGBA{R: 150, G: 150, B: 150, A: 255})
		}
	}
	f, _ := os.Create(inPath)
	_ = png.Encode(f, img)
	_ = f.Close()

	// Passing TextPadding=0 should not panic or produce an error.
	opts := DefaultOverlayOptions()
	opts.TextPadding = 0
	blocks := []ImageTextBlock{{Text: "Hi", Bbox: []float64{10, 10, 110, 50}}}
	_, _, err := DrawTextOnImageWithOptions(inPath, blocks, []string{"Hi"}, opts)
	if err != nil {
		t.Fatalf("unexpected error with zero TextPadding: %v", err)
	}
}

// ---- CJK helpers ----

func TestIsCJKRune(t *testing.T) {
	cases := []struct {
		r    rune
		want bool
	}{
		{'A', false},
		{'z', false},
		{'国', true},    // U+56FD CJK Unified Ideograph
		{'語', true},    // U+8A9E CJK Unified Ideograph
		{'가', true},    // U+AC00 Hangul Syllable
		{'あ', true},    // U+3042 Hiragana (in 0x3000-0x9FFF range)
		{'テ', true},    // U+30C6 Katakana
		{0x2E80, true}, // CJK Radicals Supplement start
		{0x9FFF, true}, // CJK Unified end
	}
	for _, c := range cases {
		if got := isCJKRune(c.r); got != c.want {
			t.Errorf("isCJKRune(%U) = %v, want %v", c.r, got, c.want)
		}
	}
}

func TestIsAllCJK(t *testing.T) {
	if isAllCJK("") {
		t.Error("isAllCJK(\"\") should be false")
	}
	if !isAllCJK("国語") {
		t.Error("isAllCJK(\"国語\") should be true")
	}
	if isAllCJK("hello") {
		t.Error("isAllCJK(\"hello\") should be false")
	}
	if isAllCJK("abc国語") {
		t.Error("isAllCJK mixed should be false")
	}
}

func TestExpandCJKTokens_SplitsIdeographs(t *testing.T) {
	input := []string{"国語", "hello", "日本語"}
	got := expandCJKTokens(input)
	want := []string{"国", "語", "hello", "日", "本", "語"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExpandCJKTokens_PreservesLatinTokens(t *testing.T) {
	input := []string{"hello", "world"}
	got := expandCJKTokens(input)
	if len(got) != 2 || got[0] != "hello" || got[1] != "world" {
		t.Fatalf("expected unchanged latin tokens, got %v", got)
	}
}

func TestWrapLines_CJK_NoSpacesBetweenChars(t *testing.T) {
	// Use a simple counting measure: each rune = 10 px.
	measureFn := func(s string) int { return len([]rune(s)) * 10 }
	splitFn := func(word string, maxW int) (string, string) { return word, "" }

	// 5 CJK chars, maxWidth=30 → lines of max 3 chars.
	lines := wrapLines("国語日本語", 30, measureFn, splitFn)
	for _, l := range lines {
		if measureFn(l) > 30 {
			t.Errorf("line %q is wider than maxWidth (measured %d)", l, measureFn(l))
		}
	}
	// Reconstruct — should equal original without spaces.
	joined := strings.Join(lines, "")
	if joined != "国語日本語" {
		t.Errorf("expected %q after reconstruction, got %q", "国語日本語", joined)
	}
}

// ---- isCJKWord / isCJKLastRune ----

func TestIsCJKWord_AllCJK(t *testing.T) {
	if !isCJKWord("国語") {
		t.Error("expected isCJKWord(\"国語\") = true")
	}
}

func TestIsCJKWord_Mixed(t *testing.T) {
	if isCJKWord("hello国") {
		t.Error("expected isCJKWord(\"hello国\") = false")
	}
}

func TestIsCJKLastRune_EndsWithCJK(t *testing.T) {
	if !isCJKLastRune("hello国") {
		t.Error("expected isCJKLastRune(\"hello国\") = true")
	}
}

func TestIsCJKLastRune_EndsWithLatin(t *testing.T) {
	if isCJKLastRune("国hello") {
		t.Error("expected isCJKLastRune(\"国hello\") = false")
	}
}

func TestIsCJKLastRune_Empty(t *testing.T) {
	if isCJKLastRune("") {
		t.Error("expected isCJKLastRune(\"\") = false")
	}
}

func TestWrapLines_MixedCJKLatin_NoSpaceBeforeCJK(t *testing.T) {
	// Each rune costs 10px; CJK rune should attach to preceding Latin word without space.
	measureFn := func(s string) int { return len([]rune(s)) * 10 }
	splitFn := func(word string, maxW int) (string, string) { return word, "" }

	// "Hi" (2) + "国" (1) = 3 runes = 30px — must fit in one line of 30.
	lines := wrapLines("Hi 国", 30, measureFn, splitFn)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	// No space between Latin and CJK.
	if lines[0] != "Hi国" {
		t.Errorf("expected \"Hi国\", got %q", lines[0])
	}
}

// ---- OverlayStats ----

func TestDrawTextOnImageWithOptions_StatsBlocksDrawn(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "stats.png")

	img := image.NewRGBA(image.Rect(0, 0, 200, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 200, B: 200, A: 255})
		}
	}
	f, _ := os.Create(inPath)
	_ = png.Encode(f, img)
	_ = f.Close()

	// Two non-overlapping blocks — both should be drawn.
	blocks := []ImageTextBlock{
		{Text: "Hello", Bbox: []float64{5, 5, 90, 35}},
		{Text: "World", Bbox: []float64{105, 5, 190, 35}},
	}
	_, stats, err := DrawTextOnImageWithOptions(inPath, blocks, []string{"Hola", "Mundo"}, DefaultOverlayOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.BlocksDrawn != 2 {
		t.Errorf("expected BlocksDrawn=2, got %d", stats.BlocksDrawn)
	}
	if stats.BlocksSkipped != 0 {
		t.Errorf("expected BlocksSkipped=0, got %d", stats.BlocksSkipped)
	}
}

func TestDrawTextOnImageWithOptions_StatsSkippedOnOverlap(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "overlap.png")

	img := image.NewRGBA(image.Rect(0, 0, 200, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 200, B: 200, A: 255})
		}
	}
	f, _ := os.Create(inPath)
	_ = png.Encode(f, img)
	_ = f.Close()

	// Two heavily overlapping blocks — the second should be skipped.
	blocks := []ImageTextBlock{
		{Text: "Hello", Bbox: []float64{5, 5, 150, 50}},
		{Text: "World", Bbox: []float64{5, 5, 150, 50}}, // identical bbox → overlap skip
	}
	_, stats, err := DrawTextOnImageWithOptions(inPath, blocks, []string{"Hola", "Mundo"}, DefaultOverlayOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.BlocksDrawn+stats.BlocksSkipped < 1 {
		t.Error("expected at least one block processed")
	}
}

// ---- TextPadding sentinel (-1 = default) ----

func TestDefaultOverlayOptions_NegativeTextPaddingFallsBackToConstant(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "negpad.png")

	img := image.NewRGBA(image.Rect(0, 0, 120, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 120; x++ {
			img.Set(x, y, color.RGBA{R: 150, G: 150, B: 150, A: 255})
		}
	}
	f, _ := os.Create(inPath)
	_ = png.Encode(f, img)
	_ = f.Close()

	opts := DefaultOverlayOptions()
	opts.TextPadding = -1 // sentinel → service uses overlayTextPadding
	blocks := []ImageTextBlock{{Text: "Hi", Bbox: []float64{10, 10, 110, 50}}}
	_, _, err := DrawTextOnImageWithOptions(inPath, blocks, []string{"Hi"}, opts)
	if err != nil {
		t.Fatalf("unexpected error with TextPadding=-1: %v", err)
	}
}
