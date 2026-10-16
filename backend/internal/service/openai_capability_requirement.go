package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
)

// OpenAIAccountRequirement 是从请求体解析出的账号能力需求：Daybreak 等级与 service tier。
// 零值表示没有需求。需求以显式参数穿过调度链，不走 context：漏注入时静默等于「无需求」
// 会把 Ultrafast 请求路由到无权限账号并按 Ultrafast 计费，正是本能力要消灭的故障。
type OpenAIAccountRequirement struct {
	CyberProgram string
	ServiceTier  string
}

const (
	openAICapabilityReasonDaybreakPrefix = "daybreak_not_supported:"
	openAICapabilityReasonTierPrefix     = "service_tier_not_supported:"
)

// IsZero 报告是否没有任何需求。
func (r OpenAIAccountRequirement) IsZero() bool {
	return r.CyberProgram == "" && r.ServiceTier == ""
}

// Names 返回需求涉及的能力名，供错误消息使用。
func (r OpenAIAccountRequirement) Names() []string {
	names := make([]string, 0, 2)
	if r.CyberProgram != "" {
		names = append(names, r.CyberProgram)
	}
	if r.ServiceTier != "" {
		names = append(names, r.ServiceTier)
	}
	return names
}

// AppliesTo 报告能力过滤是否适用于该账号：只有使用 ChatGPT/Codex 协议的 OpenAI OAuth 类账号
// 有探测来源。API Key 账号（priority 是其原生合法参数）与其他平台账号没有能力记录，
// 若按「无记录即无能力」会被永久排除，因此直接放行并交给上游处理。
func (r OpenAIAccountRequirement) AppliesTo(account *Account) bool {
	return account != nil && account.IsOpenAIOAuthLike()
}

// SatisfiedBy 判断账号能力记录是否满足完整需求；不满足时返回命名原因
// （daybreak_not_supported:<program> / service_tier_not_supported:<tier>）。
// 无记录的 OAuth 账号视为不具备任何能力。
func (r OpenAIAccountRequirement) SatisfiedBy(account *Account) (bool, string) {
	if r.IsZero() || !r.AppliesTo(account) {
		return true, ""
	}
	caps := account.CodexCapabilities()
	if r.CyberProgram != "" && !caps.SupportsCyberProgram(r.CyberProgram) {
		return false, openAICapabilityReasonDaybreakPrefix + r.CyberProgram
	}
	if r.ServiceTier != "" && !caps.SupportsServiceTier(r.ServiceTier) {
		return false, openAICapabilityReasonTierPrefix + r.ServiceTier
	}
	return true, ""
}

// ParseOpenAIAccountRequirement 从请求体顶层 access_programs.cyber 与 service_tier 解析需求。
//   - cyber 为 daybreak_blue / daybreak_red 时构成 Daybreak 需求；standard、缺省与未知值不构成需求
//     （未知值交给上游报错）。
//   - service_tier 归一化沿用 normalizeOpenAIServiceTier：fast→priority；priority / ultrafast
//     构成需求；flex / auto / default / scale、缺省与未知值不构成需求。
//
// requested 是客户端原始需求，用于分组白名单准入；routing 是分组改写后的选号需求：
// 分组开启强制 Fast 时 tier 折叠为 priority（与转发层无条件改写为 priority 的行为一致），
// 否则等于 requested。
func ParseOpenAIAccountRequirement(body []byte, group *Group) (requested, routing OpenAIAccountRequirement) {
	if len(body) > 0 && gjson.ValidBytes(body) {
		switch program := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "access_programs.cyber").String())); program {
		case CodexCyberProgramBlue, CodexCyberProgramRed:
			requested.CyberProgram = program
		}
		switch tier := normalizedOpenAIServiceTierValue(gjson.GetBytes(body, "service_tier").String()); tier {
		case OpenAIFastTierPriority, OpenAIFastTierUltrafast:
			requested.ServiceTier = tier
		}
	}
	return requested, foldOpenAIRoutingRequirement(requested, group)
}

// ParseOpenAIMessagesAccountRequirement 是 Messages（Anthropic 形态）入口的解析：转换层
// （AnthropicRequest → ResponsesRequest）不携带请求体 service_tier，该路径上唯一到达上游的
// tier 是 anthropic-beta fast-mode 头注入的 priority。准入与路由必须与实际转发一致：
// 请求体 service_tier 不构成需求，fast-mode 头构成 Fast 需求；access_programs 照常解析。
func ParseOpenAIMessagesAccountRequirement(body []byte, anthropicBeta string, group *Group) (requested, routing OpenAIAccountRequirement) {
	requested, _ = ParseOpenAIAccountRequirement(body, group)
	requested.ServiceTier = ""
	if containsBetaToken(anthropicBeta, claude.BetaFastMode) {
		requested.ServiceTier = OpenAIFastTierPriority
	}
	return requested, foldOpenAIRoutingRequirement(requested, group)
}

// foldOpenAIRoutingRequirement 把客户端原始需求折叠为选号需求：分组开启强制 Fast 时 tier
// 折叠为 priority（与转发层无条件改写为 priority 一致），否则等于原始需求。
func foldOpenAIRoutingRequirement(requested OpenAIAccountRequirement, group *Group) OpenAIAccountRequirement {
	routing := requested
	if group != nil && groupSupportsOpenAIFast(group.Platform) && group.ForceOpenAIFast {
		routing.ServiceTier = OpenAIFastTierPriority
	}
	return routing
}
