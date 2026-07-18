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

const apiKeyAuthSnapshotVersion = 15 // v15: include group web search per-call pricing

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
	if s.authCfg.negativeEnabled() {
		negativeSize := defaultNegativeAuthCacheSize
		if s.authCfg.l1Size > 0 && s.authCfg.l1Size < negativeSize {
			negativeSize = s.authCfg.l1Size
		}
		cache, err := ristretto.NewCache(&ristretto.Config{
			NumCounters: int64(negativeSize) * 10,
			MaxCost:     int64(negativeSize),
			BufferItems: 64,
		})
		if err == nil {
			s.authNegativeCacheL1 = cache
		}
	}
	if s.authCfg.l1Enabled() {
		cache, err := ristretto.NewCache(&ristretto.Config{
			NumCounters: int64(s.authCfg.l1Size) * 10,
			MaxCost:     int64(s.authCfg.l1Size),
			BufferItems: 64,
		})
		if err == nil {
			s.authCacheL1 = cache
		}
	}
}

// StartAuthCacheInvalidationSubscriber starts the Pub/Sub subscriber for L1 cache invalidation.
// This should be called after the service is fully initialized.
func (s *APIKeyService) StartAuthCacheInvalidationSubscriber(ctx context.Context) {
	if s.cache == nil || (s.authCacheL1 == nil && s.authNegativeCacheL1 == nil) {
		return
	}
	s.authInvalidationStart.Do(func() {
		subscriberCtx, cancel := context.WithCancel(ctx)
		subscriberCtx = withAuthCacheSubscriptionReady(subscriberCtx, func() {
			s.authInvalidationConnected.Store(true)
		})
		s.authInvalidationCancel = cancel
		s.authInvalidationWG.Add(1)
		go func() {
			defer s.authInvalidationWG.Done()
			backoff := time.Second
			for {
				err := s.cache.SubscribeAuthCacheInvalidation(subscriberCtx, func(cacheKey string) {
					s.invalidateLocalAuthCache(cacheKey)
				})
				wasConnected := s.authInvalidationConnected.Swap(false)
				if subscriberCtx.Err() != nil {
					return
				}
				if wasConnected {
					backoff = time.Second
				}
				s.authInvalidationFailures.Add(1)
				if err == nil {
					err = errors.New("auth cache invalidation subscription closed")
				}
				slog.Warn("failed to start auth cache invalidation subscriber; retrying", "error", err, "retry_in", backoff)
				timer := time.NewTimer(backoff)
				select {
				case <-subscriberCtx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				if backoff < 30*time.Second {
					backoff *= 2
					if backoff > 30*time.Second {
						backoff = 30 * time.Second
					}
				}
			}
		}()
	})
}

func (s *APIKeyService) invalidateLocalAuthCache(cacheKey string) {
	if s == nil {
		return
	}
	if s.authCacheL1 != nil {
		s.authCacheL1.Del(cacheKey)
	}
	if s.authNegativeCacheL1 != nil {
		s.authNegativeCacheL1.Del(cacheKey)
	}
}

type AuthCacheInvalidationSubscriberHealth struct {
	Connected bool   `json:"connected"`
	Failures  uint64 `json:"failures"`
}

func (s *APIKeyService) AuthCacheInvalidationSubscriberHealth() AuthCacheInvalidationSubscriberHealth {
	if s == nil {
		return AuthCacheInvalidationSubscriberHealth{}
	}
	return AuthCacheInvalidationSubscriberHealth{
		Connected: s.authInvalidationConnected.Load(),
		Failures:  s.authInvalidationFailures.Load(),
	}
}

func (s *APIKeyService) StopAuthCacheInvalidationSubscriber() {
	if s == nil {
		return
	}
	s.authInvalidationStop.Do(func() {
		if s.authInvalidationCancel != nil {
			s.authInvalidationCancel()
		}
		s.authInvalidationWG.Wait()
	})
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
	if s.authNegativeCacheL1 != nil {
		if val, ok := s.authNegativeCacheL1.Get(cacheKey); ok {
			if entry, ok := val.(*APIKeyAuthCacheEntry); ok && entry.NotFound {
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
	if entry == nil {
		return
	}
	if entry.NotFound {
		if s.authNegativeCacheL1 != nil && s.authCfg.negativeTTL > 0 {
			_ = s.authNegativeCacheL1.SetWithTTL(cacheKey, entry, 1, s.authCfg.jitterTTL(s.authCfg.negativeTTL))
		}
		return
	}
	if s.authCacheL1 == nil {
		return
	}
	ttl := s.authCfg.l1TTL
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
	if s.authNegativeCacheL1 != nil {
		s.authNegativeCacheL1.Del(cacheKey)
	}
	if s.cache == nil {
		return
	}
	_ = s.cache.DeleteAuthCache(ctx, cacheKey)
	// Publish invalidation message to other instances
	_ = s.cache.PublishAuthCacheInvalidation(ctx, cacheKey)
}

func (s *APIKeyService) loadAuthCacheEntry(ctx context.Context, key, cacheKey string) (*APIKeyAuthCacheEntry, error) {
	apiKey, err := s.lookupAPIKeyForAuth(ctx, key)
	if err != nil {
		if errors.Is(err, ErrAPIKeyNotFound) {
			entry := &APIKeyAuthCacheEntry{NotFound: true}
			if s.authCfg.negativeEnabled() {
				// Invalid keys are attacker-controlled and high-cardinality. Keep their
				// negative entries in the bounded process-local cache; do not amplify
				// random-key scans into Redis writes on every instance.
				s.setAuthCacheL1(cacheKey, entry)
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

func (s *APIKeyService) lookupAPIKeyForAuth(ctx context.Context, key string) (*APIKey, error) {
	if s == nil || s.apiKeyRepo == nil {
		return nil, ErrAPIKeyNotFound
	}
	if s.authLookupSlots == nil {
		return s.apiKeyRepo.GetByKeyForAuth(ctx, key)
	}
	s.authLookupTotal.Add(1)
	select {
	case s.authLookupSlots <- struct{}{}:
		s.authLookupInFlight.Add(1)
		defer func() {
			s.authLookupInFlight.Add(-1)
			<-s.authLookupSlots
		}()
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		s.authLookupRejected.Add(1)
		return nil, ErrAPIKeyAuthOverloaded
	}
	return s.apiKeyRepo.GetByKeyForAuth(ctx, key)
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
			IsExclusive:                     apiKey.Group.IsExclusive,
			Status:                          apiKey.Group.Status,
			SubscriptionType:                apiKey.Group.SubscriptionType,
			RateMultiplier:                  apiKey.Group.RateMultiplier,
			DailyLimitUSD:                   apiKey.Group.DailyLimitUSD,
			WeeklyLimitUSD:                  apiKey.Group.WeeklyLimitUSD,
			MonthlyLimitUSD:                 apiKey.Group.MonthlyLimitUSD,
			AllowImageGeneration:            apiKey.Group.AllowImageGeneration,
			AllowBatchImageGeneration:       apiKey.Group.AllowBatchImageGeneration,
			ImageRateIndependent:            apiKey.Group.ImageRateIndependent,
			ImageRateMultiplier:             apiKey.Group.ImageRateMultiplier,
			ImagePrice1K:                    apiKey.Group.ImagePrice1K,
			ImagePrice2K:                    apiKey.Group.ImagePrice2K,
			ImagePrice4K:                    apiKey.Group.ImagePrice4K,
			VideoRateIndependent:            apiKey.Group.VideoRateIndependent,
			VideoRateMultiplier:             apiKey.Group.VideoRateMultiplier,
			VideoPrice480P:                  apiKey.Group.VideoPrice480P,
			VideoPrice720P:                  apiKey.Group.VideoPrice720P,
			VideoPrice1080P:                 apiKey.Group.VideoPrice1080P,
			WebSearchPricePerCall:           apiKey.Group.WebSearchPricePerCall,
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
			ModelsListConfig:                apiKey.Group.ModelsListConfig,
			RPMLimit:                        apiKey.Group.RPMLimit,
			PeakRateEnabled:                 apiKey.Group.PeakRateEnabled,
			PeakStart:                       apiKey.Group.PeakStart,
			PeakEnd:                         apiKey.Group.PeakEnd,
			PeakRateMultiplier:              apiKey.Group.PeakRateMultiplier,
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
		},
	}
	if snapshot.Group != nil {
		apiKey.Group = &Group{
			ID:                              snapshot.Group.ID,
			Name:                            snapshot.Group.Name,
			Platform:                        snapshot.Group.Platform,
			IsExclusive:                     snapshot.Group.IsExclusive,
			Status:                          snapshot.Group.Status,
			Hydrated:                        true,
			SubscriptionType:                snapshot.Group.SubscriptionType,
			RateMultiplier:                  snapshot.Group.RateMultiplier,
			DailyLimitUSD:                   snapshot.Group.DailyLimitUSD,
			WeeklyLimitUSD:                  snapshot.Group.WeeklyLimitUSD,
			MonthlyLimitUSD:                 snapshot.Group.MonthlyLimitUSD,
			AllowImageGeneration:            snapshot.Group.AllowImageGeneration,
			AllowBatchImageGeneration:       snapshot.Group.AllowBatchImageGeneration,
			ImageRateIndependent:            snapshot.Group.ImageRateIndependent,
			ImageRateMultiplier:             snapshot.Group.ImageRateMultiplier,
			ImagePrice1K:                    snapshot.Group.ImagePrice1K,
			ImagePrice2K:                    snapshot.Group.ImagePrice2K,
			ImagePrice4K:                    snapshot.Group.ImagePrice4K,
			VideoRateIndependent:            snapshot.Group.VideoRateIndependent,
			VideoRateMultiplier:             snapshot.Group.VideoRateMultiplier,
			VideoPrice480P:                  snapshot.Group.VideoPrice480P,
			VideoPrice720P:                  snapshot.Group.VideoPrice720P,
			VideoPrice1080P:                 snapshot.Group.VideoPrice1080P,
			WebSearchPricePerCall:           snapshot.Group.WebSearchPricePerCall,
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
			ModelsListConfig:                snapshot.Group.ModelsListConfig,
			RPMLimit:                        snapshot.Group.RPMLimit,
			PeakRateEnabled:                 snapshot.Group.PeakRateEnabled,
			PeakStart:                       snapshot.Group.PeakStart,
			PeakEnd:                         snapshot.Group.PeakEnd,
			PeakRateMultiplier:              snapshot.Group.PeakRateMultiplier,
		}
	}
	s.compileAPIKeyIPRules(apiKey)
	return apiKey
}
