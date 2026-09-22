package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var ErrAPIKeyConcurrencyExceeded = errors.New("API key concurrency limit reached")

// APIKeyConcurrencyLimiter extends the existing statistics cache without
// changing account/user scheduling or requiring unrelated cache implementations.
type APIKeyConcurrencyLimiter interface {
	APIKeyConcurrencyCache
	AcquireAPIKeySlot(ctx context.Context, apiKeyID int64, limit int, requestID string) (bool, error)
	RefreshAPIKeySlot(ctx context.Context, apiKeyID int64, requestID string) (bool, error)
}

// AcquireAPIKeySlot reserves one logical request, including any user/account
// wait. A key cannot fill the shared user's wait queue beyond its own limit.
// The owner cancels forwarding on lease loss; unlimited keys retain fail-open
// statistics behavior. All successful slots are renewed for long generations.
func (s *ConcurrencyService) AcquireAPIKeySlot(ctx context.Context, apiKeyID int64, limit int, onLeaseLost func()) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var cache APIKeyConcurrencyLimiter
	if s != nil {
		cache, _ = s.cache.(APIKeyConcurrencyLimiter)
	}
	if cache == nil || apiKeyID <= 0 {
		if limit > 0 {
			return nil, errors.New("API key concurrency cache is unavailable")
		}
		return s.TrackAPIKeySlot(ctx, apiKeyID), nil
	}
	requestID := generateRequestID()
	acquireCtx, cancel := context.WithTimeout(ctx, apiKeySlotTrackTimeout)
	acquired, err := cache.AcquireAPIKeySlot(acquireCtx, apiKeyID, limit, requestID)
	cancel()
	if err != nil {
		// A timed-out script may still have committed. Release only our member.
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), apiKeySlotTrackTimeout)
		_ = cache.ReleaseAPIKeySlot(releaseCtx, apiKeyID, requestID)
		releaseCancel()
		if limit == 0 {
			return func() {}, nil
		}
		return nil, err
	}
	if !acquired {
		return nil, ErrAPIKeyConcurrencyExceeded
	}
	return keepAPIKeySlot(ctx, cache, apiKeyID, requestID, limit > 0, onLeaseLost, 20*time.Second), nil
}

func keepAPIKeySlot(ctx context.Context, cache APIKeyConcurrencyLimiter, apiKeyID int64, requestID string, enforce bool, onLeaseLost func(), interval time.Duration) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	release := func() {
		once.Do(func() {
			close(stop)
			<-done // No refresh may race with the final removal.
			releaseCtx, cancel := context.WithTimeout(context.Background(), apiKeySlotTrackTimeout)
			defer cancel()
			if err := cache.ReleaseAPIKeySlot(releaseCtx, apiKeyID, requestID); err != nil {
				logger.LegacyPrintf("service.concurrency", "Failed to release API key slot: key_id=%d error=%v", apiKeyID, err)
			}
		})
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				refreshCtx, cancel := context.WithTimeout(ctx, apiKeySlotTrackTimeout)
				owned, err := cache.RefreshAPIKeySlot(refreshCtx, apiKeyID, requestID)
				cancel()
				if err != nil || !owned {
					logger.LegacyPrintf("service.concurrency", "API key slot lease lost: key_id=%d owned=%t error=%v", apiKeyID, owned, err)
					if enforce && onLeaseLost != nil {
						onLeaseLost()
					}
					return
				}
			}
		}
	}()
	stopOnCancel := context.AfterFunc(ctx, release)
	return func() {
		stopOnCancel()
		release()
	}
}
