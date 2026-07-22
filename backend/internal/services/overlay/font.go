package overlay

import (
	"fmt"
	"image"
	"log/slog"
	"math"
	"os"
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
	overlayDetectedShadowMinDarkness = 0.08
	overlayDetectedShadowMaxAlpha = uint8(180)
	overlayPatchFeatherPx         = 2
	overlayPatchBlurRadius        = 1
	overlayFitOverflowPct = 0.03
	overlayVerticalAspectThreshold = 2.2
	overlayVerticalCJKMinRatio = 0.8
	overlayVerticalMinFontBoost = 4
	overlayVerticalNarrowWidthThreshold = 24
	overlayVerticalNarrowFontBoost      = 2
	overlayVerticalMinPadding = 2
	overlayVerticalExtraStrokePx = 1
	overlayTextContrastThreshold = 0.20
)

var overlayFontData = parseOverlayFont()

var (
	embeddedGoBoldFont      = mustParseEmbeddedFont(gobold.TTF, "go bold")
	embeddedGoBoldFaceCache sync.Map
	embeddedGoMonoFont      = mustParseEmbeddedFont(gomono.TTF, "go mono")
	embeddedGoMonoFaceCache sync.Map
)

var overlayFaceCache sync.Map

var overlayAscentCache sync.Map
var fallbackAscentCache sync.Map

type fallbackFontEntry struct {
	name      string
	paths     []string
	embedData []byte
	once      sync.Once
	data      *opentype.Font
	faceCache sync.Map
}

var fallbackFonts = []*fallbackFontEntry{
	{
		name: "NotoSans",
		paths: []string{
			"/usr/share/fonts/noto/NotoSans-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoSans-Regular.ttf",
			"/usr/share/fonts/google-noto/NotoSans-Regular.ttf",
		},
	},
	{
		name: "NotoSansCJK",
		paths: []string{
			"/usr/share/fonts/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/noto/NotoSansCJKsc-Regular.otf",
			"/usr/share/fonts/noto/NotoSansCJKtc-Regular.otf",
			"/usr/share/fonts/noto/NotoSansCJKjp-Regular.otf",
			"/usr/share/fonts/noto/NotoSansCJKkr-Regular.otf",
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
			"/usr/share/fonts/noto/NotoSansArabic-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSansArabic-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoSansArabic-Regular.ttf",
			"/usr/share/fonts/google-noto/NotoSansArabic-Regular.ttf",
		},
		embedData: embeddedNotoSansArabic,
	},
	{
		name: "NotoNaskhArabic",
		paths: []string{
			"/usr/share/fonts/noto/NotoNaskhArabic-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoNaskhArabic-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoNaskhArabic-Regular.ttf",
			"/usr/share/fonts/google-noto/NotoNaskhArabic-Regular.ttf",
		},
		embedData: embeddedNotoNaskhArabic,
	},
	{
		name: "NotoSansDevanagari",
		paths: []string{
			"/usr/share/fonts/noto/NotoSansDevanagari-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSansDevanagari-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoSansDevanagari-Regular.ttf",
			"/usr/share/fonts/google-noto/NotoSansDevanagari-Regular.ttf",
		},
		embedData: embeddedNotoSansDevanagari,
	},
	{
		name: "NotoSansBengali",
		paths: []string{
			"/usr/share/fonts/noto/NotoSansBengali-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSansBengali-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoSansBengali-Regular.ttf",
			"/usr/share/fonts/google-noto/NotoSansBengali-Regular.ttf",
		},
		embedData: embeddedNotoSansBengali,
	},
	{
		name: "DejaVuSans",
		paths: []string{
			"/usr/share/fonts/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/System/Library/Fonts/Supplemental/Arial Unicode MS.ttf",
			`C:\Windows\Fonts\arialuni.ttf`,
		},
	},
}

// fontClass is the detected typographic category for an OCR block.
type fontClass int

const (
	sansFontClass  fontClass = iota
	serifFontClass
	monoFontClass
)

// shadowHint holds the detected drop-shadow geometry for a rendered text block.
type shadowHint struct {
	dx, dy int
	alpha  uint8
}

// BlockFontStyle holds the inferred typographic style for one OCR block.
type BlockFontStyle struct {
	Bold   bool
	Class  fontClass
	Shadow shadowHint
}

const (
	inkDensityBoldThreshold = 0.15
	serifEdgeRatioThreshold = 1.40
	monoColumnCVThreshold   = 0.50
	monoCharWidthRatioMin   = 0.44
	monoCharWidthRatioMax   = 0.82
	monoMinChars            = 3
	styleSampleStride       = 2
)

