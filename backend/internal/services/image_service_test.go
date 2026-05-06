package services

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

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
