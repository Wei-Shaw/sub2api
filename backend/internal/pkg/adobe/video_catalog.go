package adobe

import (
	"fmt"
	"sort"
	"strings"
)

// VideoResolution 是 Firefly 视频的输出分辨率档位。
type VideoResolution string

const (
	VideoResolution720P  VideoResolution = "720p"
	VideoResolution1080P VideoResolution = "1080p"
)

// videoRatioSuffixes 是视频支持的比例及其 id 后缀。视频只支持横竖两种。
var videoRatioSuffixes = map[string]string{
	"16:9": "16x9",
	"9:16": "9x16",
}

// videoSizes 是各分辨率 × 比例的像素宽高。
var videoSizes = map[VideoResolution]map[string]Size{
	VideoResolution720P: {
		"16:9": {1280, 720},
		"9:16": {720, 1280},
	},
	VideoResolution1080P: {
		"16:9": {1920, 1080},
		"9:16": {1080, 1920},
	},
}

// VideoModelConf 是一个具体视频模型（家族 + 时长 + 比例 + 分辨率）的上游参数。
type VideoModelConf struct {
	// ModelID 是本条目对应的完整模型 id。
	ModelID string
	// Family 是族名（如 sora2 / veo31 / kling-o3）。
	Family string
	// UpstreamModel 是上游 model 串（如 openai:firefly:colligo:sora2）。
	UpstreamModel string
	// UpstreamModelID 是 payload.modelId（sora / veo / kling）。
	UpstreamModelID string
	// UpstreamModelVersion 是 payload.modelVersion。
	UpstreamModelVersion string
	// Engine 决定 BuildVideoPayload 走哪条分支。
	Engine   string
	Duration int
	// AspectRatio 是比例，如 "16:9"。
	AspectRatio      string
	OutputResolution VideoResolution
	// GenerateAudio 是该模型的默认音频开关。
	GenerateAudio bool
	// ReferenceMode 非空时表示该模型走参考图模式。
	ReferenceMode string
	Description   string
}

// Size 返回该模型的像素宽高。
func (c VideoModelConf) Size() Size {
	return videoSizes[c.OutputResolution][c.AspectRatio]
}

// MaxInputImages 返回该模型可接受的最大输入图数量，与上游入口一致。
func (c VideoModelConf) MaxInputImages() int {
	if c.Engine == EngineVeo31Standard && c.ReferenceMode == ReferenceModeImage {
		return 3
	}
	switch c.Engine {
	case EngineVeo31Fast, EngineVeo31Standard, EngineKlingO3, EngineKling3:
		return 2
	default:
		return 1
	}
}

// videoFamilySpec 描述一个视频模型族，驱动目录展开。
type videoFamilySpec struct {
	family               string
	prefix               string
	upstreamModel        string
	upstreamModelID      string
	upstreamModelVersion string
	engine               string
	durations            []int
	ratios               []string
	resolutions          []VideoResolution
	// resolutionInID 决定分辨率是否拼进 model id：veo31 系列拼，sora/kling 固定不拼。
	resolutionInID bool
	generateAudio  bool
	referenceMode  string
	label          string
}

// videoFamilySpecs 的顺序即对外展示顺序。
var videoFamilySpecs = []videoFamilySpec{
	{
		family: "sora2", prefix: "firefly-sora2",
		upstreamModel:   "openai:firefly:colligo:sora2",
		upstreamModelID: "sora", upstreamModelVersion: "sora-2",
		engine:    EngineSora2,
		durations: []int{4, 8, 12}, ratios: []string{"9:16", "16:9"},
		resolutions: []VideoResolution{VideoResolution720P},
		label:       "Sora 2",
	},
	{
		family: "sora2-pro", prefix: "firefly-sora2-pro",
		upstreamModel:   "openai:firefly:colligo:sora2-pro",
		upstreamModelID: "sora", upstreamModelVersion: "sora-2",
		engine:    EngineSora2,
		durations: []int{4, 8, 12}, ratios: []string{"9:16", "16:9"},
		resolutions: []VideoResolution{VideoResolution720P},
		label:       "Sora 2 Pro",
	},
	{
		family: "veo31", prefix: "firefly-veo31",
		upstreamModel:   "google:firefly:colligo:veo31",
		upstreamModelID: "veo", upstreamModelVersion: "3.1-generate",
		engine:    EngineVeo31Standard,
		durations: []int{4, 6, 8}, ratios: []string{"16:9", "9:16"},
		resolutions:    []VideoResolution{VideoResolution1080P, VideoResolution720P},
		resolutionInID: true,
		label:          "Veo 3.1",
	},
	{
		family: "veo31-ref", prefix: "firefly-veo31-ref",
		upstreamModel:   "google:firefly:colligo:veo31",
		upstreamModelID: "veo", upstreamModelVersion: "3.1-generate",
		engine:    EngineVeo31Standard,
		durations: []int{4, 6, 8}, ratios: []string{"16:9", "9:16"},
		resolutions:    []VideoResolution{VideoResolution1080P, VideoResolution720P},
		resolutionInID: true,
		referenceMode:  ReferenceModeImage,
		label:          "Veo 3.1 Reference",
	},
	{
		family: "veo31-fast", prefix: "firefly-veo31-fast",
		upstreamModel:   "google:firefly:colligo:veo31-fast",
		upstreamModelID: "veo", upstreamModelVersion: "3.1-fast-generate",
		engine:    EngineVeo31Fast,
		durations: []int{4, 6, 8}, ratios: []string{"16:9", "9:16"},
		resolutions:    []VideoResolution{VideoResolution1080P, VideoResolution720P},
		resolutionInID: true,
		label:          "Veo 3.1 Fast",
	},
	{
		family: "kling-o3", prefix: "firefly-kling-o3",
		upstreamModel:   "kling:firefly:colligo:o3",
		upstreamModelID: "kling", upstreamModelVersion: "kling_o3_pro_reference_to_video",
		engine:    EngineKlingO3,
		durations: []int{5, 15}, ratios: []string{"16:9", "9:16"},
		resolutions: []VideoResolution{VideoResolution1080P},
		label:       "Kling O3",
	},
	{
		family: "kling3", prefix: "firefly-kling3",
		upstreamModel:   "kling:firefly:colligo:3.0",
		upstreamModelID: "kling", upstreamModelVersion: "kling_v3_standard_i2v",
		engine:    EngineKling3,
		durations: []int{5, 10, 15}, ratios: []string{"16:9", "9:16"},
		resolutions:   []VideoResolution{VideoResolution720P},
		generateAudio: true,
		label:         "Kling 3.0",
	},
}

