package jobs

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	internalservices "loklingo/backend/internal/services"
	"loklingo/backend/services"
)

type imageEchoTranslSvc struct{}

func (s *imageEchoTranslSvc) Translate(in services.TranslationInput) (string, error) {
	return "DE:" + in.Text, nil
}

func TestWorker_ImageJob_RendersOutputImage(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "input.png")

	img := image.NewRGBA(image.Rect(0, 0, 120, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 120; x++ {
			img.Set(x, y, color.RGBA{R: 230, G: 230, B: 230, A: 255})
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

	store := &mockWorkerStore{}
	trans := &mockTranslSvc{result: "Hallo"}
	ocr := &mockOCRSvc{blocks: []internalservices.OCRTextBlock{{Text: "Hello", Bbox: []float64{10, 10, 90, 40}}}}
	w := NewWorker(store, trans, &mockPDFSvc{}, ocr, 0)

	job := &Job{
		ID:       "img-1",
		Type:     TypeImage,
		Mode:     ModeOverlay,
		FilePath: inPath,
		Source:   "en",
		Target:   "de",
	}

	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed, got %s, err=%s", store.lastJob.Status, store.lastJob.ErrorMsg)
	}
	if store.lastJob.OutputFilePath == "" {
		t.Fatal("expected output_file_path to be set")
	}
	if _, err := os.Stat(store.lastJob.OutputFilePath); err != nil {
		t.Fatalf("expected rendered output image to exist: %v", err)
	}
}

func TestWorker_ImageJob_FailsWithoutOCRClient(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "input.png")
	f, err := os.Create(inPath)
	if err != nil {
		t.Fatalf("create input image: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatalf("encode input image: %v", err)
	}
	_ = f.Close()

	store := &mockWorkerStore{}
	w := NewWorker(store, &mockTranslSvc{}, &mockPDFSvc{}, nil, 0)
	job := &Job{ID: "img-2", Type: TypeImage, Mode: ModeOverlay, FilePath: inPath, Source: "en", Target: "de"}

	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusFailed {
		t.Fatalf("expected status=failed, got %s", store.lastJob.Status)
	}
}

func TestWorker_ImageJob_OverlayMapsTranslatedSegmentsToBlocks(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "input.png")

	img := image.NewRGBA(image.Rect(0, 0, 120, 80))
	f, err := os.Create(inPath)
	if err != nil {
		t.Fatalf("create input image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatalf("encode input image: %v", err)
	}
	_ = f.Close()

	store := &mockWorkerStore{}
	ocr := &mockOCRSvc{blocks: []internalservices.OCRTextBlock{
		{Text: "Hello", Bbox: []float64{10, 10, 50, 30}},
		{Text: " ", Bbox: []float64{55, 10, 90, 30}},
		{Text: "World", Bbox: []float64{10, 35, 50, 55}},
	}}
	w := NewWorker(store, &imageEchoTranslSvc{}, &mockPDFSvc{}, ocr, 0, WithPDFChunkWordRange(1, 1))

	job := &Job{
		ID:       "img-3",
		Type:     TypeImage,
		Mode:     ModeOverlay,
		FilePath: inPath,
		Source:   "en",
		Target:   "de",
	}

	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed, got %s, err=%s", store.lastJob.Status, store.lastJob.ErrorMsg)
	}
	if store.lastJob.Text != "Hello\nWorld" {
		t.Fatalf("expected source text for translated segments, got %q", store.lastJob.Text)
	}
	if store.lastJob.TranslatedText != "DE:Hello\nDE:World" {
		t.Fatalf("expected translated text mapped to OCR segments, got %q", store.lastJob.TranslatedText)
	}
	if store.lastJob.OutputFilePath == "" {
		t.Fatal("expected output_file_path to be set")
	}
}

