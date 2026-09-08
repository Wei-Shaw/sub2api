package backend

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"

	"github.com/HugoSmits86/nativewebp"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const previewMaxEdge = 640

func createCompressedPreview(data []byte) ([]byte, string, error) {
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode image preview: %w", err)
	}
	preview := resizePreview(source)
	var output bytes.Buffer
	if imageHasTransparency(preview) {
		if err := nativewebp.Encode(&output, preview, nil); err != nil {
			return nil, "", fmt.Errorf("encode WebP preview: %w", err)
		}
		return output.Bytes(), "image/webp", nil
	}
	if err := jpeg.Encode(&output, preview, &jpeg.Options{Quality: 62}); err != nil {
		return nil, "", fmt.Errorf("encode JPEG preview: %w", err)
	}
	return output.Bytes(), "image/jpeg", nil
}

func resizePreview(source image.Image) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= previewMaxEdge && height <= previewMaxEdge {
		return source
	}
	scale := float64(previewMaxEdge) / float64(max(width, height))
	target := image.NewNRGBA(image.Rect(0, 0, int(float64(width)*scale+0.5), int(float64(height)*scale+0.5)))
	draw.CatmullRom.Scale(target, target.Bounds(), source, bounds, draw.Over, nil)
	return target
}

func imageHasTransparency(source image.Image) bool {
	bounds := source.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := source.At(x, y).RGBA()
			if alpha != 0xffff {
				return true
			}
		}
	}
	return false
}
