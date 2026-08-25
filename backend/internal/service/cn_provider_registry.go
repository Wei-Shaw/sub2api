package service

import (
	"fmt"
	"net/http"
	"sort"
)

// 国产 OpenAI 兼容供应商（CN provider）注册表。
//
// 背景：CN 供应商体系最初以内建 switch 散落实现（平台集合判定、默认端点矩阵、
// 余额/额度探测解析、调度阈值平台列表……），新增一家供应商需要改动十余处且
// 容易漏改（migration 157 的 platform CHECK 约束曾导致新用户注册拿不到配额行，
// 见 migration 224 的修复）。本注册表把「平台元数据 + 端点矩阵 + 探测行为钩子」
// 收敛为单一权威来源：
//
//   - 平台集合判定（IsCNProvider）、配额/调度阈值平台列表、快照平台列表等
//     全部由注册表派生，注册新平台即自动生效；
//   - 余额/额度探测的平台差异（端点构造、鉴权方式、响应解析）通过可选钩子
//     注入；钩子为 nil 表示该平台无对应探测端点，退化为纯响应式限流
//     （与 zhipu 无公开余额端点的现状同型）；
//   - 内建供应商（kimi/zhipu/deepseek）在 cn_provider_{kimi,zhipu,deepseek}.go
//     中各以一个 RegisterCNProvider 调用完成注册；新增供应商只需新增一个
//     同类文件，无需改动任何现有代码路径。
//
// 注册时机约束：RegisterCNProvider 仅允许在包级 var 初始化或 init() 中调用
// （非并发安全）；运行期注册表只读。

// CNEndpointKey 是默认端点矩阵的键：Protocol 取 APIProtocol*，Mode 取
// AccountMode*（空串表示模式无关的通用端点）。查找顺序为精确 (protocol, mode)
// 命中后回退 (protocol, "")。
type CNEndpointKey struct {
	Protocol string
	Mode     string
}

// CNBalanceProbe 是余额探测钩子，覆盖平台特定的端点构造与 2xx 响应解析。
// 请求执行、代理解析、出站 URL 安全校验、Extra 快照落库由通用层
// （cn_provider_balance_service.go）完成。
type CNBalanceProbe interface {
	// BalanceURL 返回余额探测端点（可由账号 base_url 衍生）。
	BalanceURL(account *Account) string
	// ParseBalance 解析 2xx 响应体：entries 首条为主币种；available 为上游
	// 健康标记（无此概念的平台恒返回 true）；errMsg 非空表示响应结构非法
	// （结果不置 Success，不落快照）。
	ParseBalance(body []byte) (entries []CNProviderBalanceEntry, available bool, errMsg string)
}

// CNQuotaProbe 是 Coding Plan 滚动窗口额度探测钩子。
type CNQuotaProbe interface {
	// QuotaURL 返回额度探测端点（可由账号 base_url 衍生）。
	QuotaURL(account *Account) string
	// QuotaAuthHeader 返回 Authorization 头值（kimi 为 "Bearer "+key；
	// zhipu 为 key 本体，不加 Bearer 前缀）。
	QuotaAuthHeader(apiKey string) string
	// SetQuotaHeaders 写入平台特定的追加请求头（无则空实现）。
	SetQuotaHeaders(req *http.Request)
	// ParseQuota 解析 2xx 响应体为用量窗口列表；planLevel 为套餐档位（可空）；
	// errMsg 非空表示业务级错误（HTTP 2xx 但 success=false 之类）。
	ParseQuota(body []byte) (tiers []CNQuotaTier, planLevel string, errMsg string)
}

// CNProviderSpec 描述一家国产 OpenAI 兼容供应商的注册定义。
type CNProviderSpec struct {
	// Code 是平台标识（account.platform 值），如 "kimi"。注册后不可变。
	Code string
	// DisplayName 是展示名，如 "Kimi (月之暗面)"。
	DisplayName string

	// DefaultBaseURLs 是默认端点矩阵：账号未显式配置 credentials.base_url /
	// api_base_urls 时按 (协议, 模式) 取官方默认端点。
	DefaultBaseURLs map[CNEndpointKey]string

	// BalanceProbe 余额探测钩子；nil 表示该平台无公开余额端点（纯响应式 402/429）。
	BalanceProbe CNBalanceProbe
	// QuotaProbe Coding Plan 额度探测钩子；nil 表示该平台无滚动窗口端点。
	QuotaProbe CNQuotaProbe

	// SupportsNativeResponses 报告该平台是否提供原生 OpenAI Responses 端点
	// （deepseek：/responses，无 /v1 前缀、无状态）。为 false 的平台在
	// adaptive 协议下对 Responses 形状请求回退 Chat Completions 转换链。
	SupportsNativeResponses bool

	// MatchCodingPlanBaseURL 按账号 base_url（已转小写）识别 coding plan 归属，
	// 供 GetCodingPlanProvider 反推；nil 表示该平台无 coding plan 形态。
	MatchCodingPlanBaseURL func(lowerBaseURL string) bool

	// ModelDetectPrefixes 是「模型名 → 平台」识别的前缀列表（如 kimi-、moonshot-），
	// ModelDetectAliases 是 "provider/model" 形式的 provider 段别名（如 kimi、moonshot）。
	// 供 composite 路由的 DetectModelPlatform 使用；不同平台的前缀不得重叠。
	ModelDetectPrefixes []string
	ModelDetectAliases  []string

	// LiteLLMProvider 是 LiteLLM 定价目录中该平台对应的 vendor 键
	// （如 kimi → "moonshot"）；空串表示定价目录中无独立 vendor 条目。
	LiteLLMProvider string
}

