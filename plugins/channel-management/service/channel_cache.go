package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/platforms"
	"github.com/Wei-Shaw/sub2api/plugins/channel-management/internal/wildcard"
)

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

const (
	channelCacheTTL       = 10 * time.Minute
	channelErrorTTL       = 5 * time.Second // DB 错误时的短缓存
	channelCacheDBTimeout = 10 * time.Second
)

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
//
// Cache keys store the platform 经过 platforms.Normalize 规范化后的形式，
// 与 lookup 路径（lookupPricingAcrossPlatforms / matchingPlatforms）保持一致；
// 否则 isPlatformPricingMatch 用 normalized 比较通过、但 cache key 是原始大小写，
// 会造成"过滤通过但取不到值"的隐性 miss。
func expandPricingToCache(cache *channelCache, ch *Channel, gid int64, platform string) {
	for j := range ch.ModelPricing {
		pricing := &ch.ModelPricing[j]
		if !isPlatformPricingMatch(platform, pricing.Platform) {
			continue
		}
		pricingPlatform := platforms.Normalize(pricing.Platform)
		gpKey := channelGroupPlatformKey{groupID: gid, platform: pricingPlatform}
		for _, model := range pricing.Models {
			if rawPrefix, isWild := wildcard.SplitSuffix(model); isWild {
				prefix := strings.ToLower(rawPrefix)
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
//
// ch.ModelMapping 的 key 是渠道行写入时的原始 platform 字符串（DB JSON 字段，
// 大小写未规范化），所以这里要先用 normalized 比对找到对应 bucket，写入 cache
// 时则统一用 normalized key（与 expandPricingToCache / lookupMappingAcrossPlatforms
// 保持一致）。
func expandMappingToCache(cache *channelCache, ch *Channel, gid int64, platform string) {
	normPlatform := platforms.Normalize(platform)
	gpKey := channelGroupPlatformKey{groupID: gid, platform: normPlatform}
	for mappingPlatform, platformMapping := range ch.ModelMapping {
		if !platforms.Match(mappingPlatform, platform) {
			continue
		}
		for src, dst := range platformMapping {
			if rawPrefix, isWild := wildcard.SplitSuffix(src); isWild {
				prefix := strings.ToLower(rawPrefix)
				cache.wildcardMappingByGP[gpKey] = append(cache.wildcardMappingByGP[gpKey], &wildcardMappingEntry{
					prefix: prefix,
					target: dst,
				})
			} else {
				key := channelModelKey{groupID: gid, platform: normPlatform, model: strings.ToLower(src)}
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
// group's platform. Delegates to platforms.Match (trim + lowercase + strict
// equality) so the channel-management plugin uses one consistent rule on
// every read path.
func isPlatformPricingMatch(groupPlatform, pricingPlatform string) bool {
	return platforms.Match(groupPlatform, pricingPlatform)
}

// matchingPlatforms returns the platforms whose pricing/mapping rows are
// considered for groupPlatform. Platforms are strictly disjoint, so this
// always returns the input platform alone (after normalising it so cache
// lookups hit the keys that expandPricingToCache wrote).
func matchingPlatforms(groupPlatform string) []string {
	return []string{platforms.Normalize(groupPlatform)}
}

// invalidateCache drops the in-memory snapshot and synchronously rebuilds it
// in the background. Used by the CRUD path after a successful DB write so the
// next read returns up-to-date data.
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
