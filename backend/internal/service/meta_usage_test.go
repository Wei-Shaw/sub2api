//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	metapkg "github.com/Wei-Shaw/sub2api/internal/pkg/meta"
	"github.com/stretchr/testify/require"
)

func TestMetaSubscriptionUsageExtraUpdates(t *testing.T) {
	now := time.Now()
	windowReset := now.Add(2 * time.Hour).Unix()
	weeklyReset := now.Add(3 * 24 * time.Hour).Unix()
	updates := metaSubscriptionUsageExtraUpdates(&metapkg.SubscriptionUsage{
		Tier:   "tier-1",
		Window: &metapkg.SubscriptionUsageWindow{ResetsAt: windowReset, UsedPercent: 10},
		Weekly: &metapkg.SubscriptionUsageWindow{ResetsAt: weeklyReset, UsedPercent: 80},
	}, now)
	require.Equal(t, 0.10, updates[metaUsageWindowUtilizationKey])
	require.Equal(t, windowReset, updates[metaUsageWindowResetKey])
	require.Equal(t, 0.80, updates[metaUsageWeeklyUtilizationKey])
	require.Equal(t, weeklyReset, updates[metaUsageWeeklyResetKey])
	require.Equal(t, "tier-1", updates[metaUsageTierKey])

	stale := metaSubscriptionUsageExtraUpdates(&metapkg.SubscriptionUsage{
		Window: &metapkg.SubscriptionUsageWindow{ResetsAt: now.Add(-48 * time.Hour).Unix(), UsedPercent: 5},
	}, now)
	require.Empty(t, stale)

	require.Empty(t, metaSubscriptionUsageExtraUpdates(&metapkg.SubscriptionUsage{}, now))
}

func TestGetMetaUsageReadsSampledWindows(t *testing.T) {
	now := time.Now()
	account := metaTestAccount(map[string]any{"api_key": "sk-muse"}, AccountTypeAPIKey)
	account.Extra = map[string]any{
		metaUsageWindowUtilizationKey: 0.25,
		metaUsageWindowResetKey:       float64(now.Add(time.Hour).Unix()),
		metaUsageWeeklyUtilizationKey: 0.80,
		metaUsageWeeklyResetKey:       float64(now.Add(72 * time.Hour).Unix()),
		metaUsageTierKey:              "tier-1",
		metaUsageSampledAtKey:         now.Add(-5 * time.Minute).UTC().Format(time.RFC3339),
	}

	svc := &AccountUsageService{
		accountRepo:  &metaAccountRepoStub{account: account},
		usageLogRepo: &usageBatchLogRepoStub{},
		cache:        NewUsageCache(),
	}
	usage, err := svc.getUsageForAccount(context.Background(), account, false)
	require.NoError(t, err)
	require.Equal(t, "passive", usage.Source)
	require.NotNil(t, usage.FiveHour)
	require.InDelta(t, 25.0, usage.FiveHour.Utilization, 0.001)
	require.NotNil(t, usage.SevenDay)
	require.InDelta(t, 80.0, usage.SevenDay.Utilization, 0.001)
	require.Equal(t, "tier-1", usage.SubscriptionTierRaw)
	require.NotNil(t, usage.UpdatedAt)

	passive, err := svc.getPassiveUsageForAccount(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "passive", passive.Source)
	require.NotNil(t, passive.SevenDay)
}

func TestMetaUsageEmptyBeforeFirstSample(t *testing.T) {
	account := metaTestAccount(map[string]any{"api_key": "sk-muse"}, AccountTypeAPIKey)
	svc := &AccountUsageService{
		accountRepo:  &metaAccountRepoStub{account: account},
		usageLogRepo: &usageBatchLogRepoStub{},
		cache:        NewUsageCache(),
	}
	usage, err := svc.getMetaUsage(context.Background(), account)
	require.NoError(t, err)
	require.NotNil(t, usage.FiveHour)
	require.Zero(t, usage.FiveHour.Utilization)
	require.Nil(t, usage.SevenDay)
	require.Empty(t, usage.SubscriptionTierRaw)
}
