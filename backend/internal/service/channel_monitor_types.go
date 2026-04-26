package service

import "time"

// MonitorBodyOverrideMode 自定义请求体处理模式。
//
//   - off     使用 adapter 默认 body（忽略 BodyOverride）
//   - merge   adapter 默认 body 与 BodyOverride 浅合并（用户优先；
//     model/messages/contents 等关键字段在 checker 黑名单内会被静默丢弃）
//   - replace 完全用 BodyOverride 作为 body；跳过 challenge 校验，
//     改成 HTTP 2xx + 响应非空即视为可用（用户负责构造 body）
const (
	MonitorBodyOverrideModeOff     = "off"
	MonitorBodyOverrideModeMerge   = "merge"
	MonitorBodyOverrideModeReplace = "replace"
)

// ChannelMonitor 渠道监控配置（service 层模型，不直接暴露 ent 类型）。
type ChannelMonitor struct {
	ID              int64
	Name            string
	Provider        string
	Endpoint        string
	APIKey          string // 解密后的明文 API Key（仅在 service 内部使用，handler 层不应直接序列化返回）
	PrimaryModel    string
	ExtraModels     []string
	GroupName       string
	Enabled         bool
	IntervalSeconds int
	LastCheckedAt   *time.Time
	CreatedBy       int64
	CreatedAt       time.Time
	UpdatedAt       time.Time

	// 请求自定义快照（来自模板拷贝 or 用户手填，运行时直接读取）
	TemplateID       *int64            // 仅用于 UI 分组 + 一键应用，运行时不用
	ExtraHeaders     map[string]string // 与 adapter 默认 headers 合并，用户优先
	BodyOverrideMode string            // off / merge / replace
	BodyOverride     map[string]any    // 仅 mode != off 时使用

	// APIKeyDecryptFailed 表示 APIKey 字段无法解密（密钥不一致或损坏）。
	// 此时 APIKey 为空字符串，runner / RunCheck 必须跳过该监控并提示重填。
	APIKeyDecryptFailed bool
}

// ChannelMonitorListParams 列表查询过滤参数。
type ChannelMonitorListParams struct {
	Page     int
	PageSize int
	Provider string
	Enabled  *bool
	Search   string
}

// ChannelMonitorCreateParams 创建参数。
type ChannelMonitorCreateParams struct {
	Name             string
	Provider         string
	Endpoint         string
	APIKey           string
	PrimaryModel     string
	ExtraModels      []string
	GroupName        string
	Enabled          bool
	IntervalSeconds  int
	CreatedBy        int64
	TemplateID       *int64
	ExtraHeaders     map[string]string
	BodyOverrideMode string
	BodyOverride     map[string]any
}

// ChannelMonitorUpdateParams 更新参数（指针字段表示"未提供则不更新"）。
type ChannelMonitorUpdateParams struct {
	Name            *string
	Provider        *string
	Endpoint        *string
	APIKey          *string // 空字符串表示不修改；非空字符串覆盖
	PrimaryModel    *string
	ExtraModels     *[]string
	GroupName       *string
	Enabled         *bool
	IntervalSeconds *int
	// 自定义快照字段：指针为 nil 表示不更新，非 nil 覆盖
	// TemplateID *(*int64)：用 ** 表达三态：nil=不更新；&nil=清空；&&id=设为 id。
	// 简化处理：用 ClearTemplate 显式标志 + TemplateID（普通指针）
	TemplateID       *int64
	ClearTemplate    bool // true 时无视 TemplateID，把监控的 template_id 置空
	ExtraHeaders     *map[string]string
	BodyOverrideMode *string
	BodyOverride     *map[string]any
}

// CheckResult 单个模型一次检测的结果。
type CheckResult struct {
	Model         string
	Status        string // operational / degraded / failed / error
	LatencyMs     *int
	PingLatencyMs *int
	Message       string
	CheckedAt     time.Time
}

// UserMonitorView 用户只读视图：监控概览（含主模型最近状态 + 7d 可用率 + 附加模型最近状态）。
type UserMonitorView struct {
	ID                   int64
	Name                 string
	Provider             string
	GroupName            string
	PrimaryModel         string
	PrimaryStatus        string
	PrimaryLatencyMs     *int
	PrimaryPingLatencyMs *int    // 主模型最近一次 ping 延迟
	Availability7d       float64 // 0-100
	ExtraModels          []ExtraModelStatus
	Timeline             []UserMonitorTimelinePoint // 主模型最近 N 个历史点（按 checked_at DESC，最新在前）
}

// UserMonitorTimelinePoint 用户视图 timeline 单点数据（去除 message 以减小响应体）。
type UserMonitorTimelinePoint struct {
	Status        string    `json:"status"`
	LatencyMs     *int      `json:"latency_ms"`
	PingLatencyMs *int      `json:"ping_latency_ms"`
	CheckedAt     time.Time `json:"checked_at"`
}

// ExtraModelStatus 附加模型最近一次状态。
type ExtraModelStatus struct {
	Model     string
	Status    string
	LatencyMs *int
}

// UserMonitorDetail 用户只读视图：监控详情（含全部模型 7d/15d/30d 可用率与平均延迟）。
type UserMonitorDetail struct {
	ID        int64
	Name      string
	Provider  string
	GroupName string
	Models    []ModelDetail
}

// ModelDetail 单个模型的可用率/延迟统计。
type ModelDetail struct {
	Model           string
	LatestStatus    string
	LatestLatencyMs *int
	Availability7d  float64 // 0-100
	Availability15d float64
	Availability30d float64
	AvgLatency7dMs  *int
}

// ChannelMonitorHistoryRow 历史记录入库行（service 层向 repository 提交的数据）。
type ChannelMonitorHistoryRow struct {
	MonitorID     int64
	Model         string
	Status        string
	LatencyMs     *int
	PingLatencyMs *int
	Message       string
	CheckedAt     time.Time
}

// ChannelMonitorHistoryEntry 历史记录查询返回行（含 ent 主键 ID）。
type ChannelMonitorHistoryEntry struct {
	ID            int64
	Model         string
	Status        string
	LatencyMs     *int
	PingLatencyMs *int
	Message       string
	CheckedAt     time.Time
}

// ChannelMonitorLatest 最近一次检测的简明信息（用于 UserMonitorView 聚合）。
type ChannelMonitorLatest struct {
	Model         string
	Status        string
	LatencyMs     *int
	PingLatencyMs *int
	CheckedAt     time.Time
}

// ChannelMonitorAvailability 单个模型在某窗口内的可用率与平均延迟（用于 UserMonitorDetail 聚合）。
type ChannelMonitorAvailability struct {
	Model             string
	WindowDays        int
	TotalChecks       int
	OperationalChecks int // operational + degraded 视为可用
	AvailabilityPct   float64
	AvgLatencyMs      *int
}

// MonitorStatusSummary 监控状态聚合（admin list 用，单次 repo 查询消除前端 N+1）。
// PrimaryStatus / PrimaryLatencyMs 描述主模型最近状态；Availability7d 是主模型 7 天可用率；
// ExtraModels 描述附加模型最近状态（用于 hover 展示）。
type MonitorStatusSummary struct {
	PrimaryStatus    string // 空字符串表示无历史
	PrimaryLatencyMs *int
	Availability7d   float64 // 0-100，无历史时为 0
	ExtraModels      []ExtraModelStatus
}
