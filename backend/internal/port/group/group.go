// Package group contains the port interfaces (repository abstractions)
// for the group bounded context. DTO/value types live in internal/domain;
// this package only owns the persistence port contracts and update VOs.
package group

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// SortOrderUpdate is a batch sort-order mutation entry.
type SortOrderUpdate struct {
	ID        int64 `json:"id"`
	SortOrder int   `json:"sort_order"`
}

// Repository persists groups and membership projections used by gateway paths.
type Repository interface {
	Create(ctx context.Context, group *domain.Group) error
	GetByID(ctx context.Context, id int64) (*domain.Group, error)
	GetByIDLite(ctx context.Context, id int64) (*domain.Group, error)
	Update(ctx context.Context, group *domain.Group) error
	Delete(ctx context.Context, id int64) error
	DeleteCascade(ctx context.Context, id int64) ([]int64, error)

	List(ctx context.Context, params pagination.PaginationParams) ([]domain.Group, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]domain.Group, *pagination.PaginationResult, error)
	ListActive(ctx context.Context) ([]domain.Group, error)
	ListActiveByPlatform(ctx context.Context, platform string) ([]domain.Group, error)

	ExistsByName(ctx context.Context, name string) (bool, error)
	GetAccountCount(ctx context.Context, groupID int64) (total int64, active int64, err error)
	DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error)
	// GetAccountIDsByGroupIDs returns deduplicated account IDs across groups.
	GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error)
	// BindAccountsToGroup binds accounts to a group.
	BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error
	// UpdateSortOrders batch-updates group sort order.
	UpdateSortOrders(ctx context.Context, updates []SortOrderUpdate) error
}

// DuplicateRepository is the write capability used by admin one-click copy.
type DuplicateRepository interface {
	// FindByDuplicateOperationID is the read-only recovery lookup after an
	// ambiguous idempotency-store failure.
	FindByDuplicateOperationID(ctx context.Context, operationID string) (*domain.Group, error)
	// CreateFromSource atomically persists the group, copies the source group's
	// exact account priorities, and writes the scheduler outbox event.
	CreateFromSource(ctx context.Context, group *domain.Group, sourceGroupID int64) error
}

// AdminRepository makes group-duplication an explicit admin-service dependency
// without widening gateway-only group test doubles.
type AdminRepository interface {
	Repository
	DuplicateRepository
}
