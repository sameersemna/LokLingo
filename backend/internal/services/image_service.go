package services

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"log/slog"
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
	"golang.org/x/text/unicode/bidi"
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
	overlayShadowAlpha   = uint8(96)
	overlayLumThreshold  = 0.5
	// A box is considered strongly vertical when height is at least this multiple of width.
	overlayVerticalAspectThreshold = 2.2
	// Require most runes to be CJK before applying vertical rendering.
	overlayVerticalCJKMinRatio = 0.8
)

var overlayFontData = mustParseOverlayFont()

// overlayFaceCache stores pre-loaded font.Face values keyed on integer font size
// to avoid redundant OpenType face allocations during the fitting loop.
var overlayFaceCache sync.Map

// overlayAscentCache stores the Ascent metric for goregular at each font size.
// fallbackAscentCache stores the Ascent metric for whichever fallback font was
// chosen at each font size; accurate enough for background-height estimation.
var overlayAscentCache sync.Map
var fallbackAscentCache sync.Map

// fallbackFontEntry is a lazily-loaded broad-Unicode font tried in order when
// goregular cannot cover the text. Each entry is loaded at most once.
type fallbackFontEntry struct {
	name      string
	paths     []string
	once      sync.Once
	data      *opentype.Font // nil if not found on this system
	faceCache sync.Map
}

