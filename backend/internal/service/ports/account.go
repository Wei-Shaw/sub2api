package ports

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type AccountRepository interface {
	Create(ctx context.Context, account *model.Account) error
	GetByID(ctx context.Context, id int64) (*model.Account, error)
	// GetByCRSAccountID finds an account previously synced from CRS.
	// Returns (nil, nil) if not found.
	GetByCRSAccountID(ctx context.Context, crsAccountID string) (*model.Account, error)
	Update(ctx context.Context, account *model.Account) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]model.Account, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string) ([]model.Account, *pagination.PaginationResult, error)
	ListByGroup(ctx context.Context, groupID int64) ([]model.Account, error)
	ListActive(ctx context.Context) ([]model.Account, error)
	ListByPlatform(ctx context.Context, platform string) ([]model.Account, error)

	UpdateLastUsed(ctx context.Context, id int64) error
	SetError(ctx context.Context, id int64, errorMsg string) error
	SetSchedulable(ctx context.Context, id int64, schedulable bool) error
	BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error

	ListSchedulable(ctx context.Context) ([]model.Account, error)
	ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]model.Account, error)
	ListSchedulableByPlatform(ctx context.Context, platform string) ([]model.Account, error)
	ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]model.Account, error)

	SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error
	SetOverloaded(ctx context.Context, id int64, until time.Time) error
	ClearRateLimit(ctx context.Context, id int64) error
	UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
	BulkUpdate(ctx context.Context, ids []int64, updates AccountBulkUpdate) (int64, error)
}

// AccountBulkUpdate describes the fields that can be updated in a bulk operation.
// Nil pointers mean "do not change".
type AccountBulkUpdate struct {
	Name        *string
	ProxyID     *int64
	Concurrency *int
	Priority    *int
	Status      *string
	Credentials map[string]any
	Extra       map[string]any
}
