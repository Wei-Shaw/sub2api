package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
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

	// 分组平台查询
	GetGroupPlatforms(ctx context.Context, groupIDs []int64) (map[int64]string, error)

	// 模型定价
	ListModelPricing(ctx context.Context, channelID int64) ([]ChannelModelPricing, error)
	CreateModelPricing(ctx context.Context, pricing *ChannelModelPricing) error
	UpdateModelPricing(ctx context.Context, pricing *ChannelModelPricing) error
	DeleteModelPricing(ctx context.Context, id int64) error
	ReplaceModelPricing(ctx context.Context, channelID int64, pricingList []ChannelModelPricing) error
}

// channelModelKey 渠道缓存复合键（显式包含 platform 防止跨平台同名模型冲突）
type channelModelKey struct {
	groupID  int64
	platform string // 平台标识
	model    string // lowercase
}

// channelGroupPlatformKey 通配符定价缓存键
type channelGroupPlatformKey struct {
	groupID  int64
	platform string
}

// wildcardPricingEntry 通配符定价条目
type wildcardPricingEntry struct {
	prefix  string
	pricing *ChannelModelPricing
}

// channelCache 渠道缓存快照（扁平化哈希结构，热路径 O(1) 查找）
type channelCache struct {
	// 热路径查找
	pricingByGroupModel    map[channelModelKey]*ChannelModelPricing // (groupID, platform, model) → 定价
	wildcardByGroupPlatform map[channelGroupPlatformKey][]*wildcardPricingEntry // (groupID, platform) → 通配符定价（前缀长度降序）
	mappingByGroupModel    map[channelModelKey]string                // (groupID, platform, model) → 映射目标
	channelByGroupID       map[int64]*Channel                        // groupID → 渠道
	groupPlatform          map[int64]string                          // groupID → platform

	// 冷路径（CRUD 操作）
	byID     map[int64]*Channel
	loadedAt time.Time
}

// ChannelMappingResult 渠道映射查找结果
type ChannelMappingResult struct {
	MappedModel        string // 映射后的模型名（无映射时等于原始模型名）
	ChannelID          int64  // 渠道 ID（0 = 无渠道关联）
	Mapped             bool   // 是否发生了映射
	BillingModelSource string // 计费模型来源（"requested" / "upstream"）
}

// BuildModelMappingChain 根据映射结果和上游实际模型构建映射链描述。
// reqModel: 客户端请求的原始模型名。
// upstreamModel: 上游实际使用的模型名（ForwardResult.UpstreamModel）。
// 返回空字符串表示无映射。
func (r ChannelMappingResult) BuildModelMappingChain(reqModel, upstreamModel string) string {
	if !r.Mapped {
		if upstreamModel != "" && upstreamModel != reqModel {
			return reqModel + "→" + upstreamModel
		}
		return ""
	}
	if upstreamModel != "" && upstreamModel != r.MappedModel {
		return reqModel + "→" + r.MappedModel + "→" + upstreamModel
	}
	return reqModel + "→" + r.MappedModel
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
			pricingByGroupModel:    make(map[channelModelKey]*ChannelModelPricing),
			wildcardByGroupPlatform: make(map[channelGroupPlatformKey][]*wildcardPricingEntry),
			mappingByGroupModel:    make(map[channelModelKey]string),
			channelByGroupID:       make(map[int64]*Channel),
			groupPlatform:          make(map[int64]string),
			byID:                   make(map[int64]*Channel),
			loadedAt:               time.Now().Add(channelCacheTTL - channelErrorTTL), // 使剩余 TTL = errorTTL
		}
		s.cache.Store(errorCache)
		return nil, fmt.Errorf("list all channels: %w", err)
	}

	// 收集所有 groupID，批量查询 platform
	var allGroupIDs []int64
	for i := range channels {
		allGroupIDs = append(allGroupIDs, channels[i].GroupIDs...)
	}
	groupPlatforms := make(map[int64]string)
	if len(allGroupIDs) > 0 {
		groupPlatforms, err = s.repo.GetGroupPlatforms(dbCtx, allGroupIDs)
		if err != nil {
			slog.Warn("failed to load group platforms for channel cache", "error", err)
			// 降级：继续构建缓存但无法按平台过滤
		}
	}

	cache := &channelCache{
		pricingByGroupModel:    make(map[channelModelKey]*ChannelModelPricing),
		wildcardByGroupPlatform: make(map[channelGroupPlatformKey][]*wildcardPricingEntry),
		mappingByGroupModel:    make(map[channelModelKey]string),
		channelByGroupID:       make(map[int64]*Channel),
		groupPlatform:          groupPlatforms,
		byID:                   make(map[int64]*Channel, len(channels)),
		loadedAt:               time.Now(),
	}

	for i := range channels {
		ch := &channels[i]
		cache.byID[ch.ID] = ch

		// 展开到分组维度
		for _, gid := range ch.GroupIDs {
			cache.channelByGroupID[gid] = ch
			platform := groupPlatforms[gid] // e.g. "anthropic"

			// 只展开该平台的模型定价到 (groupID, platform, model) → *ChannelModelPricing
			for j := range ch.ModelPricing {
				pricing := &ch.ModelPricing[j]
				if pricing.Platform != platform {
					continue // 跳过非本平台的定价
				}
				for _, model := range pricing.Models {
					if strings.HasSuffix(model, "*") {
						// 通配符模型 → 存入 wildcardByGroupPlatform
						prefix := strings.ToLower(strings.TrimSuffix(model, "*"))
						gpKey := channelGroupPlatformKey{groupID: gid, platform: platform}
						cache.wildcardByGroupPlatform[gpKey] = append(cache.wildcardByGroupPlatform[gpKey], &wildcardPricingEntry{
							prefix:  prefix,
							pricing: pricing,
						})
					} else {
						key := channelModelKey{groupID: gid, platform: platform, model: strings.ToLower(model)}
						cache.pricingByGroupModel[key] = pricing
					}
				}
			}

			// 只展开该平台的模型映射到 (groupID, platform, model) → target
			if platformMapping, ok := ch.ModelMapping[platform]; ok {
				for src, dst := range platformMapping {
					key := channelModelKey{groupID: gid, platform: platform, model: strings.ToLower(src)}
					cache.mappingByGroupModel[key] = dst
				}
			}
		}
	}

	// 通配符条目按前缀长度降序排列（最长前缀优先匹配）
	for gpKey, entries := range cache.wildcardByGroupPlatform {
		sort.Slice(entries, func(i, j int) bool {
			return len(entries[i].prefix) > len(entries[j].prefix)
		})
		cache.wildcardByGroupPlatform[gpKey] = entries
	}

	s.cache.Store(cache)
	return cache, nil
}

