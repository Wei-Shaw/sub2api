//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

type apiKeyLifecycleCache struct {
	mu sync.Mutex
	stubConcurrencyCacheForTest
	id              string
	expires         time.Time
	trackErr        error
	refreshErr      error
	releaseFailures int
	releases        []string
	refreshes       int
	blockRefresh    bool
}

func (c *apiKeyLifecycleCache) TrackAPIKeySlot(_ context.Context, _ int64, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.id = id
	c.expires = time.Now().Add(3 * time.Second)
	return c.trackErr
}
func (c *apiKeyLifecycleCache) APIKeySlotRefreshInterval() time.Duration { return time.Second }
func (c *apiKeyLifecycleCache) RefreshAPIKeySlot(ctx context.Context, _ int64, id string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshes++
	if c.refreshErr != nil {
		return false, c.refreshErr
	}
	if c.blockRefresh {
		<-ctx.Done()
		return false, ctx.Err()
	}
	if c.id != id || !time.Now().Before(c.expires) {
		return false, nil
	}
	c.expires = time.Now().Add(3 * time.Second)
	return true, nil
}
func (c *apiKeyLifecycleCache) ReleaseAPIKeySlot(ctx context.Context, _ int64, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	c.releases = append(c.releases, id)
	if len(c.releases) <= c.releaseFailures {
		return errors.New("transient release failure")
	}
	if c.id == id {
		c.id = ""
	}
	return nil
}

func TestAPIKeySlotLifecycleAmbiguousWriteAndReleaseRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &apiKeyLifecycleCache{trackErr: errors.New("write applied but reply lost"), releaseFailures: 1}
		release := NewConcurrencyService(cache).TrackAPIKeySlot(context.Background(), 8)
		id := cache.id
		release()
		release()
		require.Equal(t, []string{id, id}, cache.releases)
		require.Empty(t, cache.id)
	})
}

type apiKeyAdmissionLifecycleCache struct{ apiKeyLifecycleCache }

func (*apiKeyAdmissionLifecycleCache) APIKeySlotTTL() time.Duration { return 3 * time.Second }

func (c *apiKeyAdmissionLifecycleCache) AcquireAPIKeySlot(ctx context.Context, id int64, _ int, request string) (bool, error) {
	c.mu.Lock()
	busy := c.id != "" && time.Now().Before(c.expires)
	c.mu.Unlock()
	if busy {
		return false, nil
	}
	err := c.TrackAPIKeySlot(ctx, id, request)
	return err == nil, err
}

func TestAPIKeyAdmissionAmbiguousFailureClosesAndCleansUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &apiKeyAdmissionLifecycleCache{apiKeyLifecycleCache: apiKeyLifecycleCache{trackErr: errors.New("write applied but reply lost"), releaseFailures: 1}}
		ctx, cancel := WithAPIKeyAdmissionOwner(context.Background())
		defer cancel()
		result, err := NewConcurrencyService(cache).AcquireAPIKeySlot(ctx, 8, 1)
		require.Error(t, err)
		require.Nil(t, result, "ambiguous admission must never forward")
		require.Len(t, cache.releases, 2)
		require.NotEmpty(t, cache.releases[0])
		require.Equal(t, cache.releases[0], cache.releases[1])
		require.Empty(t, cache.id)
	})
}

func TestAPIKeyAdmissionRequiresCacheCapability(t *testing.T) {
	for _, cache := range []ConcurrencyCache{nil, &stubConcurrencyCacheForTest{}} {
		svc := NewConcurrencyService(cache)
		result, err := svc.AcquireAPIKeySlot(context.Background(), 8, 1)
		require.Error(t, err)
		require.Nil(t, result)
		result, err = svc.AcquireAPIKeySlot(context.Background(), 8, 0)
		require.NoError(t, err)
		require.True(t, result.Acquired)
		result.ReleaseFunc()
	}
}

func TestAPIKeyAdmissionRenewsUntilExplicitRelease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &apiKeyAdmissionLifecycleCache{}
		ctx, cancel := context.WithCancel(context.Background())
		ctx, cancelOwner := WithAPIKeyAdmissionOwner(ctx)
		defer cancelOwner()
		result, err := NewConcurrencyService(cache).AcquireAPIKeySlot(ctx, 8, 1)
		require.NoError(t, err)
		require.True(t, result.Acquired)
		id := cache.id
		cancel()
		time.Sleep(7 * time.Second)
		synctest.Wait()
		require.Equal(t, id, cache.id, "WS cancellation must not release before upstream shutdown")
		require.True(t, time.Now().Before(cache.expires))
		require.Positive(t, cache.refreshes)
		result.ReleaseFunc()
		result.ReleaseFunc()
		require.Empty(t, cache.id)
		require.Equal(t, []string{id}, cache.releases)
	})
}

func TestAPIKeySlotLifecycleLongRequestAndStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &apiKeyLifecycleCache{}
		release := NewConcurrencyService(cache).TrackAPIKeySlot(context.Background(), 8)
		synctest.Wait()
		time.Sleep(10 * time.Second) // virtual time; crosses the original three-second TTL
		synctest.Wait()
		require.True(t, time.Now().Before(cache.expires))
		require.NotEmpty(t, cache.id)
		release()
		refreshes := cache.refreshes
		time.Sleep(5 * time.Second)
		synctest.Wait()
		require.Equal(t, refreshes, cache.refreshes)
		require.Len(t, cache.releases, 1)
	})
}

func TestAPIKeySlotLifecycleMissingMemberStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &apiKeyLifecycleCache{}
		release := NewConcurrencyService(cache).TrackAPIKeySlot(context.Background(), 8)
		synctest.Wait()
		cache.mu.Lock()
		cache.id = "" // simulate administrative removal
		cache.mu.Unlock()
		time.Sleep(time.Second)
		synctest.Wait()
		require.Empty(t, cache.id)
		require.Len(t, cache.releases, 1)
		release()
		require.Equal(t, 1, cache.refreshes)
	})
}

func TestAPIKeySlotLifecycleCancellationJoinsRefreshAndBoundsCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := &apiKeyLifecycleCache{blockRefresh: true, releaseFailures: 100}
		ctx, cancel := context.WithCancel(context.Background())
		release := NewConcurrencyService(cache).TrackAPIKeySlot(ctx, 8)
		synctest.Wait()
		time.Sleep(time.Second)
		synctest.Wait()
		require.Equal(t, 1, cache.refreshes)
		cancel()
		release()
		require.Len(t, cache.releases, 3, "cleanup uses detached context but bounded attempts")
		release()
		require.Len(t, cache.releases, 3)
	})
}
