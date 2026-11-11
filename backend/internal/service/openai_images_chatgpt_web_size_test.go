//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptwebimage"
	"github.com/stretchr/testify/require"
)

func TestChatGPTWebImagesAcceptCherryStudioExplicitSizes(t *testing.T) {
	for _, size := range []string{"", "auto", "256x256", "512x512", "1024x1024", "1536x1024", "1024x1536", "2048x2048", "4096x4096", "64x64"} {
		t.Run(size, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"model": "gpt-image-2", "prompt": "A cat", "n": 1, "size": size})
			require.NoError(t, err)
			c, _ := webImageTestContext()
			c.Request.Header.Set("Content-Type", "application/json")
			parsed, err := (&OpenAIGatewayService{}).ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			require.Nil(t, validateChatGPTWebImagesRequest(parsed))
		})
	}
}

func TestChatGPTWebImagesRejectInvalidSizesBeforeCredentials(t *testing.T) {
	for _, size := range []string{"1024", "1024x", "x1024", "0x1024", "63x1024", "1024x63", "4097x1024", "1024x4097", "1024x-1", "1024x+512", "1024x1.5", "1024 x1024", "1024x1024x1024", strings.Repeat("9", 100) + "x1024"} {
		t.Run(size, func(t *testing.T) {
			request := webImageTestRequest()
			request.Size = size
			c, rec := webImageTestContext()
			_, err := (&OpenAIGatewayService{}).ForwardImages(context.Background(), c, webImageTestAccount(), nil, request, "")
			var public *OpenAIImagesUpstreamError
			require.ErrorAs(t, err, &public)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, "chatgpt_web_image_invalid_size", public.Code)
			require.Equal(t, "size", public.Param)
		})
	}
}

func TestChatGPTWebImageSizePromptPreservesIntentAndBounds(t *testing.T) {
	prompt := "Draw a cat sitting by a window."
	for _, size := range []string{"", "auto"} {
		actual, err := chatGPTWebImagePromptForSize(prompt, size)
		require.Nil(t, err)
		require.Equal(t, prompt, actual)
	}
	actual, err := chatGPTWebImagePromptForSize(prompt, "1536x1024")
	require.Nil(t, err)
	require.True(t, strings.HasPrefix(actual, prompt+"\n\n"))
	require.Contains(t, actual, "1536 x 1024 pixels (width x height)")
	require.Contains(t, actual, "central safe area")
	require.Contains(t, actual, "crop the edges")
	request := webImageTestRequest()
	request.Prompt = strings.Repeat("a", 64<<10)
	request.Size = "1024x1024"
	require.Equal(t, "prompt", validateChatGPTWebImagesRequest(request).Param)
	request.Size = "auto"
	require.Nil(t, validateChatGPTWebImagesRequest(request))
}

func TestChatGPTWebImagesExplicitSizeReturnsActualResizedPNG(t *testing.T) {
	// Red and blue edges surround a green square. A proportional center crop
	// to square must preserve the green center instead of stretching the edges.
	src := image.NewNRGBA(image.Rect(0, 0, 192, 64))
	green := color.NRGBA{G: 255, A: 255}
	for y := 0; y < 64; y++ {
		for x := 0; x < 192; x++ {
			pixel := green
			if x < 64 {
				pixel = color.NRGBA{R: 255, A: 255}
			} else if x >= 128 {
				pixel = color.NRGBA{B: 255, A: 255}
			}
			src.SetNRGBA(x, y, pixel)
		}
	}
	var source bytes.Buffer
	require.NoError(t, png.Encode(&source, src))
	original := append([]byte(nil), source.Bytes()...)
	for _, size := range []string{"64x64", "128x64", "64x128"} {
		t.Run(size, func(t *testing.T) {
			c, rec := webImageTestContext()
			request := webImageTestRequest()
			request.Size = size
			result, err := writeChatGPTWebImagesResult(c, request, chatgptwebimage.Result{Images: []chatgptwebimage.Image{{Bytes: source.Bytes(), Format: "png"}}}, time.Now())
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, rec.Code)
			var payload struct {
				Size string `json:"size"`
				Data []struct {
					B64 string `json:"b64_json"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
			require.Equal(t, size, payload.Size)
			require.Len(t, payload.Data, 1)
			data, err := base64.StdEncoding.DecodeString(payload.Data[0].B64)
			require.NoError(t, err)
			decoded, err := png.Decode(bytes.NewReader(data))
			require.NoError(t, err)
			width, height, sizeErr := parseChatGPTWebImageSize(size)
			require.Nil(t, sizeErr)
			require.Equal(t, image.Rect(0, 0, width, height), decoded.Bounds())
			if size == "64x64" {
				require.Equal(t, green, color.NRGBAModel.Convert(decoded.At(2, 2)))
				require.Equal(t, green, color.NRGBAModel.Convert(decoded.At(61, 61)))
			}
			require.Equal(t, size, result.ImageInputSize)
			require.Equal(t, size, result.ImageOutputSize)
			require.Equal(t, []string{size}, result.ImageOutputSizes)
			require.Equal(t, normalizeOpenAIImageSizeTier(size), result.ImageSize)
			require.Equal(t, "resize-center-crop", rec.Header().Get("X-Sub2API-Image-Size-Adaptation"))
			require.Equal(t, "192x64", rec.Header().Get("X-Sub2API-Image-Original-Size"))
			require.Equal(t, original, source.Bytes(), "the stored upstream PNG must remain unchanged")
		})
	}
}

func TestChatGPTWebImagesAutoAndMatchingSizePreserveOriginalPNG(t *testing.T) {
	var source bytes.Buffer
	require.NoError(t, png.Encode(&source, image.NewNRGBA(image.Rect(0, 0, 128, 64))))
	for _, size := range []string{"", "auto", "128x64"} {
		t.Run(size, func(t *testing.T) {
			c, rec := webImageTestContext()
			request := webImageTestRequest()
			request.Size = size
			result, err := writeChatGPTWebImagesResult(c, request, chatgptwebimage.Result{Images: []chatgptwebimage.Image{{Bytes: source.Bytes()}}}, time.Now())
			require.NoError(t, err)
			var payload struct {
				Data []struct {
					B64 string `json:"b64_json"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
			data, err := base64.StdEncoding.DecodeString(payload.Data[0].B64)
			require.NoError(t, err)
			require.Equal(t, source.Bytes(), data)
			require.Equal(t, "128x64", result.ImageOutputSize)
			require.Empty(t, rec.Header().Get("X-Sub2API-Image-Size-Adaptation"))
		})
	}
}
