package overlay

import (
	"image"
	"sort"
)

type overlayCandidate struct {
	box          image.Rectangle
	text         string
	originalText string
	regionClass  string
}

type textLayout struct {
	fontSize   float64
	lineHeight int
	topInset   int
	lines      []string
}

// ImageTextBlock represents one OCR block with an axis-aligned bbox [x1, y1, x2, y2].
type ImageTextBlock struct {
	Text        string    `json:"text"`
	Bbox        []float64 `json:"bbox"`
	RegionClass string    `json:"region_class,omitempty"`
}

// OverlayStats holds counters from a single DrawTextOnImageWithOptions call.
type OverlayStats struct {
	BlocksDrawn   int
	BlocksSkipped int
	FontWarnings  []string
}

type OverlayOptions struct {
	BgAlpha                   uint8
	MaxFontSize               int
	MinFontSize               int
	JPEGQuality               int
	TextPadding               int
	BboxShrinkPx              int
	PatchFeatherPx            int
	PatchBlurRadius           int
	EraseBBox                 bool
	DisableFontStyleDetection bool
	DisableColorSampling      bool
	StudioMode                bool
	TextureAwareFill          bool
	AntiHaloRadius            int
	TitleFontBoost            int
	HeadingFontBoost          int
	CodeFontMono              bool
	RegionClass               string
}

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

func DefaultStudioOptions() OverlayOptions {
	return OverlayOptions{
		BgAlpha:                   overlayBgAlpha,
		MaxFontSize:               overlayMaxFontSize + 8,
		MinFontSize:               overlayMinFontSize,
		JPEGQuality:               95,
		TextPadding:               overlayTextPadding + 2,
		BboxShrinkPx:              1,
		PatchFeatherPx:            4,
		PatchBlurRadius:           2,
		EraseBBox:                 true,
		DisableFontStyleDetection: false,
		DisableColorSampling:      false,
		StudioMode:                true,
		TextureAwareFill:          true,
		AntiHaloRadius:            2,
		TitleFontBoost:            6,
		HeadingFontBoost:          3,
		CodeFontMono:              true,
	}
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
