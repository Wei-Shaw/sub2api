//go:build unit

package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSetAccountModelRateLimitSnapshot_CopyOnWrite(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	existingLimit := map[string]any{
		"rate_limited_at":     now.Format(time.RFC3339),
		"rate_limit_reset_at": now.Add(time.Hour).Format(time.RFC3339),
	}
	limits := map[string]any{"claude-opus-5": existingLimit}
	extra := map[string]any{
		modelRateLimitsKey:           limits,
		"session_window_utilization": 0.5,
	}
	account := &Account{ID: 1, Platform: PlatformAnthropic, Extra: extra}
	before := *account

	setAccountModelRateLimitSnapshot(account, anthropicFableRateLimitKey, now.Add(2*time.Hour), "  threshold  ", now)

	// 调用前的 Extra 与嵌套 limits 原样保留。
	require.Len(t, extra, 2)
	require.Equal(t, 0.5, extra["session_window_utilization"])
	require.Len(t, limits, 1)
	require.Equal(t, existingLimit, limits["claude-opus-5"])
	require.NotContains(t, limits, anthropicFableRateLimitKey)
	require.False(t, before.isModelRateLimitedWithContext(ctx, "claude-fable-5"))

	// 新 Extra 同时带有旧条目与新限流，本请求后续读取结果与原地写入一致。
	require.Equal(t, 0.5, account.Extra["session_window_utilization"])
	newLimits, ok := account.Extra[modelRateLimitsKey].(map[string]any)
	require.True(t, ok)
	require.Len(t, newLimits, 2)
	require.Equal(t, existingLimit, newLimits["claude-opus-5"])
	require.Equal(t, map[string]any{
		"rate_limited_at":     now.Format(time.RFC3339),
		"rate_limit_reset_at": now.Add(2 * time.Hour).Format(time.RFC3339),
		"reason":              "threshold",
	}, newLimits[anthropicFableRateLimitKey])
	require.Equal(t, "threshold", account.modelRateLimitReason(anthropicFableRateLimitKey))
	require.True(t, account.isModelRateLimitedWithContext(ctx, "claude-fable-5"))
	require.True(t, account.isModelRateLimitedWithContext(ctx, "claude-opus-5"))
	require.False(t, account.isModelRateLimitedWithContext(ctx, "claude-sonnet-5"))
}

func TestSetAccountModelRateLimitSnapshot_InitializesMissingOrInvalidLimits(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	empty := &Account{Platform: PlatformAnthropic}
	setAccountModelRateLimitSnapshot(empty, "claude-opus-5", now.Add(time.Hour), "", now)
	require.True(t, empty.isModelRateLimitedWithContext(ctx, "claude-opus-5"))
	require.Empty(t, empty.modelRateLimitReason("claude-opus-5"))

	invalidExtra := map[string]any{modelRateLimitsKey: "corrupted", "keep": true}
	invalid := &Account{Platform: PlatformAnthropic, Extra: invalidExtra}
	setAccountModelRateLimitSnapshot(invalid, "claude-opus-5", now.Add(time.Hour), "r", now)
	require.True(t, invalid.isModelRateLimitedWithContext(ctx, "claude-opus-5"))
	require.Equal(t, true, invalid.Extra["keep"])
	require.Equal(t, "corrupted", invalidExtra[modelRateLimitsKey])

	unchanged := &Account{Platform: PlatformAnthropic, Extra: map[string]any{"k": 1}}
	setAccountModelRateLimitSnapshot(unchanged, "   ", now.Add(time.Hour), "r", now)
	require.Equal(t, map[string]any{"k": 1}, unchanged.Extra)
}

