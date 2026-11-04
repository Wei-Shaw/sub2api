package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// Opt-in, at the wire boundary: full_resolution tool results and historical
// images must obey the same limit as newly attached images. Never fetch URLs.
func guardAnthropicImages(ctx context.Context, c *gin.Context, cfg *config.Config, body []byte) ([]byte, error) {
	if cfg == nil || !cfg.Gateway.AnthropicImageGuardEnabled {
		return body, nil
	}
	result, err := normalizeAnthropicImages(ctx, body)
	if err != nil && ctx.Err() == nil && c != nil && !c.Writer.Written() {
		MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "error": gin.H{"type": "invalid_request_error", "message": err.Error()}})
	}
	return result, err
}

const (
	anthropicImageMaxEdge = 3000
	// Bound allocations before decoding attacker-controlled raster dimensions.
	anthropicImageMaxPixels     = 16_000_000
	anthropicImageRequestPixels = 32_000_000
	anthropicImageMaxBytes      = 5 * 1024 * 1024
)

// Only visit Messages image blocks and tool_result.content, not arbitrary tool
// inputs, schemas or signed thinking blocks. sjson preserves untouched fields.
type anthropicImageRef struct {
	path   string
	source gjson.Result
}

func anthropicImagePaths(body []byte) []anthropicImageRef {
	var paths []anthropicImageRef
	var visit func(gjson.Result, string)
	visit = func(content gjson.Result, path string) {
		if !content.IsArray() {
			return
		}
		for i, block := range content.Array() {
			p := path + "." + strconv.Itoa(i)
			switch block.Get("type").String() {
			case "image":
				paths = append(paths, anthropicImageRef{path: p, source: block.Get("source")})
			case "tool_result":
				visit(block.Get("content"), p+".content")
			}
		}
	}
	for i, message := range gjson.GetBytes(body, "messages").Array() {
		visit(message.Get("content"), "messages."+strconv.Itoa(i)+".content")
	}
	return paths
}

func normalizeAnthropicImages(ctx context.Context, body []byte) ([]byte, error) {
	paths := anthropicImagePaths(body)
	result := body
	remainingPixels := int64(anthropicImageRequestPixels)
	for _, ref := range paths {
		path := ref.path
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		source := ref.source
		fail := func(reason string) ([]byte, error) {
			return nil, fmt.Errorf("image guard: %s.source: %s", path, reason)
		}
		if source.Get("type").String() != "base64" {
			return fail("cannot validate remote/file image dimensions; supply a base64 image or disable gateway.anthropic_image_guard_enabled")
		}
		encoded := source.Get("data").String()
		if len(encoded) > base64.StdEncoding.EncodedLen(anthropicImageMaxBytes) {
			return fail("image exceeds the 5 MiB decode budget; use a smaller image or crop")
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return fail("invalid base64 image")
		}
		if len(data) > anthropicImageMaxBytes {
			return fail("image exceeds the 5 MiB decode budget; use a smaller image or crop")
		}
		ic, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return fail("unsupported or invalid image header")
		}
		switch format {
		case "png", "jpeg", "gif", "webp":
		default:
			return fail("unsupported image format")
		}
		if source.Get("media_type").String() != "image/"+format {
			return fail("image media_type does not match its encoded format")
		}
		if ic.Width <= anthropicImageMaxEdge && ic.Height <= anthropicImageMaxEdge {
			continue
		}
		if ic.Width <= 0 || ic.Height <= 0 || int64(ic.Width) > int64(anthropicImageMaxPixels)/int64(ic.Height) {
			return fail("resize pixel budget exceeded; use smaller images or crops")
		}
		pixels := int64(ic.Width) * int64(ic.Height)
		if pixels > remainingPixels {
			return fail("resize pixel budget exceeded; use smaller images or crops")
		}
		remainingPixels -= pixels
		// JPEG EXIF orientation would be lost by Go's encoder. Refuse rather than
		// silently rotate a photo; callers can supply an orientation-normalized PNG.
		if format == "jpeg" && bytes.Contains(data, []byte("Exif\x00\x00")) {
			return fail("oversized JPEG has EXIF metadata; normalize orientation or supply PNG before resizing")
		}
		src, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return fail("cannot decode oversized image")
		}
		w, h := ic.Width, ic.Height
		if w >= h {
			h = max(1, h*anthropicImageMaxEdge/w)
			w = anthropicImageMaxEdge
		} else {
			w = max(1, w*anthropicImageMaxEdge/h)
			h = anthropicImageMaxEdge
		}
		dst := image.NewNRGBA(image.Rect(0, 0, w, h))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out := &imageGuardBuffer{remaining: anthropicImageMaxBytes}
		mime := "image/png"
		if format == "jpeg" {
			mime = "image/jpeg"
			err = jpeg.Encode(out, dst, &jpeg.Options{Quality: 90})
		} else {
			err = png.Encode(out, dst)
		}
		if err != nil {
			return fail("resized image exceeds encoding budget or cannot be encoded; use a smaller crop")
		}
		result, err = sjson.SetBytes(result, path+".source.data", base64.StdEncoding.EncodeToString(out.Bytes()))
		if err != nil {
			return fail("cannot replace image data")
		}
		result, err = sjson.SetBytes(result, path+".source.media_type", mime)
		if err != nil {
			return fail("cannot replace image media_type")
		}
	}
	return result, nil
}

// Bound output allocation even for noisy images that compress poorly.
type imageGuardBuffer struct {
	bytes.Buffer
	remaining int
}

func (b *imageGuardBuffer) Write(p []byte) (int, error) {
	if len(p) > b.remaining {
		return 0, io.ErrShortWrite
	}
	n, err := b.Buffer.Write(p)
	b.remaining -= n
	return n, err
}
