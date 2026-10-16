package service

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strconv"
	"strings"

	"golang.org/x/image/draw"
)

// The web provider has no native size control. These bounds apply to the PNG
// that the Images adapter produces, not to the resolution generated upstream.
const (
	chatGPTWebImageMinDimension = 64
	chatGPTWebImageMaxDimension = 4096
	chatGPTWebImageMaxPixels    = 16 << 20
)

func parseChatGPTWebImageSize(size string) (width, height int, err *OpenAIImagesUpstreamError) {
	size = strings.TrimSpace(size)
	if size == "" || size == "auto" {
		return 0, 0, nil
	}
	invalid := func() (int, int, *OpenAIImagesUpstreamError) {
		return 0, 0, chatGPTWebImageRequestError("chatgpt_web_image_invalid_size", "size must be auto or WIDTHxHEIGHT, with each dimension between 64 and 4096 pixels and at most 16777216 pixels in total", "size")
	}
	w, h, ok := strings.Cut(size, "x")
	if !ok || w == "" || h == "" {
		return invalid()
	}
	// Do not accept signs, decimals, whitespace, or a second separator.
	for _, dimension := range []string{w, h} {
		for _, digit := range dimension {
			if digit < '0' || digit > '9' {
				return invalid()
			}
		}
	}
	var parseErr error
	width, parseErr = strconv.Atoi(w)
	if parseErr != nil || width < chatGPTWebImageMinDimension || width > chatGPTWebImageMaxDimension {
		return invalid()
	}
	height, parseErr = strconv.Atoi(h)
	if parseErr != nil || height < chatGPTWebImageMinDimension || height > chatGPTWebImageMaxDimension || int64(width)*int64(height) > chatGPTWebImageMaxPixels {
		return invalid()
	}
	return width, height, nil
}

func chatGPTWebImagePromptForSize(prompt, size string) (string, *OpenAIImagesUpstreamError) {
	width, height, err := parseChatGPTWebImageSize(size)
	if err != nil || width == 0 {
		return prompt, err
	}
	prompt += fmt.Sprintf("\n\nOutput composition: target %d x %d pixels (width x height). Compose the image for this aspect ratio. Keep the entire main subject and any text within a generous central safe area; the server will resize proportionally and crop the edges to the requested dimensions if needed. Do not draw a frame or print these dimensions on the image.", width, height)
	if len(prompt) > 64<<10 {
		return "", chatGPTWebImageRequestError("invalid_request", "prompt plus the requested size instruction exceeds the ChatGPT web image size limit", "prompt")
	}
	return prompt, nil
}

// resizeChatGPTWebImage preserves proportions and fills the requested canvas.
// Crop in source coordinates first so extreme source aspect ratios cannot
// allocate a large intermediate canvas. The original stored result is untouched.
func resizeChatGPTWebImage(src image.Image, width, height int) ([]byte, error) {
	bounds := src.Bounds()
	crop := bounds
	if int64(bounds.Dx())*int64(height) > int64(bounds.Dy())*int64(width) {
		cropWidth := max(1, int(int64(bounds.Dy())*int64(width)/int64(height)))
		crop.Min.X += (bounds.Dx() - cropWidth) / 2
		crop.Max.X = crop.Min.X + cropWidth
	} else {
		cropHeight := max(1, int(int64(bounds.Dx())*int64(height)/int64(width)))
		crop.Min.Y += (bounds.Dy() - cropHeight) / 2
		crop.Max.Y = crop.Min.Y + cropHeight
	}
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, draw.Src, nil)
	var output chatGPTWebImagePNGBuffer
	if err := png.Encode(&output, dst); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// Bound the encoded output as well as the decoded pixel allocation.
type chatGPTWebImagePNGBuffer struct{ bytes.Buffer }

func (b *chatGPTWebImagePNGBuffer) Write(p []byte) (int, error) {
	if int64(b.Len())+int64(len(p)) > openAIImageMaxDownloadBytes {
		return 0, errors.New("resized image exceeds the PNG byte limit")
	}
	return b.Buffer.Write(p)
}
