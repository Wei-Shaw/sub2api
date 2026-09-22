package adobe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// timeNow 是可替换的时钟，供测试固定 seed。
var timeNow = time.Now

// Size 是像素宽高，直接作为 payload.size 序列化。
type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// String 返回 gpt-image 的宽高字面量（"宽x高"）。
func (s Size) String() string { return fmt.Sprintf("%dx%d", s.Width, s.Height) }

// GPTImageDetailLevelFromQuality 把 OpenAI 的 quality 档位映射成上游 detailLevel。
//
// 未指定模型版本时按 v2 / 1.5 的上限 5。gpt-image-2.5 的 schema 是 1-7，
// 用 GPTImageDetailLevelFromQualityForVersion。
func GPTImageDetailLevelFromQuality(qualityLevel string) int {
	return gptImageDetailLevelFromQuality(qualityLevel, 5)
}

// GPTImageDetailLevelFromQualityForVersion 按上游 modelVersion 选择 detailLevel 上限：
// gpt-image-2.5-* 为 7，其余为 5。
func GPTImageDetailLevelFromQualityForVersion(qualityLevel, modelVersion string) int {
	return gptImageDetailLevelFromQuality(qualityLevel, gptImageDetailLevelCap(modelVersion))
}

func gptImageDetailLevelCap(modelVersion string) int {
	if strings.Contains(strings.ToLower(modelVersion), "gpt-image-2.5") {
		return 7
	}
	return 5
}

func gptImageDetailLevelFromQuality(qualityLevel string, max int) int {
	switch strings.ToLower(strings.TrimSpace(qualityLevel)) {
	case "high":
		if max < 5 {
			return max
		}
		return 5
	case "xhigh", "max":
		return max
	case "medium":
		return 3
	default:
		return 1
	}
}


// seedNow 生成提交用的随机种子（复刻上游前端的取值方式）。
func seedNow() int { return int(timeNow().Unix() % 999999) }

// SeedNow 是未显式指定 ImagePayloadOptions.Seed 时使用的 Firefly 风格种子。
// 同秒并发出图应传 SeedNow()+i，避免撞种。
func SeedNow() int { return seedNow() }

func payloadSeed(opts ImagePayloadOptions) int {
	if opts.Seed != nil {
		return *opts.Seed
	}
	return seedNow()
}

// ImagePayloadOptions 是构造图像提交体所需的输入。
type ImagePayloadOptions struct {
	Prompt               string
	AspectRatio          string
	OutputResolution     OutputResolution
	UpstreamModelID      string
	UpstreamModelVersion string
	// PayloadKind 决定用哪套构造器。零值回落到「按 UpstreamModelID 猜」：
	// gpt-image 走自由 WxH 构造器，其余走 banana。
	PayloadKind PayloadKind
	// SizePixels：gpt-image 2/2.5 是请求 WxH 原样；1.5 / enum-size 是 catalog
	// NearestSize 的结果；banana 是档位方图。payload 层不再做二次像素决策。
	// 零值表示省略顶层 size（对齐 UI「自动」）。
	SizePixels Size
	// QualityLevel 是 OpenAI 的 quality；DetailLevel 为 nil 时由它推导。
	QualityLevel string
	// DetailLevel 显式指定上游 detailLevel，优先于 QualityLevel。
	DetailLevel *int
	// SourceImageIDs 是已上传的参考图 id，非空即走图生图。
	SourceImageIDs []string
	// Edit 为 true 表示来自 /v1/images/edits。Firefly Image Edit 抓包走
	// submodule=ff-image-editor；Generate 页参考图仍是 ff-image-generate。
	Edit bool
	// Background 是 OpenAI 的 background 参数（transparent / opaque / auto）。
	// gpt-image 只把非 auto 的 background 塞进 modelSpecificPayload。
	Background string
	// Seed 写入 payload 的 seeds[0]。未设时用 SeedNow()。
	Seed *int
}

