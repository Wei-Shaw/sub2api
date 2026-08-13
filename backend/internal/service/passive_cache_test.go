package service

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
)

func TestSetPassiveCacheSweepsExpiredEntries(t *testing.T) {
	cache := gocache.NewFrom(time.Minute, 0, map[string]gocache.Item{})
	cache.Set("expired", struct{}{}, time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	var writes atomic.Uint64
	for i := 0; i < passiveCacheSweepEvery; i++ {
		setPassiveCache(cache, strconv.Itoa(i), struct{}{}, time.Minute, &writes)
	}

	_, exists := cache.Items()["expired"]
	require.False(t, exists)
}
