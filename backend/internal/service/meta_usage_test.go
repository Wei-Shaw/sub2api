//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	metapkg "github.com/Wei-Shaw/sub2api/internal/pkg/meta"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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
	require.Empty(t, usage.SubscriptionTierRaw)
	require.Equal(t, "tier-1", usage.SubscriptionTierID)
	require.NotNil(t, usage.UpdatedAt)

	passive, err := svc.getPassiveUsageForAccount(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, "passive", passive.Source)
	require.NotNil(t, passive.SevenDay)
}

func TestGetMetaUsageNamesTierFromMintedCredentials(t *testing.T) {
	const maxID = "28367280949604159"
	const powerID = "27681631238169137"
	cases := []struct {
		name        string
		credentials map[string]any
		sampled     string
		wantName    string
		wantID      string
	}{
		{"sampled id matches minted id", map[string]any{"subs_tier_id": maxID, "subs_tier_name": "Muse Code Max Usage"}, maxID, "Muse Code Max Usage", maxID},
		{"no sample yet uses minted plan", map[string]any{"subs_tier_id": powerID, "subs_tier_name": "Muse Code Power Usage"}, "", "Muse Code Power Usage", powerID},
		{"tier changed since mint", map[string]any{"subs_tier_id": powerID, "subs_tier_name": "Muse Code Power Usage"}, maxID, "", maxID},
		{"api key account without mint", map[string]any{"api_key": "sk-muse"}, powerID, "", powerID},
		{"minted id without name", map[string]any{"subs_tier_id": maxID}, maxID, "", maxID},
		{"nothing known", map[string]any{"api_key": "sk-muse"}, "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := metaTestAccount(tc.credentials, AccountTypeOAuth)
			account.Extra = map[string]any{}
			if tc.sampled != "" {
				account.Extra[metaUsageTierKey] = " " + tc.sampled + " "
			}
			svc := &AccountUsageService{
				accountRepo:  &metaAccountRepoStub{account: account},
				usageLogRepo: &usageBatchLogRepoStub{},
				cache:        NewUsageCache(),
			}
			usage, err := svc.getMetaUsage(context.Background(), account)
			require.NoError(t, err)
			require.Equal(t, tc.wantName, usage.SubscriptionTierRaw)
			require.Equal(t, tc.wantID, usage.SubscriptionTierID)
		})
	}
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
	require.Empty(t, usage.SubscriptionTierID)
}

func TestStripMetaSubscriptionUsage(t *testing.T) {
	body := []byte(`{"type":"response.completed","subscription":{"tier":"muse-pro","window":{"resets_at":1792272519,"used_percent":42}},"usage":{"input_tokens":1}}`)
	stripped, usage := stripMetaSubscriptionUsage(body)
	require.NotNil(t, usage)
	require.Equal(t, "muse-pro", usage.Tier)
	require.False(t, gjson.GetBytes(stripped, "subscription").Exists())
	require.True(t, gjson.GetBytes(stripped, "usage.input_tokens").Exists())

	untouched, nilUsage := stripMetaSubscriptionUsage([]byte(`{"type":"response.completed"}`))
	require.Nil(t, nilUsage)
	require.Equal(t, `{"type":"response.completed"}`, string(untouched))

	keptBody, parsed := stripMetaSubscriptionUsage([]byte(`{"subscription":{"tier":"muse-pro"}}`))
	require.Nil(t, parsed)
	require.False(t, gjson.GetBytes(keptBody, "subscription").Exists())
}
