package service

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ChannelMonitor 全局常量。
// 这些是 MVP 阶段的硬编码值，按需可以提到 config 中。
const (
	// monitorRequestTimeout 单次模型请求总超时（含 Body 读取）。
	monitorRequestTimeout = 45 * time.Second
	// monitorPingTimeout HEAD 请求 endpoint origin 的超时。
	monitorPingTimeout = 8 * time.Second
	// monitorDegradedThreshold 主请求成功但耗时超过该阈值视为 degraded。
	monitorDegradedThreshold = 6 * time.Second
	// monitorHistoryRetentionDays 明细历史保留天数。
	// 60s 默认间隔 * 30 天 ≈ 43200 行/monitor/model，一般部署总量 <= 2M 行，
	// PG 无压力；所以直接保留完整明细一个月，可用率查询可以全走原始行不依赖聚合。
	// 聚合表 channel_monitor_daily_rollups 仍然保留，作为长期历史回填/降级查询的兜底。
	monitorHistoryRetentionDays = 30
	// monitorRollupRetentionDays 日聚合保留天数。
	// 日聚合行由 RunDailyMaintenance 在超过该窗口后软删。
	monitorRollupRetentionDays = 30
	// monitorMaintenanceMaxDaysPerRun 单次维护任务最多聚合的天数。
	// 用于限制首次上线回填（30 天）+ 少量余量，避免长事务。
	monitorMaintenanceMaxDaysPerRun = 35
	// monitorWorkerConcurrency 调度器并发执行的监控数（pond 池容量）。
	monitorWorkerConcurrency = 5
	// monitorStartupLoadTimeout Start 时一次性加载所有 enabled monitor 的总超时。
	monitorStartupLoadTimeout = 10 * time.Second
	// monitorMinIntervalSeconds / monitorMaxIntervalSeconds 用户配置的检测间隔上下限。
	monitorMinIntervalSeconds = 15
	monitorMaxIntervalSeconds = 3600
	// monitorMessageMaxBytes message 字段最大字节数（与 schema/migration 一致）。
	monitorMessageMaxBytes = 500
	// monitorResponseMaxBytes 单次模型响应最大读取字节，防止 OOM。
	monitorResponseMaxBytes = 64 * 1024
	// monitorErrorBodySnippetMaxBytes 非 2xx 响应时保留上游 body 片段的最大字节数。
	// 留 300 字节足够覆盖典型结构化错误（如 `{"error":{"message":"..."}}`），
	// 又给 "upstream HTTP <status>: " 前缀留出余量，避免最终被 monitorMessageMaxBytes (500) 截得太狠。
	monitorErrorBodySnippetMaxBytes = 300
	// monitorChallengeMin / monitorChallengeMax challenge 操作数范围。
	monitorChallengeMin = 1
	monitorChallengeMax = 50

	// providerOpenAIPath OpenAI Chat Completions 路径（Kimi / DeepSeek 同为 OpenAI 兼容）。
	providerOpenAIPath = "/v1/chat/completions"
	// providerGrokPath Grok OpenAI-compatible Chat Completions 路径。
	providerGrokPath = "/v1/chat/completions"
	// providerZhipuPath 智谱 OpenAI 兼容 Chat Completions 路径（前缀与官方不同）。
	providerZhipuPath = "/api/paas/v4/chat/completions"
	// providerOpenAIResponsesPath OpenAI Responses API 路径。
	providerOpenAIResponsesPath = "/v1/responses"
	// providerAnthropicPath Anthropic Messages 路径。
	providerAnthropicPath = "/v1/messages"
	// providerGeminiPathTemplate Gemini generateContent 路径模板（含 model 占位）。
	providerGeminiPathTemplate = "/v1beta/models/%s:generateContent"

	// MonitorProviderOpenAI 等 provider 字符串常量（也是 ent enum 的实际值）。
	// 后 4 个 provider（antigravity/kimi/zhipu/deepseek）为配额模式引入：
	// antigravity 无探活 adapter（仅配额），其余 3 个复用 OpenAI 兼容探活。
	MonitorProviderOpenAI      = "openai"
	MonitorProviderAnthropic   = "anthropic"
	MonitorProviderGemini      = "gemini"
	MonitorProviderGrok        = "grok"
	MonitorProviderAntigravity = "antigravity"
	MonitorProviderKimi        = "kimi"
	MonitorProviderZhipu       = "zhipu"
	MonitorProviderDeepseek    = "deepseek"

	// MonitorCheckMode 检测模式（channel_monitors.check_mode）。
	//   probe       - LLM 探活（默认，原有行为）
	//   quota       - 仅查关联账号用量/余额，零 LLM 成本
	//   quota_probe - 探活 + 配额并存（配额快照挂到主模型历史行）
	MonitorCheckModeProbe      = "probe"
	MonitorCheckModeQuota      = "quota"
	MonitorCheckModeQuotaProbe = "quota_probe"

	// MonitorDefaultQuotaModel 是 quota 模式监控未显式指定模型时占位的虚拟模型名
	// （primary_model 列 NotEmpty，用 "quota" 让历史行/时间线机制无需特判）。
	MonitorDefaultQuotaModel = "quota"

	// monitorQuotaFetchCacheTTL 配额快照缓存时长。多个监控可能关联同一账号，
	// 而 interval 最小 15s 且国产配额服务无缓存，TTL 防止打爆上游配额端点。
	monitorQuotaFetchCacheTTL = 5 * time.Minute
	// monitorQuotaErrorCacheTTL 失败快照的负缓存时长：失败也短缓存，避免
	// 故障/凭据失效期间每次调度（最小 15s）都带真实凭据打上游；到期自动重试。
	monitorQuotaErrorCacheTTL = 60 * time.Second
	// monitorQuotaFetchTimeout singleflight 内单次配额抓取的总超时
	// （脱离调用方 ctx，防止某个监控的取消波及共享同一账号的其他监控）。
	monitorQuotaFetchTimeout = 45 * time.Second
	// monitorQuotaDegradedUsedPercent 任一用量窗口使用率超过该阈值时，
	// 配额检查状态记为 degraded（对齐账号页展示阈值）。
	monitorQuotaDegradedUsedPercent = 90.0

	// MonitorDefaultGrokModel 是新增 Grok 监控未显式指定模型时使用的轻量测活模型。
	MonitorDefaultGrokModel = "grok-4.5"

	// MonitorStatusOperational 等监控状态字符串常量（与 ent enum 一致）。
	MonitorStatusOperational = "operational"
	MonitorStatusDegraded    = "degraded"
	MonitorStatusFailed      = "failed"
	MonitorStatusError       = "error"

	// monitorAvailability7Days / 15 / 30 用于聚合查询窗口。
	monitorAvailability7Days  = 7
	monitorAvailability15Days = 15
	monitorAvailability30Days = 30

	// MonitorHistoryDefaultLimit 历史查询默认返回条数（handler 层共享）。
	MonitorHistoryDefaultLimit = 100
	// MonitorHistoryMaxLimit 历史查询最大返回条数（handler 层共享）。
	MonitorHistoryMaxLimit = 1000

	// monitorTimelineMaxPoints 用户视图 timeline 每个监控最多返回的历史点数。
	monitorTimelineMaxPoints = 60

	// monitorEndpointResolveTimeout validateEndpoint 解析 hostname 的最长耗时。
	monitorEndpointResolveTimeout = 5 * time.Second

	// ---- checker / runner 行为参数（消除 magic 值）----

	// monitorAnthropicAPIVersion Anthropic Messages API 版本头。
	monitorAnthropicAPIVersion = "2023-06-01"
	// monitorChallengeMaxTokens 单次 challenge 请求的 max_tokens（足够回答个位数算术）。
	monitorChallengeMaxTokens = 50

	// monitorRunOneBuffer runOne 的总超时缓冲（除请求超时与 ping 超时外的额外裕量）。
	monitorRunOneBuffer = 10 * time.Second

	// monitorIdleConnTimeout HTTP transport 空闲连接关闭超时。
	monitorIdleConnTimeout = 30 * time.Second
	// monitorTLSHandshakeTimeout HTTP transport TLS 握手超时。
	monitorTLSHandshakeTimeout = 10 * time.Second
	// monitorResponseHeaderTimeout HTTP transport 等待响应头超时。
	monitorResponseHeaderTimeout = 30 * time.Second
	// monitorPingDiscardMaxBytes ping 时丢弃响应体的最大字节数。
	monitorPingDiscardMaxBytes = 1024

	// monitorDialTimeout 自定义 dialer 单次连接超时。
	monitorDialTimeout = 10 * time.Second
	// monitorDialKeepAlive 自定义 dialer keep-alive 间隔。
	monitorDialKeepAlive = 30 * time.Second
)

