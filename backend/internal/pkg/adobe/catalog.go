package adobe

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// OutputResolution 是 Firefly 的输出分辨率档位。
type OutputResolution string

const (
	Resolution1K OutputResolution = "1K"
	Resolution2K OutputResolution = "2K"
	Resolution4K OutputResolution = "4K"
)

// DefaultOutputResolution 是未指定分辨率时的默认档位。
const DefaultOutputResolution = Resolution2K

// ImageModelIDPrefix 是所有 Firefly 图像/视频模型 id 的统一前缀，用于路由判定。
const ImageModelIDPrefix = "firefly-"

// DefaultImageModelID 是未指定模型时的回退模型。
const DefaultImageModelID = "firefly-nano-banana-pro-2k-16x9"

// allResolutions 是注册目录时展开的分辨率档位。
var allResolutions = []OutputResolution{Resolution1K, Resolution2K, Resolution4K}

// nanoBananaRatioSuffixes 是 nano-banana / nano-banana-pro 的比例枚举（Google 原生 10 种）。
// 顶层 size 只发方图档位，比例走 modelSpecificPayload.aspectRatio，不要自制 WxH。
var nanoBananaRatioSuffixes = map[string]string{
	"1:1":  "1x1",
	"16:9": "16x9",
	"9:16": "9x16",
	"4:3":  "4x3",
	"3:4":  "3x4",
	"3:2":  "3x2",
	"2:3":  "2x3",
	"4:5":  "4x5",
	"5:4":  "5x4",
	"21:9": "21x9",
}

// nanoBanana2RatioSuffixes 在 Google 10 种之上额外支持 Firefly banana-3 的超长横幅/竖幅。
var nanoBanana2RatioSuffixes = map[string]string{
	"1:1":  "1x1",
	"16:9": "16x9",
	"9:16": "9x16",
	"4:3":  "4x3",
	"3:4":  "3x4",
	"3:2":  "3x2",
	"2:3":  "2x3",
	"4:5":  "4x5",
	"5:4":  "5x4",
	"21:9": "21x9",
	"1:8":  "1x8",
	"1:4":  "1x4",
	"4:1":  "4x1",
	"8:1":  "8x1",
}

// PayloadKind 决定用哪一套 payload 构造器。上游 schema 的形状不是「所有 gpt-image 一样、
// 所有 nano-banana 一样」，而是按上游家族 + 版本走。列出来是为了让 payload 层不再靠
// UpstreamModelID 猜——这是 Step 7 前 payload 分派的老毛病。
type PayloadKind int

const (
	// PayloadKindUnset 是零值。dispatch 遇到它会回落到「按 UpstreamModelID 猜」的老路径。
	// 保留是为了让 Step 7 之前的调用方 / 老测试无需同步改动。
	PayloadKindUnset PayloadKind = iota
	// PayloadKindGPTImage 是历史零值回落路径。v1.5 / v2 已改走 PayloadKindGPTImage25。
	PayloadKindGPTImage
	// PayloadKindGPTImage25 对 gpt-image v1.5 / v2 / v2.5：无 outputResolution；
	// 有合法 WxH 时发顶层 size:{width,height}（含 4K，不夹紧）；空/auto 则省略。
	// v2.5 Auto 额外写 modelSpecificPayload.size:"auto"。caiClaimVersion:2。
	PayloadKindGPTImage25
	// PayloadKindNanoBanana 对 gemini-flash 家族：方图档位 + 可选 aspectRatio
	PayloadKindNanoBanana
	// PayloadKindSizeEnum 对 flux / imagen / gpt-4o-image / runway-gen4-image：
	// 顶层 size:{width,height} 从枚举里挑，无 outputResolution / modelSpecificPayload / aspectRatio
	PayloadKindSizeEnum
)

// Size 是 payload 里 {"width": w, "height": h} 的对应类型。
// 在 payloads.go 里作为出图像素尺寸使用；这里公开是让 catalog 声明允许尺寸表。
// 之所以定义在 catalog.go 而非 payloads.go：catalog 里的 spec 依赖它。
// payloads.go 会 alias 到本类型（同包，同一 Size）。
// （若移动到 shared 包会破坏「adobe 包对内部零依赖」的约束。）