func TestWorker_ImageJob_LayoutMode_RendersOutputImage(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "input.png")

	img := image.NewRGBA(image.Rect(0, 0, 120, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 120; x++ {
			img.Set(x, y, color.RGBA{R: 210, G: 220, B: 230, A: 255})
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

	store := &mockWorkerStore{}
	trans := &mockTranslSvc{result: "Hallo"}
	ocr := &mockOCRSvc{blocks: []internalservices.OCRTextBlock{{Text: "Hello", Bbox: []float64{10, 10, 90, 40}}}}
	w := NewWorker(store, trans, &mockPDFSvc{}, ocr, 0)

	job := &Job{
		ID:       "img-layout-1",
		Type:     TypeImage,
		Mode:     ModeLayout,
		FilePath: inPath,
		Source:   "en",
		Target:   "de",
	}

	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed, got %s, err=%s", store.lastJob.Status, store.lastJob.ErrorMsg)
	}
	if store.lastJob.OutputFilePath == "" {
		t.Fatal("expected output_file_path to be set")
	}
	if _, err := os.Stat(store.lastJob.OutputFilePath); err != nil {
		t.Fatalf("expected rendered output image to exist: %v", err)
	}
}

func TestWorker_ImageJob_OCROnly_CompletesWithoutRendering(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "input.png")

	img := image.NewRGBA(image.Rect(0, 0, 100, 40))
	f, err := os.Create(inPath)
	if err != nil {
		t.Fatalf("create input image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatalf("encode input image: %v", err)
	}
	_ = f.Close()

	store := &mockWorkerStore{}
	ocr := &mockOCRSvc{blocks: []internalservices.OCRTextBlock{{Text: "Hello", Bbox: []float64{10, 10, 60, 30}}}}
	w := NewWorker(store, &mockTranslSvc{result: "IGNORED"}, &mockPDFSvc{}, ocr, 0)

	job := &Job{
		ID:       "img-ocr-only-1",
		Type:     TypeImage,
		Mode:     ModeOCROnly,
		FilePath: inPath,
		Source:   "en",
		Target:   "de",
	}

	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed, got %s, err=%s", store.lastJob.Status, store.lastJob.ErrorMsg)
	}
	if store.lastJob.TranslatedText != "Hello" {
		t.Fatalf("expected OCR text in translated_text, got %q", store.lastJob.TranslatedText)
	}
	if store.lastJob.OutputFilePath != "" {
		t.Fatalf("expected no rendered output path for OCR-only mode, got %q", store.lastJob.OutputFilePath)
	}
}

func TestWorker_ImageJob_NoOCRText_CompletesWithPassthroughOutput(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "input.png")

	img := image.NewRGBA(image.Rect(0, 0, 100, 40))
	f, err := os.Create(inPath)
	if err != nil {
		t.Fatalf("create input image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatalf("encode input image: %v", err)
	}
	_ = f.Close()

	store := &mockWorkerStore{}
	ocr := &mockOCRSvc{blocks: []internalservices.OCRTextBlock{{Text: "   ", Bbox: []float64{10, 10, 60, 30}}}}
	w := NewWorker(store, &mockTranslSvc{result: "IGNORED"}, &mockPDFSvc{}, ocr, 0)

	job := &Job{
		ID:       "img-no-text-1",
		Type:     TypeImage,
		Mode:     ModeOverlay,
		FilePath: inPath,
		Source:   "auto",
		Target:   "en",
	}

	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected status=completed, got %s, err=%s", store.lastJob.Status, store.lastJob.ErrorMsg)
	}
	if store.lastJob.OutputFilePath == "" {
		t.Fatal("expected passthrough output_file_path to be set")
	}
	if _, err := os.Stat(store.lastJob.OutputFilePath); err != nil {
		t.Fatalf("expected passthrough output image to exist: %v", err)
	}
	if store.lastJob.ErrorMsg != "" {
		t.Fatalf("expected empty error message, got %q", store.lastJob.ErrorMsg)
	}
}

