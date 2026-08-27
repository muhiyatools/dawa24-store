package ingest_test

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
)

func TestApplyWatermark_JPEG(t *testing.T) {
	// Create a dummy 400x300 image
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{200, 220, 240, 255}}, image.Point{}, draw.Src)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("encode dummy jpeg: %v", err)
	}

	watermarked, err := ingest.ApplyWatermark(buf.Bytes(), "jpg")
	if err != nil {
		t.Fatalf("ApplyWatermark error: %v", err)
	}
	if len(watermarked) == 0 {
		t.Fatal("ApplyWatermark returned empty buffer")
	}

	decoded, _, err := image.Decode(bytes.NewReader(watermarked))
	if err != nil {
		t.Fatalf("failed to decode watermarked image: %v", err)
	}
	if decoded.Bounds().Dx() != 400 || decoded.Bounds().Dy() != 300 {
		t.Fatalf("bounds mismatch: got %dx%d, want 400x300", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

func TestApplyWatermark_PNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 250, 250))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{100, 150, 200, 255}}, image.Point{}, draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode dummy png: %v", err)
	}

	watermarked, err := ingest.ApplyWatermark(buf.Bytes(), "png")
	if err != nil {
		t.Fatalf("ApplyWatermark error: %v", err)
	}
	if len(watermarked) == 0 {
		t.Fatal("ApplyWatermark returned empty buffer")
	}

	decoded, _, err := image.Decode(bytes.NewReader(watermarked))
	if err != nil {
		t.Fatalf("failed to decode watermarked image: %v", err)
	}
	if decoded.Bounds().Dx() != 250 || decoded.Bounds().Dy() != 250 {
		t.Fatalf("bounds mismatch: got %dx%d, want 250x250", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}