var videoModelCatalog = buildVideoModelCatalog()

// VideoModelIDs 是全部视频模型 id（已排序）。
//
// 与图像不同，视频不做族级 id：时长是 id 的一部分且无法从 OpenAI 的请求参数推导，
// 因此模型列表必须列出全量组合。
var VideoModelIDs = buildVideoModelIDs()

// VideoFamily 是对外展示用的族信息摘要。
type VideoFamily struct {
	Family         string
	Label          string
	Durations      []int
	Ratios         []string
	Resolutions    []VideoResolution
	ResolutionInID bool
}

// VideoFamilies 供前端渲染可选的视频族与其参数组合。
var VideoFamilies = buildVideoFamilies()

func buildVideoModelCatalog() map[string]VideoModelConf {
	catalog := make(map[string]VideoModelConf)
	for _, spec := range videoFamilySpecs {
		for _, duration := range spec.durations {
			for _, ratio := range spec.ratios {
				suffix, ok := videoRatioSuffixes[ratio]
				if !ok {
					continue
				}
				for _, resolution := range spec.resolutions {
					id := fmt.Sprintf("%s-%ds-%s", spec.prefix, duration, suffix)
					if spec.resolutionInID {
						id = fmt.Sprintf("%s-%s", id, resolution)
					}
					catalog[id] = VideoModelConf{
						ModelID:              id,
						Family:               spec.family,
						UpstreamModel:        spec.upstreamModel,
						UpstreamModelID:      spec.upstreamModelID,
						UpstreamModelVersion: spec.upstreamModelVersion,
						Engine:               spec.engine,
						Duration:             duration,
						AspectRatio:          ratio,
						OutputResolution:     resolution,
						GenerateAudio:        spec.generateAudio,
						ReferenceMode:        spec.referenceMode,
						Description: fmt.Sprintf("%s (%ds %s %s)",
							spec.label, duration, ratio, resolution),
					}
				}
			}
		}
	}
	return catalog
}

func buildVideoModelIDs() []string {
	ids := make([]string, 0, len(videoModelCatalog))
	for id := range videoModelCatalog {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func buildVideoFamilies() []VideoFamily {
	families := make([]VideoFamily, 0, len(videoFamilySpecs))
	for _, spec := range videoFamilySpecs {
		families = append(families, VideoFamily{
			Family:         spec.family,
			Label:          spec.label,
			Durations:      append([]int(nil), spec.durations...),
			Ratios:         append([]string(nil), spec.ratios...),
			Resolutions:    append([]VideoResolution(nil), spec.resolutions...),
			ResolutionInID: spec.resolutionInID,
		})
	}
	return families
}

// ResolveVideoModel 按完整视频模型 id 查目录。
func ResolveVideoModel(modelID string) (VideoModelConf, bool) {
	conf, ok := videoModelCatalog[strings.ToLower(strings.TrimSpace(modelID))]
	return conf, ok
}

// IsVideoModelID 判定 id 是否为已注册的 Firefly 视频模型。
func IsVideoModelID(modelID string) bool {
	_, ok := ResolveVideoModel(modelID)
	return ok
}

// VideoSizeFor 按分辨率与比例取像素宽高；组合不支持时返回 false。
func VideoSizeFor(resolution VideoResolution, aspectRatio string) (Size, bool) {
	size, ok := videoSizes[resolution][aspectRatio]
	return size, ok
}

// VideoPayloadOptionsFor 把模型配置摊平成构造 payload 所需的输入，
// 只留 prompt / 参考图 / 负向提示词等按请求变化的部分给调用方填。
func VideoPayloadOptionsFor(conf VideoModelConf) VideoPayloadOptions {
	return VideoPayloadOptions{
		UpstreamModel:        conf.UpstreamModel,
		UpstreamModelID:      conf.UpstreamModelID,
		UpstreamModelVersion: conf.UpstreamModelVersion,
		Engine:               conf.Engine,
		Duration:             conf.Duration,
		AspectRatio:          conf.AspectRatio,
		Size:                 conf.Size(),
		GenerateAudio:        conf.GenerateAudio,
		ReferenceMode:        conf.ReferenceMode,
	}
}
