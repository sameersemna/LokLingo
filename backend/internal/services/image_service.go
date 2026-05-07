package services

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	overlayTextPadding   = 6
	overlayLineSpacing   = 2
	overlayMinFontSize   = 8
	overlayMaxFontSize   = 32
	overlayBboxShrinkPx  = 2
	overlayMinDrawWidth  = 24
	overlayMinDrawHeight = 24
	overlayMaxOverlapPct = 0.45
	overlayNearbyBoxDist = 30
	overlayBgAlpha       = uint8(220)
	overlayLumThreshold  = 0.5
)

var overlayFontData = mustParseOverlayFont()

// overlayFaceCache stores pre-loaded font.Face values keyed on integer font size
// to avoid redundant OpenType face allocations during the fitting loop.
var overlayFaceCache sync.Map

// systemFaceCache stores font.Face values for the system fallback font.
var systemFaceCache sync.Map

// overlayAscentCache and systemAscentCache store the Ascent metric (pixels) for
// each font size so Metrics() is called at most once per size per font family.
var overlayAscentCache sync.Map
var systemAscentCache sync.Map

// systemFontPaths lists candidate paths for a broad-Unicode system font.
// The first readable file is used as a fallback for non-Latin scripts.
var systemFontPaths = []string{
	"/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
	"/usr/share/fonts/opentype/noto/NotoSans-Regular.ttf",
	"/usr/share/fonts/noto/NotoSans-Regular.ttf",
	"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	"/System/Library/Fonts/Supplemental/Arial Unicode MS.ttf",
	`C:\Windows\Fonts\arialuni.ttf`,
}

var (
	systemFontOnce sync.Once
	systemFontData *opentype.Font // nil if no suitable system font found
)

type overlayCandidate struct {
	box  image.Rectangle
	text string
}

// ImageTextBlock represents one OCR block with an axis-aligned bbox [x1, y1, x2, y2].
type ImageTextBlock struct {
	Text string    `json:"text"`
	Bbox []float64 `json:"bbox"`
}

// OverlayStats holds counters from a single DrawTextOnImageWithOptions call.
type OverlayStats struct {
	BlocksDrawn   int // blocks for which text was successfully rendered
	BlocksSkipped int // blocks that were filtered out (overlap, duplicate, empty layout, etc.)
}

// DrawTextOnImage loads an image, draws translated text inside OCR block regions,
// and saves a new image next to the source file using default rendering options.
//
// Output path format: <original>_translated<ext>
func DrawTextOnImage(imagePath string, blocks []ImageTextBlock, translatedTexts []string) (string, error) {
	outPath, _, err := DrawTextOnImageWithOptions(imagePath, blocks, translatedTexts, DefaultOverlayOptions())
	return outPath, err
}

