package service

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log/slog"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
)

const (
	adobeMaxBatchCount      = 10
	adobeFanOutConcurrency  = 2
	adobeJPEGDefaultQuality = 100
	adobeOutputFormatPNG    = "png"
	adobeOutputFormatJPEG   = "jpeg"
)

func adobeImageRequestCount(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func validateAdobeImageParams(req *OpenAIImagesRequest) (int, string, error) {
	n := adobeImageRequestCount(req.N)
	if n > adobeMaxBatchCount {
		return 0, "", adobe.NewRequestError(
			fmt.Sprintf("adobe channel n must be between 1 and %d, got n=%d", adobeMaxBatchCount, n))
	}
	format, err := normalizeAdobeOutputFormat(req.OutputFormat)
	if err != nil {
		return 0, "", err
	}
	if format == adobeOutputFormatJPEG && strings.EqualFold(strings.TrimSpace(req.Background), "transparent") {
		return 0, "", adobe.NewRequestError(
			"adobe channel does not support output_format=jpeg with background=transparent")
	}
	return n, format, nil
}

func normalizeAdobeOutputFormat(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return "", nil
	case adobeOutputFormatPNG:
		return adobeOutputFormatPNG, nil
	case adobeOutputFormatJPEG, "jpg":
		return adobeOutputFormatJPEG, nil
	default:
		return "", adobe.NewRequestError(
			fmt.Sprintf("adobe channel does not support output_format=%q", strings.TrimSpace(raw)))
	}
}

func applyAdobeOutputFormat(data []byte, format string, compression *int) []byte {
	converted, err := transcodeAdobeImage(data, format, compression)
	if err != nil {
		slog.Warn("adobe image transcode failed, using original bytes", "error", err, "output_format", format)
		return data
	}
	if len(converted) == 0 {
		return data
	}
	return converted
}

func transcodeAdobeImage(data []byte, format string, compression *int) ([]byte, error) {
	if format == "" || len(data) == 0 {
		return data, nil
	}
	src := sniffAdobeImageFormat(data)
	if src == format && !adobeShouldRecompressJPEG(src, format, compression) {
		return data, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	if format == adobeOutputFormatJPEG && imageHasTransparency(img) {
		return nil, fmt.Errorf("cannot convert transparent image to jpeg")
	}
	return encodeAdobeImage(img, format, compression)
}

func sniffAdobeImageFormat(data []byte) string {
	switch detectedImageContentType(data) {
	case "image/png":
		return adobeOutputFormatPNG
	case "image/jpeg":
		return adobeOutputFormatJPEG
	}
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return adobeOutputFormatPNG
	}
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8 {
		return adobeOutputFormatJPEG
	}
	return ""
}

func adobeShouldRecompressJPEG(src, format string, compression *int) bool {
	return src == adobeOutputFormatJPEG && format == adobeOutputFormatJPEG &&
		compression != nil && *compression < adobeJPEGDefaultQuality
}

func encodeAdobeImage(img image.Image, format string, compression *int) ([]byte, error) {
	var buf bytes.Buffer
	switch format {
	case adobeOutputFormatPNG:
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
	case adobeOutputFormatJPEG:
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: adobeJPEGQuality(compression)}); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported output format %q", format)
	}
	return buf.Bytes(), nil
}

func adobeJPEGQuality(compression *int) int {
	if compression == nil {
		return adobeJPEGDefaultQuality
	}
	quality := *compression
	if quality < 1 {
		return 1
	}
	if quality > adobeJPEGDefaultQuality {
		return adobeJPEGDefaultQuality
	}
	return quality
}

func imageHasTransparency(img image.Image) bool {
	if img == nil {
		return false
	}
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := img.At(x, y).RGBA()
			if alpha < 0xffff {
				return true
			}
		}
	}
	return false
}