// ImageModelConf 是一个具体图像模型（家族 + 分辨率 + 比例）的上游参数。
type ImageModelConf struct {
	// ModelID 是本条目对应的完整模型 id。
	// PayloadKindNanoBanana 走三维组合，ModelID 是 family-res-ratio；
	// PayloadKindGPTImage25 / PayloadKindSizeEnum 只到族级，ModelID 就等于 Family。
	ModelID string
	// Family 是族级模型 id（如 firefly-gpt-image-2）。
	Family string
	// UpstreamModel 是上游 model 串（如 openai:firefly:gpt-image）。
	UpstreamModel string
	// UpstreamModelID 是 payload.modelId。
	UpstreamModelID string
	// UpstreamModelVersion 是 payload.modelVersion。
	UpstreamModelVersion string
	// PayloadKind 决定 payload 层用哪一套构造器（见 PayloadKind 常量）。
	PayloadKind PayloadKind
	// OutputResolution 对 banana 是方图档位；对 gpt-image / enum-size 由将发送
	// 的像素长边推算（用于计费）。gpt-image 不把该字段放进 payload。
	OutputResolution OutputResolution
	AspectRatio      string
	Description      string
	// SizePixels 对 PayloadKindGPTImage25 自由 WxH 是请求原样（空/auto 为零值）；
	// 对 1.5 / enum-size 是允许集里 NearestSize 挑好的尺寸；对 banana 是档位方图。
	SizePixels Size
}

// IsGPTImage 判定是否 gpt-image 家族——该家族的 payload 形状与 nano-banana 系不同
// （像素尺寸表、图生图的 referenceBlobs.usage 取值都不一样）。
func (c ImageModelConf) IsGPTImage() bool {
	return c.UpstreamModelID == upstreamModelIDGPTImage
}

const upstreamModelIDGPTImage = "gpt-image"

// imageFamilySpec 描述一个图像模型族，同时驱动目录展开与族级 id 解析。
type imageFamilySpec struct {
	familyID             string
	upstreamModel        string
	upstreamModelID      string
	upstreamModelVersion string
	// payloadKind 决定 payload 层用哪一套构造器（Step 7 之前是按 upstreamModelID 猜的）。
	payloadKind PayloadKind
	// ratioSuffixes 只对 PayloadKindNanoBanana 有意义——会展开成
	// family × resolution × ratio 全组合。gpt-image / enum-size 这两项是 nil。
	ratioSuffixes map[string]string
	// ratiosSorted 是 ratioSuffixes 的键按「比例数值升序、数值相同按标签字典序」
	// 排好的切片。NearestRatio 必须遍历它而不是直接遍历 map——map 迭代顺序随机，
	// 平局时会让同一个 size 在不同进程里解析出不同比例。
	ratiosSorted []string
	// supportedSizes 对 gpt-image 1.5 与 PayloadKindSizeEnum：上游 size.enum 原样搬移。
	// 自由 WxH 的 gpt-image 2 / 2.5 不走这张表：请求像素原样发给上游。
	supportedSizes []Size
	label          string
}

// imageFamilySpecs 的顺序即对外展示顺序。
var imageFamilySpecs = buildImageFamilySpecs()

// fluxSupportedSizes / imagenSupportedSizes / gpt4oImageSupportedSizes /
// runwayGen4ImageSupportedSizes 均取自 discovery 的 requestSchema.size.enum，
// 顺序保留原始表——便于对照 testdata/discovery_snapshot.json 复核。
var (
	fluxSupportedSizes = []Size{
		{1024, 768}, {1440, 1440}, {768, 1024}, {576, 1024}, {1024, 576},
	}
	imagenSupportedSizes = []Size{
		{1280, 896}, {1024, 1024}, {768, 1408}, {1408, 768}, {896, 1280},
	}
	gpt4oImageSupportedSizes = []Size{
		{1024, 1024}, {1536, 1024}, {1024, 1536},
	}
	// gpt-image 1.5 的 size.enum，与 snapshot / discovery 三档一致。
	gptImage15SupportedSizes = []Size{
		{1024, 1024}, {1536, 1024}, {1024, 1536},
	}
	runwayGen4ImageSupportedSizes = []Size{
		{1920, 1080}, {1080, 1920}, {1024, 1024}, {1360, 768}, {1080, 1080},
		{1168, 880}, {1440, 1080}, {1080, 1440}, {1808, 768}, {2112, 912},
	}
)