// DrawTextOnImageWithOptions is like DrawTextOnImage but accepts explicit rendering
// options so callers (e.g., the jobs worker) can tune background opacity, font
// sizes, and JPEG quality on a per-job basis. It also returns OverlayStats with
// counts of blocks drawn vs skipped for observability.
func DrawTextOnImageWithOptions(imagePath string, blocks []ImageTextBlock, translatedTexts []string, opts OverlayOptions) (string, OverlayStats, error) {
	if strings.TrimSpace(imagePath) == "" {
		return "", OverlayStats{}, fmt.Errorf("image path is required")
	}
	if len(blocks) == 0 {
		return "", OverlayStats{}, fmt.Errorf("at least one block is required")
	}
	if len(translatedTexts) == 0 {
		return "", OverlayStats{}, fmt.Errorf("at least one translated text is required")
	}
	// Apply safe defaults for zero-value fields so callers can partially override.
	if opts.TextPadding < 0 {
		opts.TextPadding = overlayTextPadding
	}
	if opts.MaxFontSize <= 0 {
		opts.MaxFontSize = overlayMaxFontSize
	}
	if opts.MinFontSize <= 0 {
		opts.MinFontSize = overlayMinFontSize
	}
	if opts.JPEGQuality <= 0 || opts.JPEGQuality > 100 {
		opts.JPEGQuality = 90
	}

	in, err := os.Open(imagePath)
	if err != nil {
		return "", OverlayStats{}, fmt.Errorf("open image: %w", err)
	}
	defer in.Close()

	srcImg, format, err := image.Decode(in)
	if err != nil {
		return "", OverlayStats{}, fmt.Errorf("decode image: %w", err)
	}

	bounds := srcImg.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, srcImg, bounds.Min, draw.Src)
	count := min(len(blocks), len(translatedTexts))
	candidates := make([]overlayCandidate, 0, count)
	for i := 0; i < count; i++ {
		blk := blocks[i]
		translated := strings.TrimSpace(translatedTexts[i])
		if translated == "" || len(blk.Bbox) != 4 {
			continue
		}

		x1 := clampToBounds(int(math.Round(blk.Bbox[0])), bounds.Min.X, bounds.Max.X)
		y1 := clampToBounds(int(math.Round(blk.Bbox[1])), bounds.Min.Y, bounds.Max.Y)
		x2 := clampToBounds(int(math.Round(blk.Bbox[2])), bounds.Min.X, bounds.Max.X)
		y2 := clampToBounds(int(math.Round(blk.Bbox[3])), bounds.Min.Y, bounds.Max.Y)
		x1, y1, x2, y2, ok := prepareDrawableBox(x1, y1, x2, y2)
		if !ok {
			continue
		}
		candidates = append(candidates, overlayCandidate{box: image.Rect(x1, y1, x2, y2), text: translated})
	}
	sortOverlayCandidates(candidates)
	drawn := make([]overlayCandidate, 0, count)
	drawnBoxes := make([]image.Rectangle, 0, count)
	skipped := 0
	for _, candidate := range candidates {
		box := candidate.box
		if shouldSkipForOverlap(box, drawnBoxes) {
			skipped++
			continue
		}
		if isDuplicateNearbyText(candidate, drawn) {
			skipped++
			continue
		}

		layout := fitTextLayout(
			candidate.text,
			box.Dx()-(opts.TextPadding*2),
			box.Dy()-(opts.TextPadding*2),
			opts.MinFontSize,
			opts.MaxFontSize,
		)
		if len(layout.lines) == 0 {
			skipped++
			continue
		}

		// Load face for metrics (baseline computation). Rendering uses per-segment faces.
		face, err := faceForText(candidate.text, layout.fontSize)
		if err != nil {
			return "", OverlayStats{}, fmt.Errorf("load overlay font: %w", err)
		}

		// Compute the minimum background height needed to contain the laid-out text.
		// This shrink-wraps the white rectangle to the actual text extent instead of
		// flooding the entire OCR bbox.
		ascentCacheMap := &overlayAscentCache
		if needsFallbackFont(candidate.text) && loadSystemFont() != nil {
			ascentCacheMap = &systemAscentCache
		}
		ascent := cachedAscent(face, ascentCacheMap, layout.fontSize)
		pad := opts.TextPadding
		textH := pad + ascent + layout.topInset +
			(len(layout.lines)-1)*layout.lineHeight + pad
		paintBox := image.Rect(box.Min.X, box.Min.Y, box.Max.X, min(box.Min.Y+textH, box.Max.Y))

		// Sample luminance of the source region (before painting) for ink selection.
		srcLum := avgRegionLuminance(rgba, box)

		// Paint the shrink-wrapped background.
		draw.Draw(rgba, paintBox, &image.Uniform{C: color.RGBA{R: 255, G: 255, B: 255, A: opts.BgAlpha}}, image.Point{}, draw.Over)

		inkColor := inkColorForBackground(srcLum)
		rtl := isRTLText(candidate.text)
		baselineY := box.Min.Y + ascent + pad + layout.topInset
		for _, line := range layout.lines {
			if baselineY > paintBox.Max.Y-pad {
				break
			}
			dotX := box.Min.X + pad
			if rtl {
				// Right-align each line for RTL scripts (Arabic, Hebrew).
				lineWidth := measureLinePx(line, layout.fontSize)
				dotX = box.Max.X - pad - lineWidth
				if dotX < box.Min.X+pad {
					dotX = box.Min.X + pad
				}
			}
			drawTextLine(rgba, layout.fontSize, line, dotX, baselineY, inkColor, paintBox)
			baselineY += layout.lineHeight
		}
		drawn = append(drawn, candidate)
		drawnBoxes = append(drawnBoxes, box)
	}
	stats := OverlayStats{BlocksDrawn: len(drawn), BlocksSkipped: skipped}

	ext := strings.ToLower(filepath.Ext(imagePath))
	base := strings.TrimSuffix(imagePath, filepath.Ext(imagePath))
	outPath := base + "_translated" + ext

	out, err := os.Create(outPath)
	if err != nil {
		return "", OverlayStats{}, fmt.Errorf("create output image: %w", err)
	}

	var encErr error
	isJPEG := format == "jpeg" || format == "jpg"
	quality := opts.JPEGQuality // already sanitised by the guard at function entry
	if isJPEG {
		encErr = jpeg.Encode(out, rgba, &jpeg.Options{Quality: quality})
	} else {
		encErr = png.Encode(out, rgba)
	}
	closeErr := out.Close()
	if encErr != nil {
		return "", OverlayStats{}, fmt.Errorf("encode image: %w", encErr)
	}
	if closeErr != nil {
		return "", OverlayStats{}, fmt.Errorf("close output image: %w", closeErr)
	}

	// Compression guard: if a JPEG output is more than 2× the input file size,
	// re-encode at a reduced quality (−20 pts, floored at 60) to prevent
	// translated images from bloating relative to the originals.
	if isJPEG {
		if inInfo, err1 := os.Stat(imagePath); err1 == nil {
			if outInfo, err2 := os.Stat(outPath); err2 == nil {
				if outInfo.Size() > 2*inInfo.Size() {
					reducedQ := quality - 20
					if reducedQ < 60 {
						reducedQ = 60
					}
					if reducedQ < quality {
						if f, e := os.Create(outPath); e == nil {
							_ = jpeg.Encode(f, rgba, &jpeg.Options{Quality: reducedQ})
							_ = f.Close()
						}
					}
				}
			}
		}
	}

	return outPath, stats, nil
}