var (
	styledNotoSans = &fallbackFontEntry{
		name: "NotoSans",
		paths: []string{
			"/usr/share/fonts/noto/NotoSans-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoSans-Regular.ttf",
			"/usr/share/fonts/google-noto/NotoSans-Regular.ttf",
			"/usr/share/fonts/noto/NotoSans.ttf",
			"/usr/share/fonts/truetype/noto/NotoSans.ttf",
		},
	}

	styledNotoSansBold = &fallbackFontEntry{
		name: "NotoSans-Bold",
		paths: []string{
			"/usr/share/fonts/noto/NotoSans-Bold.ttf",
			"/usr/share/fonts/truetype/noto/NotoSans-Bold.ttf",
			"/usr/share/fonts/opentype/noto/NotoSans-Bold.ttf",
			"/usr/share/fonts/google-noto/NotoSans-Bold.ttf",
		},
	}

	styledNotoSerif = &fallbackFontEntry{
		name: "NotoSerif",
		paths: []string{
			"/usr/share/fonts/noto/NotoSerif-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSerif-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoSerif-Regular.ttf",
			"/usr/share/fonts/google-noto/NotoSerif-Regular.ttf",
			"/usr/share/fonts/noto/NotoSerif.ttf",
			"/usr/share/fonts/truetype/noto/NotoSerif.ttf",
		},
	}

	styledNotoSerifBold = &fallbackFontEntry{
		name: "NotoSerif-Bold",
		paths: []string{
			"/usr/share/fonts/noto/NotoSerif-Bold.ttf",
			"/usr/share/fonts/truetype/noto/NotoSerif-Bold.ttf",
			"/usr/share/fonts/opentype/noto/NotoSerif-Bold.ttf",
			"/usr/share/fonts/google-noto/NotoSerif-Bold.ttf",
		},
	}

	styledNotoSansMono = &fallbackFontEntry{
		name: "NotoSansMono",
		paths: []string{
			"/usr/share/fonts/noto/NotoSansMono-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSansMono-Regular.ttf",
			"/usr/share/fonts/opentype/noto/NotoSansMono-Regular.ttf",
			"/usr/share/fonts/google-noto/NotoSansMono-Regular.ttf",
			"/usr/share/fonts/noto/NotoMono-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoMono-Regular.ttf",
		},
	}
)

func pixelLuminance(img *image.RGBA, x, y int) float64 {
	c := img.RGBAAt(x, y)
	return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
}

func isInkPixel(img *image.RGBA, x, y int, inkIsDark bool) bool {
	lum := pixelLuminance(img, x, y)
	if inkIsDark {
		return lum < 128
	}
	return lum >= 128
}

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

func serifEdgeRatio(img *image.RGBA, box image.Rectangle) float64 {
	box = box.Intersect(img.Bounds())
	if box.Dx() < 3 || box.Dy() < 3 {
		return 1.0
	}

	var sumH, sumV float64
	for y := box.Min.Y + 1; y < box.Max.Y-1; y += styleSampleStride {
		for x := box.Min.X + 1; x < box.Max.X-1; x += styleSampleStride {
			gy := math.Abs(
				pixelLuminance(img, x-1, y-1)*-1 + pixelLuminance(img, x, y-1)*-2 + pixelLuminance(img, x+1, y-1)*-1 +
					pixelLuminance(img, x-1, y+1)*1 + pixelLuminance(img, x, y+1)*2 + pixelLuminance(img, x+1, y+1)*1,
			)
			gx := math.Abs(
				pixelLuminance(img, x-1, y-1)*-1 + pixelLuminance(img, x-1, y)*-2 + pixelLuminance(img, x-1, y+1)*-1 +
					pixelLuminance(img, x+1, y-1)*1 + pixelLuminance(img, x+1, y)*2 + pixelLuminance(img, x+1, y+1)*1,
			)
			sumH += gy
			sumV += gx
		}
	}

	if sumH < 1.0 && sumV < 1.0 {
		return 1.0
	}
	return (sumH + 1.0) / (sumV + 1.0)
}

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

