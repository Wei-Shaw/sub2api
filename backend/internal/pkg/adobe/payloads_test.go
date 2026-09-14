//go:build unit

package adobe

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSizeFromRatio(t *testing.T) {
	require.Equal(t, Size{2752, 1536}, SizeFromRatio("16:9", Resolution2K))
	require.Equal(t, Size{2048, 2048}, SizeFromRatio("1:1", Resolution2K))
	require.Equal(t, Size{1024, 1024}, SizeFromRatio("1:1", Resolution1K))
	require.Equal(t, Size{4096, 4096}, SizeFromRatio("1:1", Resolution4K))

	// 分辨率缺省时按 2K 处理。
	require.Equal(t, SizeFromRatio("16:9", Resolution2K), SizeFromRatio("16:9", ""))
	// 未知比例回退 16:9。
	require.Equal(t, SizeFromRatio("16:9", Resolution2K), SizeFromRatio("7:3", Resolution2K))
}

func TestGPTImagePixelsFromRatio(t *testing.T) {
	size, ok := GPTImagePixelsFromRatio("16:9", Resolution2K)
	require.True(t, ok)
	require.Equal(t, Size{2560, 1440}, size)

	// gpt-image 不支持 nano-banana2 的超长比例。
	_, ok = GPTImagePixelsFromRatio("1:8", Resolution2K)
	require.False(t, ok)
}

func TestGPTImageDetailLevelFromQuality(t *testing.T) {
	tests := map[string]int{
		"low":     1,
		"medium":  3,
		"high":    5,
		"":        1,
		"unknown": 1,
		// gpt-image-2.5 新增档位：上游只理解 1-5，取最高档。
		"xhigh": 5,
		"max":   5,
		"HIGH":  5,
		" max ": 5,
	}
	for quality, want := range tests {
		require.Equal(t, want, GPTImageDetailLevelFromQuality(quality), "quality=%q", quality)
	}
}

func TestBuildImagePayloadCandidatesGPTImageText2Image(t *testing.T) {
	candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
		Prompt:               "a cat",
		AspectRatio:          "16:9",
		OutputResolution:     Resolution2K,
		UpstreamModelID:      "gpt-image",
		UpstreamModelVersion: "2",
		QualityLevel:         "high",
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	p := candidates[0]
	require.Equal(t, "gpt-image", p["modelId"])
	require.Equal(t, "2", p["modelVersion"])
	require.Equal(t, "2560x1440", p["modelSpecificPayload"].(map[string]any)["size"])
	require.Equal(t, 5, p["generationSettings"].(map[string]any)["detailLevel"])
	require.Equal(t, Size{2560, 1440}, p["size"])
	require.Equal(t, "2K", p["outputResolution"])
	require.Equal(t, "text2image", p["generationMetadata"].(map[string]any)["module"])
	require.Empty(t, p["referenceBlobs"])
}

func TestBuildImagePayloadCandidatesGPTImageDetailLevelOverride(t *testing.T) {
	detail := 3
	candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
		Prompt:           "x",
		AspectRatio:      "1:1",
		OutputResolution: Resolution2K,
		UpstreamModelID:  "gpt-image",
		QualityLevel:     "high", // 显式 DetailLevel 优先
		DetailLevel:      &detail,
	})
	require.NoError(t, err)
	require.Equal(t, 3, candidates[0]["generationSettings"].(map[string]any)["detailLevel"])
}

func TestBuildImagePayloadCandidatesGPTImageEdit(t *testing.T) {
	candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
		Prompt:               "edit",
		AspectRatio:          "1:1",
		OutputResolution:     Resolution2K,
		UpstreamModelID:      "gpt-image",
		UpstreamModelVersion: "2",
		SourceImageIDs:       []string{"img1"},
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	p := candidates[0]
	// 实证：gpt-image edit 必须 usage=subject，否则 Adobe 400
	// "Image edit use case requires a reference image"。
	require.Equal(t, []any{map[string]any{"id": "img1", "usage": "subject"}}, p["referenceBlobs"])
	require.Equal(t, "image2image", p["generationMetadata"].(map[string]any)["module"])
	// Adobe 新 API 拒收 referenceImages，确保不再出现该字段。
	require.NotContains(t, p, "referenceImages")
	require.NotContains(t, p, "referenceVideos")
}