func clampToBounds(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

// prepareDrawableBox shrinks the OCR bbox slightly and skips boxes that are too
// small to draw readable translated text without visual clutter.
func prepareDrawableBox(x1, y1, x2, y2 int) (int, int, int, int, bool) {
	x1 += overlayBboxShrinkPx
	y1 += overlayBboxShrinkPx
	x2 -= overlayBboxShrinkPx
	y2 -= overlayBboxShrinkPx

	if x2 <= x1 || y2 <= y1 {
		return 0, 0, 0, 0, false
	}
	if (x2-x1) < overlayMinDrawWidth || (y2-y1) < overlayMinDrawHeight {
		return 0, 0, 0, 0, false
	}
	return x1, y1, x2, y2, true
}

func shouldSkipForOverlap(candidate image.Rectangle, existing []image.Rectangle) bool {
	candidateArea := rectArea(candidate)
	if candidateArea <= 0 {
		return true
	}
	for _, box := range existing {
		overlap := rectArea(candidate.Intersect(box))
		if overlap == 0 {
			continue
		}
		overlapRatio := float64(overlap) / float64(candidateArea)
		if overlapRatio >= overlayMaxOverlapPct {
			return true
		}
	}
	return false
}

// shadowColorFor returns a 50%-opacity contrasting colour to use as a text drop-shadow.
func shadowColorFor(ink color.Color) color.Color {
	r, g, b, _ := ink.RGBA()
	// Invert (0xffff → 0, 0 → 0xffff) and scale to 8-bit with 50% alpha.
	inv := func(c uint32) uint8 { return uint8((0xffff - c) >> 8) }
	return color.RGBA{R: inv(r), G: inv(g), B: inv(b), A: 128}
}

// drawTextLine renders line at (dotX, baselineY) with a 1-pixel drop-shadow.
// Text is split into script segments and each segment is drawn with the
// appropriate font family (goregular or system fallback) so mixed-script lines
// (e.g. "Hello 世界") do not produce tofu boxes.
// clipRect restricts all pixel writes to the overlay box, preventing the
// shadow offset from bleeding into adjacent image regions.
func drawTextLine(dst *image.RGBA, fontSize float64, line string, dotX, baselineY int, inkColor color.Color, clipRect image.Rectangle) {
	clipped := dst.SubImage(clipRect).(*image.RGBA)
	shadow := shadowColorFor(inkColor)
	dot := fixed.P(dotX, baselineY)
	for _, seg := range splitIntoScriptSegments(line) {
		face, err := segFace(seg.useSystem, fontSize)
		if err != nil {
			continue
		}
		// Shadow: offset +1,+1 from current ink position.
		sd := &font.Drawer{
			Dst:  clipped,
			Src:  image.NewUniform(shadow),
			Face: face,
			Dot:  fixed.P(dot.X.Ceil()+1, baselineY+1),
		}
		sd.DrawString(seg.text)
		// Ink: draw at current dot, advance dot for next segment.
		d := &font.Drawer{
			Dst:  clipped,
			Src:  image.NewUniform(inkColor),
			Face: face,
			Dot:  dot,
		}
		d.DrawString(seg.text)
		dot = d.Dot
	}
}

// measureStringPx returns the pixel advance width of s rendered with face.
func measureStringPx(face font.Face, s string) int {
	return (&font.Drawer{Face: face}).MeasureString(s).Ceil()
}

// fitWordToWidth returns the longest prefix of word that fits within maxWidth
// pixels and the remaining suffix. At least one rune is always consumed.
func fitWordToWidth(word string, maxWidth int, face font.Face) (string, string) {
	if measureStringPx(face, word) <= maxWidth {
		return word, ""
	}
	runes := []rune(word)
	lo, hi := 1, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if measureStringPx(face, string(runes[:mid])) <= maxWidth {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(runes[:lo]), string(runes[lo:])
}

// normalizeOverlayText lowercases and collapses whitespace for similarity checks.
func normalizeOverlayText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// isNearbyBox returns true if the two rectangles are within maxDist pixels of each other.
func isNearbyBox(r1, r2 image.Rectangle, maxDist int) bool {
	dx := 0
	if r1.Max.X < r2.Min.X {
		dx = r2.Min.X - r1.Max.X
	} else if r2.Max.X < r1.Min.X {
		dx = r1.Min.X - r2.Max.X
	}
	dy := 0
	if r1.Max.Y < r2.Min.Y {
		dy = r2.Min.Y - r1.Max.Y
	} else if r2.Max.Y < r1.Min.Y {
		dy = r1.Min.Y - r2.Max.Y
	}
	return dx <= maxDist && dy <= maxDist
}

// isDuplicateNearbyText returns true if any already-drawn candidate is spatially
// nearby and has normalized text that is identical to, or a substring of, the
// candidate's text (or vice-versa).
func isDuplicateNearbyText(candidate overlayCandidate, drawn []overlayCandidate) bool {
	norm := normalizeOverlayText(candidate.text)
	if norm == "" {
		return false
	}
	for _, d := range drawn {
		if !isNearbyBox(candidate.box, d.box, overlayNearbyBoxDist) {
			continue
		}
		dNorm := normalizeOverlayText(d.text)
		if dNorm == "" {
			continue
		}
		if norm == dNorm || strings.Contains(dNorm, norm) || strings.Contains(norm, dNorm) {
			return true
		}
	}
	return false
}

func rectArea(rect image.Rectangle) int {
	if rect.Dx() <= 0 || rect.Dy() <= 0 {
		return 0
	}
	return rect.Dx() * rect.Dy()
}

func sortOverlayCandidates(candidates []overlayCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		areaI := rectArea(candidates[i].box)
		areaJ := rectArea(candidates[j].box)
		if areaI != areaJ {
			return areaI > areaJ
		}
		if candidates[i].box.Min.Y != candidates[j].box.Min.Y {
			return candidates[i].box.Min.Y < candidates[j].box.Min.Y
		}
		return candidates[i].box.Min.X < candidates[j].box.Min.X
	})
}

