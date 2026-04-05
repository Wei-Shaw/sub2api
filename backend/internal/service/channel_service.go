package service

import (
	"context"
	"fmt"
	"log/slog"
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
	ErrChannelNotFound       = infraerrors.NotFound("CHANNEL_NOT_FOUND", "channel not found")
	ErrChannelExists         = infraerrors.Conflict("CHANNEL_EXISTS", "channel name already exists")
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

// wildcardMappingEntry 通配符映射条目
type wildcardMappingEntry struct {
	prefix string
	target string
}

// channelCache 渠道缓存快照（扁平化哈希结构，热路径 O(1) 查找）
type channelCache struct {
	// 热路径查找
	pricingByGroupModel     map[channelModelKey]*ChannelModelPricing            // (groupID, platform, model) → 定价
	wildcardByGroupPlatform map[channelGroupPlatformKey][]*wildcardPricingEntry // (groupID, platform) → 通配符定价（前缀长度降序）
	mappingByGroupModel     map[channelModelKey]string                          // (groupID, platform, model) → 映射目标
	wildcardMappingByGP     map[channelGroupPlatformKey][]*wildcardMappingEntry // (groupID, platform) → 通配符映射（前缀长度降序）
	channelByGroupID        map[int64]*Channel                                  // groupID → 渠道
	groupPlatform           map[int64]string                                    // groupID → platform

	// 冷路径（CRUD 操作）
	byID     map[int64]*Channel
	loadedAt time.Time
}

// ChannelMappingResult 渠道映射查找结果
type ChannelMappingResult struct {
	MappedModel        string // 映射后的模型名（无映射时等于原始模型名）
	ChannelID          int64  // 渠道 ID（0 = 无渠道关联）
	Mapped             bool   // 是否发生了映射
	BillingModelSource string // 计费模型来源（"requested" / "upstream" / "channel_mapped"）
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

// ToUsageFields 将渠道映射结果转为使用记录字段
func (r ChannelMappingResult) ToUsageFields(reqModel, upstreamModel string) ChannelUsageFields {
	channelMappedModel := reqModel
	if r.Mapped {
		channelMappedModel = r.MappedModel
	}
	return ChannelUsageFields{
		ChannelID:          r.ChannelID,
		OriginalModel:      reqModel,
		ChannelMappedModel: channelMappedModel,
		BillingModelSource: r.BillingModelSource,
		ModelMappingChain:  r.BuildModelMappingChain(reqModel, upstreamModel),
	}
}

const (
	channelCacheTTL       = 10 * time.Minute
	channelErrorTTL       = 5 * time.Second // DB 错误时的短缓存
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
	if cached, ok := s.cache.Load().(*channelCache); ok && cached != nil {
		if time.Since(cached.loadedAt) < channelCacheTTL {
			return cached, nil
		}
	}

	result, err, _ := s.cacheSF.Do("channel_cache", func() (any, error) {
		// 双重检查
		if cached, ok := s.cache.Load().(*channelCache); ok && cached != nil {
			if time.Since(cached.loadedAt) < channelCacheTTL {
				return cached, nil
			}
		}
		return s.buildCache(ctx)
	})
	if err != nil {
		return nil, err
	}
	cache, ok := result.(*channelCache)
	if !ok {
		return nil, fmt.Errorf("unexpected cache type")
	}
	return cache, nil
}

// newEmptyChannelCache 创建空的渠道缓存（所有 map 已初始化）
func newEmptyChannelCache() *channelCache {
	return &channelCache{
		pricingByGroupModel:     make(map[channelModelKey]*ChannelModelPricing),
		wildcardByGroupPlatform: make(map[channelGroupPlatformKey][]*wildcardPricingEntry),
		mappingByGroupModel:     make(map[channelModelKey]string),
		wildcardMappingByGP:     make(map[channelGroupPlatformKey][]*wildcardMappingEntry),
		channelByGroupID:        make(map[int64]*Channel),
		groupPlatform:           make(map[int64]string),
		byID:                    make(map[int64]*Channel),
	}
}

// expandPricingToCache 将渠道的模型定价展开到缓存（按分组+平台维度）。
// 各平台严格独立：antigravity 分组只匹配 antigravity 定价，不会匹配 anthropic/gemini 的定价。
// 查找时通过 lookupPricingAcrossPlatforms() 在本平台内查找。
func expandPricingToCache(cache *channelCache, ch *Channel, gid int64, platform string) {
	for j := range ch.ModelPricing {
		pricing := &ch.ModelPricing[j]
		if !isPlatformPricingMatch(platform, pricing.Platform) {
			continue // 跳过非本平台的定价
		}
		// 使用定价条目的原始平台作为缓存 key，防止跨平台同名模型冲突
		pricingPlatform := pricing.Platform
		gpKey := channelGroupPlatformKey{groupID: gid, platform: pricingPlatform}
		for _, model := range pricing.Models {
			if strings.HasSuffix(model, "*") {
				prefix := strings.ToLower(strings.TrimSuffix(model, "*"))
				cache.wildcardByGroupPlatform[gpKey] = append(cache.wildcardByGroupPlatform[gpKey], &wildcardPricingEntry{
					prefix:  prefix,
					pricing: pricing,
				})
			} else {
				key := channelModelKey{groupID: gid, platform: pricingPlatform, model: strings.ToLower(model)}
				cache.pricingByGroupModel[key] = pricing
			}
		}
	}
}

// expandMappingToCache 将渠道的模型映射展开到缓存（按分组+平台维度）。
// 各平台严格独立：antigravity 分组只匹配 antigravity 映射。
func expandMappingToCache(cache *channelCache, ch *Channel, gid int64, platform string) {
	for _, mappingPlatform := range matchingPlatforms(platform) {
		platformMapping, ok := ch.ModelMapping[mappingPlatform]
		if !ok {
			continue
		}
		// 使用映射条目的原始平台作为缓存 key，防止跨平台同名映射冲突
		gpKey := channelGroupPlatformKey{groupID: gid, platform: mappingPlatform}
		for src, dst := range platformMapping {
			if strings.HasSuffix(src, "*") {
				prefix := strings.ToLower(strings.TrimSuffix(src, "*"))
				cache.wildcardMappingByGP[gpKey] = append(cache.wildcardMappingByGP[gpKey], &wildcardMappingEntry{
					prefix: prefix,
					target: dst,
				})
			} else {
				key := channelModelKey{groupID: gid, platform: mappingPlatform, model: strings.ToLower(src)}
				cache.mappingByGroupModel[key] = dst
			}
		}
	}
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
		errorCache := newEmptyChannelCache()
		errorCache.loadedAt = time.Now().Add(-(channelCacheTTL - channelErrorTTL)) // 使剩余 TTL = errorTTL
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
			errorCache := newEmptyChannelCache()
			errorCache.loadedAt = time.Now().Add(-(channelCacheTTL - channelErrorTTL))
			s.cache.Store(errorCache)
			return nil, fmt.Errorf("get group platforms: %w", err)
		}
	}

	cache := newEmptyChannelCache()
	cache.groupPlatform = groupPlatforms
	cache.byID = make(map[int64]*Channel, len(channels))
	cache.loadedAt = time.Now()

	for i := range channels {
		ch := &channels[i]
		cache.byID[ch.ID] = ch

		for _, gid := range ch.GroupIDs {
			cache.channelByGroupID[gid] = ch
			platform := groupPlatforms[gid]
			expandPricingToCache(cache, ch, gid, platform)
			expandMappingToCache(cache, ch, gid, platform)
		}
	}

	// 通配符条目保持配置顺序（最先匹配到优先）

	s.cache.Store(cache)
	return cache, nil
}