func TestBuildImagePayloadCandidatesGPTImageUnsupportedRatio(t *testing.T) {
	_, err := BuildImagePayloadCandidates(ImagePayloadOptions{
		Prompt:           "x",
		AspectRatio:      "1:8",
		OutputResolution: Resolution2K,
		UpstreamModelID:  "gpt-image",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported gpt-image ratio")
	require.False(t, IsRotatable(err))
}

func TestBuildImagePayloadCandidatesNanoBananaText2Image(t *testing.T) {
	candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
		Prompt:               "a dog",
		AspectRatio:          "9:16",
		OutputResolution:     Resolution2K,
		UpstreamModelID:      "gemini-flash",
		UpstreamModelVersion: "nano-banana-2",
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	p := candidates[0]
	require.Equal(t, Size{1536, 2752}, p["size"])
	msp := p["modelSpecificPayload"].(map[string]any)
	require.Equal(t, "9:16", msp["aspectRatio"])
	require.Equal(t, false, msp["parameters"].(map[string]any)["addWatermark"])
	require.Equal(t, []any{}, p["referenceBlobs"])
	require.Equal(t, false, p["groundSearch"])
	require.Equal(t, false, p["skipCai"])
}

// 没给比例（或给了 auto）时不应凭空补一个 aspectRatio。
func TestBuildImagePayloadCandidatesNanoBananaOmitsAspectRatio(t *testing.T) {
	for _, ratio := range []string{"", "auto", "  ", "AUTO"} {
		candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
			Prompt:          "x",
			AspectRatio:     ratio,
			UpstreamModelID: "gemini-flash",
		})
		require.NoError(t, err)
		msp := candidates[0]["modelSpecificPayload"].(map[string]any)
		require.NotContains(t, msp, "aspectRatio", "ratio=%q 不应带 aspectRatio", ratio)
	}
}

func TestBuildImagePayloadCandidatesNanoBananaEdit(t *testing.T) {
	candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
		Prompt:           "edit",
		AspectRatio:      "1:1",
		OutputResolution: Resolution2K,
		UpstreamModelID:  "gemini-flash",
		SourceImageIDs:   []string{"a", "b"},
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	p := candidates[0]
	require.Equal(t, "image2image", p["generationMetadata"].(map[string]any)["module"])
	// nano-banana 与 gpt-image 恰好相反：这里必须是 general，用 subject 会 400
	// "Only general reference images are supported for Google Nano-Banana"。
	require.Equal(t, []any{
		map[string]any{"id": "a", "usage": "general"},
		map[string]any{"id": "b", "usage": "general"},
	}, p["referenceBlobs"])
}

