package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"

	_ "image/jpeg"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const openAIImageMaskMaxPixels = 4096 * 4096

// openAIImageMaskCompositor enforces the Images API mask contract on OAuth
// responses. The upstream Responses image tool treats masks as guidance, so
// protected pixels must be restored before the generated image reaches the
// client.
type openAIImageMaskCompositor struct {
	source image.Image
	mask   image.Image
	width  int
	height int
}

func newOpenAIImageMaskCompositor(parsed *OpenAIImagesRequest) (*openAIImageMaskCompositor, error) {
	if parsed == nil || !parsed.IsEdits() || !parsed.Multipart || parsed.MaskUpload == nil {
		return nil, nil
	}
	if len(parsed.Uploads) == 0 {
		return nil, fmt.Errorf("masked image edits require an image file")
	}

	source, sourceFormat, err := decodeOpenAIImageForMask(parsed.Uploads[0].Data, "image")
	if err != nil {
		return nil, err
	}
	if sourceFormat != "png" && sourceFormat != "jpeg" && sourceFormat != "webp" {
		return nil, fmt.Errorf("unsupported image format %q for masked edit", sourceFormat)
	}
	mask, maskFormat, err := decodeOpenAIImageForMask(parsed.MaskUpload.Data, "mask")
	if err != nil {
		return nil, err
	}
	if maskFormat != "png" {
		return nil, fmt.Errorf("mask must be a PNG image")
	}

	width := source.Bounds().Dx()
	height := source.Bounds().Dy()
	if mask.Bounds().Dx() != width || mask.Bounds().Dy() != height {
		return nil, fmt.Errorf("mask dimensions must match the source image")
	}
	return &openAIImageMaskCompositor{source: source, mask: mask, width: width, height: height}, nil
}

func decodeOpenAIImageForMask(data []byte, field string) (image.Image, string, error) {
	if len(data) == 0 {
		return nil, "", fmt.Errorf("%s image is empty", field)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode %s image metadata: %w", field, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > openAIImageMaskMaxPixels/cfg.Height {
		return nil, "", fmt.Errorf("%s image exceeds the masked edit pixel limit", field)
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode %s image: %w", field, err)
	}
	return decoded, strings.ToLower(strings.TrimSpace(format)), nil
}

func (c *openAIImageMaskCompositor) applyResult(result *openAIResponsesImageResult) error {
	if c == nil || result == nil {
		return nil
	}
	raw := normalizeOpenAIImageBase64(result.Result)
	generatedBytes, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return fmt.Errorf("decode generated image base64: %w", err)
	}
	generated, _, err := decodeOpenAIImageForMask(generatedBytes, "generated")
	if err != nil {
		return err
	}

	generated = c.resizeGenerated(generated)
	out := image.NewNRGBA(image.Rect(0, 0, c.width, c.height))
	sourceBounds := c.source.Bounds()
	maskBounds := c.mask.Bounds()
	generatedBounds := generated.Bounds()
	for y := 0; y < c.height; y++ {
		for x := 0; x < c.width; x++ {
			src, ok := color.NRGBAModel.Convert(c.source.At(sourceBounds.Min.X+x, sourceBounds.Min.Y+y)).(color.NRGBA)
			if !ok {
				return fmt.Errorf("convert source pixel to NRGBA")
			}
			gen, ok := color.NRGBAModel.Convert(generated.At(generatedBounds.Min.X+x, generatedBounds.Min.Y+y)).(color.NRGBA)
			if !ok {
				return fmt.Errorf("convert generated pixel to NRGBA")
			}
			_, _, _, alpha16 := c.mask.At(maskBounds.Min.X+x, maskBounds.Min.Y+y).RGBA()
			keep := uint32(alpha16 >> 8)
			edit := uint32(255) - keep
			out.SetNRGBA(x, y, color.NRGBA{
				R: uint8((uint32(src.R)*keep + uint32(gen.R)*edit + 127) / 255),
				G: uint8((uint32(src.G)*keep + uint32(gen.G)*edit + 127) / 255),
				B: uint8((uint32(src.B)*keep + uint32(gen.B)*edit + 127) / 255),
				A: uint8((uint32(src.A)*keep + uint32(gen.A)*edit + 127) / 255),
			})
		}
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, out); err != nil {
		return fmt.Errorf("encode masked image: %w", err)
	}
	result.Result = base64.StdEncoding.EncodeToString(encoded.Bytes())
	result.OutputFormat = "png"
	result.Size = fmt.Sprintf("%dx%d", c.width, c.height)
	return nil
}

func (c *openAIImageMaskCompositor) resizeGenerated(generated image.Image) image.Image {
	if generated.Bounds().Dx() == c.width && generated.Bounds().Dy() == c.height {
		return generated
	}
	resized := image.NewNRGBA(image.Rect(0, 0, c.width, c.height))
	xdraw.CatmullRom.Scale(resized, resized.Bounds(), generated, generated.Bounds(), xdraw.Over, nil)
	return resized
}
