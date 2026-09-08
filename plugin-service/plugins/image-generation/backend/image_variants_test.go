package backend

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/HugoSmits86/nativewebp"
)

func TestCreateCompressedPreviewDecodesWebP(t *testing.T) {
	input := image.NewRGBA(image.Rect(0, 0, 2, 2))
	input.Set(0, 0, color.RGBA{R: 255, A: 255})
	var source bytes.Buffer
	if err := nativewebp.Encode(&source, input, nil); err != nil {
		t.Fatal(err)
	}
	preview, contentType, err := createCompressedPreview(source.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) == 0 || contentType == "" {
		t.Fatalf("preview size=%d contentType=%q", len(preview), contentType)
	}
}

func TestCreateCompressedPreviewConstrainsLongestEdge(t *testing.T) {
	input := image.NewRGBA(image.Rect(0, 0, 2000, 1000))
	for y := 0; y < 1000; y++ {
		for x := 0; x < 2000; x++ {
			input.Set(x, y, color.RGBA{R: 80, G: 120, B: 180, A: 255})
		}
	}
	var source bytes.Buffer
	if err := jpeg.Encode(&source, input, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}

	preview, contentType, err := createCompressedPreview(source.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "image/jpeg" {
		t.Fatalf("content type = %q", contentType)
	}
	decoded, _, err := image.Decode(bytes.NewReader(preview))
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Bounds().Size(); got.X != 640 || got.Y != 320 {
		t.Fatalf("size = %v", got)
	}
}

func TestCreateCompressedPreviewUsesWebPForTransparency(t *testing.T) {
	input := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	input.Set(0, 0, color.NRGBA{R: 255, A: 80})
	var source bytes.Buffer
	if err := png.Encode(&source, input); err != nil {
		t.Fatal(err)
	}

	preview, contentType, err := createCompressedPreview(source.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "image/webp" {
		t.Fatalf("content type = %q", contentType)
	}
	if len(preview) < 12 || string(preview[:4]) != "RIFF" || string(preview[8:12]) != "WEBP" {
		t.Fatal("preview is not WebP")
	}
}

func TestCreateCompressedPreviewRejectsInvalidData(t *testing.T) {
	if _, _, err := createCompressedPreview([]byte("not-image")); err == nil {
		t.Fatal("expected decode error")
	}
}
