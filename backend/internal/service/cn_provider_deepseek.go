package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

// DeepSeek 内建注册。
//
//   - 余额：{base}/user/balance（由账号 base_url 衍生，支持自定义域名），
//     balance_infos[] 多币种 + is_available 健康标记。
//   - 无 Coding Plan 滚动窗口端点（QuotaProbe 为 nil）；余额型平台走余额检测
//     而非调度阈值。
//   - 唯一提供原生 OpenAI Responses 端点的 CN 供应商（/responses，无 /v1
//     前缀、无状态，适配 Codex）。
var deepseekProviderSpec = CNProviderSpec{
	Code:        PlatformDeepseek,
	DisplayName: "DeepSeek",
	DefaultBaseURLs: map[CNEndpointKey]string{
		{APIProtocolChatCompletions, ""}: DefaultDeepseekBaseURL,
		{APIProtocolResponses, ""}:       DefaultDeepseekBaseURL,
		{APIProtocolAnthropic, ""}:       DefaultDeepseekAnthropicBaseURL,
	},
	BalanceProbe:            deepseekBalanceProbe{},
	QuotaProbe:              nil, // 无 Coding Plan 窗口端点
	SupportsNativeResponses: true,
	MatchCodingPlanBaseURL:  nil, // deepseek coding 无法路由额度端点
	LiteLLMProvider:         "deepseek",
	ModelDetectPrefixes:     []string{"deepseek-"},
	ModelDetectAliases:      []string{"deepseek"},
}

// deepseekBalanceProbe 实现 DeepSeek 余额探测。
type deepseekBalanceProbe struct{}

func (deepseekBalanceProbe) BalanceURL(account *Account) string {
	// Anthropic 协议账号的凭证 base_url 指向 /anthropic 端点，余额探测需回退
	// 到 OpenAI 格式 base（协议感知）再拼接 /user/balance。
	return strings.TrimRight(account.GetOpenAIFormatBaseURL(), "/") + "/user/balance"
}

func (deepseekBalanceProbe) ParseBalance(body []byte) ([]CNProviderBalanceEntry, bool, string) {
	// is_available 缺省视为 true（健康）；显式存在时取其值。
	available := true
	if v := gjson.GetBytes(body, "is_available"); v.Exists() {
		available = v.Bool()
	}
	// balance_infos 逐条解析：双币种账号同时返回 CNY + USD（数组顺序即
	// 主次序，首条为主币种）。
	balanceInfos := gjson.GetBytes(body, "balance_infos")
	if !balanceInfos.Exists() || !balanceInfos.IsArray() {
		return nil, available, "Invalid balance response: missing balance_infos"
	}
	var entries []CNProviderBalanceEntry
	balanceInfos.ForEach(func(_, info gjson.Result) bool {
		currency := strings.ToUpper(strings.TrimSpace(info.Get("currency").String()))
		totalBalance := info.Get("total_balance")
		balance, ok := cnParseF64(totalBalance.Value())
		if !totalBalance.Exists() || !ok {
			return true
		}
		if currency == "" {
			currency = "CNY"
		}
		entries = append(entries, CNProviderBalanceEntry{Currency: currency, Balance: balance})
		return true
	})
	if len(entries) == 0 {
		return nil, available, "Invalid balance response: no valid balance entries"
	}
	return entries, available, ""
}