// fallbackFonts is the ordered list of fonts tried for non-Latin text.
// Fonts are tried in order; the first one whose face covers all runes wins.
var fallbackFonts = []*fallbackFontEntry{
	{
		name: "NotoSans",
		paths: []string{
			// Alpine (apk font-noto)
			"/usr/share/fonts/noto/NotoSans-Regular.ttf",
			// Debian/Ubuntu
			"/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoSans-Regular.ttf",
			"/usr/share/fonts/google-noto/NotoSans-Regular.ttf",
		},
	},
	{
		name: "NotoSansCJK",
		paths: []string{
			// Alpine (apk font-noto-cjk)
			"/usr/share/fonts/noto/NotoSansCJK-Regular.ttc",
			// Debian/Ubuntu opentype path
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/google-noto-cjk/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.otf",
		},
	},
	{
		name: "NotoSansArabic",
		paths: []string{
			// Alpine (apk font-noto-arabic)
			"/usr/share/fonts/noto/NotoSansArabic-Regular.ttf",
			// Debian/Ubuntu
			"/usr/share/fonts/truetype/noto/NotoSansArabic-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoSansArabic-Regular.ttf",
			"/usr/share/fonts/google-noto/NotoSansArabic-Regular.ttf",
		},
	},
	{
		name: "DejaVuSans",
		paths: []string{
			// Alpine (apk font-dejavu)
			"/usr/share/fonts/dejavu/DejaVuSans.ttf",
			// Debian/Ubuntu
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/System/Library/Fonts/Supplemental/Arial Unicode MS.ttf",
			`C:\Windows\Fonts\arialuni.ttf`,
		},
	},
}

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
	BlocksDrawn   int      // blocks for which text was successfully rendered
	BlocksSkipped int      // blocks that were filtered out (overlap, duplicate, empty layout, etc.)
	FontWarnings  []string // non-empty when a block contained runes with no covering fallback font
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
		x1, y1, x2, y2, ok := prepareDrawableBoxWithShrink(x1, y1, x2, y2, opts.BboxShrinkPx)
		if !ok {
			continue
		}
		candidates = append(candidates, overlayCandidate{box: image.Rect(x1, y1, x2, y2), text: translated})
	}
	sortOverlayCandidates(candidates)
	drawn := make([]overlayCandidate, 0, count)
	drawnBoxes := make([]image.Rectangle, 0, count)
	skipped := 0
	var fontWarnings []string
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
		if shouldRenderVerticalText(candidate.text, box) {
			drew, err := drawVerticalTextBlock(rgba, candidate, box, opts)
			if err != nil {
				slog.Error("vertical text render failed, skipping block",
					"text", candidate.text,
					"err", err,
				)
				skipped++
				continue
			}
			if drew {
				drawn = append(drawn, candidate)
				drawnBoxes = append(drawnBoxes, box)
				continue
			}
			// If the vertical path is not confident enough to draw, fall back to the
			// existing horizontal renderer.
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
			slog.Error("overlay font load failed, skipping block",
				"text", candidate.text,
				"font_size", layout.fontSize,
				"err", err,
			)
			skipped++
			continue
		}

		if opts.EraseBBox {
			// Erase original OCR text using a gradient-preserving patch fill so that
			// gradients and shadows are reproduced instead of a flat color block.
			patchFillBBox(rgba, box)
		}

		// Warn once per block when no system font can cover the text (tofu risk).
		if w := fontCoverageWarning(candidate.text); w != "" {
			fontWarnings = append(fontWarnings, w)
			slog.Warn("font coverage gap detected", "warning", w)
		}

		ascentCacheMap := &overlayAscentCache
		if needsFallbackFont(candidate.text) {
			ascentCacheMap = &fallbackAscentCache
		}
		ascent := cachedAscent(face, ascentCacheMap, layout.fontSize)
		pad := opts.TextPadding
		nLines := len(layout.lines)
		availContentH := box.Dy() - 2*pad

		// Keep line spacing consistent inside each block: line height depends on
		// font size and does not stretch to fill extra bbox space.
		blockH := layout.topInset + ascent + (nLines-1)*layout.lineHeight
		vertOffset := max(0, (availContentH-blockH)/2)
		baselineY := box.Min.Y + pad + vertOffset + layout.topInset + ascent
		baselineY += baselineScriptOffset(candidate.text, layout.fontSize)

		// Choose ink based on the erased background region luminance.
		srcLum := avgRegionLuminance(rgba, box)
		inkColor := inkColorForBackground(srcLum)
		rtl := isRTLText(candidate.text)
		for _, line := range layout.lines {
			if baselineY > box.Max.Y-pad {
				break
			}
			renderLine := reorderBidiForRendering(line)
			dotX := box.Min.X + pad
			if rtl {
				// Right-align each line for RTL scripts (Arabic, Hebrew).
				lineWidth := measureLinePx(renderLine, layout.fontSize)
				dotX = box.Max.X - pad - lineWidth
				if dotX < box.Min.X+pad {
					dotX = box.Min.X + pad
				}
			}
			drawTextLine(rgba, layout.fontSize, renderLine, dotX, baselineY, inkColor, box)
			baselineY += layout.lineHeight
		}
		drawn = append(drawn, candidate)
		drawnBoxes = append(drawnBoxes, box)
	}
	stats := OverlayStats{BlocksDrawn: len(drawn), BlocksSkipped: skipped, FontWarnings: fontWarnings}

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
	return prepareDrawableBoxWithShrink(x1, y1, x2, y2, overlayBboxShrinkPx)
}

func prepareDrawableBoxWithShrink(x1, y1, x2, y2 int, shrinkPx int) (int, int, int, int, bool) {
	if shrinkPx < 0 {
		shrinkPx = 0
	}
	x1 += shrinkPx
	y1 += shrinkPx
	x2 -= shrinkPx
	y2 -= shrinkPx

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

// shadowColorFor returns a softened contrasting colour used as a text drop-shadow.
func shadowColorFor(ink color.Color) color.Color {
	r, g, b, _ := ink.RGBA()
	inkR := uint8(r >> 8)
	inkG := uint8(g >> 8)
	inkB := uint8(b >> 8)
	inv := func(c uint8) uint8 { return 255 - c }
	// Mostly inverted for contrast, slightly blended toward ink tone to avoid
	// harsh halo edges on textured backgrounds.
	blend := func(inverted, ink uint8) uint8 {
		return uint8((int(inverted)*8 + int(ink)*2) / 10)
	}
	return color.RGBA{
		R: blend(inv(inkR), inkR),
		G: blend(inv(inkG), inkG),
		B: blend(inv(inkB), inkB),
		A: overlayShadowAlpha,
	}
}

func baselineScriptOffset(text string, fontSize float64) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	total := 0
	cjk := 0
	rtl := 0
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if isCJKRune(r) {
			cjk++
		}
		if isRTLRune(r) {
			rtl++
		}
	}
	if total == 0 {
		return 0
	}
	// CJK runs can look optically low with Latin-centric ascent metrics.
	if cjk*10 >= total*7 {
		return -max(1, int(math.Round(fontSize*0.04)))
	}
	// RTL runs can read better with a tiny downward optical nudge.
	if rtl*2 >= total {
		return max(1, int(math.Round(fontSize*0.03)))
	}
	return 0
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
		face, err := segFace(seg, fontSize)
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

