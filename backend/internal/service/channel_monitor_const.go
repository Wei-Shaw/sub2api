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

	// providerOpenAIPath OpenAI Chat Completions 路径。
	providerOpenAIPath = "/v1/chat/completions"
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
		"CHANNEL_MONITOR_INVALID_PROVIDER", "provider must be one of openai/anthropic/gemini",
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