var rawImageFamilySpecs = []imageFamilySpec{
	{
		familyID:             "firefly-gpt-image-2",
		upstreamModel:        "openai:firefly:gpt-image",
		upstreamModelID:      upstreamModelIDGPTImage,
		upstreamModelVersion: "2",
		payloadKind:          PayloadKindGPTImage25,
		label:                "Firefly GPT Image 2",
	},
	{
		familyID:             "firefly-gpt-image-1.5",
		upstreamModel:        "openai:firefly:gpt-image",
		upstreamModelID:      upstreamModelIDGPTImage,
		upstreamModelVersion: "1.5",
		payloadKind:          PayloadKindGPTImage25,
		supportedSizes:       gptImage15SupportedSizes,
		label:                "Firefly GPT Image 1.5",
	},
	// Step 7：加入 gpt-image v2.5-flare 与 v2.5-prism。上游 modelVersion 用真名——
	// prism 的 UI displayName 是 "GPT Image 2.5 Sunburst"，label 保留 Sunburst 便于识别。
	{
		familyID:             "firefly-gpt-image-2-5-flare",
		upstreamModel:        "openai:firefly:gpt-image",
		upstreamModelID:      upstreamModelIDGPTImage,
		upstreamModelVersion: "gpt-image-2.5-flare",
		payloadKind:          PayloadKindGPTImage25,
		label:                "Firefly GPT Image 2.5 Flare",
	},
	{
		familyID:             "firefly-gpt-image-2-5-prism",
		upstreamModel:        "openai:firefly:gpt-image",
		upstreamModelID:      upstreamModelIDGPTImage,
		upstreamModelVersion: "gpt-image-2.5-prism",
		payloadKind:          PayloadKindGPTImage25,
		label:                "Firefly GPT Image 2.5 Sunburst",
	},
	{
		familyID:             "firefly-nano-banana-pro",
		upstreamModel:        "google:firefly:colligo:nano-banana-pro",
		upstreamModelID:      "gemini-flash",
		upstreamModelVersion: "nano-banana-2",
		payloadKind:          PayloadKindNanoBanana,
		ratioSuffixes:        nanoBananaRatioSuffixes,
		label:                "Firefly Nano Banana Pro",
	},
	{
		// Step 7 bug 修正：改前 upstreamModelVersion 写成 nano-banana-2，与 Pro 撞车，
		// 普通版请求实际打在 Pro 上。上游 discovery 里 nano-banana = Gemini 2.5 (Nano Banana)。
		familyID:             "firefly-nano-banana",
		upstreamModel:        "google:firefly:colligo:nano-banana-pro",
		upstreamModelID:      "gemini-flash",
		upstreamModelVersion: "nano-banana",
		payloadKind:          PayloadKindNanoBanana,
		ratioSuffixes:        nanoBananaRatioSuffixes,
		label:                "Firefly Nano Banana",
	},
	{
		familyID:             "firefly-nano-banana2",
		upstreamModel:        "google:firefly:colligo:nano-banana-pro",
		upstreamModelID:      "gemini-flash",
		upstreamModelVersion: "nano-banana-3",
		payloadKind:          PayloadKindNanoBanana,
		ratioSuffixes:        nanoBanana2RatioSuffixes,
		label:                "Firefly Nano Banana 2",
	},
	// Step 7：新增 enum-size 家族。这些家族的上游 schema 都用顶层 size:{width,height}
	// 从有限枚举里挑，无 outputResolution/modelSpecificPayload，共用 buildSizeEnumPayloads。
	{
		familyID:             "firefly-flux-pro",
		upstreamModel:        "bfl:firefly:flux",
		upstreamModelID:      "flux",
		upstreamModelVersion: "fluxPro",
		payloadKind:          PayloadKindSizeEnum,
		supportedSizes:       fluxSupportedSizes,
		label:                "Firefly FLUX Pro",
	},
	{
		familyID:             "firefly-flux-ultra",
		upstreamModel:        "bfl:firefly:flux",
		upstreamModelID:      "flux",
		upstreamModelVersion: "fluxUltra",
		payloadKind:          PayloadKindSizeEnum,
		supportedSizes:       fluxSupportedSizes,
		label:                "Firefly FLUX Ultra",
	},
	{
		familyID:             "firefly-imagen-4",
		upstreamModel:        "google:firefly:imagen",
		upstreamModelID:      "imagen",
		upstreamModelVersion: "4.0-generate",
		payloadKind:          PayloadKindSizeEnum,
		supportedSizes:       imagenSupportedSizes,
		label:                "Firefly Imagen 4",
	},
	{
		familyID:             "firefly-imagen-4-fast",
		upstreamModel:        "google:firefly:imagen",
		upstreamModelID:      "imagen",
		upstreamModelVersion: "4.0-fast-generate",
		payloadKind:          PayloadKindSizeEnum,
		supportedSizes:       imagenSupportedSizes,
		label:                "Firefly Imagen 4 Fast",
	},
	{
		familyID:             "firefly-gpt-4o-image",
		upstreamModel:        "openai:firefly:gpt-4o-image",
		upstreamModelID:      "gpt-4o-image",
		upstreamModelVersion: "default",
		payloadKind:          PayloadKindSizeEnum,
		supportedSizes:       gpt4oImageSupportedSizes,
		label:                "Firefly GPT-4o Image",
	},
	{
		familyID:             "firefly-runway-gen4-image",
		upstreamModel:        "runway:firefly:runway-gen4-image",
		upstreamModelID:      "runway-gen4-image",
		upstreamModelVersion: "default",
		payloadKind:          PayloadKindSizeEnum,
		supportedSizes:       runwayGen4ImageSupportedSizes,
		label:                "Firefly Runway Gen-4 Image",
	},
}

