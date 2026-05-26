package service

import (
	"context"
	"sync/atomic"

	infraerrors "github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/pagination"

	"golang.org/x/sync/singleflight"
)

var (
	ErrChannelNotFound       = infraerrors.NotFound("CHANNEL_NOT_FOUND", "channel not found")
	ErrChannelExists         = infraerrors.Conflict("CHANNEL_EXISTS", "channel name already exists")
	ErrGroupAlreadyInChannel = infraerrors.Conflict(
		"GROUP_ALREADY_IN_CHANNEL",
		"one or more groups already belong to another channel",
	)
)

// AuthCacheInvalidator is the minimal interface the channel service needs
// from the core's API-key auth cache. The plugin wires nil here today; once
// the auth cache is exposed through the SDK, the host can implement this
// interface and inject it.
type AuthCacheInvalidator interface {
	InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64)
}

// ChannelRepository is the data-access contract the service depends on. It is
// implemented in the plugin's repository package against the SDK's *sql.DB.
type ChannelRepository interface {
	Create(ctx context.Context, channel *Channel) error
	GetByID(ctx context.Context, id int64) (*Channel, error)
	Update(ctx context.Context, channel *Channel) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Channel, *pagination.PaginationResult, error)
	ListAll(ctx context.Context) ([]Channel, error)
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
	ListModelPricing(ctx context.Context, channelID int64) ([]ChannelModelPricing, error)
	CreateModelPricing(ctx context.Context, pricing *ChannelModelPricing) error
	UpdateModelPricing(ctx context.Context, pricing *ChannelModelPricing) error
	DeleteModelPricing(ctx context.Context, id int64) error
	ReplaceModelPricing(ctx context.Context, channelID int64, pricingList []ChannelModelPricing) error
}

// ChannelService is the channel-management service exposed by the plugin.
//
// The implementation is split across several files in this package, all
// sharing the same receiver type:
//
//   - channel_service.go (this file): construction, setters, trivial repo
//     pass-throughs (GetByID, List, GroupPlatforms).
//   - channel_cache.go:   in-memory pricing/mapping cache lifecycle and the
//     Redis cache writer plumbing (rebuild / invalidate).
//   - channel_lookup.go:  hot-path read APIs (GetChannelForGroup,
//     GetChannelModelPricing, ResolveChannelMapping*, IsModelRestricted).
//   - channel_validation.go: input validation and conflict detection.
//   - channel_crud.go:    Create / Update / Delete plus afterCommit
//     orchestration that consolidates cache invalidation, rebuild and
//     event publishing.
type ChannelService struct {
	repo                 ChannelRepository
	authCacheInvalidator AuthCacheInvalidator
	cacheWriter          *CacheWriter          // optional Redis cache writer (meta/mapping only post-P4)
	eventPublisher       PricingEventPublisher // optional in-process pricing event broker (P4)

	cache   atomic.Value // *channelCache
	cacheSF singleflight.Group
}

// NewChannelService constructs a ChannelService. authCacheInvalidator may be
// nil if the plugin host does not provide an auth-cache invalidator.
//
// Cache writer must be attached separately via SetCacheWriter once the plugin
// context (Redis client) is available.
func NewChannelService(repo ChannelRepository, authCacheInvalidator AuthCacheInvalidator) *ChannelService {
	return &ChannelService{
		repo:                 repo,
		authCacheInvalidator: authCacheInvalidator,
	}
}

// SetCacheWriter wires the Redis cache writer that mirrors channel state to
// the gateway-side cache. Passing nil disables cache writes entirely (used by
// unit tests or when the plugin runs without Redis).
func (s *ChannelService) SetCacheWriter(writer *CacheWriter) {
	s.cacheWriter = writer
}

// SetEventPublisher wires the in-process pricing event broker. After
// every successful CRUD commit ChannelService publishes UPSERT/DELETE
// events for the affected (group, platform, model) tuples so the host's
// PricingExtension Watch stream can keep its cache fresh sub-second
// without round-tripping through Redis.
//
// Passing nil disables broadcasting; the host's 5-minute List re-sync
// then becomes the only freshness mechanism (matches the pre-P4
// behaviour).
func (s *ChannelService) SetEventPublisher(publisher PricingEventPublisher) {
	s.eventPublisher = publisher
}

// GroupPlatforms exposes the repo-backed group→platform lookup so the cache
// writer can resolve which Redis namespace each group lives in.
func (s *ChannelService) GroupPlatforms(ctx context.Context, groupIDs []int64) (map[int64]string, error) {
	return s.repo.GetGroupPlatforms(ctx, groupIDs)
}

// GetByID returns the channel with id loaded, or ErrChannelNotFound.
func (s *ChannelService) GetByID(ctx context.Context, id int64) (*Channel, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns a paginated channel list.
func (s *ChannelService) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Channel, *pagination.PaginationResult, error) {
	return s.repo.List(ctx, params, status, search)
}