// background 是 OpenAI 原生参数，随 size 一起走 modelSpecificPayload 透传。
// 生产实测：不传时上游返回无 alpha 通道的 RGB，传 transparent 返回带真透明像素的 RGBA。
func TestBuildImagePayloadCandidatesBackground(t *testing.T) {
	build := func(background string, sourceIDs []string) map[string]any {
		t.Helper()
		candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
			Prompt:           "x",
			AspectRatio:      "1:1",
			OutputResolution: Resolution2K,
			UpstreamModelID:  "gpt-image",
			Background:       background,
			SourceImageIDs:   sourceIDs,
		})
		require.NoError(t, err)
		return candidates[0]["modelSpecificPayload"].(map[string]any)
	}

	t.Run("transparent 与 opaque 落进 modelSpecificPayload", func(t *testing.T) {
		require.Equal(t, "transparent", build("transparent", nil)["background"])
		require.Equal(t, "opaque", build("opaque", nil)["background"])
		require.Equal(t, "transparent", build("  TRANSPARENT  ", nil)["background"])
	})

	t.Run("空值与 auto 一个字段都不发", func(t *testing.T) {
		// 老链路 payload 必须字节级不变，故这里断言 modelSpecificPayload 只有 size。
		for _, background := range []string{"", "auto", "  ", "AUTO"} {
			msp := build(background, nil)
			require.NotContains(t, msp, "background", "background=%q 不应写入字段", background)
			require.Len(t, msp, 1, "background=%q 时 modelSpecificPayload 应只有 size", background)
		}
	})

	t.Run("图生图候选同样带上 background", func(t *testing.T) {
		require.Equal(t, "transparent", build("transparent", []string{"img1"})["background"])
	})

	t.Run("非 gpt-image 家族不注入", func(t *testing.T) {
		candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
			Prompt:          "x",
			AspectRatio:     "1:1",
			UpstreamModelID: "gemini-flash",
			Background:      "transparent",
		})
		require.NoError(t, err)
		msp := candidates[0]["modelSpecificPayload"].(map[string]any)
		require.NotContains(t, msp, "background")
	})
}

// ---- 视频 ----

func baseVideoOptions() VideoPayloadOptions {
	return VideoPayloadOptions{
		Prompt:               "a cat surfing",
		UpstreamModel:        "openai:firefly:colligo:sora2",
		UpstreamModelID:      "sora",
		UpstreamModelVersion: "sora-2",
		Engine:               EngineSora2,
		Duration:             8,
		AspectRatio:          "16:9",
		Size:                 Size{1280, 720},
		GenerateAudio:        false,
	}
}

func TestBuildVideoPayloadSoraText2Video(t *testing.T) {
	opts := baseVideoOptions()
	opts.NegativePrompt = "blurry"
	p := BuildVideoPayload(opts)

	require.Equal(t, "sora", p["modelId"])
	require.Equal(t, "sora-2", p["modelVersion"])
	require.Equal(t, "openai:firefly:colligo:sora2", p["model"])
	require.Equal(t, 8, p["duration"])
	require.Equal(t, 24, p["fps"])
	require.Equal(t, Size{1280, 720}, p["size"])
	require.Equal(t, false, p["generateAudio"])
	require.Equal(t, map[string]any{"module": "text2video"}, p["generationMetadata"])
	require.Equal(t, "blurry", p["negativePrompt"])
	require.Equal(t, map[string]any{"storeInputs": true}, p["output"])
	require.Equal(t, []any{}, p["referenceBlobs"])
	require.Equal(t, []any{}, p["referenceFrames"])
	require.NotContains(t, p, "engine")

	// sora 的 prompt 是一段序列化后的 JSON，而非裸文本。
	var prompt map[string]any
	require.NoError(t, json.Unmarshal([]byte(p["prompt"].(string)), &prompt))
	require.Equal(t, map[string]any{
		"id":              float64(1),
		"duration_sec":    float64(8),
		"prompt_text":     "a cat surfing",
		"negative_prompt": "blurry",
	}, prompt)
}

func TestBuildVideoPayloadSoraOmitsEmptyNegativePrompt(t *testing.T) {
	p := BuildVideoPayload(baseVideoOptions())
	var prompt map[string]any
	require.NoError(t, json.Unmarshal([]byte(p["prompt"].(string)), &prompt))
	require.NotContains(t, prompt, "negative_prompt")
}

// sora 只吃首帧，且即便带图也保持 text2video module。
func TestBuildVideoPayloadSoraImage2Video(t *testing.T) {
	opts := baseVideoOptions()
	opts.SourceImageIDs = []string{"img-a", "img-b"}
	p := BuildVideoPayload(opts)

	require.Equal(t, map[string]any{"module": "text2video"}, p["generationMetadata"])
	require.Equal(t, []any{
		map[string]any{"id": "img-a", "usage": "general", "promptReference": 1},
	}, p["referenceBlobs"])
	require.Equal(t, []any{map[string]any{"localBlobRef": "img-a"}, nil}, p["referenceFrames"])
}