// drawVerticalTextBlock renders confident CJK vertical text top-to-bottom.
// It returns drew=false when the block cannot be confidently rendered and
// callers should fall back to horizontal drawing.
func drawVerticalTextBlock(dst *image.RGBA, candidate overlayCandidate, box image.Rectangle, opts OverlayOptions) (drew bool, err error) {
	runes := verticalCJKRunes(candidate.text)
	if len(runes) == 0 {
		return false, nil
	}

	pad := opts.TextPadding
	availW := box.Dx() - (pad * 2)
	availH := box.Dy() - (pad * 2)
	if availW <= 0 || availH <= 0 {
		return false, nil
	}

	fontSize := min(opts.MaxFontSize, max(opts.MinFontSize, availW))
	if fontSize <= 0 {
		fontSize = opts.MinFontSize
	}

	for fontSize >= opts.MinFontSize {
		lineHeight := approximateLineHeight(fontSize)
		topInset := approximateTopInset(fontSize)
		if lineHeight <= 0 {
			break
		}
		f, ferr := faceForText(string(runes), float64(fontSize))
		if ferr != nil {
			return false, ferr
		}
		ascentCache := &overlayAscentCache
		if needsFallbackFont(string(runes)) {
			ascentCache = &fallbackAscentCache
		}
		ascent := cachedAscent(f, ascentCache, float64(fontSize))
		textH := pad + ascent + topInset + (len(runes)-1)*lineHeight + pad

		maxGlyphW := 0
		for _, r := range runes {
			w := measureLinePx(string(r), float64(fontSize))
			if w > maxGlyphW {
				maxGlyphW = w
			}
		}

		if textH <= box.Dy() && maxGlyphW <= availW {
			if opts.EraseBBox {
				patchFillBBox(dst, box)
			}
			srcLum := avgRegionLuminance(dst, box)
			inkColor := inkColorForBackground(srcLum)

			nGlyphs := len(runes)
			availContentH := box.Dy() - 2*pad
			// Vertical centering with consistent font-size-based line spacing.
			blockH := topInset + ascent + (nGlyphs-1)*lineHeight
			vertOffset := max(0, (availContentH-blockH)/2)
			dotX := box.Min.X + pad + max(0, (availW-maxGlyphW)/2)
			baselineY := box.Min.Y + pad + vertOffset + topInset + ascent
			baselineY += baselineScriptOffset(string(runes), float64(fontSize))
			for _, r := range runes {
				if baselineY > box.Max.Y-pad {
					break
				}
				drawTextLine(dst, float64(fontSize), string(r), dotX, baselineY, inkColor, box)
				baselineY += lineHeight
			}
			return true, nil
		}
		fontSize--
	}

	return false, nil
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
	// BboxShrinkPx shrinks OCR bboxes inward before drawing. Default: 2.
	// Set to 0 to preserve original bbox coordinates.
	BboxShrinkPx int
	// EraseBBox controls whether OCR bboxes are erased before drawing translated
	// text. The erase uses a gradient-preserving patch fill (median per 4×4 cell,
	// bilinear-interpolated) to avoid flat color blocks and preserve texture.
	// Default: true.
	EraseBBox bool
}