// invalidateCache 使缓存失效，让下次读取时自然重建
func (s *ChannelService) invalidateCache() {
	s.cache.Store((*channelCache)(nil))
	s.cacheSF.Forget("channel_cache")
}

// matchWildcard 在通配符定价中查找匹配项（最长前缀优先）
func (c *channelCache) matchWildcard(groupID int64, platform, modelLower string) *ChannelModelPricing {
	gpKey := channelGroupPlatformKey{groupID: groupID, platform: platform}
	wildcards := c.wildcardByGroupPlatform[gpKey]
	for _, wc := range wildcards {
		if strings.HasPrefix(modelLower, wc.prefix) {
			return wc.pricing
		}
	}
	return nil
}

// GetChannelForGroup 获取分组关联的渠道（热路径 O(1)）
func (s *ChannelService) GetChannelForGroup(ctx context.Context, groupID int64) (*Channel, error) {
	cache, err := s.loadCache(ctx)
	if err != nil {
		return nil, err
	}

	ch, ok := cache.channelByGroupID[groupID]
	if !ok || !ch.IsActive() {
		return nil, nil
	}

	return ch.Clone(), nil
}

// GetChannelModelPricing 获取指定分组+模型的渠道定价（热路径 O(1)）
func (s *ChannelService) GetChannelModelPricing(ctx context.Context, groupID int64, model string) *ChannelModelPricing {
	cache, err := s.loadCache(ctx)
	if err != nil {
		slog.Warn("failed to load channel cache", "group_id", groupID, "error", err)
		return nil
	}

	// 检查渠道是否启用
	ch, ok := cache.channelByGroupID[groupID]
	if !ok || !ch.IsActive() {
		return nil
	}

	platform := cache.groupPlatform[groupID]
	key := channelModelKey{groupID: groupID, platform: platform, model: strings.ToLower(model)}
	pricing, ok := cache.pricingByGroupModel[key]
	if !ok {
		// 精确查找失败，尝试通配符匹配
		pricing = cache.matchWildcard(groupID, platform, strings.ToLower(model))
		if pricing == nil {
			return nil
		}
	}

	cp := pricing.Clone()
	return &cp
}

// ResolveChannelMapping 解析渠道级模型映射（热路径 O(1)）
// 返回映射结果，包含映射后的模型名、渠道 ID、计费模型来源。
func (s *ChannelService) ResolveChannelMapping(ctx context.Context, groupID int64, model string) ChannelMappingResult {
	cache, err := s.loadCache(ctx)
	if err != nil {
		return ChannelMappingResult{MappedModel: model}
	}

	ch, ok := cache.channelByGroupID[groupID]
	if !ok || !ch.IsActive() {
		return ChannelMappingResult{MappedModel: model}
	}

	result := ChannelMappingResult{
		MappedModel:        model,
		ChannelID:          ch.ID,
		BillingModelSource: ch.BillingModelSource,
	}
	if result.BillingModelSource == "" {
		result.BillingModelSource = BillingModelSourceRequested
	}

	platform := cache.groupPlatform[groupID]
	key := channelModelKey{groupID: groupID, platform: platform, model: strings.ToLower(model)}
	if mapped, ok := cache.mappingByGroupModel[key]; ok {
		result.MappedModel = mapped
		result.Mapped = true
	}

	return result
}

