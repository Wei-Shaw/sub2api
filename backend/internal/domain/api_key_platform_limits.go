package domain

// APIKeyPlatformLimit 是某个上游来源（平台）上的 API Key 限额子集。
//
// 字段语义与 api_keys 表上的同名列完全一致：
//   - 0（或字段缺省）= 该维度不额外限制，只受 key 级总限额约束
//   - > 0            = 该来源上的 USD 上限
//
// 平台级限额是 key 级限额的**子限额**：两者同时生效，任一触顶即拒绝。
type APIKeyPlatformLimit struct {
	Quota       float64 `json:"quota,omitempty"`
	RateLimit5h float64 `json:"rate_limit_5h,omitempty"`
	RateLimit1d float64 `json:"rate_limit_1d,omitempty"`
	RateLimit7d float64 `json:"rate_limit_7d,omitempty"`
}

// HasAnyLimit 报告该来源是否至少配置了一档限额。
func (l APIKeyPlatformLimit) HasAnyLimit() bool {
	return l.Quota > 0 || l.RateLimit5h > 0 || l.RateLimit1d > 0 || l.RateLimit7d > 0
}

// APIKeyPlatformLimits 是 platform -> 限额的映射，键取值范围与
// service.AllowedQuotaPlatforms 一致（anthropic/openai/gemini/...）。
//
// 只有 composite 分组的 key 会真正用到多个键；单平台分组的 key 至多命中一个键，
// 此时平台级限额等价于一层更严格的 key 限额。
type APIKeyPlatformLimits map[string]APIKeyPlatformLimit

// HasAnyLimit 报告是否存在任何一个来源配置了限额。
func (m APIKeyPlatformLimits) HasAnyLimit() bool {
	for _, limit := range m {
		if limit.HasAnyLimit() {
			return true
		}
	}
	return false
}

// Limit 返回指定来源的限额配置；未配置时返回零值与 false。
func (m APIKeyPlatformLimits) Limit(platform string) (APIKeyPlatformLimit, bool) {
	if m == nil || platform == "" {
		return APIKeyPlatformLimit{}, false
	}
	limit, ok := m[platform]
	if !ok || !limit.HasAnyLimit() {
		return APIKeyPlatformLimit{}, false
	}
	return limit, true
}
