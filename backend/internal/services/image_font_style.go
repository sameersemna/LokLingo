package services

// image_font_style.go — font-style detection and styled font selection for OCR block overlays.
//
// Heuristics applied per block (on original pixels, before erasing):
//   - Ink density  → normal vs bold
//   - Sobel H/V edge ratio → serif vs sans
//   - Column-ink CV + char-width ratio → monospace
//
// Detected class is mapped to the closest available Noto font family:
//   - sans  → NotoSans-Regular / NotoSans-Bold
//   - serif → NotoSerif-Regular / NotoSerif-Bold
//   - mono  → NotoSansMono-Regular
//
// All detection runs on sampled pixel data for speed; each helper
// processes at most a few hundred pixels per block.

import (
	"image"
	"math"
	"strings"
	"unicode/utf8"

	"golang.org/x/image/font"
)

// fontClass is the detected typographic category for an OCR block.
type fontClass int

const (
	sansFontClass  fontClass = iota // sans-serif (default)
	serifFontClass                  // serif (e.g. Times-like)
	monoFontClass                   // monospace (e.g. Courier-like)
)

// BlockFontStyle holds the inferred typographic style for one OCR block.
type BlockFontStyle struct {
	Bold  bool
	Class fontClass
}

// -- Detection thresholds (tuned conservatively to avoid over-triggering) --
const (
	// inkDensityBoldThreshold: fraction of sampled pixels identified as ink
	// above which the block is classified as bold.  Bold strokes are thicker
	// and occupy more of the bounding box than regular-weight strokes.
	inkDensityBoldThreshold = 0.20

	// serifEdgeRatioThreshold: minimum ratio of Sobel horizontal-edge density
	// to vertical-edge density required to classify a block as serif.
	// Serif crossbars produce a marked excess of horizontal edges at glyph
	// terminals relative to the stroke-boundary vertical edges.
	serifEdgeRatioThreshold = 1.40

	// monoColumnCVThreshold: coefficient of variation (std/mean) of per-column
	// ink sums below which the block is considered monospace-like.
	monoColumnCVThreshold = 0.50

	// monoCharWidthRatioMin / Max: expected range of (char width / box height)
	// for monospace fonts.  Values outside this range are treated as
	// proportional regardless of the column CV.
	monoCharWidthRatioMin = 0.44
	monoCharWidthRatioMax = 0.82

	// monoMinChars: minimum number of runes required before monospace
	// classification is attempted (short blocks are too ambiguous).
	monoMinChars = 3

	// styleSampleStride: pixel stride used for bold/serif sampling.
	// Set to 2 to halve the number of pixels examined while retaining
	// enough statistical signal for the heuristics.
	styleSampleStride = 2
)

// -- Styled font entries --
// Each entry follows the same lazy-load pattern as the fallback fonts in
// image_service.go.  They share the fallbackFontEntry type (same package).

var (
	// NotoSans Bold – for bold sans blocks
	styledNotoSansBold = &fallbackFontEntry{
		name: "NotoSans-Bold",
		paths: []string{
			// Alpine (apk font-noto)
			"/usr/share/fonts/noto/NotoSans-Bold.ttf",
			// Debian/Ubuntu truetype
			"/usr/share/fonts/truetype/noto/NotoSans-Bold.ttf",
			// Debian/Ubuntu opentype
			"/usr/share/fonts/opentype/noto/NotoSans-Bold.ttf",
			"/usr/share/fonts/google-noto/NotoSans-Bold.ttf",
		},
	}

	// NotoSerif Regular – for regular serif blocks
	styledNotoSerif = &fallbackFontEntry{
		name: "NotoSerif",
		paths: []string{
			"/usr/share/fonts/noto/NotoSerif-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSerif-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoSerif-Regular.ttf",
			"/usr/share/fonts/google-noto/NotoSerif-Regular.ttf",
			// Some distros ship the variant without "Regular" in the name
			"/usr/share/fonts/noto/NotoSerif.ttf",
			"/usr/share/fonts/truetype/noto/NotoSerif.ttf",
		},
	}

	// NotoSerif Bold – for bold serif blocks
	styledNotoSerifBold = &fallbackFontEntry{
		name: "NotoSerif-Bold",
		paths: []string{
			"/usr/share/fonts/noto/NotoSerif-Bold.ttf",
			"/usr/share/fonts/truetype/noto/NotoSerif-Bold.ttf",
			"/usr/share/fonts/opentype/noto/NotoSerif-Bold.ttf",
			"/usr/share/fonts/google-noto/NotoSerif-Bold.ttf",
		},
	}

	// NotoSansMono Regular – for monospace blocks
	styledNotoSansMono = &fallbackFontEntry{
		name: "NotoSansMono",
		paths: []string{
			// Alpine
			"/usr/share/fonts/noto/NotoSansMono-Regular.ttf",
			// Debian/Ubuntu
			"/usr/share/fonts/truetype/noto/NotoSansMono-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoSansMono-Regular.ttf",
			"/usr/share/fonts/google-noto/NotoSansMono-Regular.ttf",
			// Older packages used NotoMono
			"/usr/share/fonts/noto/NotoMono-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoMono-Regular.ttf",
		},
	}
)

