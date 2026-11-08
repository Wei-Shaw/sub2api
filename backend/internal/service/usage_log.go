package service

import (
	"fmt"
	"strings"
	"time"
)

const (
	BillingTypeBalance      int8 = 0 // 钱包余额
	BillingTypeSubscription int8 = 1 // 订阅套餐
)

type RequestType int16

const (
	RequestTypeUnknown      RequestType = 0
	RequestTypeSync         RequestType = 1
	RequestTypeStream       RequestType = 2
	RequestTypeWSV2         RequestType = 3
	RequestTypeCyberBlocked RequestType = 4 // cyber_policy 命中（透传但被上游安全策略拒绝）
	RequestTypeLive         RequestType = 5
)

func (t RequestType) IsValid() bool {
	switch t {
	case RequestTypeUnknown, RequestTypeSync, RequestTypeStream, RequestTypeWSV2, RequestTypeCyberBlocked, RequestTypeLive:
		return true
	default:
		return false
	}
}

func (t RequestType) Normalize() RequestType {
	if t.IsValid() {
		return t
	}
	return RequestTypeUnknown
}

func (t RequestType) String() string {
	switch t.Normalize() {
	case RequestTypeSync:
		return "sync"
	case RequestTypeStream:
		return "stream"
	case RequestTypeWSV2:
		return "ws_v2"
	case RequestTypeCyberBlocked:
		return "cyber"
	case RequestTypeLive:
		return "live"
	default:
		return "unknown"
	}
}

func RequestTypeFromInt16(v int16) RequestType {
	return RequestType(v).Normalize()
}

func ParseUsageRequestType(value string) (RequestType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "unknown":
		return RequestTypeUnknown, nil
	case "sync":
		return RequestTypeSync, nil
	case "stream":
		return RequestTypeStream, nil
	case "ws_v2":
		return RequestTypeWSV2, nil
	case "cyber":
		return RequestTypeCyberBlocked, nil
	case "live":
		return RequestTypeLive, nil
	default:
		return RequestTypeUnknown, fmt.Errorf("invalid request_type, allowed values: unknown, sync, stream, ws_v2, cyber, live")
	}
}

func RequestTypeFromLegacy(stream bool, openAIWSMode bool) RequestType {
	if openAIWSMode {
		return RequestTypeWSV2
	}
	if stream {
		return RequestTypeStream
	}
	return RequestTypeSync
}

func ApplyLegacyRequestFields(requestType RequestType, fallbackStream bool, fallbackOpenAIWSMode bool) (stream bool, openAIWSMode bool) {
	switch requestType.Normalize() {
	case RequestTypeSync:
		return false, false
	case RequestTypeStream:
		return true, false
	case RequestTypeWSV2:
		return true, true
	default:
		return fallbackStream, fallbackOpenAIWSMode
	}
}

