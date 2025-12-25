package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"gorm.io/gorm"
)

type GroupRepository interface {
	Create(ctx context.Context, group *model.Group) error
	GetByID(ctx context.Context, id int64) (*model.Group, error)
	Update(ctx context.Context, group *model.Group) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]model.Group, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status string, isExclusive *bool) ([]model.Group, *pagination.PaginationResult, error)
	ListActive(ctx context.Context) ([]model.Group, error)
	ListActiveByPlatform(ctx context.Context, platform string) ([]model.Group, error)

	ExistsByName(ctx context.Context, name string) (bool, error)
	GetAccountCount(ctx context.Context, groupID int64) (int64, error)
	DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error)

	DB() *gorm.DB
}
