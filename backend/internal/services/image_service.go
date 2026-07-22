package services

import (
	"loklingo/backend/internal/services/overlay"
)

type ImageTextBlock = overlay.ImageTextBlock
type OverlayOptions = overlay.OverlayOptions
type OverlayStats = overlay.OverlayStats

func DefaultOverlayOptions() OverlayOptions {
	return overlay.DefaultOverlayOptions()
}

func DefaultStudioOptions() OverlayOptions {
	return overlay.DefaultStudioOptions()
}

func AvailableFallbackFonts() []string {
	return overlay.AvailableFallbackFonts()
}

func DrawTextOnImage(imagePath string, blocks []ImageTextBlock, translatedTexts []string) (string, error) {
	return overlay.DrawTextOnImage(imagePath, blocks, translatedTexts)
}

func DrawTextOnImageWithOptions(imagePath string, blocks []ImageTextBlock, translatedTexts []string, opts OverlayOptions) (string, OverlayStats, error) {
	return overlay.DrawTextOnImageWithOptions(imagePath, blocks, translatedTexts, opts)
}
