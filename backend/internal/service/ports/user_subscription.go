package ports

import (
	"context"
	"time"

	"sub2api/internal/model"
	"sub2api/internal/pkg/pagination"

	"gorm.io/gorm"
)

type UserSubscriptionRepository interface {
	// WithTx 在事务上下文内返回仓库实例。
	WithTx(tx *gorm.DB) UserSubscriptionRepository
	Create(ctx context.Context, sub *model.UserSubscription) error
	GetByID(ctx context.Context, id int64) (*model.UserSubscription, error)
	GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*model.UserSubscription, error)
	GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*model.UserSubscription, error)
	Update(ctx context.Context, sub *model.UserSubscription) error
	Delete(ctx context.Context, id int64) error

	ListByUserID(ctx context.Context, userID int64) ([]model.UserSubscription, error)
	ListActiveByUserID(ctx context.Context, userID int64) ([]model.UserSubscription, error)
	ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]model.UserSubscription, *pagination.PaginationResult, error)
	List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status string) ([]model.UserSubscription, *pagination.PaginationResult, error)

	ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error)
	ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error
	UpdateStatus(ctx context.Context, subscriptionID int64, status string) error
	UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error

	ActivateWindows(ctx context.Context, id int64, start time.Time) error
	ResetDailyUsage(ctx context.Context, id int64, newWindowStart time.Time) error
	ResetWeeklyUsage(ctx context.Context, id int64, newWindowStart time.Time) error
	ResetMonthlyUsage(ctx context.Context, id int64, newWindowStart time.Time) error
	IncrementUsage(ctx context.Context, id int64, costUSD float64) error

	BatchUpdateExpiredStatus(ctx context.Context) (int64, error)
}