func TestBuildVideoPayloadVeoStandard(t *testing.T) {
	opts := baseVideoOptions()
	opts.Engine = EngineVeo31Standard
	opts.UpstreamModelID = "veo"
	opts.UpstreamModelVersion = "3.1-generate"
	opts.Duration = 6
	opts.SourceImageIDs = []string{"v1", "v2", "ignored"}
	p := BuildVideoPayload(opts)

	require.Equal(t, "veo", p["modelId"])
	require.Equal(t, "3.1-generate", p["modelVersion"])
	require.Equal(t, map[string]any{"module": "text2video"}, p["generationMetadata"])
	require.Equal(t, map[string]any{
		"parameters": map[string]any{
			"durationSeconds": 6,
			"aspectRatio":     "16:9",
			"addWaterMark":    false,
		},
	}, p["modelSpecificPayload"])
	// veo 的时长在 modelSpecificPayload 里，顶层不带 duration。
	require.NotContains(t, p, "duration")
	require.Equal(t, []any{
		map[string]any{"id": "v1", "usage": "general", "promptReference": 1},
		map[string]any{"id": "v2", "usage": "general", "promptReference": 2},
	}, p["referenceBlobs"])
}

func TestBuildVideoPayloadVeoReferenceMode(t *testing.T) {
	opts := baseVideoOptions()
	opts.Engine = EngineVeo31Standard
	opts.ReferenceMode = ReferenceModeImage
	opts.SourceImageIDs = []string{"v1", "v2", "v3", "ignored"}
	p := BuildVideoPayload(opts)

	// 参考图模式最多三张，且是独立素材（usage=asset，无 promptReference）。
	require.Equal(t, []any{
		map[string]any{"id": "v1", "usage": "asset"},
		map[string]any{"id": "v2", "usage": "asset"},
		map[string]any{"id": "v3", "usage": "asset"},
	}, p["referenceBlobs"])
	require.NotContains(t, p, "reference_mode")
}

func TestBuildVideoPayloadVeoFast(t *testing.T) {
	opts := baseVideoOptions()
	opts.Engine = EngineVeo31Fast
	opts.SourceImageIDs = []string{"first", "last"}
	p := BuildVideoPayload(opts)

	require.Equal(t, "3.1-fast-generate", p["modelVersion"])
	require.Equal(t, []any{
		map[string]any{"id": "first", "usage": "general", "promptReference": 1},
		map[string]any{"id": "last", "usage": "general", "promptReference": 2},
	}, p["referenceBlobs"])
}

// veo31-fast 即使显式传了参考图模式也走 general——参考图模式只对 standard 生效。
func TestBuildVideoPayloadVeoFastIgnoresReferenceMode(t *testing.T) {
	opts := baseVideoOptions()
	opts.Engine = EngineVeo31Fast
	opts.ReferenceMode = ReferenceModeImage
	opts.SourceImageIDs = []string{"a", "b", "c"}
	p := BuildVideoPayload(opts)

	blobs := p["referenceBlobs"].([]any)
	require.Len(t, blobs, 2)
	require.Equal(t, "general", blobs[0].(map[string]any)["usage"])
}

func TestBuildVideoPayloadKling(t *testing.T) {
	tests := []struct {
		engine       string
		modelVersion string
	}{
		{EngineKlingO3, "kling_o3_pro_reference_to_video"},
		{EngineKling3, "kling_v3_standard_i2v"},
	}
	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			opts := baseVideoOptions()
			opts.Engine = tt.engine
			opts.UpstreamModelID = "kling"
			opts.AspectRatio = "9:16"
			opts.Size = Size{720, 1280}
			opts.SourceImageIDs = []string{"k1", "k2", "ignored"}
			p := BuildVideoPayload(opts)

			require.Equal(t, "kling", p["modelId"])
			require.Equal(t, tt.modelVersion, p["modelVersion"])
			require.Equal(t, map[string]any{"module": "image2video"}, p["generationMetadata"])
			require.Equal(t, map[string]any{"aspectRatio": "9:16"}, p["generationSettings"])
			require.Equal(t, map[string]any{"storeInputs": true}, p["output"])
			// 帧序号从 1 开始。
			require.Equal(t, []any{
				map[string]any{"id": "k1", "usage": "frame", "order": 1},
				map[string]any{"id": "k2", "usage": "frame", "order": 2},
			}, p["referenceBlobs"])
		})
	}
}

