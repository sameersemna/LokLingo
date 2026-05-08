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
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
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
	// overlayDetectedShadowMinDarkness is the minimum luminance drop of the
	// exterior fringe relative to background required to accept a detected
	// shadow direction.  0.08 ≈ 20/255 — subtle but consistent.
	overlayDetectedShadowMinDarkness = 0.08
	// overlayDetectedShadowMaxAlpha caps detected-shadow opacity so it never
	// overwhelms ink on low-contrast backgrounds.
	overlayDetectedShadowMaxAlpha = uint8(180)
	overlayPatchFeatherPx         = 2
	overlayPatchBlurRadius        = 1
	// overlayFitOverflowPct is the fractional height tolerance allowed during
	// font-size fitting.  A layout whose block height exceeds maxHeight by no
	// more than this fraction is still accepted, preventing the fitter from
	// dropping to the next smaller size solely due to sub-pixel rounding.
	// 0.03 = 3 % – tight enough to avoid visible clipping, loose enough to
	// prevent unnecessarily small text.
	overlayFitOverflowPct = 0.03

	// A box is considered strongly vertical when height is at least this multiple of width.
	overlayVerticalAspectThreshold = 2.2
	// Require most runes to be CJK before applying vertical rendering.
	overlayVerticalCJKMinRatio = 0.8
	// Keep vertical CJK text from becoming too small in narrow columns.
	overlayVerticalMinFontBoost = 4
	// Very narrow vertical columns need an extra readability boost.
	overlayVerticalNarrowWidthThreshold = 24
	overlayVerticalNarrowFontBoost      = 2
	// Vertical text can tolerate tighter side padding than horizontal text.
	overlayVerticalMinPadding = 2
	// Draw a second ink pass offset by 1px to slightly thicken vertical glyphs.
	overlayVerticalExtraStrokePx = 1
	// overlayTextContrastThreshold is the minimum luminance difference between a
	// sampled pixel and the estimated background required to classify that pixel
	// as text ink during dominant-colour sampling.  0.20 (= 20 % luminance gap)
	// is tight enough to skip near-background noise while still catching most
	// coloured and monochrome text.
	overlayTextContrastThreshold = 0.20
)

var overlayFontData = mustParseOverlayFont()

