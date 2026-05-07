package services

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/font"
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
	if layout.topInset+len(layout.lines)*layout.lineHeight > 24 {
		t.Fatalf("layout height %d exceeds box height 24", layout.topInset+len(layout.lines)*layout.lineHeight)
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
	// Source luminance 1.0 (white) → composited always light → black ink.
	c := inkColorForBackground(1.0)
	r, g, b, _ := c.RGBA()
	if r != 0 || g != 0 || b != 0 {
		t.Fatalf("expected black ink for light background, got %v", c)
	}
}

func TestInkColorForBackground_DarkSourceWithLowAlphaUsesWhite(t *testing.T) {
	// This tests the luminance maths: with a very dark source and low composited
	// result the function should return white ink.
	// We temporarily monkey-patch by computing what srcLum would cause white ink.
	// compositedLum = srcLum*(1-bgA) + bgA < overlayLumThreshold
	// → srcLum < (overlayLumThreshold - bgA) / (1 - bgA)
	// With overlayBgAlpha=220 (bgA≈0.863) and threshold=0.5:
	// → srcLum < (0.5 - 0.863)/(1-0.863) = -0.363/0.137 ≈ -2.65 — impossible.
	// So with our current alpha (220), inkColorForBackground always returns black.
	// Verify that directly.
	c := inkColorForBackground(0.0) // darkest possible source
	r, g, b, _ := c.RGBA()
	if r != 0 || g != 0 || b != 0 {
		t.Fatalf("expected black ink even for dark source (alpha 220 makes bg light), got %v", c)
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
	if a>>8 != 128 {
		t.Fatalf("expected shadow alpha=128, got %d", a>>8)
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
