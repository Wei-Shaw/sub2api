package service

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	gocache "github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

type userGroupRateResolver struct {
	repo         UserGroupRateRepository
	cache        *gocache.Cache
	cacheTTL     time.Duration
	sf           *singleflight.Group
	cacheWrites  atomic.Uint64
	logComponent string
}

func newUserGroupRateResolver(repo UserGroupRateRepository, cache *gocache.Cache, cacheTTL time.Duration, sf *singleflight.Group, logComponent string) *userGroupRateResolver {
	if cacheTTL <= 0 {
		cacheTTL = defaultUserGroupRateCacheTTL
	}
	if cache == nil {
		cache = gocache.NewFrom(cacheTTL, 0, map[string]gocache.Item{})
	}
	if logComponent == "" {
		logComponent = "service.gateway"
	}
	if sf == nil {
		sf = &singleflight.Group{}
	}

	return &userGroupRateResolver{
		repo:         repo,
		cache:        cache,
		cacheTTL:     cacheTTL,
		sf:           sf,
		logComponent: logComponent,
	}
}

func (r *userGroupRateResolver) Resolve(ctx context.Context, userID, groupID int64, groupDefaultMultiplier float64) float64 {
	if r == nil || userID <= 0 || groupID <= 0 {
		return groupDefaultMultiplier
	}

	key := fmt.Sprintf("%d:%d", userID, groupID)
	if r.cache != nil {
		if cached, ok := r.cache.Get(key); ok {
			if multiplier, castOK := cached.(float64); castOK {
				userGroupRateCacheHitTotal.Add(1)
				return multiplier
			}
		}
	}
	if r.repo == nil {
		return groupDefaultMultiplier
	}
	userGroupRateCacheMissTotal.Add(1)

	value, err, shared := r.sf.Do(key, func() (any, error) {
		if r.cache != nil {
			if cached, ok := r.cache.Get(key); ok {
				if multiplier, castOK := cached.(float64); castOK {
					userGroupRateCacheHitTotal.Add(1)
					return multiplier, nil
				}
			}
		}

		userGroupRateCacheLoadTotal.Add(1)
		userRate, repoErr := r.repo.GetByUserAndGroup(ctx, userID, groupID)
		if repoErr != nil {
			return nil, repoErr
		}

		multiplier := groupDefaultMultiplier
		if userRate != nil {
			multiplier = *userRate
		}
		if r.cache != nil {
			setPassiveCache(r.cache, key, multiplier, r.cacheTTL, &r.cacheWrites)
		}
		return multiplier, nil
	})
	if shared {
		userGroupRateCacheSFSharedTotal.Add(1)
	}
	if err != nil {
		userGroupRateCacheFallbackTotal.Add(1)
		logger.LegacyPrintf(r.logComponent, "get user group rate failed, fallback to group default: user=%d group=%d err=%v", userID, groupID, err)
		return groupDefaultMultiplier
	}

	multiplier, ok := value.(float64)
	if !ok {
		userGroupRateCacheFallbackTotal.Add(1)
		return groupDefaultMultiplier
	}
	return multiplier
}
