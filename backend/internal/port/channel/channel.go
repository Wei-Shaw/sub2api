// Package channel contains the port interfaces (repository abstractions)
// for the channel bounded context. DTO/value types live in internal/domain;
// this package only owns the persistence port contracts.
package channel

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// Repository persists channels and the pricing/mapping/group projections
// used by the channel aggregate.
type Repository interface {
	Create(ctx context.Context, channel *domain.Channel) error
	GetByID(ctx context.Context, id int64) (*domain.Channel, error)
	Update(ctx context.Context, channel *domain.Channel) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]domain.Channel, *pagination.PaginationResult, error)
	ListAll(ctx context.Context) ([]domain.Channel, error)
	ExistsByName(ctx context.Context, name string) (bool, error)
	ExistsByNameExcluding(ctx context.Context, name string, excludeID int64) (bool, error)

	// 分组关联
	GetGroupIDs(ctx context.Context, channelID int64) ([]int64, error)
	SetGroupIDs(ctx context.Context, channelID int64, groupIDs []int64) error
	GetChannelIDByGroupID(ctx context.Context, groupID int64) (int64, error)
	GetGroupsInOtherChannels(ctx context.Context, channelID int64, groupIDs []int64) ([]int64, error)

	// 分组平台查询
	GetGroupPlatforms(ctx context.Context, groupIDs []int64) (map[int64]string, error)

	// 模型定价
	ListModelPricing(ctx context.Context, channelID int64) ([]domain.ChannelModelPricing, error)
	CreateModelPricing(ctx context.Context, pricing *domain.ChannelModelPricing) error
	UpdateModelPricing(ctx context.Context, pricing *domain.ChannelModelPricing) error
	DeleteModelPricing(ctx context.Context, id int64) error
	ReplaceModelPricing(ctx context.Context, channelID int64, pricingList []domain.ChannelModelPricing) error
}