func TestWorkerImageRetryKeepsSourceFile(t *testing.T) {
	ctx := context.Background()
	store := &mockWorkerStore{}
	ocr := &mockOCRSvc{err: context.DeadlineExceeded}
	w := NewWorker(store, &mockTranslSvc{}, &mockPDFSvc{}, ocr, 0)

	dir := t.TempDir()
	inPath := filepath.Join(dir, "retry-source.png")
	if err := os.WriteFile(inPath, []byte("PNGDATA"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	job := &Job{
		ID:       "img-retry-keep-file",
		Type:     TypeImage,
		Mode:     ModeOCROnly,
		FilePath: inPath,
		Source:   "auto",
		Target:   "en",
	}

	w.process(ctx, job)

	if _, statErr := os.Stat(inPath); statErr != nil {
		t.Fatalf("expected source file to remain for retry path, stat err: %v", statErr)
	}
	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusFailed {
		t.Fatalf("expected intermediate failed status before retry handling, got %q", store.lastJob.Status)
	}
}

func TestWorker_ImageJob_GracefulChunkDegradation_Completes(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "input.png")

	img := image.NewRGBA(image.Rect(0, 0, 120, 80))
	f, err := os.Create(inPath)
	if err != nil {
		t.Fatalf("create input image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatalf("encode input image: %v", err)
	}
	_ = f.Close()

	store := &mockWorkerStore{}
	ocr := &mockOCRSvc{blocks: []internalservices.OCRTextBlock{{Text: "Hello", Bbox: []float64{10, 10, 80, 40}}}}
	// Non-cancelled errors are treated as recoverable for chunk-level degradation.
	w := NewWorker(store, &mockTranslSvc{err: errors.New("provider timeout")}, &mockPDFSvc{}, ocr, 0)

	job := &Job{
		ID:       "img-degrade-1",
		Type:     TypeImage,
		Mode:     ModeOverlay,
		FilePath: inPath,
		Source:   "en",
		Target:   "de",
	}

	w.process(context.Background(), job)

	if store.lastJob == nil {
		t.Fatal("expected Update to be called")
	}
	if store.lastJob.Status != StatusCompleted {
		t.Fatalf("expected graceful completion, got %s (err=%s)", store.lastJob.Status, store.lastJob.ErrorMsg)
	}
	if store.lastJob.TranslatedText == "" {
		t.Fatal("expected translated_text to be preserved even in degraded path")
	}
}

func TestApplyLayoutModeBBoxOptions_DefaultAndGeometry(t *testing.T) {
	opts := internalservices.DefaultOverlayOptions()
	applyLayoutModeBBoxOptions(&opts, -1)

	if opts.BboxShrinkPx != 0 {
		t.Fatalf("expected BboxShrinkPx=0, got %d", opts.BboxShrinkPx)
	}
	if !opts.EraseBBox {
		t.Fatal("expected EraseBBox=true")
	}
	if opts.TextPadding != 3 {
		t.Fatalf("expected default TextPadding=3, got %d", opts.TextPadding)
	}
	if opts.PatchFeatherPx != 1 {
		t.Fatalf("expected PatchFeatherPx=1, got %d", opts.PatchFeatherPx)
	}
	if opts.PatchBlurRadius != 1 {
		t.Fatalf("expected PatchBlurRadius=1, got %d", opts.PatchBlurRadius)
	}
}

func TestApplyLayoutModeBBoxOptions_ClampRequestedPadding(t *testing.T) {
	tests := []struct {
		name      string
		requested int
		want      int
	}{
		{name: "below range", requested: 0, want: 2},
		{name: "within range lower", requested: 2, want: 2},
		{name: "within range middle", requested: 3, want: 3},
		{name: "within range upper", requested: 4, want: 4},
		{name: "above range", requested: 6, want: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := internalservices.DefaultOverlayOptions()
			applyLayoutModeBBoxOptions(&opts, tt.requested)
			if opts.TextPadding != tt.want {
				t.Fatalf("requested=%d: expected TextPadding=%d, got %d", tt.requested, tt.want, opts.TextPadding)
			}
			if opts.BboxShrinkPx != 0 {
				t.Fatalf("requested=%d: expected BboxShrinkPx=0, got %d", tt.requested, opts.BboxShrinkPx)
			}
		})
	}
}