// BuildImagePayloadCandidates 构造 /v2/3p-images/generate-async 的请求体候选列表。
//
// 返回的是「按尝试顺序排列的候选」而非单个 payload：Firefly 对不同子模块/图生图形态
// 接受的 payload 形状不同，上游用「依次尝试、命中 200 即停」兜住 schema 漂移。
// 调用方应逐个 POST 直到 200，不要只发第一个。
func BuildImagePayloadCandidates(opts ImagePayloadOptions) ([]map[string]any, error) {
	normalizedRatio := strings.ToLower(strings.TrimSpace(opts.AspectRatio))

	switch opts.PayloadKind {
	case PayloadKindGPTImage25, PayloadKindGPTImage:
		return buildGPTImage25Payloads(opts)
	case PayloadKindSizeEnum:
		return buildSizeEnumPayloads(opts)
	case PayloadKindNanoBanana:
		return buildNanoBananaPayloads(opts, normalizedRatio), nil
	}

	// 兼容未设 PayloadKind 的调用方：按 UpstreamModelID 猜。
	if strings.EqualFold(strings.TrimSpace(opts.UpstreamModelID), upstreamModelIDGPTImage) {
		return buildGPTImage25Payloads(opts)
	}
	return buildNanoBananaPayloads(opts, normalizedRatio), nil
}

// buildGPTImage25Payloads 是 gpt-image v1.5 / v2 / v2.5 的共用 payload。
//
// 抓包（Firefly UI gpt-image/2，2026-09-20 FREE/收费默认生图一致）：
//   - 不发 outputResolution
//   - 有合法 WxH 时发顶层 size:{width,height}（含 4K，不夹紧）
//   - 无顶层 size 时写 modelSpecificPayload.size:"auto"（v2 与 v2.5 相同）
//   - caiClaimVersion:2
func buildGPTImage25Payloads(opts ImagePayloadOptions) ([]map[string]any, error) {
	detailLevel := GPTImageDetailLevelFromQualityForVersion(opts.QualityLevel, opts.UpstreamModelVersion)
	if opts.DetailLevel != nil {
		detailLevel = *opts.DetailLevel
	}

	modelSpecific := map[string]any{}
	if bg := strings.ToLower(strings.TrimSpace(opts.Background)); bg != "" && bg != "auto" {
		modelSpecific["background"] = bg
	}

	hasPixels := opts.SizePixels.Width > 0 && opts.SizePixels.Height > 0
	if !hasPixels {
		modelSpecific["size"] = "auto"
	}

	base := map[string]any{
		"modelId":              opts.UpstreamModelID,
		"modelVersion":         opts.UpstreamModelVersion,
		"n":                    1,
		"prompt":               opts.Prompt,
		"seeds":                []int{payloadSeed(opts)},
		"output":               map[string]any{"storeInputs": true},
		"referenceBlobs":       []any{},
		"generationMetadata":   imageGenerationMetadata(opts),
		"modelSpecificPayload": modelSpecific,
		"generationSettings":   map[string]any{"detailLevel": detailLevel},
		"caiClaimVersion":      2,
	}
	if hasPixels {
		base["size"] = opts.SizePixels
	}

	if len(opts.SourceImageIDs) == 0 {
		return []map[string]any{base}, nil
	}

	// 图生图：抓包一律 text2image；gpt-image 参考图 usage=subject（general 会 400
	// "Image edit use case requires a reference image"）。
	edited := clonePayload(base)
	edited["referenceBlobs"] = referenceBlobs(opts.SourceImageIDs, "subject")
	return []map[string]any{edited}, nil
}

// buildSizeEnumPayloads 是 flux / imagen / gpt-4o-image / runway-gen4-image 这四类
// 家族的通用 payload。这些家族的上游 schema 都是「顶层 size:{width,height} 从枚举里选、
// 无 outputResolution / modelSpecificPayload / aspectRatio」，共用一套构造。
//
// SizePixels 必须由 catalog 层的 NearestSize 挑好——payload 层不该有第二个像素决策。
func buildSizeEnumPayloads(opts ImagePayloadOptions) ([]map[string]any, error) {
	if opts.SizePixels.Width <= 0 || opts.SizePixels.Height <= 0 {
		return nil, NewRequestError(
			fmt.Sprintf("enum-size family %q requires SizePixels", opts.UpstreamModelID))
	}
	base := map[string]any{
		"modelId":            opts.UpstreamModelID,
		"modelVersion":       opts.UpstreamModelVersion,
		"n":                  1,
		"prompt":             opts.Prompt,
		"seeds":              []int{payloadSeed(opts)},
		"output":             map[string]any{"storeInputs": true},
		"referenceBlobs":     []any{},
		"generationMetadata": imageGenerationMetadata(opts),
		"size":               opts.SizePixels,
	}
	if len(opts.SourceImageIDs) == 0 {
		return []map[string]any{base}, nil
	}
	// enum-size 家族的图生图形态多样，抓包未覆盖独立 edit——generationMetadata
	// 跟 gpt-image / banana，usage 仍沿用 subject。若上游拒收，逐个家族再补 candidate。
	edited := clonePayload(base)
	edited["referenceBlobs"] = referenceBlobs(opts.SourceImageIDs, "subject")
	return []map[string]any{edited}, nil
}