// DefaultOverlayOptions returns the default rendering options that DrawTextOnImage uses.
func DefaultOverlayOptions() OverlayOptions {
	return OverlayOptions{
		BgAlpha:      overlayBgAlpha,
		MaxFontSize:  overlayMaxFontSize,
		MinFontSize:  overlayMinFontSize,
		JPEGQuality:  90,
		TextPadding:  overlayTextPadding,
		BboxShrinkPx: overlayBboxShrinkPx,
		EraseBBox:    true,
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
	return rebalanceWrappedLines(lines, maxWidth, measureFn)
}

// rebalanceWrappedLines avoids very short final lines by moving one or two
// trailing words from the previous line when both lines remain within maxWidth.
func rebalanceWrappedLines(lines []string, maxWidth int, measureFn func(string) int) []string {
	if len(lines) < 2 || maxWidth <= 0 {
		return lines
	}
	lastIdx := len(lines) - 1
	prev := strings.TrimSpace(lines[lastIdx-1])
	last := strings.TrimSpace(lines[lastIdx])
	if prev == "" || last == "" {
		return lines
	}
	if float64(measureFn(last)) >= float64(maxWidth)*0.45 {
		return lines
	}
	prevWords := strings.Fields(prev)
	if len(prevWords) < 2 {
		return lines
	}
	absInt := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	bestPrev := prev
	bestLast := last
	bestDelta := absInt(measureFn(prev) - measureFn(last))
	for take := 1; take <= 2 && len(prevWords)-take >= 1; take++ {
		moved := strings.Join(prevWords[len(prevWords)-take:], " ")
		newPrev := strings.Join(prevWords[:len(prevWords)-take], " ")
		newLast := strings.TrimSpace(moved + " " + last)
		if measureFn(newPrev) > maxWidth || measureFn(newLast) > maxWidth {
			continue
		}
		delta := absInt(measureFn(newPrev) - measureFn(newLast))
		if delta < bestDelta {
			bestPrev = newPrev
			bestLast = newLast
			bestDelta = delta
		}
	}
	if bestPrev == prev && bestLast == last {
		return lines
	}
	out := append([]string(nil), lines...)
	out[lastIdx-1] = bestPrev
	out[lastIdx] = bestLast
	return out
}

func approximateLineHeight(fontSize int) int {
	if fontSize <= 0 {
		return 1
	}
	// Readability target: around 1.24x of font size, with hard bounds to avoid
	// cramped (too tight) or overly airy (too loose) line spacing.
	target := int(math.Round(float64(fontSize) * 1.24))
	minH := fontSize + overlayLineSpacing
	maxH := fontSize + 8
	if target < minH {
		return minH
	}
	if target > maxH {
		return maxH
	}
	return target
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

// verticalCJKRunes extracts non-space CJK runes for candidate vertical rendering.
func verticalCJKRunes(text string) []rune {
	runes := make([]rune, 0, len(text))
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		if !isCJKRune(r) {
			continue
		}
		runes = append(runes, r)
	}
	return runes
}

// shouldRenderVerticalText returns true only when heuristics are confident:
// strongly vertical bbox and mostly CJK characters.
func shouldRenderVerticalText(text string, box image.Rectangle) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	w := box.Dx()
	h := box.Dy()
	if w <= 0 || h <= 0 {
		return false
	}
	if float64(h)/float64(w) < overlayVerticalAspectThreshold {
		return false
	}

	total := 0
	cjk := 0
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if isCJKRune(r) {
			cjk++
		}
	}
	if total == 0 || cjk < 2 {
		return false
	}
	return float64(cjk)/float64(total) >= overlayVerticalCJKMinRatio
}

// hasMixedDirectionText reports whether text contains both LTR and RTL runs.
func hasMixedDirectionText(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	hasLTR := false
	hasRTL := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		props, _ := bidi.LookupRune(r)
		switch props.Class() {
		case bidi.L:
			hasLTR = true
		case bidi.R, bidi.AL:
			hasRTL = true
		}
		if hasLTR && hasRTL {
			return true
		}
	}
	return false
}

