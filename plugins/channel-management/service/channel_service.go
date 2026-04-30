package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/pagination"

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

type wildcardPricingEntry struct {
	prefix  string
	pricing *ChannelModelPricing
}

type wildcardMappingEntry struct {
	prefix string
	target string
}

// channelCache 渠道缓存快照（扁平化哈希结构，热路径 O(1) 查找）
type channelCache struct {
	pricingByGroupModel     map[channelModelKey]*ChannelModelPricing
	wildcardByGroupPlatform map[channelGroupPlatformKey][]*wildcardPricingEntry
	mappingByGroupModel     map[channelModelKey]string
	wildcardMappingByGP     map[channelGroupPlatformKey][]*wildcardMappingEntry
	channelByGroupID        map[int64]*Channel
	groupPlatform           map[int64]string

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

// BuildModelMappingChain renders a "a→b→c" string describing the model
// transformations applied. Returns "" when no mapping happened.
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

// ToUsageFields converts the mapping result into the fields downstream
// usage-recording code expects.
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

// ChannelService is the channel-management service exposed by the plugin.
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

// RebuildAllCacheNow loads all channels from the DB and pushes them to Redis.
// Called once during plugin Init so the gateway has a warm cache before
// serving traffic. Best-effort: failures are logged but do not block plugin
// startup.
func (s *ChannelService) RebuildAllCacheNow(ctx context.Context) {
	if s.cacheWriter == nil {
		return
	}
	channels, err := s.repo.ListAll(ctx)
	if err != nil {
		slog.Warn("channel cache: bootstrap list_all failed", "error", err)
		return
	}
	if err := s.cacheWriter.RebuildAllCache(ctx, channels); err != nil {
		slog.Warn("channel cache: bootstrap rebuild_all failed", "error", err)
	}
}

// rebuildCacheForChannel fetches the freshly-persisted channel from the repo
// and asks the writer to mirror it to Redis. Best-effort.
func (s *ChannelService) rebuildCacheForChannel(ctx context.Context, channelID int64) {
	if s.cacheWriter == nil || channelID == 0 {
		return
	}
	channel, err := s.repo.GetByID(ctx, channelID)
	if err != nil {
		slog.Warn("channel cache: reload after CRUD failed",
			"channel_id", channelID, "error", err)
		return
	}
	if channel == nil {
		return
	}
	if err := s.cacheWriter.RebuildCache(ctx, channel); err != nil {
		slog.Warn("channel cache: rebuild failed",
			"channel_id", channelID, "error", err)
	}
}

// invalidateCacheForChannel asks the writer to drop all Redis keys for the
// supplied channel snapshot. Caller should pass the pre-delete state so the
// writer can find every (group, platform) tuple to wipe.
func (s *ChannelService) invalidateCacheForChannel(ctx context.Context, channel *Channel) {
	if s.cacheWriter == nil || channel == nil {
		return
	}
	if err := s.cacheWriter.InvalidateCache(ctx, channel); err != nil {
		slog.Warn("channel cache: invalidate failed",
			"channel_id", channel.ID, "error", err)
	}
}

// loadCache returns the cached snapshot, rebuilding it when the TTL has
// expired. Concurrent rebuilds are coalesced by singleflight.
func (s *ChannelService) loadCache(ctx context.Context) (*channelCache, error) {
	if cached, ok := s.cache.Load().(*channelCache); ok && cached != nil {
		if time.Since(cached.loadedAt) < channelCacheTTL {
			return cached, nil
		}
	}

	result, err, _ := s.cacheSF.Do("channel_cache", func() (any, error) {
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

// newEmptyChannelCache returns a zero-state cache with all maps initialised.
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

// expandPricingToCache expands ch.ModelPricing into the lookup tables for
// the (gid, platform) bucket. Wildcard models go into the per-platform
// wildcard slice; exact names go into the flat hash.
func expandPricingToCache(cache *channelCache, ch *Channel, gid int64, platform string) {
	for j := range ch.ModelPricing {
		pricing := &ch.ModelPricing[j]
		if !isPlatformPricingMatch(platform, pricing.Platform) {
			continue
		}
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

// expandMappingToCache expands ch.ModelMapping into the lookup tables for
// the (gid, platform) bucket.
func expandMappingToCache(cache *channelCache, ch *Channel, gid int64, platform string) {
	for _, mappingPlatform := range matchingPlatforms(platform) {
		platformMapping, ok := ch.ModelMapping[mappingPlatform]
		if !ok {
			continue
		}
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

// storeErrorCache records an empty cache with a short remaining TTL so that a
// transient DB error does not stick around for the full channelCacheTTL.
func (s *ChannelService) storeErrorCache() {
	errorCache := newEmptyChannelCache()
	errorCache.loadedAt = time.Now().Add(-(channelCacheTTL - channelErrorTTL))
	s.cache.Store(errorCache)
}

// buildCache loads channels and group platforms from the DB and constructs a
// fresh snapshot. We use context.WithoutCancel so a cancelled request cannot
// poison the cache with empty data.
func (s *ChannelService) buildCache(ctx context.Context) (*channelCache, error) {
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), channelCacheDBTimeout)
	defer cancel()

	channels, groupPlatforms, err := s.fetchChannelData(dbCtx)
	if err != nil {
		return nil, err
	}

	cache := populateChannelCache(channels, groupPlatforms)
	s.cache.Store(cache)
	return cache, nil
}

func (s *ChannelService) fetchChannelData(ctx context.Context) ([]Channel, map[int64]string, error) {
	channels, err := s.repo.ListAll(ctx)
	if err != nil {
		slog.Warn("failed to build channel cache", "error", err)
		s.storeErrorCache()
		return nil, nil, fmt.Errorf("list all channels: %w", err)
	}

	var allGroupIDs []int64
	for i := range channels {
		allGroupIDs = append(allGroupIDs, channels[i].GroupIDs...)
	}

	groupPlatforms := make(map[int64]string)
	if len(allGroupIDs) > 0 {
		groupPlatforms, err = s.repo.GetGroupPlatforms(ctx, allGroupIDs)
		if err != nil {
			slog.Warn("failed to load group platforms for channel cache", "error", err)
			s.storeErrorCache()
			return nil, nil, fmt.Errorf("get group platforms: %w", err)
		}
	}
	return channels, groupPlatforms, nil
}

func populateChannelCache(channels []Channel, groupPlatforms map[int64]string) *channelCache {
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

	return cache
}

// isPlatformPricingMatch reports whether a pricing entry's platform matches a
// group's platform. Platforms are treated as strictly disjoint.
func isPlatformPricingMatch(groupPlatform, pricingPlatform string) bool {
	return groupPlatform == pricingPlatform
}

// matchingPlatforms returns the platforms whose pricing/mapping rows are
// considered for groupPlatform. Platforms are strictly disjoint, so this
// always returns the input platform alone.
func matchingPlatforms(groupPlatform string) []string {
	return []string{groupPlatform}
}

func (s *ChannelService) invalidateCache() {
	s.cache.Store((*channelCache)(nil))
	s.cacheSF.Forget("channel_cache")
	if _, err := s.buildCache(context.Background()); err != nil {
		slog.Warn("failed to rebuild channel cache after invalidation", "error", err)
	}
}

// matchWildcard scans the wildcard pricing entries for groupID/platform and
// returns the first prefix-matching pricing pointer.
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

func lookupPricingAcrossPlatforms(cache *channelCache, groupID int64, groupPlatform, modelLower string) *ChannelModelPricing {
	for _, p := range matchingPlatforms(groupPlatform) {
		key := channelModelKey{groupID: groupID, platform: p, model: modelLower}
		if pricing, ok := cache.pricingByGroupModel[key]; ok {
			return pricing
		}
	}
	for _, p := range matchingPlatforms(groupPlatform) {
		if pricing := cache.matchWildcard(groupID, p, modelLower); pricing != nil {
			return pricing
		}
	}
	return nil
}

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

// GetChannelForGroup returns a clone of the active channel attached to the
// group, or nil when none is configured.
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

type channelLookup struct {
	cache    *channelCache
	channel  *Channel
	platform string
}

// lookupGroupChannel is the shared hot-path setup: load the cache, locate the
// active channel, and return the snapshot + platform metadata for downstream
// pricing/mapping calls.
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

// GetChannelModelPricing returns a clone of the pricing row matching
// (groupID, model). Lookup is O(1) via the cache.
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

// ResolveChannelMapping resolves the channel-level model mapping for
// (groupID, model). Returns the original model wrapped in a default result
// when no channel/mapping exists.
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

// IsModelRestricted reports whether model is forbidden by the channel's
// restriction policy. Returns false when the channel does not enable
// restriction or the group has no associated channel.
func (s *ChannelService) IsModelRestricted(ctx context.Context, groupID int64, model string) bool {
	lk, err := s.lookupGroupChannel(ctx, groupID)
	if err != nil {
		slog.Warn("failed to load channel cache for model restriction check", "group_id", groupID, "error", err)
	}
	if lk == nil {
		return false
	}
	return checkRestricted(lk, groupID, model)
}

// ResolveChannelMappingAndRestrict mirrors ResolveChannelMapping but accepts a
// nullable groupID. Restriction checking has moved to the scheduler; the
// second return value is always false but kept for signature compatibility.
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

func checkRestricted(lk *channelLookup, groupID int64, model string) bool {
	if !lk.channel.RestrictModels {
		return false
	}
	modelLower := strings.ToLower(model)
	if lookupPricingAcrossPlatforms(lk.cache, groupID, lk.platform, modelLower) != nil {
		return false
	}
	return true
}

// ReplaceModelInBody rewrites the JSON body's "model" field. If the body is
// already on newModel or the JSON cannot be edited, body is returned
// unchanged.
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

// validateChannelConfig runs all the per-update validation rules in one place
// so Create and Update share the same checks.
func validateChannelConfig(pricing []ChannelModelPricing, mapping map[string]map[string]string) error {
	if err := validateNoConflictingModels(pricing); err != nil {
		return err
	}
	if err := validatePricingIntervals(pricing); err != nil {
		return err
	}
	if err := validateNoConflictingMappings(mapping); err != nil {
		return err
	}
	return validatePricingBillingMode(pricing)
}

func validatePricingBillingMode(pricing []ChannelModelPricing) error {
	for _, p := range pricing {
		if err := checkBillingModeRequirements(p); err != nil {
			return err
		}
		if err := checkPricesNotNegative(p); err != nil {
			return err
		}
		if err := checkIntervalsHavePrices(p); err != nil {
			return err
		}
	}
	return nil
}

func checkBillingModeRequirements(p ChannelModelPricing) error {
	if p.BillingMode == BillingModePerRequest || p.BillingMode == BillingModeImage {
		if p.PerRequestPrice == nil && len(p.Intervals) == 0 {
			return infraerrors.BadRequest(
				"BILLING_MODE_MISSING_PRICE",
				"per-request price or intervals required for per_request/image billing mode",
			)
		}
	}
	return nil
}

func checkPricesNotNegative(p ChannelModelPricing) error {
	checks := []struct {
		field string
		val   *float64
	}{
		{"input_price", p.InputPrice},
		{"output_price", p.OutputPrice},
		{"cache_write_price", p.CacheWritePrice},
		{"cache_read_price", p.CacheReadPrice},
		{"image_output_price", p.ImageOutputPrice},
		{"per_request_price", p.PerRequestPrice},
	}
	for _, c := range checks {
		if c.val != nil && *c.val < 0 {
			return infraerrors.BadRequest("NEGATIVE_PRICE", fmt.Sprintf("%s must be >= 0", c.field))
		}
	}
	return nil
}

func checkIntervalsHavePrices(p ChannelModelPricing) error {
	for _, iv := range p.Intervals {
		if iv.InputPrice == nil && iv.OutputPrice == nil &&
			iv.CacheWritePrice == nil && iv.CacheReadPrice == nil &&
			iv.PerRequestPrice == nil {
			return infraerrors.BadRequest(
				"INTERVAL_MISSING_PRICE",
				fmt.Sprintf("interval [%d, %s] has no price fields set for model %v",
					iv.MinTokens, formatMaxTokens(iv.MaxTokens), p.Models),
			)
		}
	}
	return nil
}

func formatMaxTokens(max *int) string {
	if max == nil {
		return "∞"
	}
	return fmt.Sprintf("%d", *max)
}

// --- CRUD ---

// Create persists a new channel. Returns ErrChannelExists when name collides
// or ErrGroupAlreadyInChannel when groups would belong to two channels.
func (s *ChannelService) Create(ctx context.Context, input *CreateChannelInput) (*Channel, error) {
	exists, err := s.repo.ExistsByName(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("check channel exists: %w", err)
	}
	if exists {
		return nil, ErrChannelExists
	}

	if err := s.checkGroupConflicts(ctx, 0, input.GroupIDs); err != nil {
		return nil, err
	}

	channel := &Channel{
		Name:                       input.Name,
		Description:                input.Description,
		Status:                     StatusActive,
		BillingModelSource:         input.BillingModelSource,
		RestrictModels:             input.RestrictModels,
		GroupIDs:                   input.GroupIDs,
		ModelPricing:               input.ModelPricing,
		ModelMapping:               input.ModelMapping,
		Features:                   input.Features,
		ApplyPricingToAccountStats: input.ApplyPricingToAccountStats,
		AccountStatsPricingRules:   input.AccountStatsPricingRules,
	}
	if channel.BillingModelSource == "" {
		channel.BillingModelSource = BillingModelSourceChannelMapped
	}

	if err := validateChannelConfig(channel.ModelPricing, channel.ModelMapping); err != nil {
		return nil, err
	}
	if err := validateAccountStatsPricingRules(channel.AccountStatsPricingRules); err != nil {
		return nil, err
	}

	if err := s.repo.Create(ctx, channel); err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}

	s.invalidateCache()
	s.rebuildCacheForChannel(ctx, channel.ID)
	// Reload the freshly-persisted channel so we publish the same shape
	// the host's PricingExtension cache will store after a future
	// ListPricingOverrides re-sync. Failure is logged but does not
	// block the CRUD path; the 5-minute re-sync still bridges the gap.
	created, err := s.repo.GetByID(ctx, channel.ID)
	if err != nil {
		return nil, err
	}
	s.publishUpsertEvent(ctx, created)
	return created, nil
}

// GetByID returns the channel with id loaded, or ErrChannelNotFound.
func (s *ChannelService) GetByID(ctx context.Context, id int64) (*Channel, error) {
	return s.repo.GetByID(ctx, id)
}

// Update applies the partial input to the channel and persists it.
func (s *ChannelService) Update(ctx context.Context, id int64, input *UpdateChannelInput) (*Channel, error) {
	channel, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}

	if err := s.applyUpdateInput(ctx, channel, input); err != nil {
		return nil, err
	}

	if err := validateChannelConfig(channel.ModelPricing, channel.ModelMapping); err != nil {
		return nil, err
	}
	if err := validateAccountStatsPricingRules(channel.AccountStatsPricingRules); err != nil {
		return nil, err
	}

	oldGroupIDs := s.getOldGroupIDs(ctx, id)
	preUpdateSnapshot := s.snapshotForCacheInvalidate(ctx, id, oldGroupIDs)
	preUpdatePricing := s.snapshotForEventPublish(ctx, id, oldGroupIDs)

	if err := s.repo.Update(ctx, channel); err != nil {
		return nil, fmt.Errorf("update channel: %w", err)
	}

	s.invalidateCache()
	s.invalidateAuthCacheForGroups(ctx, oldGroupIDs, channel.GroupIDs)

	// Wipe stale Redis keys for groups/platforms that the channel no longer
	// owns, then rebuild from the freshly persisted state.
	s.invalidateCacheForChannel(ctx, preUpdateSnapshot)
	s.rebuildCacheForChannel(ctx, id)

	updated, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// Conservative diff: emit DELETE for the pre-update tuple set and
	// UPSERT for the post-update active tuple set. The host's cache
	// applies them in arrival order so the end state is the union of
	// post-update entries, with any pre-update tuple no longer present
	// correctly evicted.
	s.publishDeleteEvent(ctx, preUpdatePricing)
	s.publishUpsertEvent(ctx, updated)
	return updated, nil
}

// snapshotForCacheInvalidate returns a minimal Channel describing the
// pre-mutation Redis footprint (old GroupIDs + platforms inferred from the
// existing pricing rows). Used to clean up stale keys before a Rebuild.
//
// Returns nil when there is no cache writer or the snapshot can't be loaded.
func (s *ChannelService) snapshotForCacheInvalidate(ctx context.Context, channelID int64, oldGroupIDs []int64) *Channel {
	if s.cacheWriter == nil {
		return nil
	}
	if len(oldGroupIDs) == 0 {
		return nil
	}
	existing, err := s.repo.GetByID(ctx, channelID)
	if err != nil || existing == nil {
		// Fall back to a stub carrying just the old group IDs so we can still
		// drop their meta keys.
		return &Channel{ID: channelID, GroupIDs: oldGroupIDs}
	}
	existing.GroupIDs = oldGroupIDs
	return existing
}

// snapshotForEventPublish loads the pre-mutation channel snapshot used
// to broadcast DELETE events for the (group, platform, model) tuples a
// channel previously owned. Unlike snapshotForCacheInvalidate, this
// path is independent of the cacheWriter — it only needs the
// eventPublisher to be wired.
//
// Returns nil when no broker is wired, the channel is unknown, or the
// pre-mutation group set is empty (no tuples to delete).
func (s *ChannelService) snapshotForEventPublish(ctx context.Context, channelID int64, oldGroupIDs []int64) *Channel {
	if s.eventPublisher == nil {
		return nil
	}
	existing, err := s.repo.GetByID(ctx, channelID)
	if err != nil || existing == nil {
		// Without a real snapshot we cannot enumerate the (platform, model)
		// tuples to delete. Fall back to a stub carrying just the group
		// IDs so the publisher can no-op cleanly; the host's 5-minute
		// re-sync covers the leftover state.
		if len(oldGroupIDs) == 0 {
			return nil
		}
		return &Channel{ID: channelID, GroupIDs: oldGroupIDs}
	}
	if len(oldGroupIDs) > 0 {
		existing.GroupIDs = oldGroupIDs
	}
	return existing
}

// publishUpsertEvent fans UPSERT events for channel out via the broker.
// Best-effort — missing channel, missing group→platform lookup, or a
// nil eventPublisher all degrade to a noop and rely on the host's
// 5-minute re-sync.
func (s *ChannelService) publishUpsertEvent(ctx context.Context, channel *Channel) {
	if s.eventPublisher == nil || channel == nil {
		return
	}
	groupPlatforms := s.resolveGroupPlatforms(ctx, channel.GroupIDs)
	s.eventPublisher.PublishChannelUpsert(channel, groupPlatforms)
}

// publishDeleteEvent broadcasts DELETE events for every
// (group, platform, model) tuple captured in channel — typically the
// pre-mutation snapshot loaded by snapshotForEventPublish. Same
// best-effort semantics as publishUpsertEvent.
func (s *ChannelService) publishDeleteEvent(ctx context.Context, channel *Channel) {
	if s.eventPublisher == nil || channel == nil {
		return
	}
	groupPlatforms := s.resolveGroupPlatforms(ctx, channel.GroupIDs)
	s.eventPublisher.PublishChannelDelete(channel, groupPlatforms)
}

// resolveGroupPlatforms wraps repo.GetGroupPlatforms with the
// best-effort logging the publish helpers expect. Returns an empty map
// on failure so the caller can still iterate without nil-checking.
func (s *ChannelService) resolveGroupPlatforms(ctx context.Context, groupIDs []int64) map[int64]string {
	if len(groupIDs) == 0 {
		return map[int64]string{}
	}
	platforms, err := s.repo.GetGroupPlatforms(ctx, groupIDs)
	if err != nil {
		slog.Warn("channel events: group platform lookup failed",
			"group_ids", groupIDs, "error", err)
		return map[int64]string{}
	}
	return platforms
}

func (s *ChannelService) applyUpdateInput(ctx context.Context, channel *Channel, input *UpdateChannelInput) error {
	if input.Name != "" && input.Name != channel.Name {
		exists, err := s.repo.ExistsByNameExcluding(ctx, input.Name, channel.ID)
		if err != nil {
			return fmt.Errorf("check channel exists: %w", err)
		}
		if exists {
			return ErrChannelExists
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
	if input.Features != nil {
		channel.Features = *input.Features
	}
	if input.GroupIDs != nil {
		if err := s.checkGroupConflicts(ctx, channel.ID, *input.GroupIDs); err != nil {
			return err
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
	if input.ApplyPricingToAccountStats != nil {
		channel.ApplyPricingToAccountStats = *input.ApplyPricingToAccountStats
	}
	if input.AccountStatsPricingRules != nil {
		channel.AccountStatsPricingRules = *input.AccountStatsPricingRules
	}
	return nil
}

func (s *ChannelService) checkGroupConflicts(ctx context.Context, channelID int64, groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return nil
	}
	conflicting, err := s.repo.GetGroupsInOtherChannels(ctx, channelID, groupIDs)
	if err != nil {
		return fmt.Errorf("check group conflicts: %w", err)
	}
	if len(conflicting) > 0 {
		return ErrGroupAlreadyInChannel
	}
	return nil
}

func (s *ChannelService) getOldGroupIDs(ctx context.Context, channelID int64) []int64 {
	// Fetch when any downstream consumer needs the pre-mutation set:
	// auth invalidation, the Redis cache writer (drops keys whose
	// (group, platform) is no longer owned), or the in-process pricing
	// event publisher (DELETE events for evicted tuples).
	if s.authCacheInvalidator == nil && s.cacheWriter == nil && s.eventPublisher == nil {
		return nil
	}
	oldGroupIDs, err := s.repo.GetGroupIDs(ctx, channelID)
	if err != nil {
		slog.Warn("failed to get old group IDs for cache invalidation", "channel_id", channelID, "error", err)
	}
	return oldGroupIDs
}

func (s *ChannelService) invalidateAuthCacheForGroups(ctx context.Context, groupIDSets ...[]int64) {
	if s.authCacheInvalidator == nil {
		return
	}
	seen := make(map[int64]struct{})
	for _, ids := range groupIDSets {
		for _, gid := range ids {
			if _, ok := seen[gid]; ok {
				continue
			}
			seen[gid] = struct{}{}
			s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, gid)
		}
	}
}

// Delete removes the channel and invalidates caches for its previously
// associated groups.
func (s *ChannelService) Delete(ctx context.Context, id int64) error {
	groupIDs, err := s.repo.GetGroupIDs(ctx, id)
	if err != nil {
		slog.Warn("failed to get group IDs before delete", "channel_id", id, "error", err)
	}

	// Snapshot the channel before deletion so the cache writer knows which
	// (group, platform) keys to wipe. Best-effort — if the lookup fails we
	// still attempt invalidation with a stub carrying just the group IDs.
	preDeleteSnapshot := s.snapshotForCacheInvalidate(ctx, id, groupIDs)
	// Capture the pre-delete pricing snapshot for event broadcast. Same
	// best-effort semantics — missing data degrades to a noop emit, and
	// the host's 5-minute re-sync still bridges the gap.
	preDeletePricing := s.snapshotForEventPublish(ctx, id, groupIDs)

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}

	s.invalidateCache()
	s.invalidateAuthCacheForGroups(ctx, groupIDs)
	s.invalidateCacheForChannel(ctx, preDeleteSnapshot)
	s.publishDeleteEvent(ctx, preDeletePricing)
	return nil
}

// List returns a paginated channel list.
func (s *ChannelService) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Channel, *pagination.PaginationResult, error) {
	return s.repo.List(ctx, params, status, search)
}

// modelEntry is one model pattern used by the conflict detector.
type modelEntry struct {
	pattern  string
	prefix   string
	wildcard bool
}

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

func toModelEntry(pattern string) modelEntry {
	lower := strings.ToLower(pattern)
	isWild := strings.HasSuffix(lower, "*")
	prefix := lower
	if isWild {
		prefix = strings.TrimSuffix(lower, "*")
	}
	return modelEntry{pattern: pattern, prefix: prefix, wildcard: isWild}
}

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

// validateAccountStatsPricingRules enforces the same per-row checks as
// validatePricingBillingMode (negative prices, billing-mode requirements,
// interval price coverage) on every Pricing entry inside every rule.
// Mirrors the host's `validatePricingEntries` helper introduced by
// release commit 9c09bd19b.
func validateAccountStatsPricingRules(rules []AccountStatsPricingRule) error {
	for i := range rules {
		if err := validatePricingBillingMode(rules[i].Pricing); err != nil {
			return err
		}
	}
	return nil
}

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

// CreateChannelInput captures the fields accepted by Create.
type CreateChannelInput struct {
	Name                       string
	Description                string
	GroupIDs                   []int64
	ModelPricing               []ChannelModelPricing
	ModelMapping               map[string]map[string]string
	BillingModelSource         string
	RestrictModels             bool
	Features                   string
	ApplyPricingToAccountStats bool
	AccountStatsPricingRules   []AccountStatsPricingRule
}

// UpdateChannelInput captures the fields accepted by Update; pointer fields
// distinguish "unset" from "set to zero".
type UpdateChannelInput struct {
	Name                       string
	Description                *string
	Status                     string
	GroupIDs                   *[]int64
	ModelPricing               *[]ChannelModelPricing
	ModelMapping               map[string]map[string]string
	BillingModelSource         string
	RestrictModels             *bool
	Features                   *string
	ApplyPricingToAccountStats *bool
	// AccountStatsPricingRules: nil = "no change"; non-nil (including
	// empty slice) = "replace with this list". Same semantics as
	// ModelPricing above.
	AccountStatsPricingRules *[]AccountStatsPricingRule
}
