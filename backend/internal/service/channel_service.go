package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"golang.org/x/sync/singleflight"
)

var (
	ErrChannelNotFound = infraerrors.NotFound("CHANNEL_NOT_FOUND", "channel not found")
	ErrChannelExists   = infraerrors.Conflict("CHANNEL_EXISTS", "channel name already exists")
	ErrGroupAlreadyInChannel = infraerrors.Conflict(
		"GROUP_ALREADY_IN_CHANNEL",
		"one or more groups already belong to another channel",
	)
)

// ChannelRepository 渠道数据访问接口
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

	// 模型定价
	ListModelPricing(ctx context.Context, channelID int64) ([]ChannelModelPricing, error)
	CreateModelPricing(ctx context.Context, pricing *ChannelModelPricing) error
	UpdateModelPricing(ctx context.Context, pricing *ChannelModelPricing) error
	DeleteModelPricing(ctx context.Context, id int64) error
	ReplaceModelPricing(ctx context.Context, channelID int64, pricingList []ChannelModelPricing) error
}

// channelCache 渠道缓存快照
type channelCache struct {
	// byID: channelID -> *Channel（含 ModelPricing）
	byID map[int64]*Channel
	// byGroupID: groupID -> channelID
	byGroupID map[int64]int64
	loadedAt  time.Time
}

const (
	channelCacheTTL    = 60 * time.Second
	channelErrorTTL    = 5 * time.Second // DB 错误时的短缓存
	channelCacheDBTimeout = 10 * time.Second
)

// ChannelService 渠道管理服务
type ChannelService struct {
	repo                 ChannelRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator

	cache   atomic.Value // *channelCache
	cacheSF singleflight.Group
}

// NewChannelService 创建渠道服务实例
func NewChannelService(repo ChannelRepository, authCacheInvalidator APIKeyAuthCacheInvalidator) *ChannelService {
	s := &ChannelService{
		repo:                 repo,
		authCacheInvalidator: authCacheInvalidator,
	}
	return s
}

// loadCache 加载或返回缓存的渠道数据
func (s *ChannelService) loadCache(ctx context.Context) (*channelCache, error) {
	if cached, ok := s.cache.Load().(*channelCache); ok {
		if time.Since(cached.loadedAt) < channelCacheTTL {
			return cached, nil
		}
	}

	result, err, _ := s.cacheSF.Do("channel_cache", func() (any, error) {
		// 双重检查
		if cached, ok := s.cache.Load().(*channelCache); ok {
			if time.Since(cached.loadedAt) < channelCacheTTL {
				return cached, nil
			}
		}
		return s.buildCache(ctx)
	})
	if err != nil {
		return nil, err
	}
	return result.(*channelCache), nil
}

// buildCache 从数据库构建渠道缓存。
// 使用独立 context 避免请求取消导致空值被长期缓存。
func (s *ChannelService) buildCache(ctx context.Context) (*channelCache, error) {
	// 断开请求取消链，避免客户端断连导致空值被长期缓存
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), channelCacheDBTimeout)
	defer cancel()

	channels, err := s.repo.ListAll(dbCtx)
	if err != nil {
		// error-TTL：失败时存入短 TTL 空缓存，防止紧密重试
		slog.Warn("failed to build channel cache", "error", err)
		errorCache := &channelCache{
			byID:      make(map[int64]*Channel),
			byGroupID: make(map[int64]int64),
			loadedAt:  time.Now().Add(channelCacheTTL - channelErrorTTL), // 使剩余 TTL = errorTTL
		}
		s.cache.Store(errorCache)
		return nil, fmt.Errorf("list all channels: %w", err)
	}

	cache := &channelCache{
		byID:      make(map[int64]*Channel, len(channels)),
		byGroupID: make(map[int64]int64),
		loadedAt:  time.Now(),
	}

	for i := range channels {
		ch := &channels[i]
		cache.byID[ch.ID] = ch
		for _, gid := range ch.GroupIDs {
			cache.byGroupID[gid] = ch.ID
		}
	}

	s.cache.Store(cache)
	return cache, nil
}

// invalidateCache 使缓存失效，让下次读取时自然重建
func (s *ChannelService) invalidateCache() {
	s.cache.Store((*channelCache)(nil))
	s.cacheSF.Forget("channel_cache")
}

// GetChannelForGroup 获取分组关联的渠道（热路径，从缓存读取）
// 返回深拷贝，不污染缓存。
func (s *ChannelService) GetChannelForGroup(ctx context.Context, groupID int64) (*Channel, error) {
	cache, err := s.loadCache(ctx)
	if err != nil {
		return nil, err
	}

	channelID, ok := cache.byGroupID[groupID]
	if !ok {
		return nil, nil
	}

	ch, ok := cache.byID[channelID]
	if !ok {
		return nil, nil
	}

	if !ch.IsActive() {
		return nil, nil
	}

	return ch.Clone(), nil
}