type UsageLog struct {
	ID        int64
	UserID    int64
	APIKeyID  int64
	AccountID int64
	RequestID string
	Model     string
	// RequestedModel is the client-requested model name recorded for stable user/admin display.
	// Empty should be treated as Model for backward compatibility with historical rows.
	RequestedModel string
	// UpstreamModel is the actual model sent to the upstream provider after mapping.
	// Nil means no mapping was applied (requested model was used as-is).
	UpstreamModel *string
	// UpstreamResponseModel is the model declared by the successful upstream
	// response before client-facing model rewrites or protocol conversion.
	UpstreamResponseModel *string
	// UpstreamModelMismatch is nil when no upstream model was observed. Otherwise
	// it compares UpstreamResponseModel with the actual model sent upstream.
	UpstreamModelMismatch *bool
	// ChannelID 渠道 ID
	ChannelID *int64
	// ModelMappingChain 模型映射链，如 "a→b→c"
	ModelMappingChain *string
	// BillingTier 计费层级标签（per_request/image 模式）
	BillingTier *string
	// BillingMode 计费模式：token/image
	BillingMode *string
	// ServiceTier records the billable request tier, e.g. OpenAI "priority" / "flex"
	// or Anthropic "fast".
	ServiceTier *string
	// ReasoningEffort is the effective effort recorded for this request after
	// group policy rewriting and model-family remapping (e.g. max -> xhigh).
	// OpenAI: "low" / "medium" / "high" / "xhigh"; Claude: "low" / "medium" / "high" / "max".
	// Nil means not provided / not applicable.
	ReasoningEffort *string
	// RequestedReasoningEffort is the client-requested effort before mapping.
	// Nil means historical rows, or that no explicit/suffix-derived effort was observed.
	RequestedReasoningEffort *string
	// InboundEndpoint is the client-facing API endpoint path, e.g. /v1/chat/completions.
	InboundEndpoint *string
	// UpstreamEndpoint is the normalized upstream endpoint path, e.g. /v1/responses.
	UpstreamEndpoint *string

	GroupID        *int64
	SubscriptionID *int64

	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int

	CacheCreation5mTokens int `gorm:"column:cache_creation_5m_tokens"`
	CacheCreation1hTokens int `gorm:"column:cache_creation_1h_tokens"`

	ImageInputTokens  int
	ImageInputCost    float64
	ImageOutputTokens int
	ImageOutputCost   float64

	InputCost                 float64
	OutputCost                float64
	CacheCreationCost         float64
	CacheReadCost             float64
	TotalCost                 float64
	ActualCost                float64
	RateMultiplier            float64
	LongContextBillingApplied bool
	// AccountRateMultiplier 账号计费倍率快照（nil 表示历史数据，按 1.0 处理）
	AccountRateMultiplier *float64
	// AccountStatsCost 账号统计定价预计算费用（nil = 使用默认公式 total_cost × account_rate_multiplier）
	AccountStatsCost *float64

	BillingType        int8
	RequestType        RequestType
	Stream             bool
	OpenAIWSMode       bool
	NativeCompactionV2 bool
	DurationMs         *int
	FirstTokenMs       *int
	UserAgent          *string
	IPAddress          *string
	// SessionID is the explicit client-provided request correlation identifier
	// (e.g. the session_id / X-Session-Id headers). Nil when the client sent no
	// valid session header. It is never derived from prompt_cache_key or content.
	SessionID *string
	// UpstreamRequestID 是直接上游在响应头中声明的请求标识，只读账户
	// extra.upstream_request_id_header 指定的头；账户未指定头名、WS 轮次
	// 与上游没有该头的路径为 nil。
	UpstreamRequestID *string
	// UpstreamCostOriginal 是这笔调用在上游（A6）账单里的**真实扣费**，原币金额。
	//
	// 与同一行的 TotalCost / ActualCost 是两个口径：后者是按本站价目表算出来的
	// 「应收」，决定用户被扣多少余额；这个是上游向我们收的「实付」，是成本。
	// 两者之差就是这笔调用的毛利。
	//
	// **不做任何汇率换算**：本站记账本就是美元口径，上游 A6 账单也是美元，两边
	// 同币种直接相减才是可比的。曾经存过一列换算后的人民币，反而让页面变成
	// 「一列美元一列人民币」，要心算汇率才能比较，因此已删除。
	//
	// 由后台取数任务拿 UpstreamRequestID 去上游反查后回填，因此：
	//   - nil  = 还没取到（上游账单尚未落库 / 重试次数已用尽），**不代表成本为 0**；
	//   - 有值 = 已取到，金额即上游账单原文，此后不再变动。
	UpstreamCostOriginal *float64
	// UpstreamCostCurrency 是 UpstreamCostOriginal 的币种（A6 为 USD）。
	// 仅用于显示货币符号，不参与任何计算；未取到成本时为空串。
	UpstreamCostCurrency string

	// UpstreamFirstTokenMs / UpstreamDurationMs 是**上游自报**的首字耗时与总耗时（毫秒）。
	//
	// 与同一行的 FirstTokenMs / DurationMs 是两个口径，不能混用：
	//   FirstTokenMs / DurationMs            = 本站观测，从网关接手请求算起，
	//                                          含中转开销 + 网络往返 + 上游全部处理时间；
	//   UpstreamFirstTokenMs / DurationMs    = 上游账单口径，只覆盖上游内部那一段。
	// 两者相减就是「中转开销」，这是判断「是不是我们的中转把请求拖慢了」的唯一依据。
	//
	// 与 UpstreamCostOriginal 共享同一套回填生命周期（由后台取数任务用
	// UpstreamRequestID 反查上游账单后写入），因此：
	//   - nil  = 尚未对账到（账单未落库 / 重试耗尽 / 该账号不支持），**不代表耗时 0**；
	//   - 有值 = 上游账单原文，此后不再变动。
	//
	// ⚠️ 精度：UpstreamFirstTokenMs 为毫秒精度；UpstreamDurationMs 由上游**整秒**值
	// 换算而来，有效精度只有秒级，不可用于毫秒级比较。
	UpstreamFirstTokenMs *int
	// UpstreamDurationMs 见 UpstreamFirstTokenMs。
	UpstreamDurationMs *int
	// UpstreamSupplierID 上游（A6）商户 ID，取自账单 marketplace_supplier_id。
	//
	// 为什么单独存商户：2026-10-10 的漏计费事件里，按**模型**聚合会得出
	// 「gpt-5.5 / gpt-6-astra 有问题」的错误结论（它们只是恰好被路由到了坏商户），
	// 按**商户**聚合才立刻干净 —— 18 个商户里只有 4 个丢，其余 14 个一笔不丢。
	// 商户是能被拉黑/切换的最小单元，归因必须落到这一级。
	//
	// 与 UpstreamCostOriginal 同生命周期：nil = 尚未对账到，非 0。
	UpstreamSupplierID *int
	// UpstreamSupplierName 上游商户名，冗余存一份便于页面直接显示。
	UpstreamSupplierName *string
	// UpstreamChannelID 上游实际计费渠道 ID（账单 channel）。
	UpstreamChannelID *int
	// UpstreamTargetChannelID 上游目标渠道 ID（账单 marketplace_target_channel_id）。
	UpstreamTargetChannelID *int

	// Cache TTL Override 标记（管理员强制替换了缓存 TTL 计费）
	CacheTTLOverridden bool

	// 图片生成字段
	ImageCount         int
	ImageSize          *string
	ImageInputSize     *string
	ImageOutputSize    *string
	ImageSizeSource    *string
	ImageSizeBreakdown map[string]int
	MediaType          *string

	// 视频生成字段（Grok 视频按秒计费；video_count>0 的行不要求 image_size）
	VideoCount           int
	VideoResolution      *string
	VideoDurationSeconds *int

	CreatedAt time.Time

	User         *User
	APIKey       *APIKey
	Account      *Account
	Group        *Group
	Subscription *UserSubscription
}

func (u *UsageLog) TotalTokens() int {
	return u.InputTokens + u.OutputTokens + u.CacheCreationTokens + u.CacheReadTokens
}

func (u *UsageLog) EffectiveRequestType() RequestType {
	if u == nil {
		return RequestTypeUnknown
	}
	if normalized := u.RequestType.Normalize(); normalized != RequestTypeUnknown {
		return normalized
	}
	return RequestTypeFromLegacy(u.Stream, u.OpenAIWSMode)
}

func (u *UsageLog) SyncRequestTypeAndLegacyFields() {
	if u == nil {
		return
	}
	requestType := u.EffectiveRequestType()
	u.RequestType = requestType
	u.Stream, u.OpenAIWSMode = ApplyLegacyRequestFields(requestType, u.Stream, u.OpenAIWSMode)
}