// imageModelCatalog 是 family × 分辨率 × 比例的全组合，键为完整模型 id。
var imageModelCatalog = buildImageModelCatalog()

// familyIDsByLengthDesc 是按长度降序排好的族级 id，供最长前缀匹配使用。
var familyIDsByLengthDesc = buildFamilyIDsByLengthDesc()

// ImageFamilyModelIDs 是对外暴露的族级模型 id 列表。
//
// 只暴露族级 id 而非全量组合：分辨率与比例可由请求的 size 推导，把 5×3×N 的全组合
// 都列进模型白名单会让管理端的选择器不可用。全量 id 仍然被 ResolveImage 接受。
var ImageFamilyModelIDs = buildImageFamilyModelIDs()

func buildImageModelCatalog() map[string]ImageModelConf {
	catalog := make(map[string]ImageModelConf)
	for _, spec := range imageFamilySpecs {
		// 只有 banana 系展开成 family-resolution-ratio 全量 id。
		// gpt-image 2 / 2.5 / 1.5 与 enum-size 只到族级，由 ResolveImage 处理。
		if len(spec.ratioSuffixes) == 0 {
			continue
		}
		for _, resolution := range allResolutions {
			for ratio, suffix := range spec.ratioSuffixes {
				modelID := fmt.Sprintf("%s-%s-%s", spec.familyID, strings.ToLower(string(resolution)), suffix)
				catalog[modelID] = ImageModelConf{
					ModelID:              modelID,
					Family:               spec.familyID,
					UpstreamModel:        spec.upstreamModel,
					UpstreamModelID:      spec.upstreamModelID,
					UpstreamModelVersion: spec.upstreamModelVersion,
					PayloadKind:          spec.payloadKind,
					OutputResolution:     resolution,
					AspectRatio:          ratio,
					Description:          fmt.Sprintf("%s (%s %s)", spec.label, resolution, ratio),
					SizePixels:           SquareFromResolution(resolution),
				}
			}
		}
	}
	return catalog
}

func buildFamilyIDsByLengthDesc() []string {
	ids := make([]string, 0, len(imageFamilySpecs))
	for _, spec := range imageFamilySpecs {
		ids = append(ids, spec.familyID)
	}
	sort.Slice(ids, func(i, j int) bool {
		if len(ids[i]) != len(ids[j]) {
			return len(ids[i]) > len(ids[j])
		}
		return ids[i] < ids[j]
	})
	return ids
}

func buildImageFamilyModelIDs() []string {
	ids := make([]string, 0, len(imageFamilySpecs))
	for _, spec := range imageFamilySpecs {
		ids = append(ids, spec.familyID)
	}
	return ids
}

// IsImageModelID 判定 id 是否带 Firefly 前缀。用于把请求路由到 Adobe 渠道。
//
// 刻意只认前缀、不查别名表：它的语义就是「这是个内部族 id」。要判定对外名请用
// IsExternalImageModelID，两者放宽成一个会让 openai_images.go 之外的判断跟着变松。
func IsImageModelID(modelID string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelID)), ImageModelIDPrefix)
}

