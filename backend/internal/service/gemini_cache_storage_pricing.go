package service

import (
	"slices"
	"strings"
)

// Gemini 显式上下文缓存存储的内置官方单价（USD / token / 小时，Standard 档）。
// 存储价与上下文长度无关（≤200K 与 >200K 同价）；Priority 档的更高存储价不在此列。
// 来源：https://ai.google.dev/gemini-api/docs/pricing
// 与 https://cloud.google.com/gemini-enterprise-agent-platform/generative-ai/pricing
const (
	geminiProCacheStoragePrice   = 4.5e-6 // $4.50 / 1M tokens / 小时
	geminiFlashCacheStoragePrice = 1e-6   // $1.00 / 1M tokens / 小时，Flash-Lite 同价

	// 3.6/3.7/3.8 Flash 的优惠价 $0.50 / 1M tokens / 小时，有效期至 2026-12-31，
	// 2027-01-01 起恢复为 geminiFlashCacheStoragePrice。AI Studio 价目页直接标注该优惠价；
	// Vertex 价目页存储价标 $1.00，优惠以「按该模型净消费返还 50% 抵扣金」发放，
	// 官方未明确存储费是否在返还范围内，两类账号统一按优惠价计。
	geminiFlashPromoCacheStoragePrice = 0.5e-6
)

// geminiCacheStorageModelPrices 是已发布型号的存储单价。模型名等于前缀，或以「前缀-」开头
// （-preview、-customtools、-001 等变体）即命中；更具体的前缀排在前面。
var geminiCacheStorageModelPrices = []struct {
	prefix string
	price  float64
}{
	{"gemini-3.8-flash-cyber", geminiFlashCacheStoragePrice},
	{"gemini-3.8-flash", geminiFlashPromoCacheStoragePrice},
	{"gemini-3.7-flash", geminiFlashPromoCacheStoragePrice},
	{"gemini-3.6-flash", geminiFlashPromoCacheStoragePrice},
	{"gemini-3.5-flash-lite", geminiFlashCacheStoragePrice},
	{"gemini-3.5-flash", geminiFlashCacheStoragePrice},
	{"gemini-3.1-flash-lite", geminiFlashCacheStoragePrice},
	{"gemini-3.1-pro", geminiProCacheStoragePrice},
	{"gemini-3-flash", geminiFlashCacheStoragePrice},
	{"gemini-3-pro", geminiProCacheStoragePrice},
	{"gemini-2.5-flash-lite", geminiFlashCacheStoragePrice},
	{"gemini-2.5-flash", geminiFlashCacheStoragePrice},
	{"gemini-2.5-pro", geminiProCacheStoragePrice},
	{"gemini-2.0-flash-lite", geminiFlashCacheStoragePrice},
	{"gemini-2.0-flash", geminiFlashCacheStoragePrice},
}

// builtinGeminiCacheStoragePrice 返回内置的显式缓存存储单价：先按已发布型号匹配，未命中时按系列兜底，
// Pro 系列取 geminiProCacheStoragePrice，Flash 与 Flash-Lite 系列取 geminiFlashCacheStoragePrice（标准价，
// 不套用优惠价）。其他 Gemini 模型与非 Gemini 模型返回 0。
func builtinGeminiCacheStoragePrice(model string) float64 {
	name := lastSegment(strings.ToLower(strings.TrimSpace(model)))
	if !strings.HasPrefix(name, "gemini-") {
		return 0
	}
	parts := strings.Split(name, "-")
	lite := slices.Contains(parts, "lite")
	for _, entry := range geminiCacheStorageModelPrices {
		// Flash-Lite 变体只匹配 Flash-Lite 条目：gemini-3.8-flash-lite 不能落到 3.8 Flash 的优惠价。
		if lite != strings.HasSuffix(entry.prefix, "-lite") {
			continue
		}
		if name == entry.prefix || strings.HasPrefix(name, entry.prefix+"-") {
			return entry.price
		}
	}
	switch {
	case slices.Contains(parts, "pro"):
		return geminiProCacheStoragePrice
	case slices.Contains(parts, "flash"):
		return geminiFlashCacheStoragePrice
	}
	return 0
}
