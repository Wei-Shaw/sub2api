package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/dgraph-io/ristretto"
)

const apiKeyAuthSnapshotVersion = 14 // v14: include multi-group API key bindings

type apiKeyAuthCacheConfig struct {
	l1Size        int
	l1TTL         time.Duration
	l2TTL         time.Duration
	negativeTTL   time.Duration
	jitterPercent int
	singleflight  bool
}

func newAPIKeyAuthCacheConfig(cfg *config.Config) apiKeyAuthCacheConfig {
	if cfg == nil {
		return apiKeyAuthCacheConfig{}
	}
	auth := cfg.APIKeyAuth
	return apiKeyAuthCacheConfig{
		l1Size:        auth.L1Size,
		l1TTL:         time.Duration(auth.L1TTLSeconds) * time.Second,
		l2TTL:         time.Duration(auth.L2TTLSeconds) * time.Second,
		negativeTTL:   time.Duration(auth.NegativeTTLSeconds) * time.Second,
		jitterPercent: auth.JitterPercent,
		singleflight:  auth.Singleflight,
	}
}

func (c apiKeyAuthCacheConfig) l1Enabled() bool {
	return c.l1Size > 0 && c.l1TTL > 0
}

func (c apiKeyAuthCacheConfig) l2Enabled() bool {
	return c.l2TTL > 0
}

func (c apiKeyAuthCacheConfig) negativeEnabled() bool {
	return c.negativeTTL > 0
}

// jitterTTL 为缓存 TTL 添加抖动，避免多个请求在同一时刻同时过期触发集中回源。
// 这里直接使用 rand/v2 的顶层函数：并发安全，无需全局互斥锁。
func (c apiKeyAuthCacheConfig) jitterTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return ttl
	}
	if c.jitterPercent <= 0 {
		return ttl
	}
	percent := c.jitterPercent
	if percent > 100 {
		percent = 100
	}
	delta := float64(percent) / 100
	randVal := rand.Float64()
	factor := 1 - delta + randVal*(2*delta)
	if factor <= 0 {
		return ttl
	}
	return time.Duration(float64(ttl) * factor)
}

func (s *APIKeyService) initAuthCache(cfg *config.Config) {
	s.authCfg = newAPIKeyAuthCacheConfig(cfg)
	if !s.authCfg.l1Enabled() {
		return
	}
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: int64(s.authCfg.l1Size) * 10,
		MaxCost:     int64(s.authCfg.l1Size),
		BufferItems: 64,
	})
	if err != nil {
		return
	}
	s.authCacheL1 = cache
}

// StartAuthCacheInvalidationSubscriber starts the Pub/Sub subscriber for L1 cache invalidation.
// This should be called after the service is fully initialized.
func (s *APIKeyService) StartAuthCacheInvalidationSubscriber(ctx context.Context) {
	if s.cache == nil || s.authCacheL1 == nil {
		return
	}
	if err := s.cache.SubscribeAuthCacheInvalidation(ctx, func(cacheKey string) {
		s.authCacheL1.Del(cacheKey)
	}); err != nil {
		// Log but don't fail - L1 cache will still work, just without cross-instance invalidation
		slog.Warn("failed to start auth cache invalidation subscriber", "error", err)
	}
}

func (s *APIKeyService) authCacheKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func (s *APIKeyService) getAuthCacheEntry(ctx context.Context, cacheKey string) (*APIKeyAuthCacheEntry, bool) {
	if s.authCacheL1 != nil {
		if val, ok := s.authCacheL1.Get(cacheKey); ok {
			if entry, ok := val.(*APIKeyAuthCacheEntry); ok {
				return entry, true
			}
		}
	}
	if s.cache == nil || !s.authCfg.l2Enabled() {
		return nil, false
	}
	entry, err := s.cache.GetAuthCache(ctx, cacheKey)
	if err != nil {
		return nil, false
	}
	s.setAuthCacheL1(cacheKey, entry)
	return entry, true
}

func (s *APIKeyService) setAuthCacheL1(cacheKey string, entry *APIKeyAuthCacheEntry) {
	if s.authCacheL1 == nil || entry == nil {
		return
	}
	ttl := s.authCfg.l1TTL
	if entry.NotFound && s.authCfg.negativeTTL > 0 && s.authCfg.negativeTTL < ttl {
		ttl = s.authCfg.negativeTTL
	}
	ttl = s.authCfg.jitterTTL(ttl)
	_ = s.authCacheL1.SetWithTTL(cacheKey, entry, 1, ttl)
}