func isMonospaceBlock(img *image.RGBA, box image.Rectangle, inkIsDark bool, originalText string) bool {
	runes := utf8RuneCount(strings.TrimSpace(originalText))
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

func detectBlockFontStyle(img *image.RGBA, box image.Rectangle, originalText string) BlockFontStyle {
	box = box.Intersect(img.Bounds())
	if box.Empty() {
		return BlockFontStyle{}
	}

	inkIsDark := avgRegionLuminance(img, box) >= 0.5

	style := BlockFontStyle{}

	density := boldInkDensity(img, box, inkIsDark)
	style.Bold = density > inkDensityBoldThreshold

	if isMonospaceBlock(img, box, inkIsDark, originalText) {
		style.Class = monoFontClass
		return style
	}

	ratio := serifEdgeRatio(img, box)
	if ratio > serifEdgeRatioThreshold {
		style.Class = serifFontClass
	} else {
		style.Class = sansFontClass
	}

	return style
}

func preferredLatinNotoEntry(style BlockFontStyle) *fallbackFontEntry {
	switch style.Class {
	case monoFontClass:
		return styledNotoSansMono
	case serifFontClass:
		if style.Bold {
			return styledNotoSerifBold
		}
		return styledNotoSerif
	default:
		if style.Bold {
			return styledNotoSansBold
		}
		return styledNotoSans
	}
}

func faceForStyle(text string, fontSize float64, style BlockFontStyle) (font.Face, error) {
	if needsFallbackFont(text) {
		return faceForFallback(text, fontSize)
	}

	if entry := preferredLatinNotoEntry(style); entry != nil {
		if f, err := entry.face(fontSize); err == nil {
			return f, nil
		}
	}

	switch style.Class {
	case monoFontClass:
		return loadFaceFromFont(embeddedGoMonoFont, &embeddedGoMonoFaceCache, fontSize)

	case serifFontClass:
		if style.Bold {
			return loadFaceFromFont(embeddedGoBoldFont, &embeddedGoBoldFaceCache, fontSize)
		}
		return loadOverlayFace(fontSize)

	default:
		if style.Bold {
			return loadFaceFromFont(embeddedGoBoldFont, &embeddedGoBoldFaceCache, fontSize)
		}
		return loadOverlayFace(fontSize)
	}
}

func segFaceStyled(seg scriptSegment, fontSize float64, style BlockFontStyle) (font.Face, error) {
	if seg.useSystem {
		return faceForFallback(seg.text, fontSize)
	}
	return faceForStyle(seg.text, fontSize, style)
}

func parseOverlayFont() *opentype.Font {
	parsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		slog.Error("parse overlay font failed", "err", err)
		return nil
	}
	return parsed
}

func mustParseEmbeddedFont(data []byte, name string) *opentype.Font {
	parsed, err := opentype.Parse(data)
	if err != nil {
		slog.Error("parse embedded font failed", "name", name, "err", err)
		return nil
	}
	return parsed
}

func parseFont(data []byte) (*opentype.Font, error) {
	f, err := opentype.Parse(data)
	if err == nil {
		return f, nil
	}
	c, err2 := opentype.ParseCollection(data)
	if err2 != nil {
		return nil, err
	}
	return c.Font(0)
}

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
			return
		}
		if len(e.embedData) > 0 {
			if parsed, err := parseFont(e.embedData); err == nil {
				e.data = parsed
			}
		}
	})
	return e.data
}

func (e *fallbackFontEntry) face(fontSize float64) (font.Face, error) {
	f := e.load()
	if f == nil {
		return nil, fmt.Errorf("fallback font %q not available on this system", e.name)
	}
	return loadFaceFromFont(f, &e.faceCache, fontSize)
}

