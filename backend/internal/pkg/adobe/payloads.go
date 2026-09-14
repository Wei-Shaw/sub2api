package adobe

import (
	"encoding/json"
	"fmt"
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

// String 返回 gpt-image 的 modelSpecificPayload.size 形式（"宽x高"）。
func (s Size) String() string { return fmt.Sprintf("%dx%d", s.Width, s.Height) }

// nano-banana 系各分辨率下的比例像素表。
var (
	nanoSizes1K = map[string]Size{
		"1:1":  {1024, 1024},
		"1:8":  {384, 3072},
		"1:4":  {512, 2048},
		"16:9": {1360, 768},
		"9:16": {768, 1360},
		"4:1":  {2048, 512},
		"4:3":  {1152, 864},
		"3:4":  {864, 1152},
		"8:1":  {3072, 384},
	}
	nanoSizes2K = map[string]Size{
		"1:1":  {2048, 2048},
		"1:8":  {768, 6144},
		"1:4":  {1024, 4096},
		"16:9": {2752, 1536},
		"9:16": {1536, 2752},
		"4:1":  {4096, 1024},
		"4:3":  {2048, 1536},
		"3:4":  {1536, 2048},
		"8:1":  {6144, 768},
	}
	nanoSizes4K = map[string]Size{
		"1:1":  {4096, 4096},
		"1:8":  {1536, 12288},
		"1:4":  {2048, 8192},
		"16:9": {5504, 3072},
		"9:16": {3072, 5504},
		"4:1":  {8192, 2048},
		"4:3":  {4096, 3072},
		"3:4":  {3072, 4096},
		"8:1":  {12288, 1536},
	}
)

// gpt-image 家族的比例像素表，与 nano-banana 系完全不同。
var (
	gptSizes1K = map[string]Size{
		"1:1":  {1024, 1024},
		"5:4":  {1120, 896},
		"9:16": {720, 1280},
		"21:9": {1456, 624},
		"16:9": {1280, 720},
		"4:3":  {1152, 864},
		"3:2":  {1248, 832},
		"4:5":  {896, 1120},
		"3:4":  {864, 1152},
		"2:3":  {832, 1248},
	}
	gptSizes2K = map[string]Size{
		"1:1":  {2048, 2048},
		"5:4":  {2240, 1792},
		"9:16": {1440, 2560},
		"21:9": {3024, 1296},
		"16:9": {2560, 1440},
		"4:3":  {2304, 1728},
		"3:2":  {2496, 1664},
		"4:5":  {1792, 2240},
		"3:4":  {1728, 2304},
		"2:3":  {1664, 2496},
	}
	gptSizes4K = map[string]Size{
		"1:1":  {2880, 2880},
		"5:4":  {3200, 2560},
		"9:16": {2160, 3840},
		"21:9": {3696, 1584},
		"16:9": {3840, 2160},
		"4:3":  {3264, 2448},
		"3:2":  {3504, 2336},
		"4:5":  {2560, 3200},
		"3:4":  {2448, 3264},
		"2:3":  {2336, 3504},
	}
)

func sizeTable(resolution OutputResolution, m1K, m2K, m4K map[string]Size) map[string]Size {
	switch OutputResolution(strings.ToUpper(string(resolution))) {
	case Resolution1K:
		return m1K
	case Resolution4K:
		return m4K
	default:
		return m2K
	}
}

// SizeFromRatio 返回 nano-banana 系模型的像素尺寸；未知比例回退 16:9。
func SizeFromRatio(ratio string, resolution OutputResolution) Size {
	table := sizeTable(resolution, nanoSizes1K, nanoSizes2K, nanoSizes4K)
	if size, ok := table[ratio]; ok {
		return size
	}
	return table["16:9"]
}

// GPTImagePixelsFromRatio 返回 gpt-image 家族的像素尺寸；该家族不支持的比例返回 false。
func GPTImagePixelsFromRatio(ratio string, resolution OutputResolution) (Size, bool) {
	size, ok := sizeTable(resolution, gptSizes1K, gptSizes2K, gptSizes4K)[ratio]
	return size, ok
}

// GPTImageDetailLevelFromQuality 把 OpenAI 的 quality 档位映射成上游 detailLevel。
//
// xhigh / max 是 gpt-image-2.5 新增的档位，Firefly 上游只理解 1-5，一律取最高档。
func GPTImageDetailLevelFromQuality(qualityLevel string) int {
	switch strings.ToLower(strings.TrimSpace(qualityLevel)) {
	case "high", "xhigh", "max":
		return 5
	case "medium":
		return 3
	default:
		return 1
	}
}

// seedNow 生成提交用的随机种子（复刻上游前端的取值方式）。
func seedNow() int { return int(timeNow().Unix() % 999999) }

// ImagePayloadOptions 是构造图像提交体所需的输入。
type ImagePayloadOptions struct {
	Prompt               string
	AspectRatio          string
	OutputResolution     OutputResolution
	UpstreamModelID      string
	UpstreamModelVersion string
	// PayloadKind 决定用哪套构造器。零值（PayloadKindGPTImage）等同于 Step 7 之前的
	// 「按 UpstreamModelID 猜」路径，保持老 conf 直接透传时的行为一致。
	PayloadKind PayloadKind
	// SizePixels：v2.5 是请求 WxH 原样；enum-size 是 catalog NearestSize 的结果。
	// payload 层不再做二次像素决策。零值表示省略顶层 size（对齐 UI「自动」）。
	SizePixels Size
	// QualityLevel 是 OpenAI 的 quality；DetailLevel 为 nil 时由它推导。
	QualityLevel string
	// DetailLevel 显式指定上游 detailLevel（1-5），优先于 QualityLevel。
	DetailLevel *int
	// SourceImageIDs 是已上传的参考图 id，非空即走图生图。
	SourceImageIDs []string
	// Background 是 OpenAI 的 background 参数（transparent / opaque / auto）。
	// gpt-image v2 放进 modelSpecificPayload；v2.5 同样只把非 auto 的 background 塞进该对象。
	Background string
}

// BuildImagePayloadCandidates 构造 /v2/3p-images/generate-async 的请求体候选列表。
//
// 返回的是「按尝试顺序排列的候选」而非单个 payload：Firefly 对不同子模块/图生图形态
// 接受的 payload 形状不同，上游用「依次尝试、命中 200 即停」兜住 schema 漂移。
// 调用方应逐个 POST 直到 200，不要只发第一个。
func BuildImagePayloadCandidates(opts ImagePayloadOptions) ([]map[string]any, error) {
	normalizedRatio := strings.ToLower(strings.TrimSpace(opts.AspectRatio))
	effectiveRatio := normalizedRatio
	if effectiveRatio == "" {
		effectiveRatio = "1:1"
	}

	switch opts.PayloadKind {
	case PayloadKindGPTImage25:
		return buildGPTImage25Payloads(opts)
	case PayloadKindSizeEnum:
		return buildSizeEnumPayloads(opts)
	case PayloadKindNanoBanana:
		return buildNanoBananaPayloads(opts, normalizedRatio, effectiveRatio), nil
	case PayloadKindGPTImage:
		return buildGPTImagePayloads(opts, effectiveRatio)
	}

	// 兼容 Step 7 之前——老调用方可能只填了 UpstreamModelID 就不设 PayloadKind。
	if strings.EqualFold(strings.TrimSpace(opts.UpstreamModelID), upstreamModelIDGPTImage) {
		return buildGPTImagePayloads(opts, effectiveRatio)
	}
	return buildNanoBananaPayloads(opts, normalizedRatio, effectiveRatio), nil
}

// buildGPTImage25Payloads 是 gpt-image v2.5-flare / v2.5-prism 的 payload。
//
// 抓包实测（Firefly UI，2:3）：v2.5 与 v2 的差异是：
//   - 不发 outputResolution
//   - 有合法 WxH 时发顶层 size:{width,height}（低档套餐拒 4K 交给上游，不夹成 1024/1536）
//   - modelSpecificPayload 默认 {}，不写 size:"auto"
//   - 多一个 caiClaimVersion:2
//
// 空 / auto 省略顶层 size，对齐 UI「自动」。
func buildGPTImage25Payloads(opts ImagePayloadOptions) ([]map[string]any, error) {
	detailLevel := GPTImageDetailLevelFromQuality(opts.QualityLevel)
	if opts.DetailLevel != nil {
		detailLevel = *opts.DetailLevel
	}

	modelSpecific := map[string]any{}
	if bg := strings.ToLower(strings.TrimSpace(opts.Background)); bg != "" && bg != "auto" {
		modelSpecific["background"] = bg
	}

	base := map[string]any{
		"modelId":              opts.UpstreamModelID,
		"modelVersion":         opts.UpstreamModelVersion,
		"n":                    1,
		"prompt":               opts.Prompt,
		"seeds":                []int{seedNow()},
		"output":               map[string]any{"storeInputs": true},
		"referenceBlobs":       []any{},
		"generationMetadata":   map[string]any{"module": "text2image", "submodule": "ff-image-generate"},
		"modelSpecificPayload": modelSpecific,
		"generationSettings":   map[string]any{"detailLevel": detailLevel},
		"caiClaimVersion":      2,
	}
	if opts.SizePixels.Width > 0 && opts.SizePixels.Height > 0 {
		base["size"] = opts.SizePixels
	}

	if len(opts.SourceImageIDs) == 0 {
		return []map[string]any{base}, nil
	}

	// 图生图分支同 v2：module=image2image + referenceBlobs.usage=subject。
	edited := clonePayload(base)
	edited["generationMetadata"] = map[string]any{"module": "image2image", "submodule": "ff-image-generate"}
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
		"seeds":              []int{seedNow()},
		"output":             map[string]any{"storeInputs": true},
		"referenceBlobs":     []any{},
		"generationMetadata": map[string]any{"module": "text2image", "submodule": "ff-image-generate"},
		"size":               opts.SizePixels,
	}
	if len(opts.SourceImageIDs) == 0 {
		return []map[string]any{base}, nil
	}
	// enum-size 家族的图生图形态多样，抓包未覆盖——沿用 gpt-image 的 module=image2image
	// + usage=subject。若上游拒收，逐个家族再补 candidate。
	edited := clonePayload(base)
	edited["generationMetadata"] = map[string]any{"module": "image2image", "submodule": "ff-image-generate"}
	edited["referenceBlobs"] = referenceBlobs(opts.SourceImageIDs, "subject")
	return []map[string]any{edited}, nil
}