type textLayout struct {
	fontSize   float64
	lineHeight int
	topInset   int
	lines      []string
}

// OverlayOptions controls the visual parameters used when rendering translated
// text overlays. Use DefaultOverlayOptions for sensible defaults.
type OverlayOptions struct {
	// BgAlpha is the background rectangle opacity [0,255]. Higher values occlude
	// more of the original image. Default: 220.
	BgAlpha uint8
	// MaxFontSize is the largest font size (pt) tried during layout. Default: 32.
	MaxFontSize int
	// MinFontSize is the smallest font size (pt) before text is truncated. Default: 8.
	MinFontSize int
	// JPEGQuality is the quality used when re-encoding JPEG images [1,100]. Default: 90.
	JPEGQuality int
	// TextPadding is the pixel gap between the overlay box edge and the text. Default: 6.
	TextPadding int
}

// DefaultOverlayOptions returns the default rendering options that DrawTextOnImage uses.
func DefaultOverlayOptions() OverlayOptions {
	return OverlayOptions{
		BgAlpha:     overlayBgAlpha,
		MaxFontSize: overlayMaxFontSize,
		MinFontSize: overlayMinFontSize,
		JPEGQuality: 90,
		TextPadding: overlayTextPadding,
	}
}

func fitTextLayout(text string, maxWidth, maxHeight, minFontSize, maxFontSize int) textLayout {
	if strings.TrimSpace(text) == "" || maxWidth <= 0 || maxHeight <= 0 {
		return textLayout{}
	}

	startSize := min(maxFontSize, max(minFontSize, maxHeight))
	for fontSize := startSize; fontSize >= minFontSize; fontSize-- {
		measureFn := func(s string) int { return measureLinePx(s, float64(fontSize)) }
		splitFn := func(word string, maxW int) (string, string) {
			f, err := faceForText(word, float64(fontSize))
			if err != nil {
				return word, ""
			}
			return fitWordToWidth(word, maxW, f)
		}
		lines := wrapLines(text, maxWidth, measureFn, splitFn)
		if len(lines) == 0 {
			continue
		}
		lineHeight := approximateLineHeight(fontSize)
		topInset := approximateTopInset(fontSize)
		if topInset+len(lines)*lineHeight <= maxHeight {
			return textLayout{
				fontSize:   float64(fontSize),
				lineHeight: lineHeight,
				topInset:   topInset,
				lines:      lines,
			}
		}
	}

	fontSize := minFontSize
	lineHeight := approximateLineHeight(fontSize)
	topInset := approximateTopInset(fontSize)
	measureFn := func(s string) int { return measureLinePx(s, float64(fontSize)) }
	splitFn := func(word string, maxW int) (string, string) {
		f, err := faceForText(word, float64(fontSize))
		if err != nil {
			return word, ""
		}
		return fitWordToWidth(word, maxW, f)
	}
	lines := wrapLines(text, maxWidth, measureFn, splitFn)
	maxLines := max(1, max(1, maxHeight-topInset)/lineHeight)
	if len(lines) > maxLines {
		lines = truncateWithEllipsisFn(lines, maxLines, maxWidth, measureFn)
	}
	return textLayout{
		fontSize:   float64(fontSize),
		lineHeight: lineHeight,
		topInset:   topInset,
		lines:      lines,
	}
}

