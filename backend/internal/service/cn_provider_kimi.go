package service

import (
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// Kimi（月之暗面 / Moonshot）内建注册。
//
//   - 余额：固定 https://api.moonshot.cn/v1/users/me/balance（与 base_url 无关，
//     Moonshot 仅此一处），data.available_balance 单币种 CNY。
//   - Coding Plan 额度：{base}/v1/usages（5h 滚动窗口 + weekly 周窗口）。
var kimiProviderSpec = CNProviderSpec{
	Code:        PlatformKimi,
	DisplayName: "Kimi (月之暗面)",
	DefaultBaseURLs: map[CNEndpointKey]string{
		{APIProtocolChatCompletions, AccountModePayG}:   DefaultKimiPayGBaseURL,
		{APIProtocolChatCompletions, AccountModeCoding}: DefaultKimiCodingBaseURL,
		{APIProtocolResponses, AccountModePayG}:         DefaultKimiPayGBaseURL,
		{APIProtocolResponses, AccountModeCoding}:       DefaultKimiCodingBaseURL,
		{APIProtocolAnthropic, AccountModePayG}:         DefaultKimiPayGAnthropicBaseURL,
		{APIProtocolAnthropic, AccountModeCoding}:       DefaultKimiCodingAnthropicBaseURL,
	},
	BalanceProbe:            kimiBalanceProbe{},
	QuotaProbe:              kimiQuotaProbe{},
	SupportsNativeResponses: false,
	MatchCodingPlanBaseURL: func(lowerBaseURL string) bool {
		return strings.Contains(lowerBaseURL, "api.kimi.com/coding")
	},
	LiteLLMProvider:     "moonshot",
	ModelDetectPrefixes: []string{"kimi-", "moonshot-"},
	ModelDetectAliases:  []string{"kimi", "moonshot"},
}

// kimiBalanceProbe 实现 Moonshot 余额探测。
type kimiBalanceProbe struct{}

func (kimiBalanceProbe) BalanceURL(_ *Account) string {
	return "https://api.moonshot.cn/v1/users/me/balance"
}

func (kimiBalanceProbe) ParseBalance(body []byte) ([]CNProviderBalanceEntry, bool, string) {
	// Moonshot：code==0 成功；data.available_balance（number），单币种 CNY。
	balance, _ := cnParseF64(gjson.GetBytes(body, "data.available_balance").Value())
	return []CNProviderBalanceEntry{{Currency: "CNY", Balance: balance}}, true, ""
}

// kimiQuotaProbe 实现 Kimi For Coding 额度探测。
type kimiQuotaProbe struct{}

// QuotaURL 根据 base_url 解析 Kimi For Coding 额度端点。
// cc-switch query_kimi 固定探测 https://api.kimi.com/coding/v1/usages
// （实测 /coding/usages 无 /v1 → 404）。coding/v1（CC 协议默认）与
// coding（Anthropic 协议默认）两种 base 统一剥掉尾部后拼回 /v1/usages，
// 协议切换不影响额度探测端点。
func (kimiQuotaProbe) QuotaURL(account *Account) string {
	base := strings.TrimSuffix(strings.TrimRight(account.GetOpenAIBaseURL(), "/"), "/v1")
	return base + "/v1/usages"
}

func (kimiQuotaProbe) QuotaAuthHeader(apiKey string) string {
	return "Bearer " + apiKey
}

func (kimiQuotaProbe) SetQuotaHeaders(_ *http.Request) {}

// ParseQuota 解析 Kimi For Coding 的 /usages 响应。
//
//   - limits[].detail.{limit,remaining,resetTime} → 5h 窗口（取首个 detail）
//   - usage.{limit,remaining,resetTime} → 周窗口
//
// utilization = (limit-remaining)/limit*100。
func (kimiQuotaProbe) ParseQuota(body []byte) ([]CNQuotaTier, string, string) {
	return parseKimiUsageTiers(body), "", ""
}

func parseKimiUsageTiers(body []byte) []CNQuotaTier {
	var tiers []CNQuotaTier

	if limits := gjson.GetBytes(body, "limits"); limits.IsArray() {
		limits.ForEach(func(_, item gjson.Result) bool {
			detail := item.Get("detail")
			if !detail.Exists() {
				return true
			}
			limit, _ := cnParseF64(detail.Get("limit").Value())
			remaining, _ := cnParseF64(detail.Get("remaining").Value())
			used := limit - remaining
			if used < 0 {
				used = 0
			}
			var util float64
			if limit > 0 {
				util = used / limit * 100
			}
			tiers = append(tiers, CNQuotaTier{
				Window:      "5h",
				UsedPercent: util,
				ResetAt:     cnNormalizeResetTime(detail.Get("resetTime").Value()),
			})
			return false // 取首个 detail 作为 5h 窗口
		})
	}

	if usage := gjson.GetBytes(body, "usage"); usage.Exists() {
		limit, _ := cnParseF64(usage.Get("limit").Value())
		remaining, _ := cnParseF64(usage.Get("remaining").Value())
		used := limit - remaining
		if used < 0 {
			used = 0
		}
		var util float64
		if limit > 0 {
			util = used / limit * 100
		}
		tiers = append(tiers, CNQuotaTier{
			Window:      "weekly",
			UsedPercent: util,
			ResetAt:     cnNormalizeResetTime(usage.Get("resetTime").Value()),
		})
	}

	return tiers
}
