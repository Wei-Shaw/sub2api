package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func videoTaskQuoteFixture() (*OpenAIGatewayService, *OpenAIRecordUsageInput, *videoUsageAtomicBillingRepoStub, *openAIRecordUsageLogRepoStub) {
	logs := &openAIRecordUsageLogRepoStub{}
	billing := &videoUsageAtomicBillingRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(logs, billing, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	groupID := int64(112)
	input := &OpenAIRecordUsageInput{
		RequireAtomicVideo: true,
		Result: &OpenAIForwardResult{
			Model: "grok-imagine-video-1.5(zj)", VideoCount: 1,
			VideoResolution: "720p", VideoDurationSeconds: 30,
		},
		APIKey: &APIKey{ID: 2, UserID: 1, Key: "secret-not-in-snapshot", GroupID: &groupID,
			Group: &Group{ID: groupID, Platform: PlatformOpenAI, RateMultiplier: 1}},
		User: &User{ID: 1}, Account: &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		PricingAt: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC),
	}
	return svc, input, billing, logs
}

func TestVideoTaskCost_KnownDefaultAndUncappedOpenAIDuration(t *testing.T) {
	svc, input, billing, logs := videoTaskQuoteFixture()
	snapshot, err := svc.ResolveVideoTaskCost(context.Background(), input)
	require.NoError(t, err)
	require.InDelta(t, 0.14*30, snapshot.Cost.TotalCost, 1e-12)
	require.Equal(t, QuantizeUsageBillingAmount(0.14*30), snapshot.Cost.ActualCost)
	require.Equal(t, 30, *snapshot.Log.VideoDurationSeconds)
	require.Zero(t, billing.calls, "quoting must never charge")
	require.Zero(t, logs.calls, "quoting must never create usage")
	body, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.NotContains(t, string(body), input.APIKey.Key)
	var restored VideoTaskCostSnapshot
	require.NoError(t, json.Unmarshal(body, &restored))
	input.VideoTaskCost = &restored
	input.Result.RequestID = "grok-video:known-default-task"
	require.NoError(t, svc.RecordUsage(context.Background(), input))
	require.Equal(t, snapshot.Command.BalanceCost, billing.lastCmd.BalanceCost)
	require.Equal(t, 30, *billing.lastCmd.VideoUsageLog.VideoDurationSeconds)
	require.Zero(t, logs.calls)

	// The legacy calculator still retains its original Grok 15-second limit.
	legacy := svc.billingService.CalculateVideoCost(input.Result.Model, "720p", 1, 30, nil, 1)
	require.InDelta(t, 0.14*15, legacy.TotalCost, 1e-12)
}

func TestVideoTaskCost_RejectsUnknownAndMissingSnapshotBeforeBilling(t *testing.T) {
	svc, input, billing, logs := videoTaskQuoteFixture()
	input.Result.Model = "unconfigured-video-model"
	snapshot, err := svc.ResolveVideoTaskCost(context.Background(), input)
	require.ErrorIs(t, err, ErrModelPricingUnavailable)
	require.Nil(t, snapshot)
	require.ErrorContains(t, svc.RecordUsage(context.Background(), input), "preflight cost snapshot")
	require.Zero(t, billing.calls)
	require.Zero(t, logs.calls)
}

