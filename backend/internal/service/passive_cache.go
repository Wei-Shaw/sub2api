package service

import (
	"sync/atomic"
	"time"

	gocache "github.com/patrickmn/go-cache"
)

const passiveCacheSweepEvery = 1024

func setPassiveCache(cache *gocache.Cache, key string, value any, expiration time.Duration, writes *atomic.Uint64) {
	if cache == nil {
		return
	}
	cache.Set(key, value, expiration)
	if writes != nil && writes.Add(1)%passiveCacheSweepEvery == 0 {
		cache.DeleteExpired()
	}
}
