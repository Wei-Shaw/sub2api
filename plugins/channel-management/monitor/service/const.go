package monitorservice

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/errors"
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
	// monitorWorkerConcurrency 是 host JobScheduler 对 monitor.run 任务的并发上限。
	// 单 host 内最多 5 个 tick handler 同时执行，避免并发体检时一次性把上游打垮。
	// 实际并发还会被 per-monitor inFlight 票据进一步收敛（同一 monitor 永远串行）。
	monitorWorkerConcurrency = 5
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

	// providerOpenAIPath OpenAI Chat Completions 路径。
	providerOpenAIPath = "/v1/chat/completions"
	// providerOpenAIResponsesPath OpenAI Responses API 路径。
	providerOpenAIResponsesPath = "/v1/responses"
	// providerAnthropicPath Anthropic Messages 路径。
	providerAnthropicPath = "/v1/messages"
	// providerGeminiPathTemplate Gemini generateContent 路径模板（含 model 占位）。
	providerGeminiPathTemplate = "/v1beta/models/%s:generateContent"

	// MonitorProviderOpenAI / Anthropic / Gemini provider 字符串常量（也是 ent enum 的实际值）。
	MonitorProviderOpenAI    = "openai"
	MonitorProviderAnthropic = "anthropic"
	MonitorProviderGemini    = "gemini"

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

	// ---- checker / runner 行为参数（消除 magic 值）----

	// monitorAnthropicAPIVersion Anthropic Messages API 版本头。
	monitorAnthropicAPIVersion = "2023-06-01"
	// monitorChallengeMaxTokens 单次 challenge 请求的 max_tokens（足够回答个位数算术）。
	monitorChallengeMaxTokens = 50

	// monitorPingDiscardMaxBytes ping 时丢弃响应体的最大字节数。
	monitorPingDiscardMaxBytes = 1024
)

// 业务错误（统一在此声明，避免散落）。
var (
	ErrChannelMonitorNotFound = infraerrors.NotFound(
		"CHANNEL_MONITOR_NOT_FOUND", "channel monitor not found",
	)
	ErrChannelMonitorInvalidProvider = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_PROVIDER", "provider must be one of openai/anthropic/gemini",
	)
	ErrChannelMonitorInvalidAPIMode = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_API_MODE", "api_mode must be chat_completions or responses; responses is only supported for openai",
	)
	ErrChannelMonitorInvalidRequestBody = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_REQUEST_BODY", "openai replace-mode body_override must include non-empty messages for chat_completions or non-empty instructions and input for responses",
	)
	ErrChannelMonitorInvalidInterval = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_INTERVAL", "interval_seconds must be in [15, 3600]",
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
	// 注：曾经的 ErrChannelMonitorEndpointPrivate / ErrChannelMonitorEndpointUnreachable
	// 与 isPrivateOrLoopbackHost 一同被移除（T13）。validateEndpoint 现在不再做
	// host 预检——SSRF 防护交由 SDK SafeOutboundHTTP 层在每次 dial 时执行（避免
	// TOCTOU 与 DNS rebinding 误判）。前端 i18n 未引用这两个错误码，可直接删除。
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