// reorderBidiForRendering performs a basic bidi visual reordering pass for
// mixed-direction text so the simple glyph renderer produces better output.
// It keeps pure-LTR and pure-RTL text unchanged.
func reorderBidiForRendering(text string) string {
	if !hasMixedDirectionText(text) {
		return text
	}
	var p bidi.Paragraph
	if _, err := p.SetString(text); err != nil {
		return text
	}
	ord, err := p.Order()
	if err != nil {
		return text
	}
	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < ord.NumRuns(); i++ {
		run := ord.Run(i)
		runText := run.String()
		if run.Direction() == bidi.RightToLeft {
			runText = bidi.ReverseString(runText)
		}
		out.WriteString(runText)
	}
	return out.String()
}

// avgRegionLuminance returns the average NTSC luminance [0,1] of image pixels
// in region r, sampled at a stride to keep the operation fast.
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

// avgRegionColor returns the average RGB color of image pixels in region r,
// sampled at a stride to keep the operation fast.
func avgRegionColor(img *image.RGBA, r image.Rectangle) color.RGBA {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	step := max(1, min(r.Dx(), r.Dy())/8)
	var sumR, sumG, sumB uint64
	samples := 0
	for y := r.Min.Y; y < r.Max.Y; y += step {
		for x := r.Min.X; x < r.Max.X; x += step {
			c := img.RGBAAt(x, y)
			sumR += uint64(c.R)
			sumG += uint64(c.G)
			sumB += uint64(c.B)
			samples++
		}
	}
	if samples == 0 {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	return color.RGBA{
		R: uint8(sumR / uint64(samples)),
		G: uint8(sumG / uint64(samples)),
		B: uint8(sumB / uint64(samples)),
		A: 255,
	}
}

// medianRegionColor computes the per-channel median color of pixels in region r
// sampled at a stride. Median is more robust than average when the region
// contains text-ink outliers: as long as background pixels outnumber ink pixels
// the median returns a background-representative color.
func medianRegionColor(img *image.RGBA, r image.Rectangle) color.RGBA {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	step := max(1, min(r.Dx(), r.Dy())/16)
	var rs, gs, bs []uint8
	for y := r.Min.Y; y < r.Max.Y; y += step {
		for x := r.Min.X; x < r.Max.X; x += step {
			c := img.RGBAAt(x, y)
			rs = append(rs, c.R)
			gs = append(gs, c.G)
			bs = append(bs, c.B)
		}
	}
	if len(rs) == 0 {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i] < rs[j] })
	sort.Slice(gs, func(i, j int) bool { return gs[i] < gs[j] })
	sort.Slice(bs, func(i, j int) bool { return bs[i] < bs[j] })
	mid := len(rs) / 2
	return color.RGBA{R: rs[mid], G: gs[mid], B: bs[mid], A: 255}
}

// patchFillBBox fills box on dst using a gradient-preserving patch fill.
// The bbox is divided into a 4×4 coarse grid; each cell's representative color
// is the per-channel median of its sampled pixels (robust to text-ink outliers).
// Every destination pixel is then written as a bilinear interpolation of its
// four surrounding cell medians, so gradients and shadows are reproduced rather
// than collapsed into a flat block.
func patchFillBBox(dst *image.RGBA, box image.Rectangle) {
	r := box.Intersect(dst.Bounds())
	if r.Empty() {
		return
	}
	const gridN = 4
	var cellColors [gridN][gridN]color.RGBA
	for gy := 0; gy < gridN; gy++ {
		for gx := 0; gx < gridN; gx++ {
			x0 := r.Min.X + gx*r.Dx()/gridN
			y0 := r.Min.Y + gy*r.Dy()/gridN
			x1 := r.Min.X + (gx+1)*r.Dx()/gridN
			y1 := r.Min.Y + (gy+1)*r.Dy()/gridN
			cell := image.Rect(x0, y0, x1, y1)
			cellColors[gy][gx] = medianRegionColor(dst, cell)
		}
	}
	w := r.Dx()
	h := r.Dy()
	if w < 2 {
		w = 2
	}
	if h < 2 {
		h = 2
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		fy := float64(y-r.Min.Y) * float64(gridN-1) / float64(h-1)
		gy0 := int(fy)
		if gy0 > gridN-2 {
			gy0 = gridN - 2
		}
		gy1 := gy0 + 1
		ty := fy - float64(gy0)
		for x := r.Min.X; x < r.Max.X; x++ {
			fx := float64(x-r.Min.X) * float64(gridN-1) / float64(w-1)
			gx0 := int(fx)
			if gx0 > gridN-2 {
				gx0 = gridN - 2
			}
			gx1 := gx0 + 1
			tx := fx - float64(gx0)
			c00 := cellColors[gy0][gx0]
			c10 := cellColors[gy0][gx1]
			c01 := cellColors[gy1][gx0]
			c11 := cellColors[gy1][gx1]
			r0 := float64(c00.R)*(1-tx) + float64(c10.R)*tx
			r1 := float64(c01.R)*(1-tx) + float64(c11.R)*tx
			g0 := float64(c00.G)*(1-tx) + float64(c10.G)*tx
			g1 := float64(c01.G)*(1-tx) + float64(c11.G)*tx
			b0 := float64(c00.B)*(1-tx) + float64(c10.B)*tx
			b1 := float64(c01.B)*(1-tx) + float64(c11.B)*tx
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(r0*(1-ty) + r1*ty),
				G: uint8(g0*(1-ty) + g1*ty),
				B: uint8(b0*(1-ty) + b1*ty),
				A: 255,
			})
		}
	}
}