// -- Pixel helpers --

// pixelLuminance returns the BT.601 luma [0, 255] for pixel (x, y).
func pixelLuminance(img *image.RGBA, x, y int) float64 {
	c := img.RGBAAt(x, y)
	return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
}

// isInkPixel reports whether the pixel at (x, y) is an ink pixel given
// whether ink is dark (true) or light (false).  The threshold is the
// mid-grey boundary (luma 128).
func isInkPixel(img *image.RGBA, x, y int, inkIsDark bool) bool {
	lum := pixelLuminance(img, x, y)
	if inkIsDark {
		return lum < 128
	}
	return lum >= 128
}

// -- Heuristic helpers --

// boldInkDensity samples the block at stride and returns the fraction of
// sampled pixels identified as ink.  Bold text has thicker strokes and
// therefore a higher ink density than regular-weight text.
func boldInkDensity(img *image.RGBA, box image.Rectangle, inkIsDark bool) float64 {
	box = box.Intersect(img.Bounds())
	if box.Empty() {
		return 0
	}
	ink, total := 0, 0
	for y := box.Min.Y; y < box.Max.Y; y += styleSampleStride {
		for x := box.Min.X; x < box.Max.X; x += styleSampleStride {
			if isInkPixel(img, x, y, inkIsDark) {
				ink++
			}
			total++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(ink) / float64(total)
}

// serifEdgeRatio computes the ratio of Sobel horizontal-edge responses to
// vertical-edge responses over the block interior.  A ratio significantly
// above 1.0 suggests prominent horizontal terminals (serifs).
// Interior pixels only are examined (border row/column skipped) to avoid
// artefacts from the box boundary.
func serifEdgeRatio(img *image.RGBA, box image.Rectangle) float64 {
	box = box.Intersect(img.Bounds())
	// Need at least 3×3 interior
	if box.Dx() < 3 || box.Dy() < 3 {
		return 1.0
	}

	var sumH, sumV float64
	for y := box.Min.Y + 1; y < box.Max.Y-1; y += styleSampleStride {
		for x := box.Min.X + 1; x < box.Max.X-1; x += styleSampleStride {
			// Sobel Gy (horizontal edges – sensitive to serif crossbars)
			gy := math.Abs(
				pixelLuminance(img, x-1, y-1)*-1 + pixelLuminance(img, x, y-1)*-2 + pixelLuminance(img, x+1, y-1)*-1 +
					pixelLuminance(img, x-1, y+1)*1 + pixelLuminance(img, x, y+1)*2 + pixelLuminance(img, x+1, y+1)*1,
			)
			// Sobel Gx (vertical edges – stroke boundaries)
			gx := math.Abs(
				pixelLuminance(img, x-1, y-1)*-1 + pixelLuminance(img, x-1, y)*-2 + pixelLuminance(img, x-1, y+1)*-1 +
					pixelLuminance(img, x+1, y-1)*1 + pixelLuminance(img, x+1, y)*2 + pixelLuminance(img, x+1, y+1)*1,
			)
			sumH += gy
			sumV += gx
		}
	}

	if sumH < 1.0 && sumV < 1.0 {
		// No meaningful edges (flat image) – treat as ambiguous (sans).
		return 1.0
	}
	// Smooth both sums by +1 to avoid division by zero while preserving the
	// signal when one component is near zero.  If sumH >> sumV the image has
	// dominant horizontal edges (serif-like crossbars); if sumV >= sumH it is
	// more likely sans (uniform stroke widths).
	return (sumH + 1.0) / (sumV + 1.0)
}

// columnInkCV computes the coefficient of variation (std/mean) of per-column
// ink pixel counts across the block width.  A low CV indicates that each
// column contains a similar amount of ink – the hallmark of monospace text.
func columnInkCV(img *image.RGBA, box image.Rectangle, inkIsDark bool) float64 {
	box = box.Intersect(img.Bounds())
	w := box.Dx()
	if w < 2 {
		return 1.0
	}

	sums := make([]float64, w)
	for x := 0; x < w; x++ {
		for y := box.Min.Y; y < box.Max.Y; y++ {
			if isInkPixel(img, box.Min.X+x, y, inkIsDark) {
				sums[x]++
			}
		}
	}

	var mean float64
	for _, s := range sums {
		mean += s
	}
	mean /= float64(w)
	if mean < 1.0 {
		// Essentially blank box – cannot classify
		return 1.0
	}

	var variance float64
	for _, s := range sums {
		d := s - mean
		variance += d * d
	}
	std := math.Sqrt(variance / float64(w))
	return std / mean
}

// isMonospaceBlock returns true when the block pixel data and original OCR
// text suggest monospace (fixed-width) type.  Two independent signals must
// both be consistent with monospace:
//  1. Estimated character width (box width / rune count) falls in the
//     monospace-typical range relative to box height.
//  2. Column ink CV is low, indicating uniform horizontal density.
func isMonospaceBlock(img *image.RGBA, box image.Rectangle, inkIsDark bool, originalText string) bool {
	runes := utf8.RuneCountInString(strings.TrimSpace(originalText))
	if runes < monoMinChars {
		return false
	}

	charWidth := float64(box.Dx()) / float64(runes)
	heightRatio := charWidth / float64(max(1, box.Dy()))
	if heightRatio < monoCharWidthRatioMin || heightRatio > monoCharWidthRatioMax {
		return false
	}

	cv := columnInkCV(img, box, inkIsDark)
	return cv < monoColumnCVThreshold
}

// -- Top-level detection --

// detectBlockFontStyle analyses the original pixels inside box (before any
// erase) and returns the inferred typographic style for the block.
// originalText is the source OCR text used for monospace width estimation.
func detectBlockFontStyle(img *image.RGBA, box image.Rectangle, originalText string) BlockFontStyle {
	box = box.Intersect(img.Bounds())
	if box.Empty() {
		return BlockFontStyle{}
	}

	// Use background luminance to decide which pixels are ink.
	inkIsDark := avgRegionLuminance(img, box) >= 0.5

	style := BlockFontStyle{}

	// Bold: stroke density above threshold.
	density := boldInkDensity(img, box, inkIsDark)
	style.Bold = density > inkDensityBoldThreshold

	// Monospace: checked first – a block unlikely to be a proportional font.
	if isMonospaceBlock(img, box, inkIsDark, originalText) {
		style.Class = monoFontClass
		return style
	}

	// Serif vs sans: based on edge direction ratio.
	ratio := serifEdgeRatio(img, box)
	if ratio > serifEdgeRatioThreshold {
		style.Class = serifFontClass
	} else {
		style.Class = sansFontClass
	}

	return style
}

// -- Font face selection for styled blocks --

// faceForStyle returns the best available font.Face for the given text and
// detected block style at fontSize.
//
// For non-Latin text (CJK, Arabic, etc.) the existing fallback chain is used
// unchanged – those fonts do not have serif/mono variants in the system
// fallback list and the visual difference is minimal for those scripts.
//
// For Latin/common scripts the styled Noto variants are tried in order:
//   - mono   → NotoSansMono → goregular fallback
//   - serif  → NotoSerif-(Bold) → goregular fallback
//   - sans   → NotoSans-Bold (if bold) → goregular
func faceForStyle(text string, fontSize float64, style BlockFontStyle) (font.Face, error) {
	// Non-Latin: use the existing script-coverage fallback chain.
	if needsFallbackFont(text) {
		return faceForFallback(text, fontSize)
	}

	switch style.Class {
	case monoFontClass:
		if f, err := styledNotoSansMono.face(fontSize); err == nil {
			return f, nil
		}
		// Fall through to goregular.

	case serifFontClass:
		entry := styledNotoSerif
		if style.Bold {
			entry = styledNotoSerifBold
		}
		if f, err := entry.face(fontSize); err == nil {
			return f, nil
		}
		// Fall through to goregular.

	default: // sansFontClass
		if style.Bold {
			if f, err := styledNotoSansBold.face(fontSize); err == nil {
				return f, nil
			}
		}
		// Regular sans → goregular (current default).
	}

	return loadOverlayFace(fontSize)
}

// segFaceStyled is like segFace but uses the detected BlockFontStyle to
// select the appropriate font variant for Latin script segments.
func segFaceStyled(seg scriptSegment, fontSize float64, style BlockFontStyle) (font.Face, error) {
	if seg.useSystem {
		// Non-Latin segment: use the coverage-based fallback chain as usual.
		return faceForFallback(seg.text, fontSize)
	}
	return faceForStyle(seg.text, fontSize, style)
}