// 同一母本的浅拷贝并发写各自的限流快照：-race 下不得出现竞争，母本与其它副本都看不到这些写入。
func TestSetAccountModelRateLimitSnapshot_ConcurrentCopiesOfSharedMaster(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	master := &Account{
		ID:       7,
		Platform: PlatformAnthropic,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				"claude-opus-5": map[string]any{"rate_limit_reset_at": now.Add(time.Hour).Format(time.RFC3339)},
			},
			"session_window_utilization": 0.2,
		},
	}
	master.PrepareSharedSnapshot()

	const workers = 16
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			own := fmt.Sprintf("model-%d", worker)
			other := fmt.Sprintf("model-%d", (worker+1)%workers)
			for i := 0; i < 50; i++ {
				replica := *master
				setAccountModelRateLimitSnapshot(&replica, own, now.Add(time.Hour), "r", now)
				if !replica.isModelRateLimitedWithContext(ctx, own) {
					t.Errorf("worker %d: own limit missing", worker)
					return
				}
				if !replica.isModelRateLimitedWithContext(ctx, "claude-opus-5") {
					t.Errorf("worker %d: inherited limit missing", worker)
					return
				}
				if replica.isModelRateLimitedWithContext(ctx, other) {
					t.Errorf("worker %d: saw another replica's limit", worker)
					return
				}
				probe := *master
				if probe.isModelRateLimitedWithContext(ctx, own) {
					t.Errorf("worker %d: master was mutated", worker)
					return
				}
			}
		}(worker)
	}
	wg.Wait()

	limits, ok := master.Extra[modelRateLimitsKey].(map[string]any)
	require.True(t, ok)
	require.Len(t, limits, 1)
	require.Len(t, master.Extra, 2)
}

type sharedSnapshotFableRepoStub struct {
	AccountRepository
	modelCalls atomic.Int64
}

func (r *sharedSnapshotFableRepoStub) SetModelRateLimit(_ context.Context, _ int64, _ string, _ time.Time, _ ...string) error {
	r.modelCalls.Add(1)
	return nil
}

// Fable 阈值在选号阶段写入候选对象：同一母本的其它副本（包括并发读取的副本）与母本本身都不得出现该限流。
func TestApplyAccountSchedulingThreshold_FableLimitStaysOnOwnSnapshotCopy(t *testing.T) {
	accountSchedulingThresholdsSF.Forget(SettingKeyAccountSchedulingThresholds)
	accountSchedulingThresholdsCache.Store(&cachedAccountSchedulingThresholds{})
	t.Cleanup(func() {
		accountSchedulingThresholdsSF.Forget(SettingKeyAccountSchedulingThresholds)
		accountSchedulingThresholdsCache.Store(&cachedAccountSchedulingThresholds{})
	})

	settingsRepo := newMockSettingRepo()
	settingsRepo.data[SettingKeyAccountSchedulingThresholds] = `{"anthropic":100}`
	repo := &sharedSnapshotFableRepoStub{}
	limiter := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	limiter.SetSettingService(NewSettingService(settingsRepo, &config.Config{}))

	now := time.Now().UTC()
	master := &Account{
		ID:          1006,
		Platform:    PlatformAnthropic,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"account_scheduling_threshold": 60},
		Extra: map[string]any{
			"passive_usage_7d_utilization":    0.40,
			"passive_usage_7d_reset":          float64(now.Add(3 * 24 * time.Hour).Unix()),
			"passive_usage_7d_oi_utilization": 0.61,
			"passive_usage_7d_oi_reset":       float64(now.Add(4 * 24 * time.Hour).Unix()),
		},
	}
	master.PrepareSharedSnapshot()

	const pairs = 8
	var wg sync.WaitGroup
	for i := 0; i < pairs; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			candidate := *master
			if limiter.ApplyAccountSchedulingThreshold(context.Background(), &candidate) {
				t.Error("the Fable-only window must not pause the whole account")
			}
			if candidate.IsSchedulableForModel("claude-fable-5") {
				t.Error("the candidate that crossed the threshold must stop scheduling Fable")
			}
			if !candidate.IsSchedulableForModel("claude-opus-5") {
				t.Error("non-Fable models must stay schedulable")
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				reader := *master
				if !reader.IsSchedulableForModel("claude-fable-5") {
					t.Error("a sibling copy must not see the Fable limit")
					return
				}
			}
		}()
	}
	wg.Wait()

	require.Equal(t, int64(pairs), repo.modelCalls.Load())
	require.NotContains(t, master.Extra, modelRateLimitsKey)
	require.True(t, master.IsSchedulableForModel("claude-fable-5"))
}