func wrapTextToWidth(text string, maxWidth int, face font.Face) []string {
	measureFn := func(s string) int { return measureStringPx(face, s) }
	splitFn := func(word string, maxW int) (string, string) { return fitWordToWidth(word, maxW, face) }
	return wrapLines(text, maxWidth, measureFn, splitFn)
}

// wrapLines is the core word-wrap implementation. measureFn measures a string
// in pixels; splitFn splits an oversized word into a fitting head and a tail.
// Both are injected so callers can use per-segment or per-face measurement.
func wrapLines(text string, maxWidth int, measureFn func(string) int, splitFn func(string, int) (string, string)) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	// Expand CJK-only tokens into individual rune strings so that the wrap
	// loop can break between any two characters (CJK text has no spaces).
	words = expandCJKTokens(words)
	if maxWidth <= 0 {
		return []string{strings.Join(words, " ")}
	}

	lines := make([]string, 0, len(words))
	current := ""

	flushCurrent := func() {
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
	}

	for _, word := range words {
		// Split words that are wider than maxWidth by themselves.
		for measureFn(word) > maxWidth {
			if current != "" {
				flushCurrent()
			}
			head, tail := splitFn(word, maxWidth)
			lines = append(lines, head)
			word = tail
		}
		if word == "" {
			continue
		}

		if current == "" {
			current = word
			continue
		}

		// CJK characters are their own word boundaries — no space needed when
		// either the incoming token or the last character of the current line is CJK.
		sep := " "
		if isCJKWord(word) || isCJKLastRune(current) {
			sep = ""
		}
		candidate := current + sep + word
		if measureFn(candidate) <= maxWidth {
			current = candidate
			continue
		}

		flushCurrent()
		current = word
	}

	flushCurrent()
	return lines
}