func buildGPTImagePayloads(opts ImagePayloadOptions, ratio string) ([]map[string]any, error) {
	detailLevel := GPTImageDetailLevelFromQuality(opts.QualityLevel)
	if opts.DetailLevel != nil {
		detailLevel = *opts.DetailLevel
	}

	pixels, ok := GPTImagePixelsFromRatio(ratio, opts.OutputResolution)
	if !ok {
		return nil, NewRequestError(fmt.Sprintf("unsupported gpt-image ratio: %s", ratio))
	}
	if pixels.Width <= 0 || pixels.Height <= 0 {
		return nil, NewRequestError("gpt-image size must be positive")
	}

	modelSpecific := map[string]any{"size": pixels.String()}
	// background 与 size 同属 OpenAI 原生参数，走同一个透传口。auto 是上游默认值，
	// 不发以免多带字段——空值与 auto 都必须让 payload 与老链路字节级一致。
	if bg := strings.ToLower(strings.TrimSpace(opts.Background)); bg != "" && bg != "auto" {
		modelSpecific["background"] = bg
	}

	base := map[string]any{
		"modelId":              opts.UpstreamModelID,
		"modelVersion":         opts.UpstreamModelVersion,
		"n":                    1,
		"prompt":               opts.Prompt,
		"seeds":                []int{seedNow()},
		"output":               map[string]any{"storeInputs": true},
		"referenceBlobs":       []any{},
		"generationMetadata":   map[string]any{"module": "text2image", "submodule": "ff-image-generate"},
		"modelSpecificPayload": modelSpecific,
		"outputResolution":     strings.ToUpper(string(defaultedResolution(opts.OutputResolution))),
		"generationSettings":   map[string]any{"detailLevel": detailLevel},
		"size":                 pixels,
	}

	if len(opts.SourceImageIDs) == 0 {
		return []map[string]any{base}, nil
	}

	// gpt-image 图生图：参考媒体必须走 referenceBlobs（每项 {id, usage}），Adobe 新
	// API 已拒收 referenceImages/referenceVideos（422 validation_error）。
	//
	// usage 必须是 "subject"：经对真实 Adobe API 实证，module=image2image + usage=subject
	// 才返回 200；usage=general 会 400 "Image edit use case requires a reference image"
	// （Adobe 不把 general blob 当 edit 源图）。与 nano-banana 恰好相反，见下。
	edited := clonePayload(base)
	edited["generationMetadata"] = map[string]any{"module": "image2image", "submodule": "ff-image-generate"}
	edited["referenceBlobs"] = referenceBlobs(opts.SourceImageIDs, "subject")
	return []map[string]any{edited}, nil
}