func TestBuildVideoPayloadKlingText2Video(t *testing.T) {
	opts := baseVideoOptions()
	opts.Engine = EngineKling3
	p := BuildVideoPayload(opts)
	require.Equal(t, map[string]any{"module": "text2video"}, p["generationMetadata"])
	require.Equal(t, []any{}, p["referenceBlobs"])
}

// 空 id 会被过滤掉，不能占掉参考图名额。
func TestBuildVideoPayloadDropsEmptySourceIDs(t *testing.T) {
	opts := baseVideoOptions()
	opts.Engine = EngineVeo31Fast
	opts.SourceImageIDs = []string{"", "  ", "real"}
	p := BuildVideoPayload(opts)

	require.Equal(t, []any{
		map[string]any{"id": "real", "usage": "general", "promptReference": 1},
	}, p["referenceBlobs"])
}

// 所有 payload 都必须能被 json.Marshal——上游只收 JSON。
func TestPayloadsAreJSONSerializable(t *testing.T) {
	candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
		Prompt:          "x",
		AspectRatio:     "1:1",
		UpstreamModelID: "gpt-image",
		Background:      "transparent",
		SourceImageIDs:  []string{"a"},
	})
	require.NoError(t, err)
	raw, err := json.Marshal(candidates[0])
	require.NoError(t, err)
	require.Contains(t, string(raw), `"background":"transparent"`)
	require.Contains(t, string(raw), `"size":{"width":`)

	for _, engine := range []string{EngineSora2, EngineVeo31Fast, EngineVeo31Standard, EngineKlingO3, EngineKling3} {
		opts := baseVideoOptions()
		opts.Engine = engine
		_, err := json.Marshal(BuildVideoPayload(opts))
		require.NoError(t, err, "engine=%s", engine)
	}
}

