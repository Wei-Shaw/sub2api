//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type mediaAppliedRepo struct {
	MiniMaxMediaRepository
	calls int
}

type mediaQuotaCache struct{ fakeIncrCache }

func (*mediaQuotaCache) GetUserPlatformQuotaCache(context.Context, int64, string) (*UserPlatformQuotaCacheEntry, bool, error) {
	return &UserPlatformQuotaCacheEntry{DailyLimitUSD: mediaFloat(10)}, true, nil
}

func (r *mediaAppliedRepo) Settle(context.Context, string) (bool, error) {
	r.calls++
	return r.calls == 1, nil
}

func TestMiniMaxMediaPlatformQuotaAppliesOnlyOnce(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		t.Run(map[bool]string{false: "wallet", true: "subscription exempt"}[subscription], func(t *testing.T) {
			cfg := &config.Config{}
			cache := &mediaQuotaCache{}
			quotaRepo := &fakeQuotaRepo{}
			repo := &mediaAppliedRepo{}
			s := NewMiniMaxMediaService(repo, &OpenAIGatewayService{cfg: cfg, userPlatformQuotaRepo: quotaRepo, billingCacheService: &BillingCacheService{cfg: cfg, cache: cache}}, nil)
			defer s.cancel()
			task := &MiniMaxMediaTask{ID: "mm_quota", UserID: 123, Bill: &MiniMaxMediaBill{ChargedUSD: 3}}
			if subscription {
				id := int64(1)
				task.SubscriptionID = &id
			}
			require.NoError(t, s.settle(context.Background(), task))
			require.NoError(t, s.settle(context.Background(), task))
			if subscription {
				require.Empty(t, cache.calls)
			} else {
				require.Len(t, cache.calls, 1)
				require.Equal(t, PlatformMiniMax, cache.calls[0].platform)
				require.Equal(t, 3.0, cache.calls[0].cost)
			}
		})
	}
}