// embeddedGoBoldFont / embeddedGoMonoFont are always-available embedded fonts
// used as fallbacks when the system Noto Bold / Mono files are absent (e.g.
// local dev without system fonts installed).  They guarantee that bold text
// uses a genuinely heavier typeface rather than falling back to goregular.
var (
	embeddedGoBoldFont      = mustParseEmbeddedFont(gobold.TTF, "go bold")
	embeddedGoBoldFaceCache sync.Map
	embeddedGoMonoFont      = mustParseEmbeddedFont(gomono.TTF, "go mono")
	embeddedGoMonoFaceCache sync.Map
)

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
			"/usr/share/fonts/noto/NotoSansCJKsc-Regular.otf",
			"/usr/share/fonts/noto/NotoSansCJKtc-Regular.otf",
			"/usr/share/fonts/noto/NotoSansCJKjp-Regular.otf",
			"/usr/share/fonts/noto/NotoSansCJKkr-Regular.otf",
			// Debian/Ubuntu opentype path
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/opentype/noto/NotoSansCJKsc-Regular.otf",
			"/usr/share/fonts/opentype/noto/NotoSansCJKtc-Regular.otf",
			"/usr/share/fonts/opentype/noto/NotoSansCJKjp-Regular.otf",
			"/usr/share/fonts/opentype/noto/NotoSansCJKkr-Regular.otf",
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
			"/usr/share/fonts/noto/NotoNaskhArabic-Regular.ttf",
			// Debian/Ubuntu
			"/usr/share/fonts/truetype/noto/NotoSansArabic-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoNaskhArabic-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoSansArabic-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoNaskhArabic-Regular.ttf",
			"/usr/share/fonts/google-noto/NotoSansArabic-Regular.ttf",
			"/usr/share/fonts/google-noto/NotoNaskhArabic-Regular.ttf",
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
	box          image.Rectangle
	text         string
	originalText string // OCR source text; used for font-style detection
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
		candidates = append(candidates, overlayCandidate{box: image.Rect(x1, y1, x2, y2), text: translated, originalText: blk.Text})
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

		blockStyle := BlockFontStyle{}
		if !opts.DisableFontStyleDetection {
			// Detect font style and shadow geometry on original pixels before erasing.
			blockStyle = detectBlockFontStyle(rgba, box, candidate.originalText)
			blockStyle.Shadow = detectShadowOffset(rgba, box)
		}
		inkColor := color.Color(inkColorForBackground(avgRegionLuminance(rgba, box)))
		if !opts.DisableColorSampling {
			// Sample dominant text colour before erasing — preserves original ink style.
			inkColor = sampleDominantTextColor(rgba, box)
		}

		if opts.EraseBBox {
			// Erase original OCR text using a gradient-preserving patch fill so that
			// gradients and shadows are reproduced instead of a flat color block.
			patchFillBBoxWithOptions(rgba, box, opts)
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

		rtl := isRTLText(candidate.text)
		for _, line := range layout.lines {
			if baselineY > box.Max.Y-pad {
				break
			}
			renderLine := reorderBidiForRendering(line)
			lineWidth := measureLinePx(renderLine, layout.fontSize)
			nRunes := len([]rune(renderLine))
			ls := computeLetterSpacing(lineWidth, box.Dx()-2*pad, nRunes, blockStyle)
			dotX := box.Min.X + pad
			if rtl {
				// Right-align each RTL line.
				dotX = box.Max.X - pad - lineWidth
				if dotX < box.Min.X+pad {
					dotX = box.Min.X + pad
				}
			}
			drawTextLineWithStyle(rgba, layout.fontSize, renderLine, dotX, baselineY, inkColor, box, blockStyle, ls)
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

// detectShadowOffset probes the pixel fringe around box in 8 directions on
// the original (un-erased) image and returns the direction whose 2-pixel-wide
// exterior strip is darkest relative to the estimated background inside box.
//
// The darkness of that strip is mapped to a shadow alpha proportional to the
// detected depth.  If no consistent shadow is found the hint is the zero
// value, which callers interpret as "use the default (1,1) offset".
func detectShadowOffset(img *image.RGBA, box image.Rectangle) shadowHint {
	bounds := img.Bounds()
	bg := medianRegionColor(img, box)
	bgLum := colorLuminance(bg)

	candidates := [][2]int{{1, 1}, {1, 0}, {0, 1}, {-1, 1}, {1, -1}, {-1, -1}, {0, -1}, {-1, 0}}

	bestDx, bestDy := 1, 1
	bestDarkness := 0.0
	for _, d := range candidates {
		dx, dy := d[0], d[1]
		// Shift the full box 1 px in the candidate direction to get the exterior
		// fringe strip (overlapping with the image border clamps naturally).
		fringe := box.Add(image.Point{X: dx, Y: dy}).Intersect(bounds)
		if fringe.Empty() {
			continue
		}
		fringeLum := avgRegionLuminance(img, fringe)
		darkness := bgLum - fringeLum
		if darkness > bestDarkness {
			bestDarkness = darkness
			bestDx, bestDy = dx, dy
		}
	}

	if bestDarkness < overlayDetectedShadowMinDarkness {
		return shadowHint{} // no clear shadow — caller uses default
	}
	// Scale alpha: darkness at threshold → ~60, at 0.15 → overlayShadowAlpha,
	// above 0.30 → cap at overlayDetectedShadowMaxAlpha.
	scaled := float64(overlayShadowAlpha) * bestDarkness / 0.15
	alpha := uint8(math.Min(float64(overlayDetectedShadowMaxAlpha), math.Max(60, scaled)))
	return shadowHint{dx: bestDx, dy: bestDy, alpha: alpha}
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
	letters := 0
	descenders := 0
	cjk := 0
	rtl := 0
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if unicode.IsLetter(r) {
			letters++
			if isDescenderRune(r) {
				descenders++
			}
		}
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

	// Apply a small optical upward nudge so text appears centered by eye.
	// 5-10% of font-size target: baseline starts at ~7% up.
	offset := -max(1, int(math.Round(fontSize*0.07)))

	// Descender-heavy strings (e.g. "gypqj") need less upward nudge.
	// Cap compensation at ~5% of font size so text does not sink too low.
	if letters > 0 && descenders > 0 {
		ratio := float64(descenders) / float64(letters)
		normalized := ratio / 0.55
		if normalized > 1.0 {
			normalized = 1.0
		}
		offset += int(math.Round(fontSize * 0.05 * normalized))
	}

	// CJK runs can look optically low with Latin-centric ascent metrics.
	if cjk*10 >= total*7 {
		offset -= max(1, int(math.Round(fontSize*0.02)))
	}

	// RTL runs typically have deeper bowls/descenders, so reduce the upward nudge.
	if rtl*2 >= total {
		offset += max(1, int(math.Round(fontSize*0.04)))
	}

	return offset
}

func isDescenderRune(r rune) bool {
	// Include common descender-heavy Latin glyphs.
	switch unicode.ToLower(r) {
	case 'g', 'j', 'p', 'q', 'y':
		return true
	}
	return false
}

// drawTextLine renders line at (dotX, baselineY) with a 1-pixel drop-shadow.
// Text is split into script segments and each segment is drawn with the
// appropriate font family (goregular or system fallback) so mixed-script lines
// (e.g. "Hello 世界") do not produce tofu boxes.
// clipRect restricts all pixel writes to the overlay box, preventing the
// shadow offset from bleeding into adjacent image regions.
func drawTextLine(dst *image.RGBA, fontSize float64, line string, dotX, baselineY int, inkColor color.Color, clipRect image.Rectangle) {
	drawTextLineStyled(dst, fontSize, line, dotX, baselineY, inkColor, clipRect, 0)
}

// drawTextLineWithStyle is like drawTextLine but selects a font face that
// matches the detected BlockFontStyle (bold, serif, mono) for Latin segments.
// It also computes adaptive stroke passes via computeStrokeOffsets.
func drawTextLineWithStyle(dst *image.RGBA, fontSize float64, line string, dotX, baselineY int, inkColor color.Color, clipRect image.Rectangle, style BlockFontStyle, letterSpacing fixed.Int26_6) {
	strokes := computeStrokeOffsets(style, fontSize)
	drawTextLineStyledWithFont(dst, fontSize, line, dotX, baselineY, inkColor, clipRect, strokes, style, letterSpacing)
}

// drawTextLineStyled renders text like drawTextLine and optionally adds a small
// extra ink pass offset horizontally to slightly thicken glyphs.
func drawTextLineStyled(dst *image.RGBA, fontSize float64, line string, dotX, baselineY int, inkColor color.Color, clipRect image.Rectangle, extraInkX int) {
	var strokes []strokeOffset
	if extraInkX > 0 {
		strokes = []strokeOffset{{extraInkX, 0}}
	}
	drawTextLineStyledWithFont(dst, fontSize, line, dotX, baselineY, inkColor, clipRect, strokes, BlockFontStyle{}, 0)
}

// drawTextLineStyledWithFont is the core text-drawing primitive.  It renders
// line at (dotX, baselineY) with a drop-shadow, splitting the text into
// script segments so each segment uses the appropriate font family.  When
// style is non-zero, Latin segments are drawn with the matching Noto variant
// (bold, serif, or mono); non-Latin segments always use the script-coverage
// fallback chain.
// strokes lists extra ink passes (dx,dy offsets) applied after the primary draw
// to simulate stroke weight; see computeStrokeOffsets for the selection logic.
// letterSpacing is a fixed.Int26_6 offset added to the dot after each glyph
// (except the last), providing adaptive per-line letter-spacing adjustment.
// Pass nil/0 for the defaults.
func drawTextLineStyledWithFont(dst *image.RGBA, fontSize float64, line string, dotX, baselineY int, inkColor color.Color, clipRect image.Rectangle, strokes []strokeOffset, style BlockFontStyle, letterSpacing fixed.Int26_6) {
	clipped := dst.SubImage(clipRect).(*image.RGBA)
	shadow := shadowColorFor(inkColor)
	// Apply detected shadow geometry; fall back to the default (1,1) offset.
	shDx, shDy := style.Shadow.dx, style.Shadow.dy
	if shDx == 0 && shDy == 0 {
		shDx, shDy = 1, 1
	}
	if style.Shadow.alpha != 0 {
		// Override alpha with the proportionally scaled detected value.
		sr, sg, sb, _ := shadow.RGBA()
		shadow = color.RGBA{
			R: uint8(sr >> 8),
			G: uint8(sg >> 8),
			B: uint8(sb >> 8),
			A: style.Shadow.alpha,
		}
	}
	dot := fixed.P(dotX, baselineY)
	shadowSrc := image.NewUniform(shadow)
	inkSrc := image.NewUniform(inkColor)
	segs := splitIntoScriptSegments(line)
	// Pre-count total runes so we know when to omit the trailing gap.
	totalRunes := 0
	for _, seg := range segs {
		totalRunes += len([]rune(seg.text))
	}
	runesDone := 0
	for _, seg := range segs {
		face, err := segFaceStyled(seg, fontSize, style)
		if err != nil {
			continue
		}
		if letterSpacing == 0 {
			// Fast path: draw whole segment at once.
			(&font.Drawer{Dst: clipped, Src: shadowSrc, Face: face, Dot: fixed.P(dot.X.Ceil()+shDx, baselineY+shDy)}).DrawString(seg.text)
			d := &font.Drawer{Dst: clipped, Src: inkSrc, Face: face, Dot: dot}
			d.DrawString(seg.text)
			for _, so := range strokes {
				(&font.Drawer{Dst: clipped, Src: inkSrc, Face: face, Dot: fixed.P(dot.X.Ceil()+so.dx, baselineY+so.dy)}).DrawString(seg.text)
			}
			runesDone += len([]rune(seg.text))
			dot = d.Dot
		} else {
			// Spacing path: draw rune-by-rune, inserting letterSpacing between each
			// adjacent pair of glyphs (gap count = totalRunes-1 across all segments).
			for _, r := range []rune(seg.text) {
				runesDone++
				rs := string(r)
				(&font.Drawer{Dst: clipped, Src: shadowSrc, Face: face, Dot: fixed.P(dot.X.Ceil()+shDx, baselineY+shDy)}).DrawString(rs)
				d := &font.Drawer{Dst: clipped, Src: inkSrc, Face: face, Dot: dot}
				d.DrawString(rs)
				for _, so := range strokes {
					(&font.Drawer{Dst: clipped, Src: inkSrc, Face: face, Dot: fixed.P(dot.X.Ceil()+so.dx, baselineY+so.dy)}).DrawString(rs)
				}
				dot = d.Dot
				if runesDone < totalRunes {
					dot.X += letterSpacing
				}
			}
		}
	}
}

func verticalTextPadding(boxWidth int, opts OverlayOptions) int {
	if boxWidth <= 0 {
		return opts.TextPadding
	}
	target := max(overlayVerticalMinPadding, boxWidth/10)
	return min(opts.TextPadding, target)
}

func verticalMinFontSize(opts OverlayOptions, availW int) int {
	minVerticalFont := opts.MinFontSize + overlayVerticalMinFontBoost
	if availW <= overlayVerticalNarrowWidthThreshold {
		minVerticalFont += overlayVerticalNarrowFontBoost
	}
	if minVerticalFont > opts.MaxFontSize {
		return opts.MaxFontSize
	}
	return minVerticalFont
}

func verticalExtraStrokeWidth() int {
	return overlayVerticalExtraStrokePx
}

// drawVerticalTextBlock renders confident CJK vertical text top-to-bottom.
// It returns drew=false when the block cannot be confidently rendered and
// callers should fall back to horizontal drawing.
func drawVerticalTextBlock(dst *image.RGBA, candidate overlayCandidate, box image.Rectangle, opts OverlayOptions) (drew bool, err error) {
	runes := verticalCJKRunes(candidate.text)
	if len(runes) == 0 {
		return false, nil
	}

	pad := verticalTextPadding(box.Dx(), opts)
	availW := box.Dx() - (pad * 2)
	availH := box.Dy() - (pad * 2)
	if availW <= 0 || availH <= 0 {
		return false, nil
	}

	minVerticalFont := verticalMinFontSize(opts, availW)
	extraStrokeW := verticalExtraStrokeWidth()
	fontSize := min(opts.MaxFontSize, max(minVerticalFont, availW))
	if fontSize <= 0 {
		fontSize = minVerticalFont
	}
	if fontSize < minVerticalFont {
		fontSize = minVerticalFont
	}

	for fontSize >= minVerticalFont {
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
		maxGlyphW += extraStrokeW

		if textH <= box.Dy() && maxGlyphW <= availW {
			inkColor := color.Color(inkColorForBackground(avgRegionLuminance(dst, box)))
			if !opts.DisableColorSampling {
				inkColor = sampleDominantTextColor(dst, box)
			}
			if opts.EraseBBox {
				patchFillBBoxWithOptions(dst, box, opts)
			}

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
				drawTextLineStyled(dst, float64(fontSize), string(r), dotX, baselineY, inkColor, box, extraStrokeW)
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
	// PatchFeatherPx controls edge feathering width in pixels when erasing OCR boxes.
	// Recommended range: 1-3. Default: 2.
	PatchFeatherPx int
	// PatchBlurRadius controls slight edge blur radius in pixels for erased OCR boxes.
	// Default: 1.
	PatchBlurRadius int
	// EraseBBox controls whether OCR bboxes are erased before drawing translated
	// text. The erase uses a gradient-preserving patch fill (median per 4×4 cell,
	// bilinear-interpolated) to avoid flat color blocks and preserve texture.
	// Default: true.
	EraseBBox bool
	// DisableFontStyleDetection bypasses OCR-source style detection and renders
	// all Latin text with the default sans regular face. Default: false.
	DisableFontStyleDetection bool
	// DisableColorSampling bypasses dominant text-colour sampling from original
	// pixels and falls back to black/white ink via background luminance.
	// Default: false.
	DisableColorSampling bool
}

// DefaultOverlayOptions returns the default rendering options that DrawTextOnImage uses.
func DefaultOverlayOptions() OverlayOptions {
	return OverlayOptions{
		BgAlpha:                   overlayBgAlpha,
		MaxFontSize:               overlayMaxFontSize,
		MinFontSize:               overlayMinFontSize,
		JPEGQuality:               90,
		TextPadding:               overlayTextPadding,
		BboxShrinkPx:              overlayBboxShrinkPx,
		PatchFeatherPx:            overlayPatchFeatherPx,
		PatchBlurRadius:           overlayPatchBlurRadius,
		EraseBBox:                 true,
		DisableFontStyleDetection: false,
		DisableColorSampling:      false,
	}
}

// blockHeight returns the pixel height consumed by a text block with the
// given parameters, matching the actual render formula used in the draw loop.
func blockHeight(topInset, ascent, lineHeight, nLines int) int {
	return topInset + ascent + (nLines-1)*lineHeight
}

func fitTextLayout(text string, maxWidth, maxHeight, minFontSize, maxFontSize int) textLayout {
	if strings.TrimSpace(text) == "" || maxWidth <= 0 || maxHeight <= 0 {
		return textLayout{}
	}

	// toleratedHeight is the maximum block height accepted during the fit loop.
	// Allowing a small overflow (overlayFitOverflowPct = 3 %) prevents the
	// fitter from downgrading to the next smaller font purely due to rounding,
	// producing text that feels naturally sized rather than overly conservative.
	toleratedHeight := int(math.Round(float64(maxHeight) * (1.0 + overlayFitOverflowPct)))

	wrapAndMeasure := func(fontSize int) (lines []string, lineHeight, topInset, ascent int) {
		measureFn := func(s string) int { return measureLinePx(s, float64(fontSize)) }
		splitFn := func(word string, maxW int) (string, string) {
			f, err := faceForText(word, float64(fontSize))
			if err != nil {
				return word, ""
			}
			return fitWordToWidth(word, maxW, f)
		}
		return wrapLines(text, maxWidth, measureFn, splitFn),
			approximateLineHeight(fontSize),
			approximateTopInset(fontSize),
			approximateAscent(fontSize)
	}

	startSize := min(maxFontSize, max(minFontSize, maxHeight))
	for fontSize := startSize; fontSize >= minFontSize; fontSize-- {
		lines, lineHeight, topInset, ascent := wrapAndMeasure(fontSize)
		if len(lines) == 0 {
			continue
		}
		// Accept if the block fits within the tolerated (slightly enlarged) height.
		if blockHeight(topInset, ascent, lineHeight, len(lines)) <= toleratedHeight {
			// Bias toward the next larger size when that size also fits within
			// the tolerated height and produces no extra lines.  This avoids
			// the conservative one-step-too-small result that occurs at size
			// boundaries where the difference is sub-pixel rounding.
			if fontSize < maxFontSize {
				largerLines, largerLH, largerTI, largerAscent := wrapAndMeasure(fontSize + 1)
				if len(largerLines) == len(lines) &&
					blockHeight(largerTI, largerAscent, largerLH, len(largerLines)) <= toleratedHeight {
					return textLayout{
						fontSize:   float64(fontSize + 1),
						lineHeight: largerLH,
						topInset:   largerTI,
						lines:      largerLines,
					}
				}
			}
			return textLayout{
				fontSize:   float64(fontSize),
				lineHeight: lineHeight,
				topInset:   topInset,
				lines:      lines,
			}
		}
	}

	// No size fit within the tolerated height – fall back to minFontSize and
	// truncate lines that cannot be accommodated.
	fontSize := minFontSize
	lines, lineHeight, topInset, ascent := wrapAndMeasure(fontSize)
	measureFn := func(s string) int { return measureLinePx(s, float64(fontSize)) }
	// How many lines fit: topInset + ascent + (N-1)*lineHeight <= toleratedHeight
	maxLines := max(1, (toleratedHeight-topInset-ascent)/lineHeight+1)
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
	// Readability target: 1.18× font size.  Tighter than the previous 1.24×
	// so the fitter can fit the same text at a slightly larger font without
	// the line spacing alone causing a size downgrade.
	// Hard bounds: minimum = fontSize + overlayLineSpacing (no cramping),
	// maximum = fontSize + 9 (not overly airy).
	target := int(math.Round(float64(fontSize) * 1.18))
	minH := fontSize + overlayLineSpacing
	maxH := fontSize + 9
	if target < minH {
		return minH
	}
	if target > maxH {
		return maxH
	}
	return target
}

func approximateTopInset(fontSize int) int {
	// Reduced from 0.15 to 0.10 so the top inset does not consume vertical
	// space that could otherwise allow a slightly larger font to fit.
	return max(1, int(math.Round(float64(fontSize)*0.10)))
}

// approximateAscent returns a conservative pixel estimate of the font ascent
// (distance from baseline to the top of most glyphs) at the given font size.
// Used by fitTextLayout to match the actual block height formula
// (topInset + ascent + (N-1)*lineHeight) without loading a font face.
// The coefficient 0.82 is calibrated to goregular; fallback fonts are similar.
func approximateAscent(fontSize int) int {
	if fontSize <= 0 {
		return 1
	}
	return max(1, int(math.Round(float64(fontSize)*0.82)))
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

// patchFillBBox fills box on dst using default erase settings.
func patchFillBBox(dst *image.RGBA, box image.Rectangle) {
	patchFillBBoxWithOptions(dst, box, DefaultOverlayOptions())
}

// boundaryContrast returns a [0,1] contrast score by comparing luminance of
// pixels just outside the bbox edge versus pixels just inside it on dst.
// It is called before the patch is written, so dst still holds the original image.
func boundaryContrast(dst *image.RGBA, r image.Rectangle) float64 {
	bounds := dst.Bounds()
	lumPx := func(x, y int) float64 {
		c := dst.RGBAAt(x, y)
		// BT.601 luma
		return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
	}
	var sumSq float64
	var count int
	// top and bottom edges
	for x := r.Min.X; x < r.Max.X; x++ {
		if r.Min.Y-1 >= bounds.Min.Y {
			d := lumPx(x, r.Min.Y-1) - lumPx(x, r.Min.Y)
			sumSq += d * d
			count++
		}
		if r.Max.Y < bounds.Max.Y {
			d := lumPx(x, r.Max.Y) - lumPx(x, r.Max.Y-1)
			sumSq += d * d
			count++
		}
	}
	// left and right edges
	for y := r.Min.Y; y < r.Max.Y; y++ {
		if r.Min.X-1 >= bounds.Min.X {
			d := lumPx(r.Min.X-1, y) - lumPx(r.Min.X, y)
			sumSq += d * d
			count++
		}
		if r.Max.X < bounds.Max.X {
			d := lumPx(r.Max.X, y) - lumPx(r.Max.X-1, y)
			sumSq += d * d
			count++
		}
	}
	if count == 0 {
		return 0
	}
	rms := math.Sqrt(sumSq / float64(count))
	return math.Min(rms/128.0, 1.0)
}

// patchFillBBoxWithOptions fills box on dst using a gradient-preserving patch fill.
// The bbox is divided into a 4×4 coarse grid; each cell's representative color
// is the per-channel median of its sampled pixels (robust to text-ink outliers).
// Every destination pixel is then written as a bilinear interpolation of its
// four surrounding cell medians, so gradients and shadows are reproduced rather
// than collapsed into a flat block.
// Feather width and blur radius are scaled up automatically on high-contrast
// boundaries so seams are better hidden on textured/high-contrast backgrounds.
func patchFillBBoxWithOptions(dst *image.RGBA, box image.Rectangle, opts OverlayOptions) {
	r := box.Intersect(dst.Bounds())
	if r.Empty() {
		return
	}

	featherPx := opts.PatchFeatherPx
	if featherPx <= 0 {
		featherPx = overlayPatchFeatherPx
	}
	if featherPx < 1 {
		featherPx = 1
	}
	if featherPx > 3 {
		featherPx = 3
	}
	blurRadius := opts.PatchBlurRadius
	if blurRadius < 0 {
		blurRadius = overlayPatchBlurRadius
	}
	if blurRadius > 2 {
		blurRadius = 2
	}

	// Adapt feather and blur to local boundary contrast.
	// contrast ∈ [0,1]; at 0 no scaling; at 1 feather scales ×2.5, blur ×2.
	contrast := boundaryContrast(dst, r)
	if contrast > 0.05 {
		featherScale := 1.0 + 1.5*contrast
		blurScale := 1.0 + 1.0*contrast
		featherPx = int(math.Round(float64(featherPx) * featherScale))
		blurRadius = int(math.Round(float64(blurRadius) * blurScale))
		if featherPx > 6 {
			featherPx = 6
		}
		if blurRadius > 4 {
			blurRadius = 4
		}
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

	patch := make([]color.RGBA, r.Dx()*r.Dy())
	idxAt := func(px, py int) int { return py*r.Dx() + px }

	for py := 0; py < r.Dy(); py++ {
		fy := float64(py) * float64(gridN-1) / float64(h-1)
		gy0 := int(fy)
		if gy0 > gridN-2 {
			gy0 = gridN - 2
		}
		gy1 := gy0 + 1
		ty := fy - float64(gy0)
		for px := 0; px < r.Dx(); px++ {
			fx := float64(px) * float64(gridN-1) / float64(w-1)
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
			patch[idxAt(px, py)] = color.RGBA{
				R: uint8(r0*(1-ty) + r1*ty),
				G: uint8(g0*(1-ty) + g1*ty),
				B: uint8(b0*(1-ty) + b1*ty),
				A: 255,
			}
		}
	}

	// Blur only a narrow band near edges to soften transition to surrounding pixels.
	if blurRadius > 0 {
		band := featherPx + blurRadius
		blurred := make([]color.RGBA, len(patch))
		copy(blurred, patch)
		for py := 0; py < r.Dy(); py++ {
			for px := 0; px < r.Dx(); px++ {
				d := edgeDistance(px, py, r.Dx(), r.Dy())
				if d > band {
					continue
				}
				var sr, sg, sb, count int
				for oy := -blurRadius; oy <= blurRadius; oy++ {
					ny := py + oy
					if ny < 0 || ny >= r.Dy() {
						continue
					}
					for ox := -blurRadius; ox <= blurRadius; ox++ {
						nx := px + ox
						if nx < 0 || nx >= r.Dx() {
							continue
						}
						c := patch[idxAt(nx, ny)]
						sr += int(c.R)
						sg += int(c.G)
						sb += int(c.B)
						count++
					}
				}
				if count == 0 {
					continue
				}
				blurred[idxAt(px, py)] = color.RGBA{
					R: uint8(sr / count),
					G: uint8(sg / count),
					B: uint8(sb / count),
					A: 255,
				}
			}
		}
		patch = blurred
	}

	// Composite with feathered alpha at edges (1-3px) to hide patch seams.
	for py := 0; py < r.Dy(); py++ {
		for px := 0; px < r.Dx(); px++ {
			x := r.Min.X + px
			y := r.Min.Y + py
			orig := dst.RGBAAt(x, y)
			fill := patch[idxAt(px, py)]

			alpha := 1.0
			d := edgeDistance(px, py, r.Dx(), r.Dy())
			if d < featherPx {
				alpha = float64(d+1) / float64(featherPx+1)
			}

			dst.SetRGBA(x, y, blendRGBA(orig, fill, alpha))
		}
	}
}

func edgeDistance(x, y, w, h int) int {
	left := x
	right := w - 1 - x
	top := y
	bottom := h - 1 - y
	d := left
	if right < d {
		d = right
	}
	if top < d {
		d = top
	}
	if bottom < d {
		d = bottom
	}
	return d
}

func blendRGBA(base, over color.RGBA, alpha float64) color.RGBA {
	if alpha <= 0 {
		return base
	}
	if alpha >= 1 {
		return over
	}
	inv := 1.0 - alpha
	return color.RGBA{
		R: uint8(float64(base.R)*inv + float64(over.R)*alpha),
		G: uint8(float64(base.G)*inv + float64(over.G)*alpha),
		B: uint8(float64(base.B)*inv + float64(over.B)*alpha),
		A: 255,
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

// colorLuminance returns the NTSC perceptual luminance [0,1] of c.
func colorLuminance(c color.RGBA) float64 {
	return 0.299*float64(c.R)/255 + 0.587*float64(c.G)/255 + 0.114*float64(c.B)/255
}

// Color-boost parameters for sampled text-ink colours.
const (
	// boostSaturationDelta is added to the HSL saturation of sampled text
	// colours so results are vivid and clearly distinct.  22 % provides
	// obvious colour impact while remaining true to the original hue intent.
	boostSaturationDelta = 0.22
	// boostMinContrast is the minimum required luminance difference between
	// the sampled ink colour and the estimated background.  0.42 ensures text
	// is always clearly readable against its local background.
	boostMinContrast = 0.42
)

// boostSampledColor increases the visual impact of a sampled text-ink colour
// by raising its HSL saturation and enforcing minimum contrast against the
// given background luminance.  This prevents washed-out or low-impact colours
// from appearing in the translated overlay.
func boostSampledColor(c color.RGBA, bgLum float64) color.RGBA {
	h, s, l := rgbToHSL(c)
	// 1. Boost saturation.
	s = math.Min(1.0, s+boostSaturationDelta)
	// 2. Enforce minimum luminance contrast against background.
	inkLum := colorLuminance(c)
	if bgLum >= overlayLumThreshold {
		// Light background — push ink darker if needed.
		if diff := bgLum - inkLum; diff < boostMinContrast {
			l = math.Max(0.0, l-(boostMinContrast-diff))
		}
	} else {
		// Dark background — push ink lighter if needed.
		if diff := inkLum - bgLum; diff < boostMinContrast {
			l = math.Min(1.0, l+(boostMinContrast-diff))
		}
	}
	r, g, b := hslToRGB(h, s, l)
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

// rgbToHSL converts an sRGB colour to hue [0, 360), saturation [0, 1], lightness [0, 1].
func rgbToHSL(c color.RGBA) (h, s, l float64) {
	r := float64(c.R) / 255
	g := float64(c.G) / 255
	b := float64(c.B) / 255
	cmax := math.Max(r, math.Max(g, b))
	cmin := math.Min(r, math.Min(g, b))
	delta := cmax - cmin
	l = (cmax + cmin) / 2
	if delta == 0 {
		return 0, 0, l
	}
	if l < 0.5 {
		s = delta / (cmax + cmin)
	} else {
		s = delta / (2 - cmax - cmin)
	}
	switch cmax {
	case r:
		h = math.Mod((g-b)/delta, 6)
	case g:
		h = (b-r)/delta + 2
	default:
		h = (r-g)/delta + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, l
}

// hslToRGB converts HSL back to sRGB byte values.
func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	if s == 0 {
		v := uint8(math.Round(l * 255))
		return v, v, v
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	rv := hueToRGB(p, q, h/360+1.0/3)
	gv := hueToRGB(p, q, h/360)
	bv := hueToRGB(p, q, h/360-1.0/3)
	return uint8(math.Round(rv * 255)), uint8(math.Round(gv * 255)), uint8(math.Round(bv * 255))
}

// hueToRGB is the standard HSL hue-to-channel helper.
func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 1.0/2:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	}
	return p
}

// sampleDominantTextColor samples pixel colours inside box on img (which must
// still hold the original, un-erased pixels) and returns the estimated
// dominant text-ink colour.
//
// Strategy:
//  1. Compute the per-channel median of sampled pixels as the background colour
//     estimate (median is robust to ink outliers while background is the majority).
//  2. Collect sampled pixels whose luminance deviates from the background by at
//     least overlayTextContrastThreshold — these are the text-ink candidates.
//  3. Average the ink-candidate pixels and return that colour.
//  4. If fewer than 10 % of sampled pixels qualify (very low-contrast box, or
//     already-erased region), fall back to inkColorForBackground for legibility.
func sampleDominantTextColor(img *image.RGBA, box image.Rectangle) color.Color {
	r := box.Intersect(img.Bounds())
	if r.Empty() {
		return inkColorForBackground(1.0)
	}
	bg := medianRegionColor(img, r)
	bgLum := colorLuminance(bg)
	step := max(1, min(r.Dx(), r.Dy())/16)
	var sumR, sumG, sumB uint64
	textPx, totalPx := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y += step {
		for x := r.Min.X; x < r.Max.X; x += step {
			totalPx++
			c := img.RGBAAt(x, y)
			if math.Abs(colorLuminance(c)-bgLum) >= overlayTextContrastThreshold {
				sumR += uint64(c.R)
				sumG += uint64(c.G)
				sumB += uint64(c.B)
				textPx++
			}
		}
	}
	if textPx < max(1, totalPx/10) {
		return inkColorForBackground(bgLum)
	}
	sampled := color.RGBA{
		R: uint8(sumR / uint64(textPx)),
		G: uint8(sumG / uint64(textPx)),
		B: uint8(sumB / uint64(textPx)),
		A: 255,
	}
	return boostSampledColor(sampled, bgLum)
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

// fontCoversText returns true if f has a real (non-notdef) glyph for every
// visible rune in text. It uses sfnt.GlyphIndex with a fresh local buffer so
// the check is thread-safe even when f is shared across goroutines. Whitespace
// and Unicode format characters (category Cf: zero-width joiners, directional
// marks, etc.) are skipped because they carry no visible glyph and many
// otherwise-correct fonts omit them from their cmap.
func fontCoversText(f *opentype.Font, text string) bool {
	var buf sfnt.Buffer
	for _, r := range text {
		if unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		x, err := f.GlyphIndex(&buf, r)
		if err != nil || x == 0 {
			return false
		}
	}
	return true
}

// faceForFallback returns the first fallback font whose face covers all runes
// in text, falling back to goregular when no entry matches.
func faceForFallback(text string, fontSize float64) (font.Face, error) {
	for _, entry := range fallbackFonts {
		f := entry.load()
		if f == nil {
			// Font not available on this system; try next.
			continue
		}
		if !fontCoversText(f, text) {
			continue
		}
		face, err := entry.face(fontSize)
		if err != nil {
			continue
		}
		return face, nil
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
	// Try each fallback; if any covers the text, we're fine.
	for _, entry := range fallbackFonts {
		f := entry.load()
		if f == nil {
			continue
		}
		if fontCoversText(f, text) {
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

// computeLetterSpacing returns a fixed.Int26_6 per-inter-glyph spacing delta for
// one rendered line.  The approach:
//
//  1. Compute raw surplus/deficit per gap:
//     rawPx = (availWidth - textWidth) / (nRunes - 1)
//  2. Dampen to 30 % so the adjustment stays subtle.
//  3. Clamp to [-2, +2] px.
//  4. Ignore micro-adjustments below ±0.25 px.
//
// Monospace blocks are exempt (their spacing is already uniform).
// Returns 0 when the line has ≤ 1 rune or either dimension is zero.
func computeLetterSpacing(textWidth, availWidth, nRunes int, style BlockFontStyle) fixed.Int26_6 {
	if style.Class == monoFontClass || nRunes <= 1 || availWidth <= 0 || textWidth <= 0 {
		return 0
	}
	gaps := nRunes - 1
	rawPx := float64(availWidth-textWidth) / float64(gaps)
	spacingPx := rawPx * 0.30
	const maxSpacingPx = 2.0
	if spacingPx > maxSpacingPx {
		spacingPx = maxSpacingPx
	} else if spacingPx < -maxSpacingPx {
		spacingPx = -maxSpacingPx
	}
	// Skip sub-quarter-pixel adjustments to avoid invisible micro-jitter.
	if spacingPx > -0.25 && spacingPx < 0.25 {
		return 0
	}
	return fixed.Int26_6(math.Round(spacingPx * 64))
}

// strokeOffset is a pixel (dx, dy) pair for an additional ink draw pass.
type strokeOffset struct{ dx, dy int }

// computeStrokeOffsets returns the set of extra ink passes needed to give text
// consistent visual weight across scripts and font styles.
//
// Strategy (first matching rule applies):
//
//   - Monospace: no extra passes — character shape precision is paramount.
//   - Bold: full 4-direction cross (+1,0),(-1,0),(0,+1),(0,-1) at all sizes up
//     to 32px.  Four passes produce strokes that are clearly heavier than
//     normal text, giving a strong visual hierarchy.  Above 32px glyphs are
//     already large enough that the bold font file weight reads clearly.
//   - Serif: no extra passes — serif contrast (thick/thin strokes) is
//     intentional; uniform thickening would destroy the rhythm.
//   - Normal sans at fontSize ≤ 16: one subtle horizontal pass (+1,0).  Small
//     sans glyphs can appear thin at low DPI; a single pixel broadens them
//     just enough to read clearly.  This also preserves the clear gap between
//     normal (1 pass) and bold (4 passes) at all small sizes.
func computeStrokeOffsets(style BlockFontStyle, fontSize float64) []strokeOffset {
	if style.Class == monoFontClass {
		return nil
	}
	if style.Bold {
		// Large glyphs are already visually heavy from the bold font file.
		if fontSize > 32 {
			return nil
		}
		// 5-pass thickening: 4-direction cross + one diagonal.
		// The diagonal pass adds weight in the natural shadow direction,
		// making bold text unmistakably heavier than normal.
		return []strokeOffset{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}}
	}
	if style.Class == serifFontClass {
		return nil
	}
	// Normal/sans at small sizes — single subtle horizontal pass.
	if fontSize <= 16 {
		return []strokeOffset{{1, 0}}
	}
	return nil
}

func mustParseOverlayFont() *opentype.Font {
	parsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		panic(fmt.Sprintf("parse overlay font: %v", err))
	}
	return parsed
}

func mustParseEmbeddedFont(data []byte, name string) *opentype.Font {
	parsed, err := opentype.Parse(data)
	if err != nil {
		panic(fmt.Sprintf("parse embedded font %s: %v", name, err))
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