func fontCoversText(f *opentype.Font, text string) bool {
	if f == nil {
		return false
	}
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

func faceForFallback(text string, fontSize float64) (font.Face, error) {
	for _, entry := range fallbackFonts {
		f := entry.load()
		if f == nil {
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
	return loadOverlayFace(fontSize)
}

func fontCoverageWarning(text string) string {
	if !needsFallbackFont(text) {
		return ""
	}
	var loaded []*opentype.Font
	for _, entry := range fallbackFonts {
		if f := entry.load(); f != nil {
			loaded = append(loaded, f)
		}
	}
	var missingScripts []string
	seenScript := make(map[string]bool)
	var buf sfnt.Buffer
	for _, r := range text {
		if unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		if !isFallbackRune(r) {
			continue
		}
		covered := false
		for _, f := range loaded {
			if idx, err := f.GlyphIndex(&buf, r); err == nil && idx != 0 {
				covered = true
				break
			}
		}
		if !covered {
			scripts := detectMissingScripts(string(r))
			for _, s := range scripts {
				if !seenScript[s] {
					seenScript[s] = true
					missingScripts = append(missingScripts, s)
				}
			}
		}
	}
	if len(missingScripts) == 0 {
		return ""
	}
	snippet := text
	if len([]rune(snippet)) > 20 {
		snippet = string([]rune(snippet)[:20]) + "…"
	}
	return fmt.Sprintf(
		"no system font covers %q (scripts: %s); install Noto fonts to fix tofu boxes",
		snippet, strings.Join(missingScripts, ", "),
	)
}

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

func AvailableFallbackFonts() []string {
	var available []string
	for _, entry := range fallbackFonts {
		if f := entry.load(); f != nil {
			available = append(available, entry.name)
		}
	}
	return available
}

func isCJKRune(r rune) bool {
	return (r >= 0x1100 && r <= 0x11FF) ||
		(r >= 0x2E80 && r <= 0x9FFF) ||
		(r >= 0xAC00 && r <= 0xD7AF) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0x20000 && r <= 0x2A6DF)
}

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

func isCJKWord(s string) bool { return isAllCJK(s) }

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

func isFallbackRune(r rune) bool {
	return (r >= 0x0600 && r <= 0x06FF) ||
		(r >= 0x0750 && r <= 0x077F) ||
		(r >= 0x08A0 && r <= 0x08FF) ||
		(r >= 0x0590 && r <= 0x05FF) ||
		(r >= 0x0900 && r <= 0x097F) ||
		(r >= 0x0980 && r <= 0x09FF) ||
		(r >= 0x0A00 && r <= 0x0A7F) ||
		(r >= 0x0A80 && r <= 0x0AFF) ||
		(r >= 0x0B00 && r <= 0x0B7F) ||
		(r >= 0x0B80 && r <= 0x0BFF) ||
		(r >= 0x0C00 && r <= 0x0C7F) ||
		(r >= 0x0C80 && r <= 0x0CFF) ||
		(r >= 0x0D00 && r <= 0x0D7F) ||
		(r >= 0x0E00 && r <= 0x0E7F) ||
		(r >= 0x1100 && r <= 0x11FF) ||
		(r >= 0x2E80 && r <= 0x2EFF) ||
		(r >= 0x3000 && r <= 0x9FFF) ||
		(r >= 0xA000 && r <= 0xA4CF) ||
		(r >= 0xAC00 && r <= 0xD7AF) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0x20000 && r <= 0x2A6DF)
}

func needsFallbackFont(text string) bool {
	for _, r := range text {
		if isFallbackRune(r) {
			return true
		}
	}
	return false
}

func faceForText(text string, fontSize float64) (font.Face, error) {
	if needsFallbackFont(text) {
		return faceForFallback(text, fontSize)
	}
	return loadOverlayFace(fontSize)
}

type scriptSegment struct {
	text      string
	useSystem bool
}

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

func segFace(seg scriptSegment, fontSize float64) (font.Face, error) {
	if seg.useSystem {
		return faceForFallback(seg.text, fontSize)
	}
	return loadOverlayFace(fontSize)
}

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
	if spacingPx > -0.25 && spacingPx < 0.25 {
		return 0
	}
	return fixed.Int26_6(math.Round(spacingPx * 64))
}

type strokeOffset struct{ dx, dy int }

func computeStrokeOffsets(style BlockFontStyle, fontSize float64) []strokeOffset {
	if style.Class == monoFontClass {
		return nil
	}
	if style.Bold {
		if fontSize > 32 {
			return nil
		}
		return []strokeOffset{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}}
	}
	if style.Class == serifFontClass {
		return nil
	}
	if fontSize <= 16 {
		return []strokeOffset{{1, 0}}
	}
	return nil
}

func isRTLRune(r rune) bool {
	return (r >= 0x0590 && r <= 0x05FF) ||
		(r >= 0x0600 && r <= 0x06FF) ||
		(r >= 0x0750 && r <= 0x077F) ||
		(r >= 0x08A0 && r <= 0x08FF) ||
		(r >= 0xFB1D && r <= 0xFB4F) ||
		(r >= 0xFB50 && r <= 0xFDFF) ||
		(r >= 0xFE70 && r <= 0xFEFF)
}

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

func loadOverlayFace(fontSize float64) (font.Face, error) {
	return loadFaceFromFont(overlayFontData, &overlayFaceCache, fontSize)
}

func loadFaceFromFont(fontData *opentype.Font, cache *sync.Map, fontSize float64) (font.Face, error) {
	if fontData == nil {
		return nil, fmt.Errorf("font data unavailable")
	}
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
	if actual, loaded := cache.LoadOrStore(key, face); loaded {
		return actual.(font.Face), nil
	}
	return face, nil
}

func cachedAscent(face font.Face, cache *sync.Map, fontSize float64) int {
	key := int(fontSize)
	if v, ok := cache.Load(key); ok {
		return v.(int)
	}
	a := face.Metrics().Ascent.Ceil()
	cache.Store(key, a)
	return a
}

func utf8RuneCount(s string) int {
	count := 0
	for range s {
		count++
	}
	return count
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