// IsModelRestricted 检查模型是否被渠道限制。
// 返回 true 表示模型被限制（不在允许列表中）。
// 如果渠道未启用模型限制或分组无渠道关联，返回 false。
func (s *ChannelService) IsModelRestricted(ctx context.Context, groupID int64, model string) bool {
	cache, err := s.loadCache(ctx)
	if err != nil {
		return false // 缓存加载失败时不限制
	}

	ch, ok := cache.channelByGroupID[groupID]
	if !ok || !ch.IsActive() || !ch.RestrictModels {
		return false
	}

	// 检查模型是否在定价列表中
	platform := cache.groupPlatform[groupID]
	key := channelModelKey{groupID: groupID, platform: platform, model: strings.ToLower(model)}
	_, exists := cache.pricingByGroupModel[key]
	if exists {
		return false
	}
	// 精确查找失败，尝试通配符匹配
	if cache.matchWildcard(groupID, platform, strings.ToLower(model)) != nil {
		return false
	}
	return true
}

// ResolveChannelMappingAndRestrict 解析渠道映射并检查模型限制（组合方法）。
// 返回映射结果和是否被限制。groupID 为 nil 时跳过。
func (s *ChannelService) ResolveChannelMappingAndRestrict(ctx context.Context, groupID *int64, model string) (ChannelMappingResult, bool) {
	var mapping ChannelMappingResult
	mapping.MappedModel = model
	if groupID == nil {
		return mapping, false
	}
	mapping = s.ResolveChannelMapping(ctx, *groupID, model)
	restricted := s.IsModelRestricted(ctx, *groupID, mapping.MappedModel)
	return mapping, restricted
}

// ReplaceModelInBody 替换请求体 JSON 中的 model 字段。
func ReplaceModelInBody(body []byte, newModel string) []byte {
	if len(body) == 0 {
		return body
	}
	if current := gjson.GetBytes(body, "model"); current.Exists() && current.String() == newModel {
		return body
	}
	newBody, err := sjson.SetBytes(body, "model", newModel)
	if err != nil {
		return body
	}
	return newBody
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
		Name:               input.Name,
		Description:        input.Description,
		Status:             StatusActive,
		BillingModelSource: input.BillingModelSource,
		RestrictModels:     input.RestrictModels,
		GroupIDs:            input.GroupIDs,
		ModelPricing:        input.ModelPricing,
		ModelMapping:        input.ModelMapping,
	}
	if channel.BillingModelSource == "" {
		channel.BillingModelSource = BillingModelSourceRequested
	}

	if err := validateNoDuplicateModels(channel.ModelPricing); err != nil {
		return nil, err
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

	if input.RestrictModels != nil {
		channel.RestrictModels = *input.RestrictModels
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

	if input.ModelMapping != nil {
		channel.ModelMapping = input.ModelMapping
	}

	if input.BillingModelSource != "" {
		channel.BillingModelSource = input.BillingModelSource
	}

	if err := validateNoDuplicateModels(channel.ModelPricing); err != nil {
		return nil, err
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

// validateNoDuplicateModels 检查定价列表中是否有重复模型（同一平台下不允许重复）
func validateNoDuplicateModels(pricingList []ChannelModelPricing) error {
	seen := make(map[string]bool)
	for _, p := range pricingList {
		for _, model := range p.Models {
			key := p.Platform + ":" + strings.ToLower(model)
			if seen[key] {
				return infraerrors.BadRequest("DUPLICATE_MODEL", fmt.Sprintf("model '%s' appears in multiple pricing entries for platform '%s'", model, p.Platform))
			}
			seen[key] = true
		}
	}
	return nil
}

// --- Input types ---

// CreateChannelInput 创建渠道输入
type CreateChannelInput struct {
	Name               string
	Description        string
	GroupIDs           []int64
	ModelPricing       []ChannelModelPricing
	ModelMapping       map[string]map[string]string // platform → {src→dst}
	BillingModelSource string
	RestrictModels     bool
}

// UpdateChannelInput 更新渠道输入
type UpdateChannelInput struct {
	Name               string
	Description        *string
	Status             string
	GroupIDs           *[]int64
	ModelPricing       *[]ChannelModelPricing
	ModelMapping       map[string]map[string]string // platform → {src→dst}
	BillingModelSource string
	RestrictModels     *bool
}