func buildNanoBananaPayloads(opts ImagePayloadOptions, normalizedRatio string) []map[string]any {
	modelSpecific := map[string]any{
		"parameters": map[string]any{"addWatermark": false},
	}
	// 空 / auto / 1:1 不发 aspectRatio：Firefly 默认方图且 schema 默认 1:1。
	if normalizedRatio != "" && normalizedRatio != "auto" && normalizedRatio != "1:1" {
		modelSpecific["aspectRatio"] = normalizedRatio
	}

	pixels := opts.SizePixels
	if pixels.Width <= 0 || pixels.Height <= 0 {
		pixels = SquareFromResolution(opts.OutputResolution)
	}

	base := map[string]any{
		"modelId":              opts.UpstreamModelID,
		"modelVersion":         opts.UpstreamModelVersion,
		"n":                    1,
		"prompt":               opts.Prompt,
		"size":                 pixels,
		"seeds":                []int{payloadSeed(opts)},
		"groundSearch":         false,
		"caiClaimVersion":      2,
		"output":               map[string]any{"storeInputs": true},
		"generationMetadata":   imageGenerationMetadata(opts),
		"modelSpecificPayload": modelSpecific,
	}

	if len(opts.SourceImageIDs) == 0 {
		base["referenceBlobs"] = []any{}
		return []map[string]any{base}
	}

	// nano-banana(Google) 图生图：usage 必须是 "general"——经实证，nano-banana 用
	// "subject" 会 400 "Only general reference images are supported for Google
	// Nano-Banana"；与 gpt-image 恰好相反，故两族不可共用同一 usage。
	// 不要发 usage=mask：discovery 与 Image Edit 抓包都只有一张 general 图。
	edited := clonePayload(base)
	edited["referenceBlobs"] = referenceBlobs(opts.SourceImageIDs, "general")
	return []map[string]any{edited}
}

// imageGenerationMetadata 对齐 Firefly Web 抓包：有无参考图都是 module=text2image；
// /v1/images/edits 用 submodule=ff-image-editor，其余用 ff-image-generate。
func imageGenerationMetadata(opts ImagePayloadOptions) map[string]any {
	submodule := "ff-image-generate"
	if opts.Edit && len(opts.SourceImageIDs) > 0 {
		submodule = "ff-image-editor"
	}
	return map[string]any{"module": "text2image", "submodule": submodule}
}

func referenceBlobs(ids []string, usage string) []any {
	blobs := make([]any, 0, len(ids))
	for _, id := range ids {
		blobs = append(blobs, map[string]any{"id": id, "usage": usage})
	}
	return blobs
}

