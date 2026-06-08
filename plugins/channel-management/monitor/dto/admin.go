// Package dto 集中渠道监控对外 HTTP 响应 / 请求结构。
//
// 拆分动机 (T20):
//   - admin_handler.go 内嵌 DTO 类型 + handler 同居 457 行；
//   - 同包仅有 ChannelMonitorExtraModelStatus 一个共享 struct；
//   - admin / user / 未来管理端报表都需要这套 view 结构。
//
// 把 DTO + mapper 独立到本包后，handler 只负责协议适配 (绑定参数、序列化、
// 状态码)，domain 转 view 的字段映射 / 时区格式化都在 dto 包内。
package dto

// ChannelMonitorCreateRequest 创建监控的请求体。
type ChannelMonitorCreateRequest struct {
	Name             string            `json:"name" binding:"required,max=100"`
	Provider         string            `json:"provider" binding:"required,oneof=openai anthropic gemini"`
	Endpoint         string            `json:"endpoint" binding:"required,max=500"`
	APIKey           string            `json:"api_key" binding:"required,max=2000"`
	PrimaryModel     string            `json:"primary_model" binding:"required,max=200"`
	ExtraModels      []string          `json:"extra_models"`
	GroupName        string            `json:"group_name" binding:"max=100"`
	Enabled          *bool             `json:"enabled"`
	IntervalSeconds  int               `json:"interval_seconds" binding:"required,min=15,max=3600"`
	TemplateID       *int64            `json:"template_id"`
	ExtraHeaders     map[string]string `json:"extra_headers"`
	BodyOverrideMode string            `json:"body_override_mode" binding:"omitempty,oneof=off merge replace"`
	BodyOverride     map[string]any    `json:"body_override"`
}

// ChannelMonitorUpdateRequest 更新监控的请求体。可选字段用指针区分零值 vs 未提供。
type ChannelMonitorUpdateRequest struct {
	Name             *string            `json:"name" binding:"omitempty,max=100"`
	Provider         *string            `json:"provider" binding:"omitempty,oneof=openai anthropic gemini"`
	Endpoint         *string            `json:"endpoint" binding:"omitempty,max=500"`
	APIKey           *string            `json:"api_key" binding:"omitempty,max=2000"`
	PrimaryModel     *string            `json:"primary_model" binding:"omitempty,max=200"`
	ExtraModels      *[]string          `json:"extra_models"`
	GroupName        *string            `json:"group_name" binding:"omitempty,max=100"`
	Enabled          *bool              `json:"enabled"`
	IntervalSeconds  *int               `json:"interval_seconds" binding:"omitempty,min=15,max=3600"`
	TemplateID       *int64             `json:"template_id"`
	ClearTemplate    bool               `json:"clear_template"`
	ExtraHeaders     *map[string]string `json:"extra_headers"`
	BodyOverrideMode *string            `json:"body_override_mode" binding:"omitempty,oneof=off merge replace"`
	BodyOverride     *map[string]any    `json:"body_override"`
}

// ChannelMonitorResponse 渠道监控配置 + 衍生汇总（List / Get / Create / Update 通用）。
type ChannelMonitorResponse struct {
	ID                  int64                            `json:"id"`
	Name                string                           `json:"name"`
	Provider            string                           `json:"provider"`
	Endpoint            string                           `json:"endpoint"`
	APIKeyMasked        string                           `json:"api_key_masked"`
	APIKeyDecryptFailed bool                             `json:"api_key_decrypt_failed"`
	PrimaryModel        string                           `json:"primary_model"`
	ExtraModels         []string                         `json:"extra_models"`
	GroupName           string                           `json:"group_name"`
	Enabled             bool                             `json:"enabled"`
	IntervalSeconds     int                              `json:"interval_seconds"`
	LastCheckedAt       *string                          `json:"last_checked_at"`
	CreatedBy           int64                            `json:"created_by"`
	CreatedAt           string                           `json:"created_at"`
	UpdatedAt           string                           `json:"updated_at"`
	PrimaryStatus       string                           `json:"primary_status"`
	PrimaryLatencyMs    *int                             `json:"primary_latency_ms"`
	Availability7d      float64                          `json:"availability_7d"`
	ExtraModelsStatus   []ChannelMonitorExtraModelStatus `json:"extra_models_status"`
	TemplateID          *int64                           `json:"template_id"`
	ExtraHeaders        map[string]string                `json:"extra_headers"`
	BodyOverrideMode    string                           `json:"body_override_mode"`
	BodyOverride        map[string]any                   `json:"body_override"`
}

// ChannelMonitorCheckResultResponse 单次 RunNow / 历史返回的轻量记录。
type ChannelMonitorCheckResultResponse struct {
	Model         string `json:"model"`
	Status        string `json:"status"`
	LatencyMs     *int   `json:"latency_ms"`
	PingLatencyMs *int   `json:"ping_latency_ms"`
	Message       string `json:"message"`
	CheckedAt     string `json:"checked_at"`
}

// ChannelMonitorHistoryItemResponse history 接口的单行响应。
type ChannelMonitorHistoryItemResponse struct {
	ID            int64  `json:"id"`
	Model         string `json:"model"`
	Status        string `json:"status"`
	LatencyMs     *int   `json:"latency_ms"`
	PingLatencyMs *int   `json:"ping_latency_ms"`
	Message       string `json:"message"`
	CheckedAt     string `json:"checked_at"`
}
