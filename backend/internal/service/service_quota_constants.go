package service

const (
	SettingKeyServiceQuotaEnabled = "service_quota_enabled"

	// SettingKeyServiceQuotaPreCheckTwoPhase 控制 PreCheck 是否走两阶段路径
	// （PreCheckSelect 在路由前选规则、PreCheckAcquire 在选定 channel/account 后抢槽位）。
	//
	// 默认 false（关闭）：CheckBillingEligibility 行为与历史版本完全一致，PreCheck 在 channel/account
	// 还未确定时被一次性调用，account/channel scope 的 concurrency / rpm 限流器仍然失效。
	//
	// true（开启）：BillingTicket.Prepare 只调 PreCheckSelect 选规则，BillingTicket.Consume 在
	// caller 选定 channel + account 后调用 PreCheckAcquire 才真正抢 concurrency / 增 rpm，
	// account/channel scope 限流器从此生效。详见 BillingCacheService.PrepareBillingCheck。
	SettingKeyServiceQuotaPreCheckTwoPhase = "service_quota_precheck_two_phase"

	ServiceQuotaLimiterRPM         = "rpm"
	ServiceQuotaLimiterTPM         = "tpm"
	ServiceQuotaLimiterTPD         = "tpd"
	ServiceQuotaLimiterDailyUSD    = "daily_usd"
	ServiceQuotaLimiterConcurrency = "concurrency"

	// ServiceQuotaCounterMode* 决定规则 Redis 计数 key 的分片方式：
	//   - user     → 规则只对关联表中列出的用户生效，每个用户独立计数
	//   - per_user → 对 scope 内所有用户生效，按 user_id 分片
	//   - shared   → 对 scope 内所有用户生效，共享同一计数器
	ServiceQuotaCounterModeUser    = "user"
	ServiceQuotaCounterModePerUser = "per_user"
	ServiceQuotaCounterModeShared  = "shared"

	ServiceQuotaWindowFixed   = "fixed"
	ServiceQuotaWindowRolling = "rolling"

	// ServiceQuotaPathOwner* 是 FetchPathIDsByOwner 的 owner 维度，对应 service_quota_paths 表的字段名。
	// 用常量代替魔法字符串避免拼错（service 层与 repo 层共享）。
	ServiceQuotaPathOwnerChannel = "channel_id"
	ServiceQuotaPathOwnerGroup   = "group_id"
	ServiceQuotaPathOwnerAccount = "account_id"

	// ServiceQuotaTokenComponent* 决定 TPM/TPD 限流器把哪几种 token 计入计数。
	// 仅 TPM/TPD 使用此配置；RPM/concurrency/daily_usd 忽略。
	//
	// 默认值（ServiceQuotaTokenComponentsDefault）= {input, output, cache_creation}：
	//   - 与 Anthropic 3.7+ ITPM(input+cache_creation) + OTPM(output) 双桶之和一致
	//   - 与 Bedrock Claude 3.7+ 公式 InputTokenCount + CacheWriteInputTokens + OutputTokenCount 一致
	//   - 剔除 cache_read：鼓励 prompt cache，与 Anthropic 3.7+/Bedrock 3.7+/Groq 一致
	//   - 与 OpenAI/Gemini 单桶相比略松（它们也算 cache_read）
	ServiceQuotaTokenComponentInput         = "input"
	ServiceQuotaTokenComponentOutput        = "output"
	ServiceQuotaTokenComponentCacheCreation = "cache_creation"
	ServiceQuotaTokenComponentCacheRead     = "cache_read"
)

// ServiceQuotaTokenComponentsDefault 是 TPM/TPD 限流器 token_components 字段的默认值，
// 与 DB DEFAULT 一致。新建限流器若未传该字段则采用此默认。
var ServiceQuotaTokenComponentsDefault = []string{
	ServiceQuotaTokenComponentInput,
	ServiceQuotaTokenComponentOutput,
	ServiceQuotaTokenComponentCacheCreation,
}

// ServiceQuotaTokenComponentsAll 是全部合法 token component 值，用于校验 input。
var ServiceQuotaTokenComponentsAll = []string{
	ServiceQuotaTokenComponentInput,
	ServiceQuotaTokenComponentOutput,
	ServiceQuotaTokenComponentCacheCreation,
	ServiceQuotaTokenComponentCacheRead,
}

// IsValidServiceQuotaTokenComponent 判断字符串是否是合法 token component。
func IsValidServiceQuotaTokenComponent(s string) bool {
	switch s {
	case ServiceQuotaTokenComponentInput,
		ServiceQuotaTokenComponentOutput,
		ServiceQuotaTokenComponentCacheCreation,
		ServiceQuotaTokenComponentCacheRead:
		return true
	}
	return false
}

// ServiceQuotaLimiterTypeUsesTokenComponents 判断 limiter type 是否使用 token_components 字段。
// 仅 TPM/TPD；其他类型应忽略此字段（service 层会强制置 nil）。
func ServiceQuotaLimiterTypeUsesTokenComponents(kind string) bool {
	return kind == ServiceQuotaLimiterTPM || kind == ServiceQuotaLimiterTPD
}

// ServiceQuotaLimiterTypeUsesCountOnArrival 判断 limiter type 是否使用 count_on_arrival 字段。
// 仅 RPM 使用；其他类型 service 层会强制置零，避免管理员误填影响判定。
func ServiceQuotaLimiterTypeUsesCountOnArrival(kind string) bool {
	return kind == ServiceQuotaLimiterRPM
}

// ServiceQuotaLimiterTypeIsInteger 判断 limiter type 的 LimitValue 业务上是否必须为整数。
//
// rpm / tpm / tpd / concurrency 都是"次数 / 个数"语义，本就是整数；
// daily_usd 是金额必须保留小数（精度由项目 CLAUDE.md 约定的 decimal 处理，本字段是 float64
// 历史遗留，等彻底切 string-based 再大改）。
//
// service 层用此判定在 normalize 阶段对整数型做 math.Round，截断浮点漂移
// （前端 NumericInput integer prop 是 UX 屏障；后端 round 是数据底线）。
func ServiceQuotaLimiterTypeIsInteger(kind string) bool {
	switch kind {
	case ServiceQuotaLimiterRPM,
		ServiceQuotaLimiterTPM,
		ServiceQuotaLimiterTPD,
		ServiceQuotaLimiterConcurrency:
		return true
	default:
		return false
	}
}
