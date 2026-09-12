package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type schedulerLatencyAccountRepo struct{ schedulerTestOpenAIAccountRepo }

func (r schedulerLatencyAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	time.Sleep(20 * time.Millisecond)
	return r.schedulerTestOpenAIAccountRepo.ListSchedulableByPlatform(ctx, platform)
}

func (r schedulerLatencyAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}

func TestOpenAISchedulerSelectReturnsRealLatency(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	for _, withAccount := range []bool{false, true} {
		name := "no_available_account"
		var accounts []Account
		if withAccount {
			name = "selected_account"
			accounts = []Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 5}}
		}
		t.Run(name, func(t *testing.T) {
			scheduler := &defaultOpenAIAccountScheduler{
				service: &OpenAIGatewayService{accountRepo: schedulerLatencyAccountRepo{schedulerTestOpenAIAccountRepo{accounts: accounts}}},
				stats:   newOpenAIAccountRuntimeStats(),
			}
			selection, decision, err := scheduler.Select(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI})
			if withAccount {
				require.NoError(t, err)
				require.NotNil(t, selection)
				require.Equal(t, int64(1), decision.SelectedAccountID)
				if selection.ReleaseFunc != nil {
					t.Cleanup(selection.ReleaseFunc)
				}
			} else {
				require.Error(t, err)
			}
			require.GreaterOrEqual(t, decision.LatencyMs, int64(20))
			metrics := scheduler.SnapshotMetrics()
			require.Equal(t, decision.LatencyMs, metrics.SchedulerLatencyMsTotal)
			require.Equal(t, float64(decision.LatencyMs), metrics.SchedulerLatencyMsAvg)
		})
	}
}

func TestOpenAISchedulerStickyHitRatioCountsSelectionOnce(t *testing.T) {
	scheduler := &defaultOpenAIAccountScheduler{stats: newOpenAIAccountRuntimeStats()}
	scheduler.metrics.recordSelect(OpenAIAccountScheduleDecision{StickyPreviousHit: true, StickySessionHit: true})
	snapshot := scheduler.SnapshotMetrics()
	require.Equal(t, int64(1), snapshot.SelectTotal)
	require.Equal(t, int64(1), snapshot.StickyPreviousHitTotal)
	require.Equal(t, int64(1), snapshot.StickySessionHitTotal)
	require.Equal(t, 1.0, snapshot.StickyHitRatio)

	scheduler.metrics.recordSelect(OpenAIAccountScheduleDecision{StickyPreviousHit: true})
	scheduler.metrics.recordSelect(OpenAIAccountScheduleDecision{StickySessionHit: true})
	scheduler.metrics.recordSelect(OpenAIAccountScheduleDecision{})
	snapshot = scheduler.SnapshotMetrics()
	require.Equal(t, int64(4), snapshot.SelectTotal)
	require.Equal(t, int64(2), snapshot.StickyPreviousHitTotal)
	require.Equal(t, int64(2), snapshot.StickySessionHitTotal)
	require.Equal(t, 0.75, snapshot.StickyHitRatio)
}