// invalidateCache 使缓存失效，让下次读取时自然重建

// isPlatformPricingMatch 判断定价条目的平台是否匹配分组平台。
// 各平台（antigravity / anthropic / gemini / openai）严格独立，不跨平台匹配。
func isPlatformPricingMatch(groupPlatform, pricingPlatform string) bool {
	return groupPlatform == pricingPlatform
}

// matchingPlatforms 返回分组平台对应的可匹配平台列表。
// 各平台严格独立，只返回自身。
func matchingPlatforms(groupPlatform string) []string {
	return []string{groupPlatform}
}
func (s *ChannelService) invalidateCache() {
	s.cache.Store((*channelCache)(nil))
	s.cacheSF.Forget("channel_cache")

	// 主动重建缓存，确保 CRUD 后立即生效
	if _, err := s.buildCache(context.Background()); err != nil {
		slog.Warn("failed to rebuild channel cache after invalidation", "error", err)
	}
}

// matchWildcard 在通配符定价中查找匹配项（最先匹配到优先）
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

// matchWildcardMapping 在通配符映射中查找匹配项（最先匹配到优先）
func (c *channelCache) matchWildcardMapping(groupID int64, platform, modelLower string) string {
	gpKey := channelGroupPlatformKey{groupID: groupID, platform: platform}
	wildcards := c.wildcardMappingByGP[gpKey]
	for _, wc := range wildcards {
		if strings.HasPrefix(modelLower, wc.prefix) {
			return wc.target
		}
	}
	return ""
}

