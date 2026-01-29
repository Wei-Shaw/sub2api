package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// SoraAccount 表示 Sora 账号扩展信息。
type SoraAccount struct {
	AccountID          int64
	AccessToken        string
	SessionToken       string
	RefreshToken       string
	ClientID           string
	Email              string
	Username           string
	Remark             string
	UseCount           int
	PlanType           string
	PlanTitle          string
	SubscriptionEnd    *time.Time
	SoraSupported      bool
	SoraInviteCode     string
	SoraRedeemedCount  int
	SoraRemainingCount int
	SoraTotalCount     int
	SoraCooldownUntil  *time.Time
	CooledUntil        *time.Time
	ImageEnabled       bool
	VideoEnabled       bool
	ImageConcurrency   int
	VideoConcurrency   int
	IsExpired          bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
REDACTED

// SoraUsageStat 表示 Sora 调用统计。
type SoraUsageStat struct {
	AccountID             int64
	ImageCount            int
	VideoCount            int
	ErrorCount            int
	LastErrorAt           *time.Time
	TodayImageCount       int
	TodayVideoCount       int
	TodayErrorCount       int
	TodayDate             *time.Time
	ConsecutiveErrorCount int
	CreatedAt             time.Time
	UpdatedAt             time.Time
REDACTED

// SoraTask 表示 Sora 任务记录。
type SoraTask struct {
	TaskID       string
	AccountID    int64
	Model        string
	Prompt       string
	Status       string
	Progress     float64
	ResultURLs   string
	ErrorMessage string
	RetryCount   int
	CreatedAt    time.Time
	CompletedAt  *time.Time
REDACTED

// SoraCacheFile 表示 Sora 缓存文件记录。
type SoraCacheFile struct {
	ID          int64
	TaskID      string
	AccountID   int64
	UserID      int64
	MediaType   string
	OriginalURL string
	CachePath   string
	CacheURL    string
	SizeBytes   int64
	CreatedAt   time.Time
REDACTED

// SoraAccountRepository 定义 Sora 账号仓储接口。
type SoraAccountRepository interface {
	GetByAccountID(ctx context.Context, accountID int64) (*SoraAccount, error)
	GetByAccountIDs(ctx context.Context, accountIDs []int64) (map[int64]*SoraAccount, error)
	Upsert(ctx context.Context, accountID int64, updates map[string]any) error
REDACTED

// SoraUsageStatRepository 定义 Sora 调用统计仓储接口。
type SoraUsageStatRepository interface {
	RecordSuccess(ctx context.Context, accountID int64, isVideo bool) error
	RecordError(ctx context.Context, accountID int64) (int, error)
	ResetConsecutiveErrors(ctx context.Context, accountID int64) error
	GetByAccountID(ctx context.Context, accountID int64) (*SoraUsageStat, error)
	GetByAccountIDs(ctx context.Context, accountIDs []int64) (map[int64]*SoraUsageStat, error)
	List(ctx context.Context, params pagination.PaginationParams) ([]*SoraUsageStat, *pagination.PaginationResult, error)
REDACTED

// SoraTaskRepository 定义 Sora 任务仓储接口。
type SoraTaskRepository interface {
	Create(ctx context.Context, task *SoraTask) error
	UpdateStatus(ctx context.Context, taskID string, status string, progress float64, resultURLs string, errorMessage string, completedAt *time.Time) error
REDACTED

// SoraCacheFileRepository 定义 Sora 缓存文件仓储接口。
type SoraCacheFileRepository interface {
	Create(ctx context.Context, file *SoraCacheFile) error
	ListOldest(ctx context.Context, limit int) ([]*SoraCacheFile, error)
	DeleteByIDs(ctx context.Context, ids []int64) error
REDACTED
