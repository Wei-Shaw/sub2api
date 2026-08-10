package service

import (
	"context"
	"time"
)

// 异步视频任务状态常量（与 AsyncMediaStatus* 语义一致，独立命名避免耦合）。
const (
	AsyncVideoStatusPending   = "pending"
	AsyncVideoStatusRunning   = "running"
	AsyncVideoStatusSucceeded = "succeeded"
	AsyncVideoStatusFailed    = "failed"
	AsyncVideoStatusRefunded  = "refunded"
	AsyncVideoStatusExpired   = "expired"
)

// 异步视频任务对外门面常量。
const (
	AsyncVideoFacadeFal = "fal" // /api/v1/model/*path fal 原生异步门面
)

// AsyncVideoTask 异步视频任务领域模型。
//
// 与 AsyncMediaTask（图片）并行独立：
//   - 计费维度：resolution × duration_seconds × price_per_second
//   - result_payload 保留 fal 上游原始 result JSON，透传给客户端
type AsyncVideoTask struct {
	ID                int64
	InternalRequestID string
	UpstreamRequestID *string
	StatusURL         *string
	ResponseURL       *string

	AccountID       *int64
	APIKeyID        int64
	UserID          int64
	OrganizationID  *int64
	PayerUserID     *int64
	BalanceSource   *string
	AuthzGeneration *int64
	GroupID         *int64
	ChannelID       *int64

	Facade         string
	RequestedModel string
	UpstreamModel  *string

	Resolution      *string // 480p / 720p / 1080p / 4k
	DurationSeconds int     // 视频时长（秒）
	AspectRatio     *string // 仅记录，不参与计费

	Status            string
	HeldCost          float64
	FinalCost         float64
	RateMultiplier    float64
	UnitPriceSnapshot float64 // 提交时的 price_per_second 快照
	UpstreamCost      float64 // 上游真实成本（如 apiz price/100，美元）；平台不回传时为 0，由 rate_multiplier 估算兜底

	RequestPayload map[string]any // 客户端原始请求
	ResultPayload  map[string]any // 上游 result 原始响应
	VideoURLs      []string       // 从 result 提取的 video url
	CosURLs        []string       // 预留转存（本轮不启用）

	ErrorReason    *string
	FailDeadlineAt *time.Time
	FinishedAt     *time.Time

	ClientIP         *string
	UserAgent        *string
	InboundEndpoint  *string
	UpstreamEndpoint *string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsTerminal 判断视频任务是否已进入终态。
func (t *AsyncVideoTask) IsTerminal() bool {
	if t == nil {
		return false
	}
	switch t.Status {
	case AsyncVideoStatusSucceeded, AsyncVideoStatusRefunded, AsyncVideoStatusExpired:
		return true
	default:
		return false
	}
}

// ResultVideoURLs 返回对外展示的视频 URL 列表，优先 COS 转存地址。
func (t *AsyncVideoTask) ResultVideoURLs() []string {
	if t == nil {
		return nil
	}
	if len(t.CosURLs) > 0 {
		return t.CosURLs
	}
	return t.VideoURLs
}

// VideoTerminalUsageLogInput 终态 usage_log 追加写入参数（视频）。
type VideoTerminalUsageLogInput struct {
	UserID          int64
	APIKeyID        int64
	AccountID       int64
	RequestID       string
	OrganizationID  *int64
	PayerUserID     *int64
	BalanceSource   *string
	AuthzGeneration *int64

	Model          string
	RequestedModel string
	UpstreamModel  string

	GroupID   *int64
	ChannelID *int64

	TotalCost      float64
	ActualCost     float64
	RateMultiplier float64

	BillingType int8
	RequestType int16

	Resolution      string
	DurationSeconds int
	AspectRatio     string
	UnitPrice       float64

	TaskID        int64
	VideoURLs     []string
	CosURLs       []string
	BillingStatus string // charged / refunded

	ClientIP         string
	UserAgent        string
	InboundEndpoint  string
	UpstreamEndpoint string
	DurationMs       int64
}

// AsyncVideoTaskRepository 视频任务仓储接口。
type AsyncVideoTaskRepository interface {
	Create(ctx context.Context, task *AsyncVideoTask) error
	GetByID(ctx context.Context, id int64) (*AsyncVideoTask, error)
	GetByInternalRequestID(ctx context.Context, internalRequestID string) (*AsyncVideoTask, error)
	GetByUpstreamRequestID(ctx context.Context, upstreamRequestID string) (*AsyncVideoTask, error)
	UpdateUpstreamRef(ctx context.Context, id int64, upstreamRequestID, statusURL, responseURL string) error
	MarkSucceeded(ctx context.Context, id int64, videoURLs, cosURLs []string, resultPayload map[string]any, finalCost float64, durationSeconds int, upstreamCost float64) (bool, error)
	MarkRefunded(ctx context.Context, id int64, status, errorReason string) (bool, error)
	ListUnfinished(ctx context.Context, limit int) ([]*AsyncVideoTask, error)
	// ListByUserAndSlug 分页列出某用户在指定模型 slug 下的历史任务。
	// slug 为空时表示全部模型（Q3-1: B 方案会按 slug 过滤，A 方案传空串）。
	// 按 created_at DESC 排序，返回 tasks 切片以及总数（用于分页）。
	ListByUserAndSlug(ctx context.Context, userID int64, slug string, offset, limit int) ([]*AsyncVideoTask, int64, error)
	InsertTerminalUsageLog(ctx context.Context, in *VideoTerminalUsageLogInput) (bool, error)
}
