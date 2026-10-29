package service

import (
	"net/url"
	"strings"
)

// Cline API（https://docs.cline.bot/api/overview）：OpenAI 兼容的多模型聚合端点，
// 基址 https://api.cline.bot/api/v1，只提供 Chat Completions（Cline 自己的客户端同样
// 只走该端点），模型名带厂商前缀（如 anthropic/claude-sonnet-4-6）。Responses 与
// Anthropic 入站由网关转换为 Chat Completions。
//
// 同一个 API Key 下有两个互不影响的额度来源（下称"钱包"），由模型名决定扣哪一个：
//   - 积分：cline-pass/* 与 cline-free/* 以外的模型按量扣账户积分，余额可查；
//   - ClinePass 订阅：cline-pass/* 模型，受 5 小时 / 7 天 / 30 天三档滚动用量上限限制，
//     各档已用百分比与重置时间可查（/api/v1/users/me/plan/usage-limits）；
//   - cline-free/* 免费模型不扣任何一边，只有逐模型的每日额度。
//
// 一边用完只影响该边的模型，所以超限冷却按钱包落在模型级冷却键上（见
// clineWalletRateLimitKey），不停整个账号，也不需要管理员声明账号类型。

const (
	// DefaultClineTestModel 是连接测试的默认模型（按量积分，价格低、长期在售）。
	DefaultClineTestModel = "deepseek/deepseek-v4-flash"
	// DefaultClinePassTestModel 是订阅了 ClinePass 的账号连接测试的默认模型。
	DefaultClinePassTestModel = "cline-pass/glm-5.3-flash"

	clineAPIHost = "api.cline.bot"

	clinePassModelPrefix = "cline-pass/"
	clineFreeModelPrefix = "cline-free/"

	// clinePassRateLimitKey / clineCreditsRateLimitKey 是两个钱包的模型级冷却键：
	// 命中后该钱包的全部模型都不再调度到该账号，另一边不受影响。
	clinePassRateLimitKey    = "cline:pass"
	clineCreditsRateLimitKey = "cline:credits"
)

// IsCline 报告账号是否为 Cline 平台账号。
func (a *Account) IsCline() bool {
	return a != nil && a.Platform == PlatformCline
}

// clineWalletRateLimitKey 返回模型所属钱包的冷却键；免费模型不属于任何钱包，返回空串。
func clineWalletRateLimitKey(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case model == "":
		return ""
	case strings.HasPrefix(model, clinePassModelPrefix):
		return clinePassRateLimitKey
	case strings.HasPrefix(model, clineFreeModelPrefix):
		return ""
	default:
		return clineCreditsRateLimitKey
	}
}

// isOfficialClineHost 报告 URL 是否指向官方 Cline API 主机。
func isOfficialClineHost(target string) bool {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https") && strings.EqualFold(parsed.Hostname(), clineAPIHost)
}

// clineAccountAPISupported 报告账号能否查询积分余额与 ClinePass 用量：API Key 账号且
// 推理端点指向官方主机。自定义中转的 Key 不发往官方账号接口。
func (a *Account) clineAccountAPISupported() bool {
	return a.IsCline() && a.Type == AccountTypeAPIKey && isOfficialClineHost(a.GetOpenAIBaseURL())
}

// clinePassSubscribed 报告最近一次用量探测是否显示账号订阅了 ClinePass（有窗口快照）。
func (a *Account) clinePassSubscribed() bool {
	if a == nil || a.Extra == nil {
		return false
	}
	for _, suffix := range []string{cnExtraSuffix5hUsed, cnExtraSuffixWeeklyUsed, cnExtraSuffixMonthlyUsed} {
		if value, ok := a.Extra[cnExtraKey(PlatformCline, suffix)]; ok && value != nil {
			return true
		}
	}
	return false
}
