package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"
)

func openAIImageMaskTestPNG(t *testing.T, width, height int, pixel func(x, y int) color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, pixel(x, y))
		}
	}
	var out bytes.Buffer
	require.NoError(t, png.Encode(&out, img))
	return out.Bytes()
}

func openAIImageMaskTestSolidPNG(t *testing.T, width, height int, pixel color.NRGBA) []byte {
	t.Helper()
	return openAIImageMaskTestPNG(t, width, height, func(_, _ int) color.NRGBA { return pixel })
}

func openAIImageMaskTestResult(t *testing.T, data []byte, format string) openAIResponsesImageResult {
	t.Helper()
	return openAIResponsesImageResult{
		Result:       base64.StdEncoding.EncodeToString(data),
		OutputFormat: format,
	}
}

func openAIImageMaskTestDecodeResult(t *testing.T, result openAIResponsesImageResult) image.Image {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(result.Result)
	require.NoError(t, err)
	img, format, err := image.Decode(bytes.NewReader(decoded))
	require.NoError(t, err)
	require.Equal(t, "png", format)
	return img
}

func TestOpenAIImageMaskCompositorEnforcesAlphaMask(t *testing.T) {
	source := openAIImageMaskTestSolidPNG(t, 2, 2, color.NRGBA{R: 200, G: 10, B: 20, A: 255})
	mask := openAIImageMaskTestPNG(t, 2, 2, func(x, y int) color.NRGBA {
		alpha := uint8(255)
		if x == 1 && y == 0 {
			alpha = 0
		}
		if x == 0 && y == 1 {
			alpha = 128
		}
		return color.NRGBA{A: alpha}
	})
	parsed := &OpenAIImagesRequest{
		Endpoint:  openAIImagesEditsEndpoint,
		Multipart: true,
		Uploads:   []OpenAIImagesUpload{{Data: source}},
		MaskUpload: &OpenAIImagesUpload{
			Data: mask,
		},
	}
	compositor, err := newOpenAIImageMaskCompositor(parsed)
	require.NoError(t, err)

	result := openAIImageMaskTestResult(t, openAIImageMaskTestSolidPNG(t, 2, 2, color.NRGBA{R: 10, G: 110, B: 220, A: 255}), "webp")
	require.NoError(t, compositor.applyResult(&result))
	require.Equal(t, "png", result.OutputFormat)
	require.Equal(t, "2x2", result.Size)

	got := openAIImageMaskTestDecodeResult(t, result)
	require.Equal(t, color.NRGBA{R: 200, G: 10, B: 20, A: 255}, color.NRGBAModel.Convert(got.At(0, 0)))
	require.Equal(t, color.NRGBA{R: 10, G: 110, B: 220, A: 255}, color.NRGBAModel.Convert(got.At(1, 0)))
	require.Equal(t, color.NRGBA{R: 105, G: 60, B: 120, A: 255}, color.NRGBAModel.Convert(got.At(0, 1)))
}

func TestOpenAIImageMaskCompositorRejectsDimensionMismatch(t *testing.T) {
	parsed := &OpenAIImagesRequest{
		Endpoint:  openAIImagesEditsEndpoint,
		Multipart: true,
		Uploads: []OpenAIImagesUpload{{
			Data: openAIImageMaskTestSolidPNG(t, 2, 2, color.NRGBA{A: 255}),
		}},
		MaskUpload: &OpenAIImagesUpload{
			Data: openAIImageMaskTestSolidPNG(t, 1, 1, color.NRGBA{A: 255}),
		},
	}

	compositor, err := newOpenAIImageMaskCompositor(parsed)
	require.Nil(t, compositor)
	require.ErrorContains(t, err, "mask dimensions must match")
}

func TestOpenAIImageMaskCompositorResizesGeneratedImageToSource(t *testing.T) {
	source := openAIImageMaskTestSolidPNG(t, 2, 2, color.NRGBA{R: 200, A: 255})
	mask := openAIImageMaskTestPNG(t, 2, 2, func(x, _ int) color.NRGBA {
		if x == 0 {
			return color.NRGBA{A: 255}
		}
		return color.NRGBA{A: 0}
	})
	compositor, err := newOpenAIImageMaskCompositor(&OpenAIImagesRequest{
		Endpoint:  openAIImagesEditsEndpoint,
		Multipart: true,
		Uploads:   []OpenAIImagesUpload{{Data: source}},
		MaskUpload: &OpenAIImagesUpload{
			Data: mask,
		},
	})
	require.NoError(t, err)

	result := openAIImageMaskTestResult(t, openAIImageMaskTestSolidPNG(t, 1, 1, color.NRGBA{B: 220, A: 255}), "png")
	require.NoError(t, compositor.applyResult(&result))

	got := openAIImageMaskTestDecodeResult(t, result)
	require.Equal(t, image.Rect(0, 0, 2, 2), got.Bounds())
	require.Equal(t, color.NRGBA{R: 200, A: 255}, color.NRGBAModel.Convert(got.At(0, 1)))
	require.Equal(t, color.NRGBA{B: 220, A: 255}, color.NRGBAModel.Convert(got.At(1, 1)))
}

func TestOpenAIImageMaskCompositorUsesFirstImageForMultiImageEdit(t *testing.T) {
	first := openAIImageMaskTestSolidPNG(t, 2, 2, color.NRGBA{R: 1, A: 255})
	second := openAIImageMaskTestSolidPNG(t, 3, 3, color.NRGBA{G: 1, A: 255})
	parsed := &OpenAIImagesRequest{
		Endpoint:  openAIImagesEditsEndpoint,
		Multipart: true,
		Uploads:   []OpenAIImagesUpload{{Data: first}, {Data: second}},
		MaskUpload: &OpenAIImagesUpload{
			Data: openAIImageMaskTestSolidPNG(t, 2, 2, color.NRGBA{A: 255}),
		},
	}

	compositor, err := newOpenAIImageMaskCompositor(parsed)
	require.NoError(t, err)
	require.NotNil(t, compositor)
	require.Equal(t, 2, compositor.width)
	require.Equal(t, 2, compositor.height)
}

func TestForwardOpenAIImagesOAuthRejectsInvalidMaskBeforeCredentials(t *testing.T) {
	source := openAIImageMaskTestSolidPNG(t, 2, 2, color.NRGBA{A: 255})
	tests := []struct {
		name       string
		mask       []byte
		wantErrMsg string
	}{
		{
			name:       "invalid PNG",
			mask:       []byte("not-an-image"),
			wantErrMsg: "decode mask image metadata",
		},
		{
			name:       "dimension mismatch",
			mask:       openAIImageMaskTestSolidPNG(t, 1, 1, color.NRGBA{A: 255}),
			wantErrMsg: "mask dimensions must match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := &OpenAIImagesRequest{
				Endpoint:  openAIImagesEditsEndpoint,
				Model:     "gpt-image-2",
				Multipart: true,
				Uploads:   []OpenAIImagesUpload{{Data: source}},
				MaskUpload: &OpenAIImagesUpload{
					Data: tt.mask,
				},
			}
			result, err := (&OpenAIGatewayService{}).forwardOpenAIImagesOAuth(
				context.Background(),
				nil,
				&Account{Type: AccountTypeOAuth},
				parsed,
				"",
			)

			require.Nil(t, result)
			require.ErrorContains(t, err, tt.wantErrMsg)
		})
	}
}
