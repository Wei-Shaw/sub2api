package adobe

// Model 是对外暴露的模型条目，形状与其它平台的模型目录一致（见 internal/pkg/kiro）。
type Model struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

// DefaultModels 是 Adobe 平台对外暴露的模型列表：图像只列族级 id，视频列全量 id。
//
// 两者的差别是刻意的：图像的分辨率与比例可从请求的 size 推导，把 5×3×N 的全组合
// 列出来会让模型选择器不可用；视频的时长是 id 的一部分且无法从请求参数推导，
// 只能列全量。
var DefaultModels = buildDefaultModels()

func buildDefaultModels() []Model {
	models := make([]Model, 0, len(ImageFamilyModelIDs)+len(VideoModelIDs))
	for _, familyID := range ImageFamilyModelIDs {
		spec, ok := imageFamilySpecByID(familyID)
		if !ok {
			continue
		}
		models = append(models, Model{
			ID:          familyID,
			Type:        "model",
			DisplayName: spec.label,
		})
	}
	for _, modelID := range VideoModelIDs {
		conf, ok := ResolveVideoModel(modelID)
		if !ok {
			continue
		}
		models = append(models, Model{
			ID:          modelID,
			Type:        "model",
			DisplayName: conf.Description,
		})
	}
	return models
}

// ImageModelIDs 返回对外暴露给用户面（/v1/models、分组模型选择器）的模型名。
//
// 刻意与内部族 id（imageFamilySpecs.familyID = "firefly-*"）区分开：
// 用户不该知道内部路由用的族 id（正如 openai 用户看不到我们上游账号的 organization id）；
// 也不该能直接请求内部 id——那是 model_mapping 的严格白名单的语义。
//
// 顺序与 DefaultAdobeModelMapping 的别名部分对齐；两处名字必须严格一致，
// 通过 models_test.go 里的守卫测试锁死（发的每个 id 都必须是默认映射的键）。
//
// 视频协议层（video_catalog.go 的 58 个 id）虽已就绪，但网关还没接线，
// 把它们列进这里等于宣传一个选了必然失败的能力，故排除。
func ImageModelIDs() []string {
	return []string{
		// gpt-image 系
		"gpt-image-2", "gpt-image-1.5",
		"gpt-image-2.5-flare", "gpt-image-2.5-sunburst", // sunburst 是 UI 名，上游 modelVersion=prism
		// Google Gemini nano-banana 系
		"nano-banana-pro", "nano-banana", "nano-banana2",
		// Black Forest Labs FLUX
		"flux-pro", "flux-ultra",
		// Google Imagen 4
		"imagen-4", "imagen-4-fast",
		// OpenAI gpt-4o-image / Runway Gen-4
		"gpt-4o-image", "runway-gen4-image",
	}
}

// DefaultModelIDs 返回 DefaultModels 的 id 列表。
func DefaultModelIDs() []string {
	ids := make([]string, 0, len(DefaultModels))
	for _, model := range DefaultModels {
		ids = append(ids, model.ID)
	}
	return ids
}
