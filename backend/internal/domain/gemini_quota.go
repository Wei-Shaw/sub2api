package domain

// GeminiUsageTotals 汇总 Gemini 账号的 Pro/Flash 请求、token 与费用
// （属 gemini-quota BC；纯标量，无跨 BC 指针）。
type GeminiUsageTotals struct {
	ProRequests   int64
	FlashRequests int64
	ProTokens     int64
	FlashTokens   int64
	ProCost       float64
	FlashCost     float64
}
