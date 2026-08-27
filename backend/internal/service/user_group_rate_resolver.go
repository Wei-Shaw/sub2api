package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	gocache "github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

type userGroupRateResolver struct {
	repo          UserGroupRateRepository
	cache         *gocache.Cache
	overrideCache *gocache.Cache
	cacheTTL      time.Duration
	sf            *singleflight.Group
	logComponent  string
}

type UserGroupRateResolution struct {
	Multiplier   float64
	UserOverride bool
}

func newUserGroupRateResolver(repo UserGroupRateRepository, cache *gocache.Cache, cacheTTL time.Duration, sf *singleflight.Group, logComponent string) *userGroupRateResolver {
	if cacheTTL <= 0 {
		cacheTTL = defaultUserGroupRateCacheTTL
	}
	if cache == nil {
		cache = gocache.New(cacheTTL, time.Minute)
	}
	if logComponent == "" {
		logComponent = "service.gateway"
	}
	if sf == nil {
		sf = &singleflight.Group{}
	}

	return &userGroupRateResolver{
		repo:          repo,
		cache:         cache,
		overrideCache: gocache.New(cacheTTL, time.Minute),
		cacheTTL:      cacheTTL,
		sf:            sf,
		logComponent:  logComponent,
	}
}

func (r *userGroupRateResolver) Resolve(ctx context.Context, userID, groupID int64, groupDefaultMultiplier float64) float64 {
	return r.ResolveWithSource(ctx, userID, groupID, groupDefaultMultiplier).Multiplier
}

func (r *userGroupRateResolver) ResolveWithSource(ctx context.Context, userID, groupID int64, groupDefaultMultiplier float64) UserGroupRateResolution {
	if r == nil || userID <= 0 || groupID <= 0 {
		return UserGroupRateResolution{Multiplier: groupDefaultMultiplier}
	}

	key := fmt.Sprintf("%d:%d", userID, groupID)
	if r.cache != nil {
		if cached, ok := r.cache.Get(key); ok {
			if multiplier, castOK := cached.(float64); castOK {
				userGroupRateCacheHitTotal.Add(1)
				isOverride := false
				if r.overrideCache != nil {
					cachedOverride, exists := r.overrideCache.Get(key)
					if exists {
						isOverride, _ = cachedOverride.(bool)
					}
				}
				return UserGroupRateResolution{Multiplier: multiplier, UserOverride: isOverride}
			}
		}
	}
	if r.repo == nil {
		return UserGroupRateResolution{Multiplier: groupDefaultMultiplier}
	}
	userGroupRateCacheMissTotal.Add(1)

	value, err, shared := r.sf.Do(key, func() (any, error) {
		if r.cache != nil {
			if cached, ok := r.cache.Get(key); ok {
				if multiplier, castOK := cached.(float64); castOK {
					userGroupRateCacheHitTotal.Add(1)
					isOverride := false
					if r.overrideCache != nil {
						cachedOverride, exists := r.overrideCache.Get(key)
						if exists {
							isOverride, _ = cachedOverride.(bool)
						}
					}
					return UserGroupRateResolution{Multiplier: multiplier, UserOverride: isOverride}, nil
				}
			}
		}

		userGroupRateCacheLoadTotal.Add(1)
		userRate, repoErr := r.repo.GetByUserAndGroup(ctx, userID, groupID)
		if repoErr != nil {
			return nil, repoErr
		}

		multiplier := groupDefaultMultiplier
		isOverride := userRate != nil
		if userRate != nil {
			multiplier = *userRate
		}
		if r.cache != nil {
			r.cache.Set(key, multiplier, r.cacheTTL)
		}
		if r.overrideCache != nil {
			r.overrideCache.Set(key, isOverride, r.cacheTTL)
		}
		return UserGroupRateResolution{Multiplier: multiplier, UserOverride: isOverride}, nil
	})
	if shared {
		userGroupRateCacheSFSharedTotal.Add(1)
	}
	if err != nil {
		userGroupRateCacheFallbackTotal.Add(1)
		logger.LegacyPrintf(r.logComponent, "get user group rate failed, fallback to group default: user=%d group=%d err=%v", userID, groupID, err)
		return UserGroupRateResolution{Multiplier: groupDefaultMultiplier}
	}

	resolution, ok := value.(UserGroupRateResolution)
	if !ok {
		userGroupRateCacheFallbackTotal.Add(1)
		return UserGroupRateResolution{Multiplier: groupDefaultMultiplier}
	}
	return resolution
}