func TestVideoTaskCost_FrozenAmountsSurviveAllLivePolicyChanges(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		name := "balance"
		if subscription {
			name = "subscription"
		}
		t.Run(name, func(t *testing.T) {
			svc, input, billing, logs := videoTaskQuoteFixture()
			price, accountRate := 0.125, 0.5
			input.APIKey.Group.VideoPrice720P = &price
			input.APIKey.Group.VideoRateIndependent = true
			input.APIKey.Group.VideoRateMultiplier = 0.25
			input.APIKey.Quota, input.APIKey.RateLimit5h = 10, 10
			input.Account.RateMultiplier = &accountRate
			input.Account.Extra = map[string]any{"quota_limit": 100.0}
			if subscription {
				input.APIKey.Group.SubscriptionType = SubscriptionTypeSubscription
				input.Subscription = &UserSubscription{ID: 7, UserID: 1, GroupID: 112}
			}
			snapshot, err := svc.ResolveVideoTaskCost(context.Background(), input)
			require.NoError(t, err)
			require.Greater(t, snapshot.Command.APIKeyQuotaCost, 0.0)
			require.Greater(t, snapshot.Command.APIKeyRateLimitCost, 0.0)
			require.Greater(t, snapshot.Command.AccountQuotaCost, 0.0)
			input.VideoTaskCost = snapshot
			input.Result.RequestID = "grok-video:frozen-" + name
			require.NoError(t, svc.RecordUsage(context.Background(), input))
			first := *billing.lastCmd
			first.VideoUsageLog = nil

			// RecordUsage may still notify quota observers with the old account.
			// A configuration reload supplies a fresh object, not an in-place write.
			nextAccount := *input.Account
			input.Account = &nextAccount
			price = 999
			nextAccountRate := 999.0
			input.Account.RateMultiplier = &nextAccountRate
			input.APIKey.Group.RateMultiplier = 999
			input.APIKey.Group.VideoRateMultiplier = 999
			input.APIKey.Group.SubscriptionType = "changed-after-acceptance"
			input.APIKey.Quota, input.APIKey.RateLimit5h = 0, 0
			input.Account.Extra = nil
			input.Account.Type = AccountTypeOAuth
			newGroupID := int64(999)
			input.APIKey.GroupID = &newGroupID
			input.APIKey.Group = nil
			// No pricing dependencies remain at settlement: a reprice would panic.
			svc.billingService, svc.resolver, svc.channelService = nil, nil, nil
			billing.result = &UsageBillingApplyResult{Applied: false}
			require.NoError(t, svc.RecordUsage(context.Background(), input))
			retry := *billing.lastCmd
			retry.VideoUsageLog = nil
			require.Equal(t, first, retry)
			require.Equal(t, int64(112), *billing.lastCmd.VideoUsageLog.GroupID)
			require.Equal(t, snapshot.Cost.ActualCost, billing.lastCmd.VideoUsageLog.ActualCost)
			require.Zero(t, logs.calls)
		})
	}
}

func TestVideoTaskCost_ExplicitFreeAndConfiguredOpenAITier(t *testing.T) {
	svc, input, _, _ := videoTaskQuoteFixture()
	input.Result.Model, input.Result.VideoResolution = "seedance-custom", "4k"
	price := 0.2
	input.APIKey.Group.ModelPricing = []ChannelModelPricing{{
		Models: []string{"seedance-custom"}, BillingMode: BillingModeVideo, PerRequestPrice: &price,
	}}
	svc.resolver = NewModelPricingResolver(nil, svc.billingService)
	snapshot, err := svc.ResolveVideoTaskCost(context.Background(), input)
	require.NoError(t, err)
	require.InDelta(t, 6.0, snapshot.Cost.TotalCost, 1e-12)
	require.Equal(t, "4k", *snapshot.Log.VideoResolution)
	input.APIKey.Group.ModelPricing[0].PerRequestPrice = nil
	_, err = svc.ResolveVideoTaskCost(context.Background(), input)
	require.ErrorIs(t, err, ErrModelPricingUnavailable, "an empty configured price is not an explicit free quote")
	input.APIKey.Group.ModelPricing[0].PerRequestPrice = &price
	price = 0
	snapshot, err = svc.ResolveVideoTaskCost(context.Background(), input)
	require.NoError(t, err)
	require.Zero(t, snapshot.Cost.TotalCost)
	require.Zero(t, snapshot.Command.BalanceCost)
}

func TestVideoTaskCost_ChannelPerRequestDoesNotMultiplySeconds(t *testing.T) {
	svc, input, _, _ := videoTaskQuoteFixture()
	input.Result.Model = "custom-video-per-request"
	svc.resolver = newOpenAIImageChannelPricingResolverForTest(t, 112, input.Result.Model, 0.75)
	snapshot, err := svc.ResolveVideoTaskCost(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 0.75, snapshot.Cost.TotalCost)
	svc.resolver = newOpenAITokenImageChannelPricingResolverForTest(t, 112, input.Result.Model)
	_, err = svc.ResolveVideoTaskCost(context.Background(), input)
	require.ErrorContains(t, err, "cannot freeze video token pricing")
}

func TestVideoTaskCost_RejectsWrongOwnerAndCorruptSnapshot(t *testing.T) {
	svc, input, billing, _ := videoTaskQuoteFixture()
	snapshot, err := svc.ResolveVideoTaskCost(context.Background(), input)
	require.NoError(t, err)
	input.VideoTaskCost = snapshot
	input.Result.RequestID = "grok-video:owner-check"
	input.User.ID++
	require.ErrorContains(t, svc.RecordUsage(context.Background(), input), "owner or account mismatch")
	input.User.ID--
	snapshot.Command.BalanceCost++
	require.ErrorContains(t, svc.RecordUsage(context.Background(), input), "charge does not match")
	require.Zero(t, billing.calls)
}
