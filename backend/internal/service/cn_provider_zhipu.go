package service

import (
	"net/http"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
)

// 智谱 GLM（bigmodel）内建注册。
//
//   - 余额：无公开余额端点（OpenAPI 规格验证），BalanceProbe 为 nil，
//     仅靠响应式 429/402（见 ratelimit_cn_providers.go）。
//   - Coding Plan 额度：{host}/api/monitor/usage/quota/limit（5h + weekly 窗口），
//     鉴权不加 Bearer 前缀，业务级错误以 success=false 表达。
var zhipuProviderSpec = CNProviderSpec{
	Code:        PlatformZhipu,
	DisplayName: "Zhipu GLM (智谱)",
	DefaultBaseURLs: map[CNEndpointKey]string{
		{APIProtocolChatCompletions, AccountModePayG}:   DefaultZhipuPayGBaseURL,
		{APIProtocolChatCompletions, AccountModeCoding}: DefaultZhipuCodingBaseURL,
		{APIProtocolResponses, AccountModePayG}:         DefaultZhipuPayGBaseURL,
		{APIProtocolResponses, AccountModeCoding}:       DefaultZhipuCodingBaseURL,
		{APIProtocolAnthropic, ""}:                      DefaultZhipuAnthropicBaseURL,
	},
	BalanceProbe:            nil, // 无公开余额端点
	QuotaProbe:              zhipuQuotaProbe{},
	SupportsNativeResponses: false,
	MatchCodingPlanBaseURL: func(lowerBaseURL string) bool {
		return strings.Contains(lowerBaseURL, "bigmodel.cn") || strings.Contains(lowerBaseURL, "api.z.ai")
	},
	LiteLLMProvider:     "zhipu",
	ModelDetectPrefixes: []string{"glm-"},
	ModelDetectAliases:  []string{"zhipu", "glm", "bigmodel"},
}

// zhipuQuotaProbe 实现智谱 Coding Plan 额度探测。
type zhipuQuotaProbe struct{}

func (zhipuQuotaProbe) QuotaURL(account *Account) string {
	return zhipuQuotaHost(account.GetOpenAIBaseURL()) + "/api/monitor/usage/quota/limit"
}

// QuotaAuthHeader 智谱额度端点鉴权不加 Bearer 前缀。
func (zhipuQuotaProbe) QuotaAuthHeader(apiKey string) string {
	return apiKey
}

func (zhipuQuotaProbe) SetQuotaHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "en-US,en")
}

// ParseQuota 解析智谱额度响应：先做业务级错误判定（HTTP 2xx 但
// success=false），再解析 data.limits 为 5h + weekly 两档。
func (zhipuQuotaProbe) ParseQuota(body []byte) ([]CNQuotaTier, string, string) {
	if success := gjson.GetBytes(body, "success"); success.Exists() && !success.Bool() {
		msg := strings.TrimSpace(gjson.GetBytes(body, "msg").String())
		if msg == "" {
			msg = "unknown zhipu quota error"
		}
		return nil, "", "API error: " + msg
	}
	data := gjson.GetBytes(body, "data")
	planLevel := strings.TrimSpace(gjson.GetBytes(body, "data.level").String())
	return parseZhipuTokenTiers(data), planLevel, ""
}

func zhipuQuotaHost(baseURL string) string {
	switch u := strings.ToLower(baseURL); {
	case strings.Contains(u, "bigmodel.cn"):
		return "https://open.bigmodel.cn"
	case strings.Contains(u, "z.ai"):
		return "https://api.z.ai"
	default:
		// 国产优先：未知域名回落国内站（与前端 zhipu 预设一致）。
		return "https://open.bigmodel.cn"
	}
}

// cnZhipuWindow 标识智谱 TOKENS_LIMIT 条目所属窗口。
type cnZhipuWindow int

const (
	cnZhipuWindowUnknown cnZhipuWindow = iota
	cnZhipuWindow5h
	cnZhipuWindowWeekly
)

// classifyZhipuWindowUnit 按 unit 字段判定窗口类型（3=5h，6=weekly）。
// unit 缺失或未识别时返回 Unknown，由调用方走 reset 时间启发式兜底。
func classifyZhipuWindowUnit(unit int64) cnZhipuWindow {
	switch unit {
	case 3:
		return cnZhipuWindow5h
	case 6:
		return cnZhipuWindowWeekly
	default:
		return cnZhipuWindowUnknown
	}
}

