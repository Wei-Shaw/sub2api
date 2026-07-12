package service

import (
	"context"
	"time"
)

// 异步媒体任务状态常量。
const (
	AsyncMediaStatusPending   = "pending"   // 已落库，尚未提交/提交中
	AsyncMediaStatusRunning   = "running"   // 已提交上游，等待出图
	AsyncMediaStatusSucceeded = "succeeded" // 出图成功并结算
	AsyncMediaStatusFailed    = "failed"    // 上游明确失败（待退费）
	AsyncMediaStatusRefunded  = "refunded"  // 已退费（失败终态）
	AsyncMediaStatusExpired   = "expired"   // 超过失败兜底时间，已退费终态
)

// 异步媒体任务对外门面常量。
const (
	AsyncMediaFacadeOpenAI = "openai" // OpenAI 伪同步门面
	AsyncMediaFacadeFal    = "fal"    // fal 原生异步门面
)

// 计费状态常量（写入 usage_logs.billing_status）。
const (
	BillingStatusCharged  = "charged"  // 已扣费
	BillingStatusRefunded = "refunded" // 已退费
)

// AsyncMediaTask 异步媒体任务领域模型，承载任务的完整生命周期。
type AsyncMediaTask struct {
	ID                int64
	InternalRequestID string
	UpstreamRequestID *string
	StatusURL         *string
	ResponseURL       *string

	AccountID *int64
	APIKeyID  int64
	UserID    int64
	GroupID   *int64
	ChannelID *int64

	Facade         string
	RequestedModel string
	UpstreamModel  *string

	ImageSize *string
	Quality   *string
	NumImages int

	Status         string
	HeldCost       float64
	FinalCost      float64
	RateMultiplier float64
	SizeTier       *string

	ImageURLs []string
	CosURLs   []string

	ErrorReason    *string
	FailDeadlineAt *time.Time
	FinishedAt     *time.Time

	// 请求元信息（提交时持久化，供终态 usage_log 回填端点/IP/UA）。
	ClientIP         *string
	UserAgent        *string
	InboundEndpoint  *string
	UpstreamEndpoint *string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsTerminal 判断任务是否处于终态（不再需要 reconciler 处理）。
func (t *AsyncMediaTask) IsTerminal() bool {
	switch t.Status {
	case AsyncMediaStatusSucceeded, AsyncMediaStatusRefunded, AsyncMediaStatusExpired:
		return true
	default:
		return false
	}
}

// ResultURLs 返回对外可见的图片地址，优先使用 COS 转存地址，缺失时回退 fal 原始地址。
func (t *AsyncMediaTask) ResultURLs() []string {
	if t == nil {
		return nil
	}
	if len(t.CosURLs) > 0 {
		return t.CosURLs
	}
	return t.ImageURLs
}

// TerminalUsageLogInput 终态 usage_log 追加写入参数。
// 异步媒体任务在终态（成功结算 / 退费）时追加写一条 usage_log，
// 通过独立、隔离的 INSERT 实现，避免触碰高并发批处理写入路径。
type TerminalUsageLogInput struct {
	UserID    int64
	APIKeyID  int64
	AccountID int64 // usage_logs.account_id NOT NULL
	RequestID string

	Model          string // 落库 model 列（NOT NULL），通常为上游模型
	RequestedModel string
	UpstreamModel  string

	GroupID   *int64
	ChannelID *int64

	TotalCost      float64
	ActualCost     float64
	RateMultiplier float64

	BillingType int8  // 0=balance / 1=subscription
	RequestType int16 // RequestTypeSync 等

	ImageCount  int
	ImageSize   string
	BillingTier string // size_tier

	TaskID        int64
	ImageURLs     []string
	CosURLs       []string
	BillingStatus string // charged / refunded

	// 请求元信息（从任务表回填，供使用记录展示端点/耗时/IP/UA）。
	ClientIP         string
	UserAgent        string
	InboundEndpoint  string
	UpstreamEndpoint string
	DurationMs       int64
}

// AsyncMediaTaskRepository 异步媒体任务仓储接口。
type AsyncMediaTaskRepository interface {
	// Create 创建任务，回填 ID/时间戳。
	Create(ctx context.Context, task *AsyncMediaTask) error
	// GetByID 按主键查询；不存在返回 (nil, nil)。
	GetByID(ctx context.Context, id int64) (*AsyncMediaTask, error)
	// GetByInternalRequestID 按内部请求 ID 查询；不存在返回 (nil, nil)。
	GetByInternalRequestID(ctx context.Context, internalRequestID string) (*AsyncMediaTask, error)
	// GetByUpstreamRequestID 按上游 request_id 查询；不存在返回 (nil, nil)。
	GetByUpstreamRequestID(ctx context.Context, upstreamRequestID string) (*AsyncMediaTask, error)
	// UpdateUpstreamRef 回填上游 request_id / status_url / response_url，并将状态推进到 running。
	UpdateUpstreamRef(ctx context.Context, id int64, upstreamRequestID, statusURL, responseURL string) error
	// MarkSucceeded 成功终态：写入图片地址、转存地址、结算费用，并将状态置 succeeded。
	// 仅当当前状态非终态时才更新（幂等：返回是否实际更新）。
	MarkSucceeded(ctx context.Context, id int64, imageURLs, cosURLs []string, finalCost float64) (bool, error)
	// MarkRefunded 退费终态：将状态由 fromStatus 集合迁移到 refunded/expired，并清零 final_cost。
	// 仅当当前状态非终态时才更新（幂等：返回是否实际更新，供退费动作去重）。
	MarkRefunded(ctx context.Context, id int64, status, errorReason string) (bool, error)
	// ListUnfinished 扫描未终结（pending/running）的任务，供 reconciler 兜底处理。
	ListUnfinished(ctx context.Context, limit int) ([]*AsyncMediaTask, error)
	// InsertTerminalUsageLog 在任务终态追加写一条 usage_log（charged/refunded）。
	// 使用 ON CONFLICT (request_id, api_key_id) DO NOTHING 保证幂等，返回是否实际写入。
	InsertTerminalUsageLog(ctx context.Context, in *TerminalUsageLogInput) (bool, error)
}
