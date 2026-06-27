//go:build unit

package service

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"
)

// makeTestPNG 生成一张实际尺寸为 w×h 的最小 PNG，并返回其 base64 编码。
func makeTestPNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func makeTestJPEG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 60}))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestDecodeImageSizeFromBase64_PNG(t *testing.T) {
	b64 := makeTestPNG(t, 1920, 1080)
	got, err := decodeImageSizeFromBase64(b64)
	require.NoError(t, err)
	require.Equal(t, "1920x1080", got)
}

func TestDecodeImageSizeFromBase64_JPEG(t *testing.T) {
	b64 := makeTestJPEG(t, 1024, 1024)
	got, err := decodeImageSizeFromBase64(b64)
	require.NoError(t, err)
	require.Equal(t, "1024x1024", got)
}

func TestDecodeImageSizeFromBase64_AcceptsDataURIPrefix(t *testing.T) {
	b64 := makeTestPNG(t, 800, 600)
	withPrefix := "data:image/png;base64," + b64
	got, err := decodeImageSizeFromBase64(withPrefix)
	require.NoError(t, err)
	require.Equal(t, "800x600", got)
}

func TestDecodeImageSizeFromBase64_RejectsCorrupt(t *testing.T) {
	_, err := decodeImageSizeFromBase64("this-is-not-valid-base64-image-content")
	require.Error(t, err)
}

func TestDecodeImageSizeFromBase64_RejectsEmpty(t *testing.T) {
	_, err := decodeImageSizeFromBase64("")
	require.Error(t, err)
}

func TestNeedDecodeImageSize(t *testing.T) {
	require.True(t, needDecodeImageSize(""))
	require.True(t, needDecodeImageSize("auto"))
	require.True(t, needDecodeImageSize("AUTO"))
	require.True(t, needDecodeImageSize("  auto  "))
	require.False(t, needDecodeImageSize("1024x1024"))
	require.False(t, needDecodeImageSize("3840x2160"))
}

// (a) 关闭开关时不解码：ImageOutputSizes 保持原状
func TestDecodeOpenAIImageOutputSizes_DisabledNoOp(t *testing.T) {
	b64 := makeTestPNG(t, 1920, 1080)
	result := &OpenAIForwardResult{
		ImageCount:        1,
		ImageOutputSizes:  []string{""},
		ImageOutputBase64: []string{b64},
	}
	g := &Group{Platform: PlatformOpenAI, ImageDecodeSizeOnRsp: false}
	DecodeOpenAIImageOutputSizes(result, g)
	require.Equal(t, []string{""}, result.ImageOutputSizes)
	require.False(t, result.imageSizeDecoded)
}

// (b) 开启开关 + size="" + 合法 PNG → 命中真实尺寸
func TestDecodeOpenAIImageOutputSizes_EnabledEmptySize_PNG(t *testing.T) {
	b64 := makeTestPNG(t, 1920, 1080)
	result := &OpenAIForwardResult{
		ImageCount:        1,
		ImageOutputSizes:  []string{""},
		ImageOutputBase64: []string{b64},
	}
	g := &Group{Platform: PlatformOpenAI, ImageDecodeSizeOnRsp: true}
	DecodeOpenAIImageOutputSizes(result, g)
	require.Equal(t, []string{"1920x1080"}, result.ImageOutputSizes)
	require.True(t, result.imageSizeDecoded)
}

// (c) 开启开关 + size="auto" + 合法 JPEG → 覆盖
func TestDecodeOpenAIImageOutputSizes_EnabledAuto_JPEG(t *testing.T) {
	b64 := makeTestJPEG(t, 1024, 1024)
	result := &OpenAIForwardResult{
		ImageCount:        1,
		ImageOutputSizes:  []string{"auto"},
		ImageOutputBase64: []string{b64},
	}
	g := &Group{Platform: PlatformOpenAI, ImageDecodeSizeOnRsp: true}
	DecodeOpenAIImageOutputSizes(result, g)
	require.Equal(t, []string{"1024x1024"}, result.ImageOutputSizes)
	require.True(t, result.imageSizeDecoded)
}

// (d) 已有 size="1024x1024" + 开关开启 → 不覆盖
func TestDecodeOpenAIImageOutputSizes_DoesNotOverrideExistingSize(t *testing.T) {
	b64 := makeTestPNG(t, 800, 600) // 真实是 800x600，但上游已声明 1024x1024
	result := &OpenAIForwardResult{
		ImageCount:        1,
		ImageOutputSizes:  []string{"1024x1024"},
		ImageOutputBase64: []string{b64},
	}
	g := &Group{Platform: PlatformOpenAI, ImageDecodeSizeOnRsp: true}
	DecodeOpenAIImageOutputSizes(result, g)
	require.Equal(t, []string{"1024x1024"}, result.ImageOutputSizes)
	require.False(t, result.imageSizeDecoded)
}