// externalImageModelAliases 把用户面模型名翻译成内部族 id。
//
// 这是协议知识（Adobe 的 gpt-image-2.5-sunburst 在上游叫 prism），不是部署配置，
// 所以它必须住在这里而不是只活在 domain.DefaultAdobeModelMapping 里：账号一旦配了
// 自定义 model_mapping（比如白名单模式产出的 "imagen-4" -> "imagen-4" 恒等对），
// 那张默认表就不再参与解析，ResolveImage 仍然要认得干净外部名。
//
// 与 domain.DefaultAdobeModelMapping 逐条一致，由 service 层的守卫测试锁死。
var externalImageModelAliases = map[string]string{
	"gpt-image-2":            "firefly-gpt-image-2",
	"gpt-image-1.5":          "firefly-gpt-image-1.5",
	"gpt-image-2.5-flare":    "firefly-gpt-image-2-5-flare",
	"gpt-image-2.5-sunburst": "firefly-gpt-image-2-5-prism", // sunburst 是 UI 名，上游 modelVersion=prism
	"gpt-image-2.5-prism":    "firefly-gpt-image-2-5-prism",
	// 更早的 gpt-image 名字：Adobe 侧没有对应版本，一律落 2。
	"gpt-image":         "firefly-gpt-image-2",
	"gpt-image-1":       "firefly-gpt-image-2",
	"gpt-image-1-mini":  "firefly-gpt-image-2",
	"nano-banana-pro":   "firefly-nano-banana-pro",
	"nano-banana":       "firefly-nano-banana",
	"nano-banana2":      "firefly-nano-banana2",
	"flux-pro":          "firefly-flux-pro",
	"flux-ultra":        "firefly-flux-ultra",
	"imagen-4":          "firefly-imagen-4",
	"imagen-4-fast":     "firefly-imagen-4-fast",
	"gpt-4o-image":      "firefly-gpt-4o-image",
	"runway-gen4-image": "firefly-runway-gen4-image",
}

// ExternalImageModelAliases 返回别名表的副本，供守卫测试与其它包比对。
func ExternalImageModelAliases() map[string]string {
	out := make(map[string]string, len(externalImageModelAliases))
	for external, familyID := range externalImageModelAliases {
		out[external] = familyID
	}
	return out
}

// IsExternalImageModelID 判定 id 是否为 Adobe 的对外模型名（不带 firefly- 前缀）。
func IsExternalImageModelID(modelID string) bool {
	_, ok := externalImageModelAliases[strings.ToLower(strings.TrimSpace(modelID))]
	return ok
}

// DisplayLabel 返回对外模型名的人类可读展示名，供管理端的模型选择器使用。
//
// 复用族 spec 的 label 而不是另建一张表：label 本来就是 "Firefly GPT Image 2"
// 这种形式，去掉产品前缀即得 "GPT Image 2"。未知名返回 ("", false)，
// 调用方应回落到原始 id（运维自定义的别名就该显示他自己写的那个名字）。
func DisplayLabel(externalID string) (string, bool) {
	familyID, ok := externalImageModelAliases[strings.ToLower(strings.TrimSpace(externalID))]
	if !ok {
		return "", false
	}
	spec, ok := imageFamilySpecByID(familyID)
	if !ok {
		return "", false
	}
	return strings.TrimPrefix(spec.label, "Firefly "), true
}

// normalizeImageModelID 把对外模型名翻译成内部族 id。
//
// 只做族级精确匹配：对外名只到族级，比例/分辨率一律从 size 推导，所以
// "imagen-4-2k-16x9" 这种「干净名 + 尺寸后缀」不在支持范围内。
// 查不到就原样返回，让下游照旧报 unknown firefly image model。
func normalizeImageModelID(modelID string) string {
	normalized := strings.ToLower(strings.TrimSpace(modelID))
	if strings.HasPrefix(normalized, ImageModelIDPrefix) {
		return normalized
	}
	if familyID, ok := externalImageModelAliases[normalized]; ok {
		return familyID
	}
	return normalized
}

// PickImageFamily 从模型 id 中截出族级 id，支持族级 id 与全量 id 两种写法。
//
// 用最长前缀匹配：firefly-nano-banana 不能吞掉 firefly-nano-banana-pro 和
// firefly-nano-banana2。
func PickImageFamily(modelID string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(modelID))
	if !strings.HasPrefix(normalized, ImageModelIDPrefix) {
		return "", false
	}
	for _, familyID := range familyIDsByLengthDesc {
		if normalized == familyID || strings.HasPrefix(normalized, familyID+"-") {
			return familyID, true
		}
	}
	return "", false
}

