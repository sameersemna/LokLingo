package overlay

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
	"strings"
)

func DrawTextOnImage(imagePath string, blocks []ImageTextBlock, translatedTexts []string) (string, error) {
	outPath, _, err := DrawTextOnImageWithOptions(imagePath, blocks, translatedTexts, DefaultOverlayOptions())
	return outPath, err
}

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
		candidates = append(candidates, overlayCandidate{box: image.Rect(x1, y1, x2, y2), text: translated, originalText: blk.Text, regionClass: blk.RegionClass})
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
		}

		blockMaxFontSize := opts.MaxFontSize
		if opts.StudioMode {
			switch candidate.regionClass {
			case "title":
				blockMaxFontSize += opts.TitleFontBoost
			case "heading":
				blockMaxFontSize += opts.HeadingFontBoost
			}
		}

		layout := fitTextLayout(
			candidate.text,
			box.Dx()-(opts.TextPadding*2),
			box.Dy()-(opts.TextPadding*2),
			opts.MinFontSize,
			blockMaxFontSize,
		)
		if len(layout.lines) == 0 {
			skipped++
			continue
		}

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
			blockStyle = detectBlockFontStyle(rgba, box, candidate.originalText)
			blockStyle.Shadow = detectShadowOffset(rgba, box)
		}
		if opts.StudioMode {
			if candidate.regionClass == "title" {
				blockStyle.Bold = true
			}
			if opts.CodeFontMono && candidate.regionClass == "code" {
				blockStyle.Class = monoFontClass
			}
		}
		inkColor := color.Color(inkColorForBackground(avgRegionLuminance(rgba, box)))
		if !opts.DisableColorSampling {
			inkColor = sampleDominantTextColor(rgba, box)
		}

		if opts.EraseBBox {
			patchFillBBoxWithOptions(rgba, box, opts)
		}

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
	quality := opts.JPEGQuality
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