// DefaultBaseURL 按 (协议, 模式) 查默认端点：coding 模式先查精确键，
// 再回退 payg 键，最后回退模式无关键（空 Mode）。与历史 switch 语义一致：
// 未设置 account_mode 的账号按 payg 端点处理。
func (s *CNProviderSpec) DefaultBaseURL(protocol, mode string) string {
	if s == nil {
		return ""
	}
	if mode == AccountModeCoding {
		if u := s.DefaultBaseURLs[CNEndpointKey{protocol, AccountModeCoding}]; u != "" {
			return u
		}
	}
	if u := s.DefaultBaseURLs[CNEndpointKey{protocol, AccountModePayG}]; u != "" {
		return u
	}
	return s.DefaultBaseURLs[CNEndpointKey{protocol, ""}]
}

// cnProviderRegistry 是注册表本体：codes 保持注册序（保证 DetectModelPlatform
// 等遍历语义确定），byCode 为索引。
var cnProviderRegistry = struct {
	codes  []string
	byCode map[string]*CNProviderSpec
}{
	byCode: make(map[string]*CNProviderSpec),
}

// RegisterCNProvider 注册一家国产供应商。重复注册同 code 或定义非法会 panic
// （编程错误应在启动期暴露）。仅允许在包级 var 初始化 / init() 中调用。
func RegisterCNProvider(spec CNProviderSpec) {
	if spec.Code == "" {
		panic("cn_provider: register with empty code")
	}
	if _, dup := cnProviderRegistry.byCode[spec.Code]; dup {
		panic(fmt.Sprintf("cn_provider: duplicate registration for %q", spec.Code))
	}
	cloned := spec
	cnProviderRegistry.codes = append(cnProviderRegistry.codes, spec.Code)
	cnProviderRegistry.byCode[spec.Code] = &cloned
	rebuildCNProviderDerivedLists()
}

// GetCNProviderSpec 返回指定平台 code 的注册定义。
func GetCNProviderSpec(code string) (*CNProviderSpec, bool) {
	spec, ok := cnProviderRegistry.byCode[code]
	return spec, ok
}

// CNProviderCodes 返回全部已注册平台 code（注册序）。
func CNProviderCodes() []string {
	out := make([]string, len(cnProviderRegistry.codes))
	copy(out, cnProviderRegistry.codes)
	return out
}

// cnProviderSpecs 返回全部已注册定义（注册序），供遍历匹配（模型识别、
// coding plan 反推等）。
func cnProviderSpecs() []*CNProviderSpec {
	out := make([]*CNProviderSpec, 0, len(cnProviderRegistry.codes))
	for _, code := range cnProviderRegistry.codes {
		out = append(out, cnProviderRegistry.byCode[code])
	}
	return out
}

// 非 CN 的具体上游平台（注册表派生列表的固定前缀）。
var concreteNonCNPlatforms = []string{
	PlatformAnthropic,
	PlatformGemini,
	PlatformOpenAI,
	PlatformAntigravity,
	PlatformGrok,
}

// ConcretePlatforms 返回全部具体上游平台（非 CN 平台 + 已注册 CN 平台，
// 顺序与历史硬编码列表一致）。调度快照、平台筛选等「全平台枚举」统一由此派生。
func ConcretePlatforms() []string {
	out := make([]string, 0, len(concreteNonCNPlatforms)+len(cnProviderRegistry.codes))
	out = append(out, concreteNonCNPlatforms...)
	out = append(out, cnProviderRegistry.codes...)
	return out
}

// rebuildCNProviderDerivedLists 在注册变化后重算派生平台列表。
// AllowedQuotaPlatforms / AllowedSchedulingThresholdPlatforms 保持公开 var
// 形态（调用点零改动），仅在 init 期随注册重建，运行期只读。
func rebuildCNProviderDerivedLists() {
	AllowedQuotaPlatforms = deriveAllowedQuotaPlatforms()
	AllowedSchedulingThresholdPlatforms = deriveAllowedSchedulingThresholdPlatforms()
}

func deriveAllowedQuotaPlatforms() []string {
	// 历史顺序：anthropic/openai/gemini/antigravity/grok + CN 平台（注册序）。
	out := []string{
		PlatformAnthropic,
		PlatformOpenAI,
		PlatformGemini,
		PlatformAntigravity,
		PlatformGrok,
	}
	return append(out, cnProviderRegistry.codes...)
}

func deriveAllowedSchedulingThresholdPlatforms() []string {
	// openai/anthropic/grok 有原生用量窗口；有 QuotaProbe 的 CN 平台（Coding Plan
	// 滚动窗口）纳入阈值评估；余额型平台（无 QuotaProbe）走余额检测而非阈值。
	out := []string{PlatformOpenAI, PlatformAnthropic, PlatformGrok}
	for _, spec := range cnProviderSpecs() {
		if spec.QuotaProbe != nil {
			out = append(out, spec.Code)
		}
	}
	return out
}

// SortedCNProviderCodes 返回按字典序排序的已注册平台 code（仅测试与调试用途）。
func SortedCNProviderCodes() []string {
	out := CNProviderCodes()
	sort.Strings(out)
	return out
}

// cnSupportsNativeResponses 报告平台是否声明了原生 OpenAI Responses 端点能力
// （当前仅 deepseek）。供路由决策使用；端点路径/body 归一化等平台实现细节
// 不在此能力位覆盖范围内。
func cnSupportsNativeResponses(platform string) bool {
	spec, ok := GetCNProviderSpec(platform)
	return ok && spec.SupportsNativeResponses
}
