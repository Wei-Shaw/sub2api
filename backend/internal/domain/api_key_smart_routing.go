package domain

// SmartRoutingConfig 是 API Key 智能路由的配置。
//
// 当 API Key 启用智能路由（smart_routing_enabled=true）时，不再绑定单一分组，
// 而是在每次请求时根据请求的模型（model）自动选择合适的分组。候选分组为
// 用户当前可用的分组（GetAvailableGroups 语义）减去 ExcludeGroupIDs。
// 随后从能服务该模型的分组中，按优先级（Priorities，数值大者优先）与
// 权重（Weights，同优先级下加权随机）选出目标分组。
type SmartRoutingConfig struct {
	// ExcludeGroupIDs 表示智能路由时排除的分组 ID 列表（永不选中）。
	ExcludeGroupIDs []int64 `json:"exclude_group_ids,omitempty"`
	// Priorities 记录 分组ID -> 优先级。数值越大，优先级越高，越优先被选中。
	Priorities map[int64]int `json:"priorities,omitempty"`
	// Weights 记录 分组ID -> 权重。当多个候选分组优先级相同时，按权重加权随机选择。
	Weights map[int64]int `json:"weights,omitempty"`
}

// IsEmpty 返回配置是否完全为空（未设置任何选项）。
func (c SmartRoutingConfig) IsEmpty() bool {
	return len(c.ExcludeGroupIDs) == 0 && len(c.Priorities) == 0 && len(c.Weights) == 0
}