func buildNanoBananaPayloads(opts ImagePayloadOptions, normalizedRatio, effectiveRatio string) []map[string]any {
	modelSpecific := map[string]any{
		"parameters": map[string]any{"addWatermark": false},
	}
	// 注意用 normalizedRatio 而非 effectiveRatio：请求没给比例时不应凭空补一个。
	if normalizedRatio != "" && normalizedRatio != "auto" {
		modelSpecific["aspectRatio"] = normalizedRatio
	}

	base := map[string]any{
		"modelId":              opts.UpstreamModelID,
		"modelVersion":         opts.UpstreamModelVersion,
		"n":                    1,
		"prompt":               opts.Prompt,
		"size":                 SizeFromRatio(effectiveRatio, opts.OutputResolution),
		"seeds":                []int{seedNow()},
		"groundSearch":         false,
		"skipCai":              false,
		"output":               map[string]any{"storeInputs": true},
		"generationMetadata":   map[string]any{"module": "text2image", "submodule": "ff-image-generate"},
		"modelSpecificPayload": modelSpecific,
	}

	if len(opts.SourceImageIDs) == 0 {
		base["referenceBlobs"] = []any{}
		return []map[string]any{base}
	}

	// nano-banana(Google) 图生图：usage 必须是 "general"——经实证，nano-banana 用
	// "subject" 会 400 "Only general reference images are supported for Google
	// Nano-Banana"；与 gpt-image 恰好相反，故两族不可共用同一 usage。
	edited := clonePayload(base)
	edited["generationMetadata"] = map[string]any{"module": "image2image", "submodule": "ff-image-generate"}
	edited["referenceBlobs"] = referenceBlobs(opts.SourceImageIDs, "general")
	return []map[string]any{edited}
}

func defaultedResolution(resolution OutputResolution) OutputResolution {
	if strings.TrimSpace(string(resolution)) == "" {
		return DefaultOutputResolution
	}
	return resolution
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
