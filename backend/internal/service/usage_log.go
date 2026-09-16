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
	// KiroCredits records Kiro credit consumption for Kiro account usage.
	KiroCredits *float64

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