// clonePayload 做一层浅拷贝，复刻 TS 的对象展开语义。
// 调用方只会整体替换顶层键，不会就地改嵌套值，故浅拷贝足够。
func clonePayload(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// 视频引擎标识。不同供应商的视频协议并不共用同一种 payload 形状。
const (
	EngineSora2         = "sora2"
	EngineVeo31Fast     = "veo31-fast"
	EngineVeo31Standard = "veo31-standard"
	EngineKlingO3       = "kling-o3"
	EngineKling3        = "kling3"
)

// ReferenceModeImage 是 veo31 的参考图模式标识。
const ReferenceModeImage = "image"

// VideoPayloadOptions 是构造视频提交体所需的输入。
type VideoPayloadOptions struct {
	Prompt               string
	UpstreamModel        string
	UpstreamModelID      string
	UpstreamModelVersion string
	Engine               string
	Duration             int
	AspectRatio          string
	Size                 Size
	GenerateAudio        bool
	// ReferenceMode 为 ReferenceModeImage 时 veo31 走参考图模式。
	ReferenceMode string
	// NegativePrompt 仅 sora 系与 veo31 使用。
	NegativePrompt string
	// SourceImageIDs 是已上传的输入图 id（首帧/尾帧/参考）。
	SourceImageIDs []string
}

// BuildVideoPayload 构造 /v2/3p-videos/generate-async 的请求体。
//
// 与图像不同，视频不做多候选：各引擎的 payload 形状差异是确定的，按 Engine 分派即可。
func BuildVideoPayload(opts VideoPayloadOptions) map[string]any {
	seed := seedNow()
	ids := nonEmpty(opts.SourceImageIDs)

	switch opts.Engine {
	case EngineVeo31Fast, EngineVeo31Standard:
		return buildVeoVideoPayload(opts, seed, ids)
	case EngineKlingO3, EngineKling3:
		return buildKlingVideoPayload(opts, seed, ids)
	default:
		return buildSoraVideoPayload(opts, seed, ids)
	}
}

func buildVeoVideoPayload(opts VideoPayloadOptions, seed int, ids []string) map[string]any {
	modelVersion := "3.1-generate"
	if opts.Engine == EngineVeo31Fast {
		modelVersion = "3.1-fast-generate"
	}

	// 参考图模式下每张图是独立素材（最多 3 张）；普通模式下是按序号绑定到 prompt 的
	// 首尾帧（最多 2 张）。
	var blobs []any
	if opts.Engine == EngineVeo31Standard && opts.ReferenceMode == ReferenceModeImage {
		blobs = referenceBlobs(head(ids, 3), "asset")
	} else {
		blobs = make([]any, 0, 2)
		for i, id := range head(ids, 2) {
			blobs = append(blobs, map[string]any{
				"id":              id,
				"usage":           "general",
				"promptReference": i + 1,
			})
		}
	}

	return map[string]any{
		"n":                  1,
		"seeds":              []int{seed},
		"modelId":            "veo",
		"modelVersion":       modelVersion,
		"output":             map[string]any{"storeInputs": true},
		"prompt":             opts.Prompt,
		"size":               opts.Size,
		"generateAudio":      opts.GenerateAudio,
		"referenceBlobs":     blobs,
		"generationMetadata": map[string]any{"module": "text2video"},
		"modelSpecificPayload": map[string]any{
			"parameters": map[string]any{
				"durationSeconds": opts.Duration,
				"aspectRatio":     opts.AspectRatio,
				"addWaterMark":    false,
			},
		},
	}
}

func buildKlingVideoPayload(opts VideoPayloadOptions, seed int, ids []string) map[string]any {
	modelVersion := "kling_v3_standard_i2v"
	if opts.Engine == EngineKlingO3 {
		modelVersion = "kling_o3_pro_reference_to_video"
	}

	module := "text2video"
	if len(ids) > 0 {
		module = "image2video"
	}

	blobs := make([]any, 0, 2)
	for i, id := range head(ids, 2) {
		blobs = append(blobs, map[string]any{"id": id, "usage": "frame", "order": i + 1})
	}

	return map[string]any{
		"n":                  1,
		"seeds":              []int{seed},
		"modelId":            "kling",
		"modelVersion":       modelVersion,
		"output":             map[string]any{"storeInputs": true},
		"prompt":             opts.Prompt,
		"size":               opts.Size,
		"generateAudio":      opts.GenerateAudio,
		"generationMetadata": map[string]any{"module": module},
		"duration":           opts.Duration,
		"generationSettings": map[string]any{"aspectRatio": opts.AspectRatio},
		"referenceBlobs":     blobs,
	}
}

// soraPrompt 是 sora 系把 prompt 再包一层 JSON 的内层结构；字段顺序需与上游一致。
type soraPrompt struct {
	ID             int    `json:"id"`
	DurationSec    int    `json:"duration_sec"`
	PromptText     string `json:"prompt_text"`
	NegativePrompt string `json:"negative_prompt,omitempty"`
}

func buildSoraVideoPayload(opts VideoPayloadOptions, seed int, ids []string) map[string]any {
	// sora 的 prompt 是一段序列化后的 JSON，而非裸文本。
	promptJSON, err := marshalJSON(soraPrompt{
		ID:             1,
		DurationSec:    opts.Duration,
		PromptText:     opts.Prompt,
		NegativePrompt: opts.NegativePrompt,
	})
	if err != nil {
		promptJSON = ""
	}

	blobs := []any{}
	frames := []any{}
	if len(ids) > 0 {
		firstID := ids[0]
		blobs = []any{map[string]any{"id": firstID, "usage": "general", "promptReference": 1}}
		frames = []any{map[string]any{"localBlobRef": firstID}, nil}
	}

	return map[string]any{
		"n":                     1,
		"seeds":                 []int{seed},
		"modelId":               "sora",
		"modelVersion":          "sora-2",
		"size":                  opts.Size,
		"duration":              opts.Duration,
		"fps":                   24,
		"prompt":                promptJSON,
		"generationMetadata":    map[string]any{"module": "text2video"},
		"model":                 opts.UpstreamModel,
		"generateAudio":         opts.GenerateAudio,
		"generateLoop":          false,
		"transparentBackground": false,
		"seed":                  fmt.Sprintf("%d", seed),
		"locale":                "en-US",
		"camera": map[string]any{
			"angle":       "none",
			"shotSize":    "none",
			"motion":      nil,
			"promptStyle": nil,
		},
		"negativePrompt":             opts.NegativePrompt,
		"jobMode":                    "standard",
		"debugGenerationEndpoint":    "",
		"referenceBlobs":             blobs,
		"referenceFrames":            frames,
		"referenceVideo":             nil,
		"cameraMotionReferenceVideo": nil,
		"characterReference":         nil,
		"editReferenceVideo":         nil,
		"output":                     map[string]any{"storeInputs": true},
	}
}

func nonEmpty(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			out = append(out, id)
		}
	}
	return out
}

