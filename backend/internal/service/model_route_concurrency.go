package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// ModelRouteConcurrencyCache is an optional Redis backend for model-routing caps.
// Implementations live beside ConcurrencyCache; mocks may omit it (fail-open).
type ModelRouteConcurrencyCache interface {
	AcquireModelRouteSlot(ctx context.Context, groupID int64, pattern string, accountID int64, accountMax, totalMax int, requestID string) (bool, error)
	ReleaseModelRouteSlot(ctx context.Context, groupID int64, pattern string, accountID int64, requestID string) error
	GetModelRouteAccountConcurrency(ctx context.Context, groupID int64, pattern string, accountID int64) (int, error)
	GetModelRouteTotalConcurrency(ctx context.Context, groupID int64, pattern string) (int, error)
}

func (s *ConcurrencyService) modelRouteCache() ModelRouteConcurrencyCache {
	if s == nil || s.cache == nil {
		return nil
	}
	cache, _ := s.cache.(ModelRouteConcurrencyCache)
	return cache
}

// ModelRouteRuleSaturated reports whether the rule-total cap is already full.
// Fail-open: Redis errors or missing cache return false.
func (s *ConcurrencyService) ModelRouteRuleSaturated(ctx context.Context, groupID int64, pattern string, maxConcurrency int) bool {
	if maxConcurrency <= 0 {
		return false
	}
	cache := s.modelRouteCache()
	if cache == nil {
		return false
	}
	total, err := cache.GetModelRouteTotalConcurrency(ctx, groupID, pattern)
	if err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: get model route total failed group=%d pattern=%s: %v", groupID, pattern, err)
		return false
	}
	return total >= maxConcurrency
}

// ModelRouteAccountSaturated reports whether a preferred account's rule concurrency is full.
func (s *ConcurrencyService) ModelRouteAccountSaturated(ctx context.Context, groupID int64, pattern string, accountID int64, maxConcurrency int) bool {
	if maxConcurrency <= 0 {
		return false
	}
	cache := s.modelRouteCache()
	if cache == nil {
		return false
	}
	count, err := cache.GetModelRouteAccountConcurrency(ctx, groupID, pattern, accountID)
	if err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: get model route account failed group=%d account=%d: %v", groupID, accountID, err)
		return false
	}
	return count >= maxConcurrency
}

// AcquireModelRouteSlot acquires rule/account model-routing counters.
// When both caps are unlimited (≤0), returns a no-op success without Redis.
func (s *ConcurrencyService) AcquireModelRouteSlot(
	ctx context.Context,
	groupID int64,
	pattern string,
	accountID int64,
	accountMax int,
	totalMax int,
) (*AcquireResult, error) {
	if accountMax <= 0 && totalMax <= 0 {
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {}}, nil
	}
	cache := s.modelRouteCache()
	if cache == nil {
		// Fail open when Redis model-route backend is unavailable.
		return &AcquireResult{Acquired: true, ReleaseFunc: func() {}}, nil
	}
	requestID := generateRequestID()
	acquired, err := cache.AcquireModelRouteSlot(ctx, groupID, pattern, accountID, accountMax, totalMax, requestID)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return &AcquireResult{Acquired: false}, nil
	}
	return &AcquireResult{
		Acquired: true,
		ReleaseFunc: func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := cache.ReleaseModelRouteSlot(bgCtx, groupID, pattern, accountID, requestID); err != nil {
				logger.LegacyPrintf("service.concurrency", "Warning: release model route slot group=%d account=%d req=%s: %v", groupID, accountID, requestID, err)
			}
		},
	}, nil
}

func chainReleaseFuncs(funcs ...func()) func() {
	var active []func()
	for _, f := range funcs {
		if f != nil {
			active = append(active, f)
		}
	}
	if len(active) == 0 {
		return func() {}
	}
	if len(active) == 1 {
		return active[0]
	}
	return func() {
		for i := len(active) - 1; i >= 0; i-- {
			active[i]()
		}
	}
}