func approximateLineHeight(fontSize int) int {
	return max(1, fontSize+overlayLineSpacing)
}

func approximateTopInset(fontSize int) int {
	// Add a small font-size-based inset so larger fonts do not feel glued to the top edge.
	return max(1, int(math.Round(float64(fontSize)*0.15)))
}

// truncateWithEllipsis keeps the first maxLines lines and appends '…' to the
// last line, trimming from its tail until the line fits within maxWidth pixels.
func truncateWithEllipsis(lines []string, maxLines int, maxWidth int, face font.Face) []string {
	return truncateWithEllipsisFn(lines, maxLines, maxWidth, func(s string) int { return measureStringPx(face, s) })
}

// truncateWithEllipsisFn is the measurement-agnostic implementation used by
// fitTextLayout (which uses per-segment measureLinePx).
func truncateWithEllipsisFn(lines []string, maxLines int, maxWidth int, measureFn func(string) int) []string {
	truncated := make([]string, maxLines)
	copy(truncated, lines[:maxLines])

	ellipsis := "…"
	bodyWidth := maxWidth - measureFn(ellipsis)
	if bodyWidth < 0 {
		bodyWidth = 0
	}

	last := truncated[len(truncated)-1]
	if measureFn(last) > bodyWidth {
		runes := []rune(last)
		fit := ""
		for i := len(runes); i > 0; i-- {
			prefix := strings.TrimRight(string(runes[:i]), " ")
			if measureFn(prefix) <= bodyWidth {
				fit = prefix
				break
			}
		}
		last = fit
	}
	truncated[len(truncated)-1] = last + ellipsis
	return truncated
}

// isRTLRune reports whether r belongs to a right-to-left script (Arabic or Hebrew).
func isRTLRune(r rune) bool {
	return (r >= 0x0590 && r <= 0x05FF) || // Hebrew
		(r >= 0x0600 && r <= 0x06FF) || // Arabic
		(r >= 0x0750 && r <= 0x077F) || // Arabic Supplement
		(r >= 0x08A0 && r <= 0x08FF) || // Arabic Extended-A
		(r >= 0xFB1D && r <= 0xFB4F) || // Hebrew Presentation Forms
		(r >= 0xFB50 && r <= 0xFDFF) || // Arabic Presentation Forms-A
		(r >= 0xFE70 && r <= 0xFEFF) // Arabic Presentation Forms-B
}

// isRTLText returns true when at least half the non-space runes in text are RTL.
func isRTLText(text string) bool {
	rtl, total := 0, 0
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if isRTLRune(r) {
			rtl++
		}
	}
	return total > 0 && rtl*2 >= total
}