// ResolveImageModel 按完整模型 id 查目录；id 为空时返回默认模型。
// 只接受全量 id（族级 id 请用 ResolveImage）。
func ResolveImageModel(modelID string) (ImageModelConf, bool) {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		conf, ok := imageModelCatalog[DefaultImageModelID]
		return conf, ok
	}
	conf, ok := imageModelCatalog[id]
	return conf, ok
}

// ImageRequest 是把一次出图请求解析成上游模型配置所需的输入。
type ImageRequest struct {
	// ModelID 可以是族级 id（firefly-gpt-image-2）或全量 id
	// （firefly-gpt-image-2-2k-16x9）。全量 id 里的分辨率/比例优先。
	ModelID string
	// Size 是 OpenAI 风格的 "宽x高"，族级 id 时用来推导比例与分辨率档位。
	Size string
	// Ratio 是显式比例（如 "16:9"），优先于 Size。
	//
	// 与 Size 推导的区别：显式 Ratio 走严格校验，该族不支持就报错；Size 推导则
	// 在族支持的比例里取最接近的，永不失败。点名要某个比例时静默换一个是错的。
	Ratio string
	// Resolution 为空时由 Size 按长边推导（见 ResolutionFromSize），
	// Size 也缺省时才回落 DefaultOutputResolution。
	Resolution OutputResolution
}

// ResolveImage 把一次出图请求解析成具体的上游模型配置。
//
// 分两种路径：
//   - 有 ratioSuffixes 的家族（nano-banana 系）：全量 id 查表；
//     族级 id 按 Ratio / Size / 默认分辨率组合出全量 id 再查表。
//     客户端省略 size 时 banana 回落 1K 方图（对齐 Firefly 默认），不走全局 2K。
//   - 无 ratioSuffixes 的家族（gpt-image 2/2.5/1.5、enum-size）：只到族级；
//     2/2.5 把请求 WxH 原样填进 SizePixels；1.5 与 enum-size 走 NearestSize。
func ResolveImage(req ImageRequest) (ImageModelConf, error) {
	// 先把对外名翻译成内部族 id：白名单模式配出来的 model_mapping 是恒等对
	// （"imagen-4" -> "imagen-4"），到这里时还没有 firefly- 前缀。
	modelID := normalizeImageModelID(req.ModelID)
	if conf, ok := imageModelCatalog[modelID]; ok {
		return conf, nil
	}

	familyID, ok := PickImageFamily(modelID)
	if !ok {
		return ImageModelConf{}, NewRequestError(fmt.Sprintf("unknown firefly image model: %q", req.ModelID))
	}

	// 必须先拿到 spec：各族支持的比例集不同，NearestRatio 要拿它当候选集。
	spec, ok := imageFamilySpecByID(familyID)
	if !ok {
		return ImageModelConf{}, NewRequestError(fmt.Sprintf("unknown firefly image family: %q", familyID))
	}

	// 非 ratio 路径：直接产出族级 conf。
	if len(spec.ratioSuffixes) == 0 {
		return resolveNonRatioFamily(spec, req)
	}

	ratio := strings.TrimSpace(req.Ratio)
	if ratio == "" || ratio == "auto" {
		ratio = NearestRatio(req.Size, spec.ratiosSorted)
	}
	resolution := req.Resolution
	if resolution == "" {
		if spec.payloadKind == PayloadKindNanoBanana && isBlankSize(req.Size) {
			resolution = Resolution1K
		} else {
			resolution = ResolutionFromSize(req.Size)
		}
	}

	// NearestRatio 的结果必然在 ratioSuffixes 里，所以这条只会为「显式 req.Ratio
	// 指定了该族不支持的比例」而触发——那种情况必须报错，不能悄悄换成最接近的。
	suffix, ok := spec.ratioSuffixes[ratio]
	if !ok {
		return ImageModelConf{}, NewRequestError(
			fmt.Sprintf("unsupported aspect ratio %q for firefly model %q", ratio, familyID))
	}

	composed := fmt.Sprintf("%s-%s-%s", familyID, strings.ToLower(string(resolution)), suffix)
	conf, ok := imageModelCatalog[composed]
	if !ok {
		return ImageModelConf{}, NewRequestError(
			fmt.Sprintf("unsupported resolution %q for firefly model %q", resolution, familyID))
	}
	return conf, nil
}

