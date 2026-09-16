//go:build unit

package service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeAdobeOutputFormat(t *testing.T) {
	pngFormat, err := normalizeAdobeOutputFormat(" PNG ")
	require.NoError(t, err)
	require.Equal(t, adobeOutputFormatPNG, pngFormat)

	jpegFormat, err := normalizeAdobeOutputFormat("JPG")
	require.NoError(t, err)
	require.Equal(t, adobeOutputFormatJPEG, jpegFormat)

	empty, err := normalizeAdobeOutputFormat("")
	require.NoError(t, err)
	require.Empty(t, empty)

	_, err = normalizeAdobeOutputFormat("webp")
	require.ErrorContains(t, err, "output_format=")
	_, err = normalizeAdobeOutputFormat("gif")
	require.ErrorContains(t, err, "output_format=")
}

func TestTranscodeAdobeImagePNGIdentity(t *testing.T) {
	src := adobeTestPNG(t, false)
	got, err := transcodeAdobeImage(src, adobeOutputFormatPNG, intPtr(50))
	require.NoError(t, err)
	require.Equal(t, src, got, "PNG→PNG must not decode/re-encode")
}

func TestTranscodeAdobeImageJPEGToPNG(t *testing.T) {
	src := adobeTestJPEG(t, 90)
	got, err := transcodeAdobeImage(src, adobeOutputFormatPNG, nil)
	require.NoError(t, err)
	require.Equal(t, adobeOutputFormatPNG, sniffAdobeImageFormat(got))
	cfg, _, err := image.DecodeConfig(bytes.NewReader(got))
	require.NoError(t, err)
	require.Equal(t, 8, cfg.Width)
	require.Equal(t, 8, cfg.Height)
}

func TestTranscodeAdobeImageOpaquePNGToJPEG(t *testing.T) {
	src := adobeTestPNG(t, false)
	got, err := transcodeAdobeImage(src, adobeOutputFormatJPEG, nil)
	require.NoError(t, err)
	require.Equal(t, adobeOutputFormatJPEG, sniffAdobeImageFormat(got))
}

func TestTranscodeAdobeImageTransparentPNGToJPEGRejected(t *testing.T) {
	src := adobeTestPNG(t, true)
	_, err := transcodeAdobeImage(src, adobeOutputFormatJPEG, nil)
	require.ErrorContains(t, err, "transparent")
}

func TestApplyAdobeOutputFormatFallsBackOnCorruptBytes(t *testing.T) {
	src := []byte("not-an-image")
	got := applyAdobeOutputFormat(src, adobeOutputFormatPNG, nil)
	require.Equal(t, src, got)
}

func TestTranscodeAdobeImageJPEGCompression(t *testing.T) {
	src := adobeTestJPEG(t, 100)
	same, err := transcodeAdobeImage(src, adobeOutputFormatJPEG, intPtr(100))
	require.NoError(t, err)
	require.Equal(t, src, same)

	sameNil, err := transcodeAdobeImage(src, adobeOutputFormatJPEG, nil)
	require.NoError(t, err)
	require.Equal(t, src, sameNil)

	recompressed, err := transcodeAdobeImage(src, adobeOutputFormatJPEG, intPtr(50))
	require.NoError(t, err)
	require.NotEqual(t, src, recompressed)
	require.Equal(t, adobeOutputFormatJPEG, sniffAdobeImageFormat(recompressed))
}

func adobeTestPNG(t *testing.T, withAlpha bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	alpha := uint8(255)
	if withAlpha {
		alpha = 128
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(80 + x*40), B: uint8(80 + y*40), A: alpha})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func adobeTestJPEG(t *testing.T, quality int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 32), G: uint8(y * 32), B: 80, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}))
	return buf.Bytes()
}
