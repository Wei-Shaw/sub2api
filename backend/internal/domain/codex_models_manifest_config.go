package domain

import "encoding/json"

// GroupCodexModelsManifestConfig 保存各平台的 Codex 模型字段覆盖，以及
// OpenAI 分组可选的固定账号发现设置。Enabled 仅控制固定账号发现。
type GroupCodexModelsManifestConfig struct {
	// ModelOverrides 仅覆盖当前目录内模型的描述，不创建模型或改变路由。
	// 使用字段级 JSON 保留上游新增字段及显式的 false、0、null。
	ModelOverrides map[string]map[string]json.RawMessage `json:"model_overrides,omitempty"`
	Enabled        bool                                  `json:"enabled"`
	// AccountIDs is the ordered list of pinned account IDs. Order decides
	// merge precedence for duplicate model slugs.
	AccountIDs []int64 `json:"account_ids,omitempty"`
	// FallbackToScheduler controls the behavior when no pinned account is
	// usable or all upstream fetches fail: fall back to the scheduler loop
	// (true) or return the upstream error / 503 (false, default).
	FallbackToScheduler bool `json:"fallback_to_scheduler,omitempty"`
}