// Step 7：gpt-image v2.5 与 v2 的 payload 差异（Firefly UI 2:3 抓包为金标准）。
func TestBuildImagePayloadCandidatesGPTImage25(t *testing.T) {
	candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
		Prompt:               "a cat",
		UpstreamModelID:      "gpt-image",
		UpstreamModelVersion: "gpt-image-2.5-flare",
		PayloadKind:          PayloadKindGPTImage25,
		QualityLevel:         "high",
		SizePixels:           Size{Width: 1024, Height: 1536},
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	p := candidates[0]

	require.NotContains(t, p, "outputResolution", "v2.5 不发 outputResolution")
	require.Equal(t, Size{Width: 1024, Height: 1536}, p["size"],
		"有 WxH 时必须发顶层 size（抓包 1024x1536）")
	msp := p["modelSpecificPayload"].(map[string]any)
	require.NotContains(t, msp, "size", "v2.5 的 modelSpecificPayload 不得再写 size:auto")
	require.Equal(t, 2, p["caiClaimVersion"], "v2.5 必须带 caiClaimVersion:2")

	require.Equal(t, "gpt-image-2.5-flare", p["modelVersion"])
	require.Equal(t, 5, p["generationSettings"].(map[string]any)["detailLevel"])

	raw, err := json.Marshal(p)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"size":{"width":1024,"height":1536}`)
	require.Contains(t, string(raw), `"caiClaimVersion":2`)
	require.NotContains(t, string(raw), `"outputResolution"`)
	require.NotContains(t, string(raw), `"size":"auto"`)
}

func TestBuildImagePayloadCandidatesGPTImage25PassesThrough4K(t *testing.T) {
	candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
		Prompt:               "a cat",
		UpstreamModelID:      "gpt-image",
		UpstreamModelVersion: "gpt-image-2.5-flare",
		PayloadKind:          PayloadKindGPTImage25,
		SizePixels:           Size{Width: 3840, Height: 2160},
	})
	require.NoError(t, err)
	require.Equal(t, Size{Width: 3840, Height: 2160}, candidates[0]["size"],
		"4K 必须原样发给上游，不得夹成 1024/1536")
}

func TestBuildImagePayloadCandidatesGPTImage25OmitsSizeWhenEmpty(t *testing.T) {
	candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
		Prompt:               "a cat",
		UpstreamModelID:      "gpt-image",
		UpstreamModelVersion: "gpt-image-2.5-flare",
		PayloadKind:          PayloadKindGPTImage25,
	})
	require.NoError(t, err)
	require.NotContains(t, candidates[0], "size")
	msp := candidates[0]["modelSpecificPayload"].(map[string]any)
	require.NotContains(t, msp, "size")
}

// 图生图时 v2.5 走 module=image2image + usage=subject（与 v2 保持一致）。
func TestBuildImagePayloadCandidatesGPTImage25Edit(t *testing.T) {
	candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
		Prompt:               "edit",
		UpstreamModelID:      "gpt-image",
		UpstreamModelVersion: "gpt-image-2.5-prism",
		PayloadKind:          PayloadKindGPTImage25,
		SourceImageIDs:       []string{"abc"},
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, "image2image", candidates[0]["generationMetadata"].(map[string]any)["module"])
	require.Equal(t, []any{map[string]any{"id": "abc", "usage": "subject"}},
		candidates[0]["referenceBlobs"])
}

// v2.5 的 background 也走 modelSpecificPayload，透传规则与 v2 一致。
func TestBuildImagePayloadCandidatesGPTImage25Background(t *testing.T) {
	candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
		Prompt:               "x",
		UpstreamModelID:      "gpt-image",
		UpstreamModelVersion: "gpt-image-2.5-flare",
		PayloadKind:          PayloadKindGPTImage25,
		Background:           "transparent",
	})
	require.NoError(t, err)
	msp := candidates[0]["modelSpecificPayload"].(map[string]any)
	require.NotContains(t, msp, "size")
	require.Equal(t, "transparent", msp["background"])
}

// enum-size 家族：顶层 size:{width,height} + 无 outputResolution / modelSpecificPayload。
func TestBuildImagePayloadCandidatesSizeEnum(t *testing.T) {
	candidates, err := BuildImagePayloadCandidates(ImagePayloadOptions{
		Prompt:               "landscape",
		UpstreamModelID:      "flux",
		UpstreamModelVersion: "fluxPro",
		PayloadKind:          PayloadKindSizeEnum,
		SizePixels:           Size{1024, 768},
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	p := candidates[0]

	require.Equal(t, Size{1024, 768}, p["size"], "顶层 size 必须是 {width,height}")
	require.NotContains(t, p, "outputResolution")
	require.NotContains(t, p, "modelSpecificPayload")
	require.Equal(t, "fluxPro", p["modelVersion"])
	require.Equal(t, "flux", p["modelId"])
}

// enum-size 家族没有 SizePixels 就应报错——catalog 层没挑好尺寸的 bug 不该被
// 静默吞掉（那会让上游收到「size 缺失」的 400，比在 Go 侧早报错难排查得多）。
func TestBuildImagePayloadCandidatesSizeEnumRequiresPixels(t *testing.T) {
	_, err := BuildImagePayloadCandidates(ImagePayloadOptions{
		Prompt:               "x",
		UpstreamModelID:      "flux",
		UpstreamModelVersion: "fluxPro",
		PayloadKind:          PayloadKindSizeEnum,
		// SizePixels 未设
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "SizePixels")
}