// lookupPricingAcrossPlatforms 在分组平台内查找模型定价。
// 各平台严格独立，只在本平台内查找（先精确匹配，再通配符）。
func lookupPricingAcrossPlatforms(cache *channelCache, groupID int64, groupPlatform, modelLower string) *ChannelModelPricing {
	for _, p := range matchingPlatforms(groupPlatform) {
		key := channelModelKey{groupID: groupID, platform: p, model: modelLower}
		if pricing, ok := cache.pricingByGroupModel[key]; ok {
			return pricing
		}
	}
	// 精确查找全部失败，依次尝试通配符匹配
	for _, p := range matchingPlatforms(groupPlatform) {
		if pricing := cache.matchWildcard(groupID, p, modelLower); pricing != nil {
			return pricing
		}
	}
	return nil
}

// lookupMappingAcrossPlatforms 在分组平台内查找模型映射。
// 逻辑与 lookupPricingAcrossPlatforms 相同：先精确查找，再通配符。
func lookupMappingAcrossPlatforms(cache *channelCache, groupID int64, groupPlatform, modelLower string) string {
	for _, p := range matchingPlatforms(groupPlatform) {
		key := channelModelKey{groupID: groupID, platform: p, model: modelLower}
		if mapped, ok := cache.mappingByGroupModel[key]; ok {
			return mapped
		}
	}
	for _, p := range matchingPlatforms(groupPlatform) {
		if mapped := cache.matchWildcardMapping(groupID, p, modelLower); mapped != "" {
			return mapped
		}
	}
	return ""
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

// channelLookup 热路径公共查找结果
type channelLookup struct {
	cache    *channelCache
	channel  *Channel
	platform string
}

// lookupGroupChannel 加载缓存并查找分组对应的渠道信息（公共热路径前置逻辑）。
// 返回 nil 且 err==nil 表示分组无活跃渠道；err!=nil 表示缓存加载失败。
func (s *ChannelService) lookupGroupChannel(ctx context.Context, groupID int64) (*channelLookup, error) {
	cache, err := s.loadCache(ctx)
	if err != nil {
		return nil, err
	}
	ch, ok := cache.channelByGroupID[groupID]
	if !ok || !ch.IsActive() {
		return nil, nil
	}
	return &channelLookup{
		cache:    cache,
		channel:  ch,
		platform: cache.groupPlatform[groupID],
	}, nil
}

// GetChannelModelPricing 获取指定分组+模型的渠道定价（热路径 O(1)）。
// 各平台严格独立，只在本平台内查找定价。
func (s *ChannelService) GetChannelModelPricing(ctx context.Context, groupID int64, model string) *ChannelModelPricing {
	lk, err := s.lookupGroupChannel(ctx, groupID)
	if err != nil {
		slog.Warn("failed to load channel cache", "group_id", groupID, "error", err)
		return nil
	}
	if lk == nil {
		return nil
	}

	modelLower := strings.ToLower(model)
	pricing := lookupPricingAcrossPlatforms(lk.cache, groupID, lk.platform, modelLower)
	if pricing == nil {
		return nil
	}

	cp := pricing.Clone()
	return &cp
}

// ResolveChannelMapping 解析渠道级模型映射（热路径 O(1)）
// 返回映射结果，包含映射后的模型名、渠道 ID、计费模型来源。
func (s *ChannelService) ResolveChannelMapping(ctx context.Context, groupID int64, model string) ChannelMappingResult {
	lk, err := s.lookupGroupChannel(ctx, groupID)
	if err != nil {
		slog.Warn("failed to load channel cache for mapping", "group_id", groupID, "error", err)
	}
	if lk == nil {
		return ChannelMappingResult{MappedModel: model}
	}
	return resolveMapping(lk, groupID, model)
}

// IsModelRestricted 检查模型是否被渠道限制。
// 返回 true 表示模型被限制（不在允许列表中）。
// 如果渠道未启用模型限制或分组无渠道关联，返回 false。
func (s *ChannelService) IsModelRestricted(ctx context.Context, groupID int64, model string) bool {
	lk, _ := s.lookupGroupChannel(ctx, groupID)
	if lk == nil {
		return false
	}
	return checkRestricted(lk, groupID, model)
}

// ResolveChannelMappingAndRestrict 解析渠道映射。
// 返回映射结果。模型限制检查已移至调度阶段（GatewayService.checkChannelPricingRestriction），
// restricted 始终返回 false，保留签名兼容性。
func (s *ChannelService) ResolveChannelMappingAndRestrict(ctx context.Context, groupID *int64, model string) (ChannelMappingResult, bool) {
	if groupID == nil {
		return ChannelMappingResult{MappedModel: model}, false
	}
	lk, _ := s.lookupGroupChannel(ctx, *groupID)
	if lk == nil {
		return ChannelMappingResult{MappedModel: model}, false
	}
	return resolveMapping(lk, *groupID, model), false
}

// resolveMapping 基于已查找的渠道信息解析模型映射。
// antigravity 分组依次尝试所有匹配平台，确保跨平台同名映射各自独立。
func resolveMapping(lk *channelLookup, groupID int64, model string) ChannelMappingResult {
	result := ChannelMappingResult{
		MappedModel:        model,
		ChannelID:          lk.channel.ID,
		BillingModelSource: lk.channel.BillingModelSource,
	}
	if result.BillingModelSource == "" {
		result.BillingModelSource = BillingModelSourceChannelMapped
	}

	modelLower := strings.ToLower(model)
	if mapped := lookupMappingAcrossPlatforms(lk.cache, groupID, lk.platform, modelLower); mapped != "" {
		result.MappedModel = mapped
		result.Mapped = true
	}

	return result
}

// checkRestricted 基于已查找的渠道信息检查模型是否被限制。
// 只在本平台的定价列表中查找。
func checkRestricted(lk *channelLookup, groupID int64, model string) bool {
	if !lk.channel.RestrictModels {
		return false
	}
	modelLower := strings.ToLower(model)
	// 使用与查找定价相同的跨平台逻辑
	if lookupPricingAcrossPlatforms(lk.cache, groupID, lk.platform, modelLower) != nil {
		return false
	}
	return true
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
		GroupIDs:           input.GroupIDs,
		ModelPricing:       input.ModelPricing,
		ModelMapping:       input.ModelMapping,
	}
	if channel.BillingModelSource == "" {
		channel.BillingModelSource = BillingModelSourceChannelMapped
	}

	if err := validateNoConflictingModels(channel.ModelPricing); err != nil {
		return nil, err
	}
	if err := validatePricingIntervals(channel.ModelPricing); err != nil {
		return nil, err
	}
	if err := validateNoConflictingMappings(channel.ModelMapping); err != nil {
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

	if err := validateNoConflictingModels(channel.ModelPricing); err != nil {
		return nil, err
	}
	if err := validatePricingIntervals(channel.ModelPricing); err != nil {
		return nil, err
	}
	if err := validateNoConflictingMappings(channel.ModelMapping); err != nil {
		return nil, err
	}

	// 先获取旧分组，Update 后旧分组关联已删除，无法再查到
	var oldGroupIDs []int64
	if s.authCacheInvalidator != nil {
		var err2 error
		oldGroupIDs, err2 = s.repo.GetGroupIDs(ctx, id)
		if err2 != nil {
			slog.Warn("failed to get old group IDs for cache invalidation", "channel_id", id, "error", err2)
		}
	}

	if err := s.repo.Update(ctx, channel); err != nil {
		return nil, fmt.Errorf("update channel: %w", err)
	}

	s.invalidateCache()

	// 失效新旧分组的 auth 缓存
	if s.authCacheInvalidator != nil {
		seen := make(map[int64]struct{}, len(oldGroupIDs)+len(channel.GroupIDs))
		for _, gid := range oldGroupIDs {
			if _, ok := seen[gid]; !ok {
				seen[gid] = struct{}{}
				s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, gid)
			}
		}
		for _, gid := range channel.GroupIDs {
			if _, ok := seen[gid]; !ok {
				seen[gid] = struct{}{}
				s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, gid)
			}
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

// modelEntry 表示一个模型模式条目（用于冲突检测）
type modelEntry struct {
	pattern  string // 原始模式（如 "claude-*" 或 "claude-opus-4"）
	prefix   string // lowercase 前缀（通配符去掉 *，精确名保持原样）
	wildcard bool
}

// conflictsBetween 检查两个模型模式是否冲突
func conflictsBetween(a, b modelEntry) bool {
	switch {
	case !a.wildcard && !b.wildcard:
		return a.prefix == b.prefix
	case a.wildcard && !b.wildcard:
		return strings.HasPrefix(b.prefix, a.prefix)
	case !a.wildcard && b.wildcard:
		return strings.HasPrefix(a.prefix, b.prefix)
	default:
		return strings.HasPrefix(a.prefix, b.prefix) ||
			strings.HasPrefix(b.prefix, a.prefix)
	}
}

// toModelEntry 将模型名转换为 modelEntry
func toModelEntry(pattern string) modelEntry {
	lower := strings.ToLower(pattern)
	isWild := strings.HasSuffix(lower, "*")
	prefix := lower
	if isWild {
		prefix = strings.TrimSuffix(lower, "*")
	}
	return modelEntry{pattern: pattern, prefix: prefix, wildcard: isWild}
}

// validateNoConflictingModels 检查定价列表中是否有冲突模型模式（同一平台下）。
// 冲突包括：精确重复、通配符之间的前缀包含、通配符与精确名的前缀匹配。
func validateNoConflictingModels(pricingList []ChannelModelPricing) error {
	byPlatform := make(map[string][]modelEntry)
	for _, p := range pricingList {
		for _, model := range p.Models {
			byPlatform[p.Platform] = append(byPlatform[p.Platform], toModelEntry(model))
		}
	}
	for platform, entries := range byPlatform {
		if err := detectConflicts(entries, platform, "MODEL_PATTERN_CONFLICT", "model patterns"); err != nil {
			return err
		}
	}
	return nil
}

// validateNoConflictingMappings 检查模型映射中是否有冲突的源模式
func validateNoConflictingMappings(mapping map[string]map[string]string) error {
	for platform, platformMapping := range mapping {
		entries := make([]modelEntry, 0, len(platformMapping))
		for src := range platformMapping {
			entries = append(entries, toModelEntry(src))
		}
		if err := detectConflicts(entries, platform, "MAPPING_PATTERN_CONFLICT", "mapping source patterns"); err != nil {
			return err
		}
	}
	return nil
}

func validatePricingIntervals(pricingList []ChannelModelPricing) error {
	for _, pricing := range pricingList {
		if err := ValidateIntervals(pricing.Intervals); err != nil {
			return infraerrors.BadRequest(
				"INVALID_PRICING_INTERVALS",
				fmt.Sprintf("invalid pricing intervals for platform '%s' models %v: %v",
					pricing.Platform, pricing.Models, err),
			)
		}
	}
	return nil
}

// detectConflicts 在一组 modelEntry 中检测冲突，返回带有 errCode 和 label 的错误
func detectConflicts(entries []modelEntry, platform, errCode, label string) error {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if conflictsBetween(entries[i], entries[j]) {
				return infraerrors.BadRequest(errCode,
					fmt.Sprintf("%s '%s' and '%s' conflict in platform '%s': overlapping match range",
						label, entries[i].pattern, entries[j].pattern, platform))
			}
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
