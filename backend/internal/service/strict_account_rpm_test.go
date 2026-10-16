//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type strictRPMTestCache struct {
	RPMCache
	mu     sync.Mutex
	counts map[int64]int
	err    error
}

func (c *strictRPMTestCache) IncrementRPM(ctx context.Context, id int64) (int, error) {
	return c.GetRPM(ctx, id) // the strict reservation has already counted it
}

func (c *strictRPMTestCache) GetRPM(_ context.Context, id int64) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[id], c.err
}

func (c *strictRPMTestCache) GetRPMBatch(_ context.Context, ids []int64) (map[int64]int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	counts := map[int64]int{}
	for _, id := range ids {
		counts[id] = c.counts[id]
	}
	return counts, c.err
}

func (c *strictRPMTestCache) ReserveRPM(_ context.Context, id int64, limit int) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return false, c.err
	}
	if c.counts[id] >= limit {
		return false, nil
	}
	c.counts[id]++
	return true, nil
}

func TestMiniMaxRPMAdmissionConcurrentPoolAndSlotRelease(t *testing.T) {
	cache := &strictRPMTestCache{counts: map[int64]int{}}
	accounts := []*Account{
		{ID: 1, Platform: PlatformMiniMax, Extra: map[string]any{"base_rpm": 10, "rpm_strategy": "sticky_exempt"}},
		{ID: 2, Platform: PlatformMiniMax, Extra: map[string]any{"base_rpm": 15, "rpm_sticky_buffer": 100}},
	}
	excluded := map[int64]struct{}{99: {}}
	var admitted, slots, unexpected atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			selection, err := selectWithMiniMaxRPMReservation(context.Background(), cache, excluded, func(skip map[int64]struct{}) (*AccountSelectionResult, error) {
				for _, account := range accounts {
					if _, ok := skip[account.ID]; ok {
						continue
					}
					slots.Add(1)
					return &AccountSelectionResult{Account: account, Acquired: true, ReleaseFunc: func() { slots.Add(-1) }}, nil
				}
				return nil, ErrNoAvailableAccounts
			})
			if err == nil {
				admitted.Add(1)
				selection.ReleaseFunc()
			} else if !errors.Is(err, ErrNoAvailableAccounts) {
				unexpected.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 25, admitted.Load(), "sticky/buffer requests must not exceed MiniMax's hard provider caps")
	require.Zero(t, unexpected.Load())
	require.Zero(t, slots.Load(), "RPM rejection must release the acquired concurrency slot")
	require.Len(t, excluded, 1, "per-request RPM exclusions must not mutate caller state")
}

func TestMiniMaxRPMCacheFailureDoesNotAdmitOrLeakSlot(t *testing.T) {
	cache := &strictRPMTestCache{err: errors.New("redis unavailable")}
	released := false
	selection, err := selectWithMiniMaxRPMReservation(context.Background(), cache, nil, func(map[int64]struct{}) (*AccountSelectionResult, error) {
		return &AccountSelectionResult{Account: &Account{ID: 1, Platform: PlatformMiniMax, Extra: map[string]any{"base_rpm": 100}}, ReleaseFunc: func() { released = true }}, nil
	})
	require.ErrorContains(t, err, "cache unavailable")
	require.Nil(t, selection)
	require.True(t, released)
}

func TestMiniMaxRPMPolicyLeavesOtherProvidersUnchanged(t *testing.T) {
	for _, platform := range []string{PlatformAnthropic, PlatformOpenAI} {
		account := &Account{ID: 1, Platform: platform, Extra: map[string]any{"base_rpm": 100}}
		selection, err := selectWithMiniMaxRPMReservation(context.Background(), nil, nil, func(map[int64]struct{}) (*AccountSelectionResult, error) {
			return &AccountSelectionResult{Account: account}, nil
		})
		require.NoError(t, err)
		require.Same(t, account, selection.Account)
	}
}

func TestMiniMaxRPMAdmissionLegacyAndAdvancedScheduler(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		name := "legacy"
		if advanced {
			name = "advanced"
		}
		t.Run(name, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			groupID := int64(42003)
			accounts := []Account{
				rpmTestAPIKeyAccount(42031, PlatformMiniMax, 0, map[string]any{"base_rpm": 4}),
				rpmTestAPIKeyAccount(42032, PlatformMiniMax, 0, map[string]any{"base_rpm": 6}),
			}
			cfg := &config.Config{}
			cfg.Gateway.Scheduling.LoadBatchEnabled = false
			cfg.Gateway.OpenAIWS.LBTopK = 2
			cache := &strictRPMTestCache{counts: map[int64]int{}}
			svc := &OpenAIGatewayService{
				accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
				cache:       &schedulerTestGatewayCache{}, cfg: cfg,
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
			}
			if advanced {
				svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
			}
			svc.SetRPMCache(cache)
			accepted := 0
			for range 30 {
				selection, _, err := svc.SelectAccountWithSchedulerForCapability(context.Background(), &groupID, "", "", "MiniMax-M3.1-Flash-Preview", nil, OpenAIUpstreamTransportAny, "", false, false, false, PlatformMiniMax)
				if err != nil {
					require.Contains(t, err.Error(), "rpm_rate_limited")
					continue
				}
				accepted++
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
			}
			require.Equal(t, 10, accepted)
		})
	}
}