// parseZhipuTokenTiers 解析智谱额度响应 data.limits 为 5h + weekly 两档。
//
// 分类优先级（对齐 cc-switch parse_zhipu_token_tiers，issue #3036）：
//  1. 显式 unit 字段（3=5h / 6=weekly）——不能用 reset 排序代替，周期末尾
//     周窗口会比 5h 更早重置，时间排序必然标反。
//  2. unit 缺失/未识别：无 nextResetTime 的条目优先归 5h（0% 状态下 5h 桶可能
//     没有 reset），其余按 reset 升序依次填入仍空缺的槽位。
//
// CREDIT_LIMIT（信用额度）与 TOKENS_LIMIT（token 窗口）度量不同：两者同时返回时
// 只让 TOKENS_LIMIT 参与 5h/weekly 槽位竞争，避免信用额度百分比污染阈值停调
// 快照；仅当无任何 TOKENS_LIMIT 条目时才降级用 CREDIT_LIMIT 展示。
// 老套餐只回 1 条 TOKENS_LIMIT，自然降级为仅 5h；新套餐回 2 条。
func parseZhipuTokenTiers(data gjson.Result) []CNQuotaTier {
	type entry struct {
		resetMs    int64
		hasReset   bool
		percentage float64
		resetISO   string
	}
	var (
		fiveHour     entry
		fiveHourSet  bool
		weekly       entry
		weeklySet    bool
		unclassified []entry
	)

	classify := func(item gjson.Result, e entry) {
		switch classifyZhipuWindowUnit(item.Get("unit").Int()) {
		case cnZhipuWindow5h:
			if !fiveHourSet {
				fiveHour, fiveHourSet = e, true
			} else {
				unclassified = append(unclassified, e)
			}
		case cnZhipuWindowWeekly:
			if !weeklySet {
				weekly, weeklySet = e, true
			} else {
				unclassified = append(unclassified, e)
			}
		default:
			unclassified = append(unclassified, e)
		}
	}
	var creditFallback []entry
	hasTokensLimit := false

	data.Get("limits").ForEach(func(_, item gjson.Result) bool {
		limitType := strings.ToUpper(strings.TrimSpace(item.Get("type").String()))
		if limitType != "TOKENS_LIMIT" && limitType != "CREDIT_LIMIT" {
			return true
		}
		percentage := 0.0
		if p, ok := cnParseF64(item.Get("percentage").Value()); ok {
			percentage = p
		}
		var (
			resetMs  int64
			hasReset bool
			resetISO string
		)
		if nr := item.Get("nextResetTime"); nr.Exists() {
			switch nr.Type {
			case gjson.Number:
				resetMs = nr.Int()
				hasReset = resetMs > 0
				resetISO = cnMillisToRFC3339(resetMs)
			case gjson.String:
				resetISO = cnNormalizeResetTime(nr.String())
				hasReset = resetISO != ""
			}
		}
		e := entry{resetMs: resetMs, hasReset: hasReset, percentage: percentage, resetISO: resetISO}
		if limitType == "TOKENS_LIMIT" {
			hasTokensLimit = true
			classify(item, e)
		} else {
			creditFallback = append(creditFallback, e)
		}
		return true
	})

	// 无任何 TOKENS_LIMIT 条目（部分套餐只报信用额度）：降级用 CREDIT_LIMIT 展示。
	if !hasTokensLimit {
		unclassified = append(unclassified, creditFallback...)
	}

	// 无 reset 的条目排前，再按 reset 升序，依次填入仍空缺的槽位。
	sort.SliceStable(unclassified, func(i, j int) bool {
		if unclassified[i].hasReset != unclassified[j].hasReset {
			return !unclassified[i].hasReset
		}
		return unclassified[i].resetMs < unclassified[j].resetMs
	})
	for _, e := range unclassified {
		switch {
		case !fiveHourSet:
			fiveHour, fiveHourSet = e, true
		case !weeklySet:
			weekly, weeklySet = e, true
		}
	}

	var tiers []CNQuotaTier
	if fiveHourSet {
		tiers = append(tiers, CNQuotaTier{Window: "5h", UsedPercent: fiveHour.percentage, ResetAt: fiveHour.resetISO})
	}
	if weeklySet {
		tiers = append(tiers, CNQuotaTier{Window: "weekly", UsedPercent: weekly.percentage, ResetAt: weekly.resetISO})
	}
	return tiers
}