// resolveNonRatioFamily 处理 gpt-image 与 enum-size 家族。
//
// 与 ratio 路径的差别：ModelID 只到族级。有 supportedSizes（1.5 / flux 等）先
// NearestSize；否则 gpt-image 2/2.5 把请求 WxH 原样填进 SizePixels（含 4K）。
// OutputResolution 只用于计费，不随 gpt-image payload 发出去。
func resolveNonRatioFamily(spec imageFamilySpec, req ImageRequest) (ImageModelConf, error) {
	var pixels Size
	if len(spec.supportedSizes) > 0 {
		pixels = NearestSize(req.Size, spec.supportedSizes)
	} else if spec.payloadKind == PayloadKindGPTImage25 {
		if width, height, ok := parseSizeWxH(req.Size); ok {
			pixels = Size{Width: width, Height: height}
		}
	}

	resolution := req.Resolution
	if resolution == "" {
		if spec.payloadKind == PayloadKindGPTImage25 && pixels.Width > 0 {
			resolution = ResolutionFromSize(pixels.String())
		} else {
			resolution = ResolutionFromSize(req.Size)
		}
	}

	aspect := strings.TrimSpace(req.Ratio)
	if aspect == "" && pixels.Width > 0 && pixels.Height > 0 {
		aspect = fmt.Sprintf("%d:%d", pixels.Width/gcd(pixels.Width, pixels.Height),
			pixels.Height/gcd(pixels.Width, pixels.Height))
	}
	return ImageModelConf{
		ModelID:              spec.familyID,
		Family:               spec.familyID,
		UpstreamModel:        spec.upstreamModel,
		UpstreamModelID:      spec.upstreamModelID,
		UpstreamModelVersion: spec.upstreamModelVersion,
		PayloadKind:          spec.payloadKind,
		OutputResolution:     resolution,
		AspectRatio:          aspect,
		Description:          fmt.Sprintf("%s (%s)", spec.label, resolution),
		SizePixels:           pixels,
	}, nil
}

// NearestSize 在允许集里选宽高比最接近请求 size 的那个。
// 选比例最近而不是像素最近：请求 512x512 打向 flux（无 1K 档）应挑 1024x768 的方图
// 附近（1440x1440），而不是最大的 1440x1440——但这个决策是「比例优先」+「精确 GCD 命中即返回」。
// 允许集为空、size 非法/为空时返回 Size{}。
func NearestSize(size string, allowed []Size) Size {
	if len(allowed) == 0 {
		return Size{}
	}
	width, height, ok := parseSizeWxH(size)
	if !ok {
		// 缺尺寸时挑允许集的第一个——上游会把它当默认尺寸，避免报「size 缺失」。
		return allowed[0]
	}

	// 精确 GCD 命中优先。
	div := gcd(width, height)
	targetW, targetH := width/div, height/div
	for _, cand := range allowed {
		cdiv := gcd(cand.Width, cand.Height)
		if cand.Width/cdiv == targetW && cand.Height/cdiv == targetH {
			return cand
		}
	}

	// 否则取宽高比最接近的（差值相同时取像素总数更接近的）。
	target := float64(width) / float64(height)
	targetPixels := float64(width) * float64(height)
	best := allowed[0]
	bestRatioDelta := math.MaxFloat64
	bestPixelDelta := math.MaxFloat64
	for _, cand := range allowed {
		delta := math.Abs(float64(cand.Width)/float64(cand.Height) - target)
		pixelDelta := math.Abs(float64(cand.Width)*float64(cand.Height) - targetPixels)
		if delta < bestRatioDelta || (delta == bestRatioDelta && pixelDelta < bestPixelDelta) {
			best = cand
			bestRatioDelta = delta
			bestPixelDelta = pixelDelta
		}
	}
	return best
}

func imageFamilySpecByID(familyID string) (imageFamilySpec, bool) {
	for _, spec := range imageFamilySpecs {
		if spec.familyID == familyID {
			return spec, true
		}
	}
	return imageFamilySpec{}, false
}

// FallbackRatio 是 size 缺省/非法时的比例。
const FallbackRatio = "1:1"

