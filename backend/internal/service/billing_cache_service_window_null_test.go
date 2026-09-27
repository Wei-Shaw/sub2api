//go:build unit

package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rateLimitWindowLoaderStub serves fixed rate-limit data and counts window resets.
type rateLimitWindowLoaderStub struct {
	data    *APIKeyRateLimitData
	resets  atomic.Int64
	release chan struct{} // non-nil: ResetRateLimitWindows blocks until closed
}

func (s *rateLimitWindowLoaderStub) GetRateLimitData(context.Context, int64) (*APIKeyRateLimitData, error) {
	return s.data, nil
}

func (s *rateLimitWindowLoaderStub) ResetRateLimitWindows(context.Context, int64) error {
	s.resets.Add(1)
	if s.release != nil {
		<-s.release
	}
	return nil
}

func TestBillingCacheRateLimitNullWindowSkipsReset(t *testing.T) {
	loader := &rateLimitWindowLoaderStub{data: &APIKeyRateLimitData{Usage5h: 3, Usage1d: 3, Usage7d: 3}}
	svc := &BillingCacheService{apiKeyRateLimitLoader: loader}
	apiKey := &APIKey{ID: 42, RateLimit5h: 1, RateLimit1d: 1, RateLimit7d: 1}

	for i := 0; i < 100; i++ {
		// A NULL window still counts as empty for the limit check, as before.
		require.NoError(t, svc.checkAPIKeyRateLimits(context.Background(), apiKey))
	}
	// The reset SQL only touches non-NULL windows, so issuing it for a NULL
	// window is a wasted UPDATE (plus cache DEL) on every request.
	require.Never(t, func() bool { return loader.resets.Load() > 0 }, 200*time.Millisecond, 10*time.Millisecond)
}

func TestBillingCacheRateLimitExpiredWindowResetsOncePerKey(t *testing.T) {
	expired := time.Now().Add(-8 * 24 * time.Hour)
	loader := &rateLimitWindowLoaderStub{
		data: &APIKeyRateLimitData{
			Usage5h: 3, Usage1d: 3, Usage7d: 3,
			Window5hStart: &expired, Window1dStart: &expired, Window7dStart: &expired,
		},
		release: make(chan struct{}),
	}
	t.Cleanup(func() { close(loader.release) })
	svc := &BillingCacheService{apiKeyRateLimitLoader: loader}
	apiKey := &APIKey{ID: 42, RateLimit5h: 1, RateLimit1d: 1, RateLimit7d: 1}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Expired windows are treated as empty, so the request passes.
			assert.NoError(t, svc.checkAPIKeyRateLimits(context.Background(), apiKey))
		}()
	}
	wg.Wait()

	require.Eventually(t, func() bool { return loader.resets.Load() == 1 }, time.Second, 5*time.Millisecond)
	require.Never(t, func() bool { return loader.resets.Load() > 1 }, 200*time.Millisecond, 10*time.Millisecond)
}