// inkColorForBackground chooses black or white ink based on background
// luminance, so translated text remains legible on the erased fill color.
func inkColorForBackground(srcLum float64) color.Color {
	if srcLum >= overlayLumThreshold {
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

// parseFont parses raw font bytes. It handles both single-font files (.ttf/.otf)
// and TrueType/OpenType collections (.ttc) by taking the first font in a collection.
func parseFont(data []byte) (*opentype.Font, error) {
	f, err := opentype.Parse(data)
	if err == nil {
		return f, nil
	}
	c, err2 := opentype.ParseCollection(data)
	if err2 != nil {
		return nil, err // return original single-parse error
	}
	return c.Font(0)
}

// load lazily loads the font for this entry, trying each candidate path in order.
func (e *fallbackFontEntry) load() *opentype.Font {
	e.once.Do(func() {
		for _, p := range e.paths {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			parsed, err := parseFont(data)
			if err != nil {
				continue
			}
			e.data = parsed
			break
		}
	})
	return e.data
}

// face returns (or creates and caches) a font.Face at fontSize for this entry.
func (e *fallbackFontEntry) face(fontSize float64) (font.Face, error) {
	f := e.load()
	if f == nil {
		return nil, fmt.Errorf("fallback font %q not available on this system", e.name)
	}
	return loadFaceFromFont(f, &e.faceCache, fontSize)
}

// fontFaceCoversText returns true if face has glyphs for every non-whitespace
// rune in text. It uses GlyphAdvance: a false ok signals a missing glyph.
func fontFaceCoversText(face font.Face, text string) bool {
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		_, ok := face.GlyphAdvance(r)
		if !ok {
			return false
		}
	}
	return true
}

// faceForFallback returns the first fallback font whose face covers all runes
// in text, falling back to goregular when no entry matches.
func faceForFallback(text string, fontSize float64) (font.Face, error) {
	for _, entry := range fallbackFonts {
		face, err := entry.face(fontSize)
		if err != nil {
			// Font not available on this system; try next.
			continue
		}
		if fontFaceCoversText(face, text) {
			return face, nil
		}
	}
	// No fallback covers the text; use goregular (may produce tofu for some runes).
	return loadOverlayFace(fontSize)
}

// fontCoverageWarning returns a non-empty string when text requires a fallback
// font but no system fallback is available or covers all runes. The string
// identifies the text snippet (truncated) and names the scripts detected so
// callers can surface actionable install hints.
func fontCoverageWarning(text string) string {
	if !needsFallbackFont(text) {
		return ""
	}
	// Try each fallback at a nominal size; if any covers the text, we're fine.
	for _, entry := range fallbackFonts {
		face, err := entry.face(12)
		if err != nil {
			continue
		}
		if fontFaceCoversText(face, text) {
			return ""
		}
	}
	// Identify which scripts are present in the text for a useful hint.
	scripts := detectMissingScripts(text)
	snippet := text
	if len([]rune(snippet)) > 20 {
		snippet = string([]rune(snippet)[:20]) + "…"
	}
	return fmt.Sprintf(
		"no system font covers %q (scripts: %s); install Noto fonts to fix tofu boxes",
		snippet, strings.Join(scripts, ", "),
	)
}

// detectMissingScripts returns human-readable script names for fallback runes
// found in text. Used to build actionable install hints in font warnings.
func detectMissingScripts(text string) []string {
	seen := make(map[string]bool)
	for _, r := range text {
		if !isFallbackRune(r) {
			continue
		}
		switch {
		case r >= 0x4E00 && r <= 0x9FFF,
			r >= 0x2E80 && r <= 0x2EFF,
			r >= 0x3000 && r <= 0x303F,
			r >= 0x3040 && r <= 0x30FF,
			r >= 0xF900 && r <= 0xFAFF,
			r >= 0x20000 && r <= 0x2A6DF:
			seen["CJK"] = true
		case r >= 0xAC00 && r <= 0xD7AF,
			r >= 0x1100 && r <= 0x11FF:
			seen["Hangul"] = true
		case r >= 0x0600 && r <= 0x06FF,
			r >= 0x0750 && r <= 0x077F,
			r >= 0x08A0 && r <= 0x08FF:
			seen["Arabic"] = true
		case r >= 0x0590 && r <= 0x05FF:
			seen["Hebrew"] = true
		case r >= 0x0900 && r <= 0x097F:
			seen["Devanagari"] = true
		case r >= 0x0980 && r <= 0x09FF:
			seen["Bengali"] = true
		case r >= 0x0B80 && r <= 0x0BFF:
			seen["Tamil"] = true
		case r >= 0x0C00 && r <= 0x0C7F:
			seen["Telugu"] = true
		case r >= 0x0C80 && r <= 0x0CFF:
			seen["Kannada"] = true
		case r >= 0x0D00 && r <= 0x0D7F:
			seen["Malayalam"] = true
		case r >= 0x0E00 && r <= 0x0E7F:
			seen["Thai"] = true
		default:
			seen["other-non-Latin"] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// AvailableFallbackFonts probes each fallback font entry and returns the names
// of fonts that were successfully loaded from the local filesystem. Useful for
// health checks and startup diagnostics.
func AvailableFallbackFonts() []string {
	var available []string
	for _, entry := range fallbackFonts {
		if f := entry.load(); f != nil {
			available = append(available, entry.name)
		}
	}
	return available
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
// (Arabic, CJK, Devanagari, Indic, Thai, Hangul, and related blocks).
func isFallbackRune(r rune) bool {
	return (r >= 0x0600 && r <= 0x06FF) || // Arabic
		(r >= 0x0750 && r <= 0x077F) || // Arabic Supplement
		(r >= 0x08A0 && r <= 0x08FF) || // Arabic Extended-A
		(r >= 0x0590 && r <= 0x05FF) || // Hebrew
		(r >= 0x0900 && r <= 0x097F) || // Devanagari
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
// For text with non-Latin runes the ordered fallback list is tried until a
// font with full glyph coverage is found, reducing tofu (□) output.
func faceForText(text string, fontSize float64) (font.Face, error) {
	if needsFallbackFont(text) {
		return faceForFallback(text, fontSize)
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
// For non-Latin segments the fallback chain is searched for glyph coverage.
func segFace(seg scriptSegment, fontSize float64) (font.Face, error) {
	if seg.useSystem {
		return faceForFallback(seg.text, fontSize)
	}
	return loadOverlayFace(fontSize)
}

// measureLinePx returns the pixel advance width of line by summing per-segment
// measurements, each rendered with the appropriate font family.
func measureLinePx(line string, fontSize float64) int {
	total := 0
	for _, seg := range splitIntoScriptSegments(line) {
		face, err := segFace(seg, fontSize)
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