// 业务错误（统一在此声明，避免散落）。
var (
	ErrChannelMonitorNotFound = infraerrors.NotFound(
		"CHANNEL_MONITOR_NOT_FOUND", "channel monitor not found",
	)
	ErrChannelMonitorInvalidProvider = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_PROVIDER", "provider must be one of openai/anthropic/gemini/grok/antigravity/kimi/zhipu/deepseek",
	)
	ErrChannelMonitorInvalidCheckMode = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_CHECK_MODE", "check_mode must be one of probe/quota/quota_probe; antigravity only supports quota",
	)
	ErrChannelMonitorAccountRequired = infraerrors.BadRequest(
		"CHANNEL_MONITOR_ACCOUNT_REQUIRED", "account_id is required for quota-based check_mode",
	)
	ErrChannelMonitorProviderIncompatible = infraerrors.BadRequest(
		"CHANNEL_MONITOR_PROVIDER_INCOMPATIBLE", "monitor provider must match the linked account platform",
	)
	ErrChannelMonitorAccountNotSupportable = infraerrors.BadRequest(
		"CHANNEL_MONITOR_ACCOUNT_NOT_SUPPORTABLE", "linked account cannot serve as a quota data source (cn coding plan must be kimi/zhipu, cn payg must be kimi/deepseek, openai requires an oauth account, anthropic requires oauth or setup-token)",
	)
	ErrChannelMonitorInvalidAPIMode = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_API_MODE", "api_mode must be chat_completions or responses; responses is only supported for openai",
	)
	ErrChannelMonitorInvalidRequestBody = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_REQUEST_BODY", "openai-compatible replace-mode body_override must include non-empty messages for chat_completions or non-empty instructions and input for responses",
	)
	ErrChannelMonitorInvalidInterval = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_INTERVAL", "interval_seconds must be in [15, 3600]",
	)
	ErrChannelMonitorInvalidJitter = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_JITTER", "jitter_seconds must be >= 0 and interval_seconds - jitter_seconds must be >= 15",
	)
	ErrChannelMonitorInvalidEndpoint = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_ENDPOINT", "endpoint must be a valid https URL",
	)
	ErrChannelMonitorEndpointScheme = infraerrors.BadRequest(
		"CHANNEL_MONITOR_ENDPOINT_SCHEME", "endpoint must use https scheme",
	)
	ErrChannelMonitorEndpointPath = infraerrors.BadRequest(
		"CHANNEL_MONITOR_ENDPOINT_PATH", "endpoint must be base origin only (no path/query/fragment)",
	)
	ErrChannelMonitorEndpointPrivate = infraerrors.BadRequest(
		"CHANNEL_MONITOR_ENDPOINT_PRIVATE", "endpoint must be a public host",
	)
	ErrChannelMonitorEndpointUnreachable = infraerrors.BadRequest(
		"CHANNEL_MONITOR_ENDPOINT_UNREACHABLE", "endpoint hostname could not be resolved",
	)
	ErrChannelMonitorMissingAPIKey = infraerrors.BadRequest(
		"CHANNEL_MONITOR_MISSING_API_KEY", "api_key is required when creating a monitor",
	)
	ErrChannelMonitorMissingPrimaryModel = infraerrors.BadRequest(
		"CHANNEL_MONITOR_MISSING_PRIMARY_MODEL", "primary_model is required",
	)
	ErrChannelMonitorAPIKeyDecryptFailed = infraerrors.InternalServer(
		"CHANNEL_MONITOR_KEY_DECRYPT_FAILED", "api key decryption failed; please re-edit the monitor with a fresh key",
	)
)

var (
	ErrChannelMonitorDisabled = infraerrors.Forbidden(
		"CHANNEL_MONITOR_DISABLED",
		"channel monitor feature is disabled",
	)
	ErrChannelMonitorActiveProbesRetired = infraerrors.Forbidden(
		"CHANNEL_MONITOR_ACTIVE_PROBES_RETIRED",
		"channel monitor active probes are retired in v2 mode",
	)
	ErrChannelMonitorModeMismatch = infraerrors.Forbidden(
		"CHANNEL_MONITOR_MODE_MISMATCH",
		"channel monitor mode does not allow this operation",
	)
)
