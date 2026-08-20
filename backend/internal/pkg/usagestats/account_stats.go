package usagestats

// AccountStats 账号使用统计
//
// cost: 账号口径费用（使用 total_cost * account_rate_multiplier）
// standard_cost: 标准费用（使用 total_cost，不含倍率）
// user_cost: 用户/API Key 口径费用（使用 actual_cost，受分组倍率影响）
type AccountStats struct {
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	Cost         float64 `json:"cost"`
	StandardCost float64 `json:"standard_cost"`
	UserCost     float64 `json:"user_cost"`
}

// WindowTokenStats 账号滚动窗口的 token 明细统计。
//
// 由 usage_logs 在 [startTime, endTime) 内聚合（滚动窗口用量历史 finalization
// 的数据源），只含 token 维度，不含费用——费用口径见统计弹窗既有区块。
type WindowTokenStats struct {
	Requests            int64 `json:"requests"`
	TokensTotal         int64 `json:"tokens_total"`
	TokensInput         int64 `json:"tokens_input"`
	TokensOutput        int64 `json:"tokens_output"`
	TokensCacheCreation int64 `json:"tokens_cache_creation"`
	TokensCacheRead     int64 `json:"tokens_cache_read"`
}
