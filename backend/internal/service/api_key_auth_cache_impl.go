package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/dgraph-io/ristretto"
)

const apiKeyAuthSnapshotVersion = 7 // v7: added UserGroupRPMOverride on user snapshot

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

	// 填充 (user, group) RPM override —— snapshot 构建时查一次 DB，后续请求零 DB 往返。
	if apiKey.GroupID != nil && *apiKey.GroupID > 0 && s.userGroupRateRepo != nil {
		override, err := s.userGroupRateRepo.GetRPMOverrideByUserAndGroup(ctx, apiKey.UserID, *apiKey.GroupID)
		if err == nil && override != nil {
			snapshot.User.UserGroupRPMOverride = override
		}
		// 查询失败或无 override 时留 nil，checkRPM 会回退到 DB 查询
	}
	if apiKey.Group != nil {
		snapshot.Group = &APIKeyAuthGroupSnapshot{
			ID:                              apiKey.Group.ID,
			Name:                            apiKey.Group.Name,
			Platform:                        apiKey.Group.Platform,
			Status:                          apiKey.Group.Status,
			SubscriptionType:                apiKey.Group.SubscriptionType,
			RateMultiplier:                  apiKey.Group.RateMultiplier,
			DailyLimitUSD:                   apiKey.Group.DailyLimitUSD,
			WeeklyLimitUSD:                  apiKey.Group.WeeklyLimitUSD,
			MonthlyLimitUSD:                 apiKey.Group.MonthlyLimitUSD,
			ImagePrice1K:                    apiKey.Group.ImagePrice1K,
			ImagePrice2K:                    apiKey.Group.ImagePrice2K,
			ImagePrice4K:                    apiKey.Group.ImagePrice4K,
			ClaudeCodeOnly:                  apiKey.Group.ClaudeCodeOnly,
			FallbackGroupID:                 apiKey.Group.FallbackGroupID,
			FallbackGroupIDOnInvalidRequest: apiKey.Group.FallbackGroupIDOnInvalidRequest,
			ModelRouting:                    apiKey.Group.ModelRouting,
			ModelRoutingEnabled:             apiKey.Group.ModelRoutingEnabled,
			MCPXMLInject:                    apiKey.Group.MCPXMLInject,
			SupportedModelScopes:            apiKey.Group.SupportedModelScopes,
			AllowMessagesDispatch:           apiKey.Group.AllowMessagesDispatch,
			DefaultMappedModel:              apiKey.Group.DefaultMappedModel,
			MessagesDispatchModelConfig:     apiKey.Group.MessagesDispatchModelConfig,
			RPMLimit:                        apiKey.Group.RPMLimit,
		}
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
		Key:         key,
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
			Email:                      snapshot.User.Email,
			Username:                   snapshot.User.Username,
			BalanceNotifyEnabled:       snapshot.User.BalanceNotifyEnabled,
			BalanceNotifyThresholdType: snapshot.User.BalanceNotifyThresholdType,
			BalanceNotifyThreshold:     snapshot.User.BalanceNotifyThreshold,
			BalanceNotifyExtraEmails:   snapshot.User.BalanceNotifyExtraEmails,
			TotalRecharged:             snapshot.User.TotalRecharged,
			RPMLimit:                   snapshot.User.RPMLimit,
			UserGroupRPMOverride:       snapshot.User.UserGroupRPMOverride,
		},
	}
	if snapshot.Group != nil {
		apiKey.Group = &Group{
			ID:                              snapshot.Group.ID,
			Name:                            snapshot.Group.Name,
			Platform:                        snapshot.Group.Platform,
			Status:                          snapshot.Group.Status,
			Hydrated:                        true,
			SubscriptionType:                snapshot.Group.SubscriptionType,
			RateMultiplier:                  snapshot.Group.RateMultiplier,
			DailyLimitUSD:                   snapshot.Group.DailyLimitUSD,
			WeeklyLimitUSD:                  snapshot.Group.WeeklyLimitUSD,
			MonthlyLimitUSD:                 snapshot.Group.MonthlyLimitUSD,
			ImagePrice1K:                    snapshot.Group.ImagePrice1K,
			ImagePrice2K:                    snapshot.Group.ImagePrice2K,
			ImagePrice4K:                    snapshot.Group.ImagePrice4K,
			ClaudeCodeOnly:                  snapshot.Group.ClaudeCodeOnly,
			FallbackGroupID:                 snapshot.Group.FallbackGroupID,
			FallbackGroupIDOnInvalidRequest: snapshot.Group.FallbackGroupIDOnInvalidRequest,
			ModelRouting:                    snapshot.Group.ModelRouting,
			ModelRoutingEnabled:             snapshot.Group.ModelRoutingEnabled,
			MCPXMLInject:                    snapshot.Group.MCPXMLInject,
			SupportedModelScopes:            snapshot.Group.SupportedModelScopes,
			AllowMessagesDispatch:           snapshot.Group.AllowMessagesDispatch,
			DefaultMappedModel:              snapshot.Group.DefaultMappedModel,
			MessagesDispatchModelConfig:     snapshot.Group.MessagesDispatchModelConfig,
			RPMLimit:                        snapshot.Group.RPMLimit,
		}
	}
	s.compileAPIKeyIPRules(apiKey)
	return apiKey
}