func (s *APIKeyService) setAuthCacheEntry(ctx context.Context, cacheKey string, entry *APIKeyAuthCacheEntry, ttl time.Duration) {
	if entry == nil {
		return
	}
	s.setAuthCacheL1(cacheKey, entry)
	if s.cache == nil || !s.authCfg.l2Enabled() {
		return
	}
	_ = s.cache.SetAuthCache(ctx, cacheKey, entry, s.authCfg.jitterTTL(ttl))
}

func (s *APIKeyService) deleteAuthCache(ctx context.Context, cacheKey string) {
	if s.authCacheL1 != nil {
		s.authCacheL1.Del(cacheKey)
	}
	if s.cache == nil {
		return
	}
	_ = s.cache.DeleteAuthCache(ctx, cacheKey)
	// Publish invalidation message to other instances
	_ = s.cache.PublishAuthCacheInvalidation(ctx, cacheKey)
}

func (s *APIKeyService) loadAuthCacheEntry(ctx context.Context, key, cacheKey string) (*APIKeyAuthCacheEntry, error) {
	apiKey, err := s.apiKeyRepo.GetByKeyForAuth(ctx, key)
	if err != nil {
		if errors.Is(err, ErrAPIKeyNotFound) {
			entry := &APIKeyAuthCacheEntry{NotFound: true}
			if s.authCfg.negativeEnabled() {
				s.setAuthCacheEntry(ctx, cacheKey, entry, s.authCfg.negativeTTL)
			}
			return entry, nil
		}
		return nil, fmt.Errorf("get api key: %w", err)
	}
	apiKey.Key = key
	snapshot := s.snapshotFromAPIKey(ctx, apiKey)
	if snapshot == nil {
		return nil, fmt.Errorf("get api key: %w", ErrAPIKeyNotFound)
	}
	entry := &APIKeyAuthCacheEntry{Snapshot: snapshot}
	s.setAuthCacheEntry(ctx, cacheKey, entry, s.authCfg.l2TTL)
	return entry, nil
}

func (s *APIKeyService) applyAuthCacheEntry(key string, entry *APIKeyAuthCacheEntry) (*APIKey, bool, error) {
	if entry == nil {
		return nil, false, nil
	}
	if entry.NotFound {
		return nil, true, ErrAPIKeyNotFound
	}
	if entry.Snapshot == nil {
		return nil, false, nil
	}
	if entry.Snapshot.Version != apiKeyAuthSnapshotVersion {
		return nil, false, nil
	}
	return s.snapshotToAPIKey(key, entry.Snapshot), true, nil
}

func (s *APIKeyService) snapshotFromAPIKey(ctx context.Context, apiKey *APIKey) *APIKeyAuthSnapshot {
	if apiKey == nil || apiKey.User == nil {
		return nil
	}
	snapshot := &APIKeyAuthSnapshot{
		Version:     apiKeyAuthSnapshotVersion,
		APIKeyID:    apiKey.ID,
		UserID:      apiKey.UserID,
		GroupID:     apiKey.GroupID,
		GroupIDs:    append([]int64(nil), apiKey.GroupIDs...),
		Name:        apiKey.Name,
		Status:      apiKey.Status,
		IPWhitelist: apiKey.IPWhitelist,
		IPBlacklist: apiKey.IPBlacklist,
		Quota:       apiKey.Quota,
		QuotaUsed:   apiKey.QuotaUsed,
		ExpiresAt:   apiKey.ExpiresAt,
		RateLimit5h: apiKey.RateLimit5h,
		RateLimit1d: apiKey.RateLimit1d,
		RateLimit7d: apiKey.RateLimit7d,
		User: APIKeyAuthUserSnapshot{
			ID:                         apiKey.User.ID,
			Status:                     apiKey.User.Status,
			Role:                       apiKey.User.Role,
			Balance:                    apiKey.User.Balance,
			Concurrency:                apiKey.User.Concurrency,
			AllowedGroups:              apiKey.User.AllowedGroups,
			Email:                      apiKey.User.Email,
			Username:                   apiKey.User.Username,
			BalanceNotifyEnabled:       apiKey.User.BalanceNotifyEnabled,
			BalanceNotifyThresholdType: apiKey.User.BalanceNotifyThresholdType,
			BalanceNotifyThreshold:     apiKey.User.BalanceNotifyThreshold,
			BalanceNotifyExtraEmails:   apiKey.User.BalanceNotifyExtraEmails,
			TotalRecharged:             apiKey.User.TotalRecharged,
			RPMLimit:                   apiKey.User.RPMLimit,
		},
	}

	if len(snapshot.GroupIDs) == 0 && apiKey.GroupID != nil && *apiKey.GroupID > 0 {
		snapshot.GroupIDs = []int64{*apiKey.GroupID}
	}

	// 填充 (user, group) RPM override —— snapshot 构建时查一次 DB，后续请求零 DB 往返。
	if s.userGroupRateRepo != nil {
		for _, groupID := range snapshot.GroupIDs {
			if groupID <= 0 {
				continue
			}
			override, err := s.userGroupRateRepo.GetRPMOverrideByUserAndGroup(ctx, apiKey.UserID, groupID)
			if err == nil && override != nil {
				if snapshot.User.UserGroupRPMOverrides == nil {
					snapshot.User.UserGroupRPMOverrides = map[string]*int{}
				}
				snapshot.User.UserGroupRPMOverrides[strconv.FormatInt(groupID, 10)] = override
				if apiKey.GroupID != nil && *apiKey.GroupID == groupID {
					snapshot.User.UserGroupRPMOverride = override
				}
			}
			// 查询失败或无 override 时留 nil，checkRPM 会回退到 DB 查询
		}
	}
	if apiKey.Group != nil {
		groupSnapshot := authGroupSnapshotFromService(apiKey.Group)
		snapshot.Group = &groupSnapshot
	}
	if len(apiKey.Groups) > 0 {
		snapshot.Groups = make([]APIKeyAuthGroupSnapshot, 0, len(apiKey.Groups))
		for i := range apiKey.Groups {
			groupSnapshot := authGroupSnapshotFromService(&apiKey.Groups[i])
			snapshot.Groups = append(snapshot.Groups, groupSnapshot)
		}
	} else if snapshot.Group != nil {
		snapshot.Groups = []APIKeyAuthGroupSnapshot{*snapshot.Group}
	}
	return snapshot
}