func head(ids []string, n int) []string {
	if len(ids) > n {
		return ids[:n]
	}
	return ids
}

// marshalJSON 是 encoding/json 的薄封装，单列出来是为了让 sora prompt 的构造保持可读。
func marshalJSON(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// payloadKeyOrder 是 generate-async 顶层键的发送顺序。
//
// 对齐 Firefly 前端 2026-09-20 抓包（FREE/收费默认生图相同）：
// n, seeds, output, prompt, referenceBlobs, caiClaimVersion, modelSpecificPayload,
// modelId, modelVersion, generationMetadata, generationSettings。
// size 有像素时插在 prompt 之后。其余键（视频/banana 扩展）按字母序接在后面，
// 避免 Go map 的 json.Marshal 把整份 body 排成字典序。
var payloadKeyOrder = []string{
	"n", "seeds", "output", "prompt", "size", "referenceBlobs",
	"caiClaimVersion", "modelSpecificPayload", "modelId", "modelVersion",
	"generationMetadata", "generationSettings",
	"groundSearch", "duration", "fps", "model", "generateAudio",
	"generateLoop", "transparentBackground", "seed", "locale", "camera",
	"negativePrompt", "jobMode", "debugGenerationEndpoint",
	"referenceFrames", "referenceVideo", "cameraMotionReferenceVideo",
	"characterReference", "editReferenceVideo",
}

// marshalPayloadJSON 按 payloadKeyOrder 序列化提交体。
func marshalPayloadJSON(payload map[string]any) ([]byte, error) {
	if payload == nil {
		return []byte("null"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	written := make(map[string]bool, len(payload))
	first := true
	write := func(key string) error {
		val, ok := payload[key]
		if !ok || written[key] {
			return nil
		}
		written[key] = true
		if !first {
			buf.WriteByte(',')
		}
		first = false
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return err
		}
		valJSON, err := json.Marshal(val)
		if err != nil {
			return err
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')
		buf.Write(valJSON)
		return nil
	}
	for _, key := range payloadKeyOrder {
		if err := write(key); err != nil {
			return nil, err
		}
	}
	extras := make([]string, 0)
	for key := range payload {
		if !written[key] {
			extras = append(extras, key)
		}
	}
	sort.Strings(extras)
	for _, key := range extras {
		if err := write(key); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