// avgRegionLuminance returns the average NTSC luminance [0,1] of the source
// image pixels in region r, sampled at a stride to keep the operation fast.
func avgRegionLuminance(img *image.RGBA, r image.Rectangle) float64 {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return 1.0
	}
	step := max(1, min(r.Dx(), r.Dy())/8)
	var sumLum float64
	samples := 0
	for y := r.Min.Y; y < r.Max.Y; y += step {
		for x := r.Min.X; x < r.Max.X; x += step {
			c := img.RGBAAt(x, y)
			sumLum += 0.299*float64(c.R)/255 + 0.587*float64(c.G)/255 + 0.114*float64(c.B)/255
			samples++
		}
	}
	if samples == 0 {
		return 1.0
	}
	return sumLum / float64(samples)
}

// inkColorForBackground chooses black or white ink based on the luminance of
// the source region after compositing with the white overlay background.
func inkColorForBackground(srcLum float64) color.Color {
	bgA := float64(overlayBgAlpha) / 255
	compositedLum := srcLum*(1-bgA) + bgA
	if compositedLum >= overlayLumThreshold {
		return color.Black
	}
	return color.White
}

// cachedAscent returns face.Metrics().Ascent.Ceil() for fontSize, caching the
// result in cache so the Metrics() walk is amortised across the draw loop.
func cachedAscent(face font.Face, cache *sync.Map, fontSize float64) int {
	key := int(fontSize)
	if v, ok := cache.Load(key); ok {
		return v.(int)
	}
	a := face.Metrics().Ascent.Ceil()
	cache.Store(key, a)
	return a
}

func loadOverlayFace(fontSize float64) (font.Face, error) {
	return loadFaceFromFont(overlayFontData, &overlayFaceCache, fontSize)
}

// loadFaceFromFont creates (or retrieves from cache) a font.Face at fontSize for the
// given parsed font. It is safe for concurrent use.
func loadFaceFromFont(fontData *opentype.Font, cache *sync.Map, fontSize float64) (font.Face, error) {
	key := int(fontSize)
	if cached, ok := cache.Load(key); ok {
		return cached.(font.Face), nil
	}
	face, err := opentype.NewFace(fontData, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil, err
	}
	// Store only if another goroutine hasn't raced us to it.
	if actual, loaded := cache.LoadOrStore(key, face); loaded {
		return actual.(font.Face), nil
	}
	return face, nil
}

// loadSystemFont returns a broad-Unicode system font, or nil when none is found.
// The result is cached after the first call.
func loadSystemFont() *opentype.Font {
	systemFontOnce.Do(func() {
		for _, p := range systemFontPaths {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			parsed, err := opentype.Parse(data)
			if err != nil {
				continue
			}
			systemFontData = parsed
			break
		}
	})
	return systemFontData
}

// isCJKRune reports whether r is a CJK ideograph or Hangul syllable — scripts
// that have no inter-word spaces, so word-wrapping must split at rune boundaries.
func isCJKRune(r rune) bool {
	return (r >= 0x1100 && r <= 0x11FF) || // Hangul Jamo
		(r >= 0x2E80 && r <= 0x9FFF) || // CJK Radicals through Unified Ideographs
		(r >= 0xAC00 && r <= 0xD7AF) || // Hangul Syllables
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0x20000 && r <= 0x2A6DF) // CJK Extension B
}

// isAllCJK returns true when every rune in s is a CJK/Hangul character.
func isAllCJK(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !isCJKRune(r) {
			return false
		}
	}
	return true
}

// isCJKWord returns true when every rune in s is a CJK/Hangul character.
// Alias kept for readability at call sites where the input is always a single token.
func isCJKWord(s string) bool { return isAllCJK(s) }

// isCJKLastRune returns true when the last rune of s is a CJK/Hangul character.
func isCJKLastRune(s string) bool {
	if s == "" {
		return false
	}
	var last rune
	for _, r := range s {
		last = r
	}
	return isCJKRune(last)
}

// expandCJKTokens replaces each CJK-only token in words with its individual
// runes as separate strings, allowing wrapLines to break at any character.
func expandCJKTokens(words []string) []string {
	out := make([]string, 0, len(words))
	for _, w := range words {
		if isAllCJK(w) {
			for _, r := range w {
				out = append(out, string(r))
			}
		} else {
			out = append(out, w)
		}
	}
	return out
}

