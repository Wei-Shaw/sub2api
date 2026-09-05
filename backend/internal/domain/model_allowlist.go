package domain

// GroupModelAllowlist 是分组级模型白名单：开启后既过滤模型列表类接口的返回内容，
// 也约束分组实际可调用的模型（网关准入在合成路由改写与调度之前完成）。
type GroupModelAllowlist struct {
	Enabled bool     `json:"enabled"`
	Models  []string `json:"models,omitempty"`
}