func (s *APIKeyService) snapshotToAPIKey(key string, snapshot *APIKeyAuthSnapshot) *APIKey {
	if snapshot == nil {
		return nil
	}
	apiKey := &APIKey{
		ID:          snapshot.APIKeyID,
		UserID:      snapshot.UserID,
		GroupID:     snapshot.GroupID,
		GroupIDs:    append([]int64(nil), snapshot.GroupIDs...),
		Key:         key,
		Name:        snapshot.Name,
		Status:      snapshot.Status,
		IPWhitelist: snapshot.IPWhitelist,
		IPBlacklist: snapshot.IPBlacklist,
		Quota:       snapshot.Quota,
		QuotaUsed:   snapshot.QuotaUsed,
		ExpiresAt:   snapshot.ExpiresAt,
		RateLimit5h: snapshot.RateLimit5h,
		RateLimit1d: snapshot.RateLimit1d,
		RateLimit7d: snapshot.RateLimit7d,
		User: &User{
			ID:                         snapshot.User.ID,
			Status:                     snapshot.User.Status,
			Role:                       snapshot.User.Role,
			Balance:                    snapshot.User.Balance,
			Concurrency:                snapshot.User.Concurrency,
			AllowedGroups:              snapshot.User.AllowedGroups,
			Email:                      snapshot.User.Email,
			Username:                   snapshot.User.Username,
			BalanceNotifyEnabled:       snapshot.User.BalanceNotifyEnabled,
			BalanceNotifyThresholdType: snapshot.User.BalanceNotifyThresholdType,
			BalanceNotifyThreshold:     snapshot.User.BalanceNotifyThreshold,
			BalanceNotifyExtraEmails:   snapshot.User.BalanceNotifyExtraEmails,
			TotalRecharged:             snapshot.User.TotalRecharged,
			RPMLimit:                   snapshot.User.RPMLimit,
			UserGroupRPMOverride:       snapshot.User.UserGroupRPMOverride,
			UserGroupRPMOverrides:      authRPMOverridesFromSnapshot(snapshot.User.UserGroupRPMOverrides),
		},
	}
	if len(apiKey.GroupIDs) == 0 && apiKey.GroupID != nil && *apiKey.GroupID > 0 {
		apiKey.GroupIDs = []int64{*apiKey.GroupID}
	}
	if snapshot.Group != nil {
		apiKey.Group = authGroupFromSnapshot(snapshot.Group)
	}
	if len(snapshot.Groups) > 0 {
		apiKey.Groups = make([]Group, 0, len(snapshot.Groups))
		for i := range snapshot.Groups {
			apiKey.Groups = append(apiKey.Groups, *authGroupFromSnapshot(&snapshot.Groups[i]))
		}
	} else if apiKey.Group != nil {
		apiKey.Groups = []Group{*apiKey.Group}
	}
	s.compileAPIKeyIPRules(apiKey)
	return apiKey
}

func authRPMOverridesFromSnapshot(snapshot map[string]*int) map[int64]*int {
	if len(snapshot) == 0 {
		return nil
	}
	out := make(map[int64]*int, len(snapshot))
	for rawGroupID, override := range snapshot {
		groupID, err := strconv.ParseInt(rawGroupID, 10, 64)
		if err != nil || groupID <= 0 {
			continue
		}
		out[groupID] = override
	}
	return out
}