// isCJKRune reports whether r belongs to a script that goregular cannot render
// (CJK, Devanagari, Indic, Thai, Hangul, and related blocks).
func isFallbackRune(r rune) bool {
	return (r >= 0x0900 && r <= 0x097F) || // Devanagari
		(r >= 0x0980 && r <= 0x09FF) || // Bengali
		(r >= 0x0A00 && r <= 0x0A7F) || // Gurmukhi
		(r >= 0x0A80 && r <= 0x0AFF) || // Gujarati
		(r >= 0x0B00 && r <= 0x0B7F) || // Oriya
		(r >= 0x0B80 && r <= 0x0BFF) || // Tamil
		(r >= 0x0C00 && r <= 0x0C7F) || // Telugu
		(r >= 0x0C80 && r <= 0x0CFF) || // Kannada
		(r >= 0x0D00 && r <= 0x0D7F) || // Malayalam
		(r >= 0x0E00 && r <= 0x0E7F) || // Thai
		(r >= 0x1100 && r <= 0x11FF) || // Hangul Jamo
		(r >= 0x2E80 && r <= 0x2EFF) || // CJK Radicals Supplement
		(r >= 0x3000 && r <= 0x9FFF) || // CJK unified ideographs and compatibility
		(r >= 0xA000 && r <= 0xA4CF) || // Yi Syllables
		(r >= 0xAC00 && r <= 0xD7AF) || // Hangul Syllables
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0x20000 && r <= 0x2A6DF) // CJK Extension B
}

// needsFallbackFont returns true when text contains at least one rune from a
// non-Latin script that goregular cannot render.
func needsFallbackFont(text string) bool {
	for _, r := range text {
		if isFallbackRune(r) {
			return true
		}
	}
	return false
}

// faceForText selects the best available font.Face for text at fontSize.
// When text contains non-Latin runes and a system fallback font is available,
// the system font is preferred to avoid rendering tofu boxes.
func faceForText(text string, fontSize float64) (font.Face, error) {
	if needsFallbackFont(text) {
		if sf := loadSystemFont(); sf != nil {
			return loadFaceFromFont(sf, &systemFaceCache, fontSize)
		}
	}
	return loadOverlayFace(fontSize)
}

// scriptSegment is a contiguous run of characters that share the same font family.
type scriptSegment struct {
	text      string
	useSystem bool // true → system fallback font; false → goregular
}

// splitIntoScriptSegments groups consecutive runes of text into runs that
// need the same font family (system fallback vs. goregular). Adjacent runes
// with the same classification are merged into a single segment.
func splitIntoScriptSegments(text string) []scriptSegment {
	if text == "" {
		return nil
	}
	runes := []rune(text)
	var segs []scriptSegment
	start := 0
	curSystem := isFallbackRune(runes[0])
	for i := 1; i <= len(runes); i++ {
		var sys bool
		if i < len(runes) {
			sys = isFallbackRune(runes[i])
		}
		if i == len(runes) || sys != curSystem {
			segs = append(segs, scriptSegment{text: string(runes[start:i]), useSystem: curSystem})
			start = i
			curSystem = sys
		}
	}
	return segs
}

// segFace returns the appropriate font.Face for a script segment at fontSize.
func segFace(useSystem bool, fontSize float64) (font.Face, error) {
	if useSystem {
		if sf := loadSystemFont(); sf != nil {
			return loadFaceFromFont(sf, &systemFaceCache, fontSize)
		}
	}
	return loadOverlayFace(fontSize)
}

// measureLinePx returns the pixel advance width of line by summing per-segment
// measurements, each rendered with the appropriate font family.
func measureLinePx(line string, fontSize float64) int {
	total := 0
	for _, seg := range splitIntoScriptSegments(line) {
		face, err := segFace(seg.useSystem, fontSize)
		if err != nil {
			continue
		}
		total += measureStringPx(face, seg.text)
	}
	return total
}

func mustParseOverlayFont() *opentype.Font {
	parsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		panic(fmt.Sprintf("parse overlay font: %v", err))
	}
	return parsed
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
