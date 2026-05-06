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
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// ImageTextBlock represents one OCR block with an axis-aligned bbox [x1, y1, x2, y2].
type ImageTextBlock struct {
	Text string    `json:"text"`
	Bbox []float64 `json:"bbox"`
}

// DrawTextOnImage loads an image, draws translated text inside OCR block regions,
// and saves a new image next to the source file.
//
// Output path format: <original>_translated<ext>
func DrawTextOnImage(imagePath string, blocks []ImageTextBlock, translatedTexts []string) (string, error) {
	if strings.TrimSpace(imagePath) == "" {
		return "", fmt.Errorf("image path is required")
	}
	if len(blocks) == 0 {
		return "", fmt.Errorf("at least one block is required")
	}
	if len(translatedTexts) == 0 {
		return "", fmt.Errorf("at least one translated text is required")
	}

	in, err := os.Open(imagePath)
	if err != nil {
		return "", fmt.Errorf("open image: %w", err)
	}
	defer in.Close()

	srcImg, format, err := image.Decode(in)
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}

	bounds := srcImg.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, srcImg, bounds.Min, draw.Src)

	count := min(len(blocks), len(translatedTexts))
	for i := 0; i < count; i++ {
		blk := blocks[i]
		translated := strings.TrimSpace(translatedTexts[i])
		if translated == "" {
			continue
		}
		if len(blk.Bbox) != 4 {
			continue
		}

		x1 := clampToBounds(int(math.Round(blk.Bbox[0])), bounds.Min.X, bounds.Max.X)
		y1 := clampToBounds(int(math.Round(blk.Bbox[1])), bounds.Min.Y, bounds.Max.Y)
		x2 := clampToBounds(int(math.Round(blk.Bbox[2])), bounds.Min.X, bounds.Max.X)
		y2 := clampToBounds(int(math.Round(blk.Bbox[3])), bounds.Min.Y, bounds.Max.Y)

		if x2 <= x1 || y2 <= y1 {
			continue
		}

		// Paint a light background in the OCR region to improve text readability.
		draw.Draw(rgba, image.Rect(x1, y1, x2, y2), &image.Uniform{C: color.RGBA{R: 255, G: 255, B: 255, A: 220}}, image.Point{}, draw.Over)

		face := basicfont.Face7x13
		d := &font.Drawer{
			Dst:  rgba,
			Src:  image.NewUniform(color.Black),
			Face: face,
			Dot:  fixed.P(x1+2, y1+face.Ascent+2),
		}
		d.DrawString(translated)
	}

	ext := strings.ToLower(filepath.Ext(imagePath))
	base := strings.TrimSuffix(imagePath, filepath.Ext(imagePath))
	outPath := base + "_translated" + ext

	out, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create output image: %w", err)
	}
	defer out.Close()

	switch format {
	case "jpeg", "jpg":
		if err := jpeg.Encode(out, rgba, &jpeg.Options{Quality: 90}); err != nil {
			return "", fmt.Errorf("encode jpeg: %w", err)
		}
	default:
		if err := png.Encode(out, rgba); err != nil {
			return "", fmt.Errorf("encode png: %w", err)
		}
	}

	return outPath, nil
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