func authGroupSnapshotFromService(group *Group) APIKeyAuthGroupSnapshot {
	if group == nil {
		return APIKeyAuthGroupSnapshot{}
	}
	return APIKeyAuthGroupSnapshot{
		ID:                              group.ID,
		Name:                            group.Name,
		Platform:                        group.Platform,
		IsExclusive:                     group.IsExclusive,
		Status:                          group.Status,
		SubscriptionType:                group.SubscriptionType,
		RateMultiplier:                  group.RateMultiplier,
		DailyLimitUSD:                   group.DailyLimitUSD,
		WeeklyLimitUSD:                  group.WeeklyLimitUSD,
		MonthlyLimitUSD:                 group.MonthlyLimitUSD,
		AllowImageGeneration:            group.AllowImageGeneration,
		ImageRateIndependent:            group.ImageRateIndependent,
		ImageRateMultiplier:             group.ImageRateMultiplier,
		ImagePrice1K:                    group.ImagePrice1K,
		ImagePrice2K:                    group.ImagePrice2K,
		ImagePrice4K:                    group.ImagePrice4K,
		ClaudeCodeOnly:                  group.ClaudeCodeOnly,
		FallbackGroupID:                 group.FallbackGroupID,
		FallbackGroupIDOnInvalidRequest: group.FallbackGroupIDOnInvalidRequest,
		ModelRouting:                    group.ModelRouting,
		ModelRoutingEnabled:             group.ModelRoutingEnabled,
		MCPXMLInject:                    group.MCPXMLInject,
		SupportedModelScopes:            group.SupportedModelScopes,
		AllowMessagesDispatch:           group.AllowMessagesDispatch,
		DefaultMappedModel:              group.DefaultMappedModel,
		MessagesDispatchModelConfig:     group.MessagesDispatchModelConfig,
		ModelsListConfig:                group.ModelsListConfig,
		RPMLimit:                        group.RPMLimit,
		PeakRateEnabled:                 group.PeakRateEnabled,
		PeakStart:                       group.PeakStart,
		PeakEnd:                         group.PeakEnd,
		PeakRateMultiplier:              group.PeakRateMultiplier,
	}
}

func authGroupFromSnapshot(snapshot *APIKeyAuthGroupSnapshot) *Group {
	if snapshot == nil {
		return nil
	}
	return &Group{
		ID:                              snapshot.ID,
		Name:                            snapshot.Name,
		Platform:                        snapshot.Platform,
		IsExclusive:                     snapshot.IsExclusive,
		Status:                          snapshot.Status,
		Hydrated:                        true,
		SubscriptionType:                snapshot.SubscriptionType,
		RateMultiplier:                  snapshot.RateMultiplier,
		DailyLimitUSD:                   snapshot.DailyLimitUSD,
		WeeklyLimitUSD:                  snapshot.WeeklyLimitUSD,
		MonthlyLimitUSD:                 snapshot.MonthlyLimitUSD,
		AllowImageGeneration:            snapshot.AllowImageGeneration,
		ImageRateIndependent:            snapshot.ImageRateIndependent,
		ImageRateMultiplier:             snapshot.ImageRateMultiplier,
		ImagePrice1K:                    snapshot.ImagePrice1K,
		ImagePrice2K:                    snapshot.ImagePrice2K,
		ImagePrice4K:                    snapshot.ImagePrice4K,
		ClaudeCodeOnly:                  snapshot.ClaudeCodeOnly,
		FallbackGroupID:                 snapshot.FallbackGroupID,
		FallbackGroupIDOnInvalidRequest: snapshot.FallbackGroupIDOnInvalidRequest,
		ModelRouting:                    snapshot.ModelRouting,
		ModelRoutingEnabled:             snapshot.ModelRoutingEnabled,
		MCPXMLInject:                    snapshot.MCPXMLInject,
		SupportedModelScopes:            snapshot.SupportedModelScopes,
		AllowMessagesDispatch:           snapshot.AllowMessagesDispatch,
		DefaultMappedModel:              snapshot.DefaultMappedModel,
		MessagesDispatchModelConfig:     snapshot.MessagesDispatchModelConfig,
		ModelsListConfig:                snapshot.ModelsListConfig,
		RPMLimit:                        snapshot.RPMLimit,
		PeakRateEnabled:                 snapshot.PeakRateEnabled,
		PeakStart:                       snapshot.PeakStart,
		PeakEnd:                         snapshot.PeakEnd,
		PeakRateMultiplier:              snapshot.PeakRateMultiplier,
	}
}
