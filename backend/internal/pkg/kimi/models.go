package kimi

// Model 描述一个 Kimi 模型（OpenAI 兼容 /models 返回结构）。
type Model struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created,omitempty"`
	OwnedBy     string `json:"owned_by"`
	DisplayName string `json:"display_name,omitempty"`
}

// 默认模型清单。
//
// 注意：不同套餐可用的 wire model id 不同（Andante → kimi-for-coding；
// Moderato → k3 / kimi-for-coding；Allegretto+ → k3[1m] / kimi-for-coding /
// kimi-for-coding-highspeed），实际上游可用列表应以
// GET https://api.kimi.com/coding/v1/models 返回为准，此处仅作默认展示与映射兜底。
var defaultModels = []Model{
	{ID: "kimi-for-coding", Object: "model", OwnedBy: "moonshot", DisplayName: "Kimi For Coding"},
	{ID: "kimi-for-coding-highspeed", Object: "model", OwnedBy: "moonshot", DisplayName: "Kimi For Coding Highspeed"},
	{ID: "k3", Object: "model", OwnedBy: "moonshot", DisplayName: "Kimi K3"},
	{ID: "k2p7", Object: "model", OwnedBy: "moonshot", DisplayName: "Kimi K2.7 Code"},
	{ID: "k2p6", Object: "model", OwnedBy: "moonshot", DisplayName: "Kimi K2.6 Code"},
	{ID: "k2p5", Object: "model", OwnedBy: "moonshot", DisplayName: "Kimi K2.5 Code"},
	{ID: "kimi-k2-thinking", Object: "model", OwnedBy: "moonshot", DisplayName: "Kimi K2 Thinking"},
}

func DefaultModels() []Model {
	out := make([]Model, len(defaultModels))
	copy(out, defaultModels)
	return out
}

func DefaultModelIDs() []string {
	models := DefaultModels()
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

func DefaultModelMapping() map[string]string {
	mapping := make(map[string]string, len(defaultModels)+3)
	for _, model := range defaultModels {
		mapping[model.ID] = model.ID
	}
	mapping["kimi"] = "kimi-for-coding"
	mapping["kimi-latest"] = "kimi-for-coding"
	mapping["kimi-code"] = "kimi-for-coding"
	return mapping
}