// SquareFromResolution 返回 banana 档位对应的方图像素。Firefly UI 用方图表示
// 1K/2K/4K，比例另走 aspectRatio；缺省档与 DefaultOutputResolution 一致（2K）。
func SquareFromResolution(resolution OutputResolution) Size {
	switch OutputResolution(strings.ToUpper(string(resolution))) {
	case Resolution1K:
		return Size{Width: 1024, Height: 1024}
	case Resolution4K:
		return Size{Width: 4096, Height: 4096}
	default:
		return Size{Width: 2048, Height: 2048}
	}
}

func isBlankSize(size string) bool {
	normalized := strings.ToLower(strings.TrimSpace(size))
	return normalized == "" || normalized == "auto"
}

// parseSizeWxH 解析 "宽x高"；非法（含 "auto" / 空 / 非正数）返回 ok=false。
func parseSizeWxH(size string) (width, height int, ok bool) {
	normalized := strings.ToLower(strings.TrimSpace(size))
	parts := strings.SplitN(normalized, "x", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || w <= 0 {
		return 0, 0, false
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// NearestRatio 在 allowed 里挑最贴近 size 宽高比的一项；size 非法/为空时回落 FallbackRatio。
//
// 先用 GCD 约分求精确比例、命中直接返回，未命中再按浮点差取最近——与仓库里
// grokImagineAspectRatioFromSize 同一套算法。
//
// 「取最近」而不是「查表命中才算」是关键：各族支持的比例集不同（nano-banana
// 10 种、nano-banana2 14 种），精确查表未命中就回落 1:1 会让 3840x2160
// 这类请求静默出方图。allowed 为空时同样回落 FallbackRatio。
func NearestRatio(size string, allowed []string) string {
	if len(allowed) == 0 {
		return FallbackRatio
	}
	width, height, ok := parseSizeWxH(size)
	if !ok {
		return FallbackRatio
	}

	div := gcd(width, height)
	exact := strconv.Itoa(width/div) + ":" + strconv.Itoa(height/div)
	for _, candidate := range allowed {
		if candidate == exact {
			return exact
		}
	}

	target := float64(width) / float64(height)
	best := FallbackRatio
	bestDelta := math.MaxFloat64
	for _, candidate := range allowed {
		value, ok := ratioValue(candidate)
		if !ok {
			continue
		}
		if delta := math.Abs(value - target); delta < bestDelta {
			bestDelta = delta
			best = candidate
		}
	}
	return best
}

// ResolutionFromSize 按长边定分辨率档位；size 非法/为空时回落 DefaultOutputResolution。
//
// 阈值必须与 service.ClassifyImageBillingTier 保持一致（≤1024→1K、≤2048→2K、否则 4K）：
// 出图档位与计费档位取自同一个 size，两边错开就是静默错账。本包对 sub2api 内部零依赖，
// 无法直接复用那个函数，对齐由 internal/service 侧的跨包守卫测试保证。
func ResolutionFromSize(size string) OutputResolution {
	width, height, ok := parseSizeWxH(size)
	if !ok {
		return DefaultOutputResolution
	}
	maxEdge := width
	if height > maxEdge {
		maxEdge = height
	}
	switch {
	case maxEdge <= 1024:
		return Resolution1K
	case maxEdge <= 2048:
		return Resolution2K
	default:
		return Resolution4K
	}
}

// ratioValue 把 "16:9" 解析成 16/9；非法返回 ok=false。
func ratioValue(ratio string) (float64, bool) {
	parts := strings.SplitN(strings.TrimSpace(ratio), ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	w, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || w <= 0 {
		return 0, false
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || h <= 0 {
		return 0, false
	}
	return float64(w) / float64(h), true
}

func gcd(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

// buildImageFamilySpecs 给每个族补上排好序的比例列表。
func buildImageFamilySpecs() []imageFamilySpec {
	specs := make([]imageFamilySpec, 0, len(rawImageFamilySpecs))
	for _, spec := range rawImageFamilySpecs {
		ratios := make([]string, 0, len(spec.ratioSuffixes))
		for ratio := range spec.ratioSuffixes {
			ratios = append(ratios, ratio)
		}
		sort.Slice(ratios, func(i, j int) bool {
			vi, iok := ratioValue(ratios[i])
			vj, jok := ratioValue(ratios[j])
			if iok && jok && vi != vj {
				return vi < vj
			}
			return ratios[i] < ratios[j]
		})
		spec.ratiosSorted = ratios
		specs = append(specs, spec)
	}
	return specs
}