// (e) 损坏 b64 → 失败留空走默认（slot 仍是空字符串）
func TestDecodeOpenAIImageOutputSizes_CorruptB64FallsBack(t *testing.T) {
	result := &OpenAIForwardResult{
		ImageCount:        1,
		ImageOutputSizes:  []string{""},
		ImageOutputBase64: []string{"corrupt-base64-payload"},
	}
	g := &Group{Platform: PlatformOpenAI, ImageDecodeSizeOnRsp: true}
	DecodeOpenAIImageOutputSizes(result, g)
	// 失败留空 (空字符串)，下游 ResolveImageBillingSize 走默认档兜底
	require.Equal(t, []string{""}, result.ImageOutputSizes)
	require.False(t, result.imageSizeDecoded)
}

// (f) URL slot（b64 为空）→ 不解码不报错
func TestDecodeOpenAIImageOutputSizes_URLSlot_NoOp(t *testing.T) {
	result := &OpenAIForwardResult{
		ImageCount:        1,
		ImageOutputSizes:  []string{""},
		ImageOutputBase64: []string{""}, // URL 模式占位空串
	}
	g := &Group{Platform: PlatformOpenAI, ImageDecodeSizeOnRsp: true}
	DecodeOpenAIImageOutputSizes(result, g)
	require.Equal(t, []string{""}, result.ImageOutputSizes)
	require.False(t, result.imageSizeDecoded)
}

// (g) 非 openai 平台开关无效
func TestDecodeOpenAIImageOutputSizes_NonOpenAIPlatformDisabled(t *testing.T) {
	b64 := makeTestPNG(t, 1920, 1080)
	result := &OpenAIForwardResult{
		ImageCount:        1,
		ImageOutputSizes:  []string{""},
		ImageOutputBase64: []string{b64},
	}
	// fal 分组即便 ImageDecodeSizeOnRsp=true 也不应触发（ImageDecodeSizeOnRspEnabled 校验）
	g := &Group{Platform: "fal", ImageDecodeSizeOnRsp: true}
	DecodeOpenAIImageOutputSizes(result, g)
	require.Equal(t, []string{""}, result.ImageOutputSizes)
	require.False(t, result.imageSizeDecoded)
}

// nil group 安全
func TestDecodeOpenAIImageOutputSizes_NilGroupSafe(t *testing.T) {
	b64 := makeTestPNG(t, 1920, 1080)
	result := &OpenAIForwardResult{
		ImageCount:        1,
		ImageOutputSizes:  []string{""},
		ImageOutputBase64: []string{b64},
	}
	require.NotPanics(t, func() {
		DecodeOpenAIImageOutputSizes(result, nil)
	})
	require.Equal(t, []string{""}, result.ImageOutputSizes)
}

// (h) 解码后 size 进入 6 档归档与矩阵命中端到端：
// ApplyOpenAIImageBillingResolution 在解码后会基于真实 size 归档，并把 Source 标为 output_decoded。
func TestApplyOpenAIImageBillingResolution_WithDecodingEndToEnd(t *testing.T) {
	b64 := makeTestPNG(t, 1920, 1080)
	result := &OpenAIForwardResult{
		ImageCount:        1,
		ImageOutputSizes:  []string{""}, // 上游未返 size
		ImageOutputBase64: []string{b64},
	}
	g := &Group{Platform: PlatformOpenAI, ImageDecodeSizeOnRsp: true}
	ApplyOpenAIImageBillingResolution(result, g)
	// 1920x1080 命中 2K 档
	require.Equal(t, ImageBillingSize2K, result.ImageSize)
	// Source 应为 output_decoded（D9）
	require.Equal(t, ImageSizeSourceOutputDecoded, result.ImageSizeSource)
}

// 关闭开关的端到端：上游不返 size 时，应走默认 2K 档兜底，Source = default
func TestApplyOpenAIImageBillingResolution_DisabledFallsBackToDefault(t *testing.T) {
	b64 := makeTestPNG(t, 1920, 1080)
	result := &OpenAIForwardResult{
		ImageCount:        1,
		ImageOutputSizes:  []string{""},
		ImageOutputBase64: []string{b64},
	}
	g := &Group{Platform: PlatformOpenAI, ImageDecodeSizeOnRsp: false}
	ApplyOpenAIImageBillingResolution(result, g)
	require.Equal(t, ImageBillingSize2K, result.ImageSize)
	require.Equal(t, ImageSizeSourceDefault, result.ImageSizeSource)
}

// nil group 的端到端兼容性：旧调用语义保留（DecodeOpenAIImageOutputSizes nil-safe，
// ApplyOpenAIImageBillingResolution 不解码、走默认档）
func TestApplyOpenAIImageBillingResolution_NilGroupBackwardsCompat(t *testing.T) {
	result := &OpenAIForwardResult{
		ImageCount:       1,
		ImageOutputSizes: []string{""},
	}
	require.NotPanics(t, func() {
		ApplyOpenAIImageBillingResolution(result, nil)
	})
	require.Equal(t, ImageBillingSize2K, result.ImageSize)
	require.Equal(t, ImageSizeSourceDefault, result.ImageSizeSource)
}
