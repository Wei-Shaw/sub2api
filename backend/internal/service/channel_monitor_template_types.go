package service

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"time"
)

// ChannelMonitorRequestTemplate 请求模板（service 层模型）。
// 作用：把一组可复用的 headers + 可选 body 覆盖配置抽出来管理，
// 被监控「应用」时以快照方式拷贝到监控本身的同名字段。
type ChannelMonitorRequestTemplate struct {
	ID               int64
	Name             string
	Provider         string
	Description      string
	ExtraHeaders     map[string]string
	BodyOverrideMode string
	BodyOverride     map[string]any
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ChannelMonitorRequestTemplateListParams 列表过滤。
type ChannelMonitorRequestTemplateListParams struct {
	Provider string // 空 = 全部；非空则按 provider 过滤
}

// ChannelMonitorRequestTemplateCreateParams 创建参数。
type ChannelMonitorRequestTemplateCreateParams struct {
	Name             string
	Provider         string
	Description      string
	ExtraHeaders     map[string]string
	BodyOverrideMode string
	BodyOverride     map[string]any
}

// ChannelMonitorRequestTemplateUpdateParams 更新参数（指针字段 = 不修改）。
// 注意 Provider 不可修改：改 provider 会让已关联监控的 body 黑名单语义错乱。
type ChannelMonitorRequestTemplateUpdateParams struct {
	Name             *string
	Description      *string
	ExtraHeaders     *map[string]string
	BodyOverrideMode *string
	BodyOverride     *map[string]any
}

// 模板相关错误（命名与现有 ErrChannelMonitor* 风格保持一致）。
var (
	ErrChannelMonitorTemplateNotFound = infraerrors.NotFound(
		"CHANNEL_MONITOR_TEMPLATE_NOT_FOUND", "channel monitor request template not found",
	)
	ErrChannelMonitorTemplateInvalidProvider = infraerrors.BadRequest(
		"CHANNEL_MONITOR_TEMPLATE_INVALID_PROVIDER", "template provider must be one of openai/anthropic/gemini",
	)
	ErrChannelMonitorTemplateMissingName = infraerrors.BadRequest(
		"CHANNEL_MONITOR_TEMPLATE_MISSING_NAME", "template name is required",
	)
	ErrChannelMonitorTemplateInvalidBodyMode = infraerrors.BadRequest(
		"CHANNEL_MONITOR_TEMPLATE_INVALID_BODY_MODE", "body_override_mode must be one of off/merge/replace",
	)
	ErrChannelMonitorTemplateBodyRequired = infraerrors.BadRequest(
		"CHANNEL_MONITOR_TEMPLATE_BODY_REQUIRED", "body_override is required when body_override_mode is merge or replace",
	)
	ErrChannelMonitorTemplateHeaderForbidden = infraerrors.BadRequest(
		"CHANNEL_MONITOR_TEMPLATE_HEADER_FORBIDDEN", "header name is forbidden (hop-by-hop or computed by HTTP client)",
	)
	ErrChannelMonitorTemplateHeaderInvalidName = infraerrors.BadRequest(
		"CHANNEL_MONITOR_TEMPLATE_HEADER_INVALID_NAME", "header name contains invalid characters",
	)
	ErrChannelMonitorTemplateProviderMismatch = infraerrors.BadRequest(
		"CHANNEL_MONITOR_TEMPLATE_PROVIDER_MISMATCH", "monitor provider does not match template provider",
	)
)