// GetChannelModelPricing 获取指定分组+模型的渠道定价（热路径）
func (s *ChannelService) GetChannelModelPricing(ctx context.Context, groupID int64, model string) *ChannelModelPricing {
	ch, err := s.GetChannelForGroup(ctx, groupID)
	if err != nil {
		slog.Warn("failed to get channel for group", "group_id", groupID, "error", err)
		return nil
	}
	if ch == nil {
		return nil
	}
	return ch.GetModelPricing(model)
}

// --- CRUD ---

// Create 创建渠道
func (s *ChannelService) Create(ctx context.Context, input *CreateChannelInput) (*Channel, error) {
	exists, err := s.repo.ExistsByName(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("check channel exists: %w", err)
	}
	if exists {
		return nil, ErrChannelExists
	}

	// 检查分组冲突
	if len(input.GroupIDs) > 0 {
		conflicting, err := s.repo.GetGroupsInOtherChannels(ctx, 0, input.GroupIDs)
		if err != nil {
			return nil, fmt.Errorf("check group conflicts: %w", err)
		}
		if len(conflicting) > 0 {
			return nil, ErrGroupAlreadyInChannel
		}
	}

	channel := &Channel{
		Name:         input.Name,
		Description:  input.Description,
		Status:       StatusActive,
		GroupIDs:     input.GroupIDs,
		ModelPricing: input.ModelPricing,
	}

	if err := s.repo.Create(ctx, channel); err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}

	s.invalidateCache()
	return s.repo.GetByID(ctx, channel.ID)
}

// GetByID 获取渠道详情
func (s *ChannelService) GetByID(ctx context.Context, id int64) (*Channel, error) {
	return s.repo.GetByID(ctx, id)
}

// Update 更新渠道
func (s *ChannelService) Update(ctx context.Context, id int64, input *UpdateChannelInput) (*Channel, error) {
	channel, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}

	if input.Name != "" && input.Name != channel.Name {
		exists, err := s.repo.ExistsByNameExcluding(ctx, input.Name, id)
		if err != nil {
			return nil, fmt.Errorf("check channel exists: %w", err)
		}
		if exists {
			return nil, ErrChannelExists
		}
		channel.Name = input.Name
	}

	if input.Description != nil {
		channel.Description = *input.Description
	}

	if input.Status != "" {
		channel.Status = input.Status
	}

	// 检查分组冲突
	if input.GroupIDs != nil {
		conflicting, err := s.repo.GetGroupsInOtherChannels(ctx, id, *input.GroupIDs)
		if err != nil {
			return nil, fmt.Errorf("check group conflicts: %w", err)
		}
		if len(conflicting) > 0 {
			return nil, ErrGroupAlreadyInChannel
		}
		channel.GroupIDs = *input.GroupIDs
	}

	if input.ModelPricing != nil {
		channel.ModelPricing = *input.ModelPricing
	}

	if err := s.repo.Update(ctx, channel); err != nil {
		return nil, fmt.Errorf("update channel: %w", err)
	}

	s.invalidateCache()

	// 失效关联分组的 auth 缓存
	if s.authCacheInvalidator != nil {
		groupIDs, err := s.repo.GetGroupIDs(ctx, id)
		if err != nil {
			slog.Warn("failed to get group IDs for cache invalidation", "channel_id", id, "error", err)
		}
		for _, gid := range groupIDs {
			s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, gid)
		}
	}

	return s.repo.GetByID(ctx, id)
}

// Delete 删除渠道
func (s *ChannelService) Delete(ctx context.Context, id int64) error {
	// 先获取关联分组用于失效缓存
	groupIDs, err := s.repo.GetGroupIDs(ctx, id)
	if err != nil {
		slog.Warn("failed to get group IDs before delete", "channel_id", id, "error", err)
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}

	s.invalidateCache()

	if s.authCacheInvalidator != nil {
		for _, gid := range groupIDs {
			s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, gid)
		}
	}

	return nil
}

// List 获取渠道列表
func (s *ChannelService) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Channel, *pagination.PaginationResult, error) {
	return s.repo.List(ctx, params, status, search)
}

// --- Input types ---

// CreateChannelInput 创建渠道输入
type CreateChannelInput struct {
	Name         string
	Description  string
	GroupIDs     []int64
	ModelPricing []ChannelModelPricing
}

// UpdateChannelInput 更新渠道输入
type UpdateChannelInput struct {
	Name         string
	Description  *string
	Status       string
	GroupIDs     *[]int64
	ModelPricing *[]ChannelModelPricing
}
