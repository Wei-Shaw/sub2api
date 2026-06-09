package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// Re-exported domain constants for service-layer consumers.
const (
	EmailBroadcastBodyFormatHTML        = domain.EmailBroadcastBodyFormatHTML
	EmailBroadcastBodyFormatText        = domain.EmailBroadcastBodyFormatText
	EmailBroadcastRecipientsModeAll     = domain.EmailBroadcastRecipientsModeAll
	EmailBroadcastRecipientsModeSelected = domain.EmailBroadcastRecipientsModeSelected
	EmailBroadcastStatusPending         = domain.EmailBroadcastStatusPending
	EmailBroadcastStatusSending         = domain.EmailBroadcastStatusSending
	EmailBroadcastStatusCompleted       = domain.EmailBroadcastStatusCompleted
	EmailBroadcastStatusFailed          = domain.EmailBroadcastStatusFailed
)

// Re-exported domain errors.
var (
	ErrEmailBroadcastNotFound           = domain.ErrEmailBroadcastNotFound
	ErrEmailBroadcastSubjectRequired    = domain.ErrEmailBroadcastSubjectRequired
	ErrEmailBroadcastBodyRequired       = domain.ErrEmailBroadcastBodyRequired
	ErrEmailBroadcastSubjectTooLong     = domain.ErrEmailBroadcastSubjectTooLong
	ErrEmailBroadcastBodyTooLong        = domain.ErrEmailBroadcastBodyTooLong
	ErrEmailBroadcastInvalidBodyFormat  = domain.ErrEmailBroadcastInvalidBodyFormat
	ErrEmailBroadcastInvalidMode        = domain.ErrEmailBroadcastInvalidMode
	ErrEmailBroadcastNoRecipients       = domain.ErrEmailBroadcastNoRecipients
	ErrEmailBroadcastTooManyRecipients  = domain.ErrEmailBroadcastTooManyRecipients
	ErrEmailBroadcastEmailNotConfigured = domain.ErrEmailBroadcastEmailNotConfigured
	ErrEmailBroadcastDeleteInFlight     = domain.ErrEmailBroadcastDeleteInFlight
)

// EmailBroadcast 是 service 层暴露的批量公告邮件聚合状态。
type EmailBroadcast struct {
	ID               int64
	Subject          string
	Body             string
	BodyFormat       string
	RecipientsMode   string
	RecipientUserIDs []int64
	Status           string
	TotalCount       int
	SuccessCount     int
	FailedCount      int
	ErrorMessage     *string
	CreatedBy        *int64
	StartedAt        *time.Time
	FinishedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// EmailBroadcastCreateInput 描述创建/发起一次批量邮件公告所需输入。
type EmailBroadcastCreateInput struct {
	Subject          string
	Body             string
	BodyFormat       string
	RecipientsMode   string
	RecipientUserIDs []int64
	CreatedBy        *int64
}

// EmailBroadcastListParams 控制 ListBroadcasts 查询的分页与过滤。
type EmailBroadcastListParams struct {
	Page     int
	PageSize int
	Status   string // 可空：按状态过滤
}

// EmailBroadcastListResult 列表查询返回值。
type EmailBroadcastListResult struct {
	Items    []EmailBroadcast
	Total    int64
	Page     int
	PageSize int
}

// EmailBroadcastStatusUpdate 用于增量更新一次广播的执行状态。
// nil 字段表示"不变"，便于把 finished_at / counts 等部分字段一次提交。
type EmailBroadcastStatusUpdate struct {
	Status       *string
	TotalCount   *int
	SuccessCount *int
	FailedCount  *int
	ErrorMessage *string
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

// EmailBroadcastRepository 定义批量邮件公告的持久化接口。
type EmailBroadcastRepository interface {
	Create(ctx context.Context, broadcast *EmailBroadcast) error
	GetByID(ctx context.Context, id int64) (*EmailBroadcast, error)
	UpdateStatus(ctx context.Context, id int64, patch EmailBroadcastStatusUpdate) error
	List(ctx context.Context, params EmailBroadcastListParams) (*EmailBroadcastListResult, error)
	Delete(ctx context.Context, id int64) error
}
