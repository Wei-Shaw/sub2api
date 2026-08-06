package service

import (
	"context"
	"errors"
	"math"

	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func profitControlTestGroup(id int64, margin, buffer float64) *Group {
	return &Group{
		ID:                   id,
		Platform:             PlatformOpenAI,
		Status:               StatusActive,
		Hydrated:             true,
		RateMultiplier:       1.0,
		SubscriptionType:     SubscriptionTypeStandard,
		ProfitControlEnabled: true,
		ProfitMinMargin:      margin,
		ProfitSafetyBuffer:   buffer,
	}
}

func profitControlTestCtx(group *Group) context.Context {
	return context.WithValue(context.Background(), ctxkey.Group, group)
}

func profitControlTestAccountWithRate(account *Account, rate float64) *Account {
	account.RateMultiplier = &rate
	return account
}

func TestResolveOpenAIProfitControlGate(t *testing.T) {
	svc := &OpenAIGatewayService{}
	groupID := int64(7)

	t.Run("nil group id yields no gate", func(t *testing.T) {
		require.Nil(t, svc.resolveOpenAIProfitControlGate(context.Background(), nil))
	})

	t.Run("no ctx group and no snapshot yields no gate", func(t *testing.T) {
		require.Nil(t, svc.resolveOpenAIProfitControlGate(context.Background(), &groupID))
	})

	t.Run("disabled group yields no gate", func(t *testing.T) {
		group := profitControlTestGroup(groupID, 0.3, 0)
		group.ProfitControlEnabled = false
		require.Nil(t, svc.resolveOpenAIProfitControlGate(profitControlTestCtx(group), &groupID))
	})

	t.Run("non openai or grok platform yields no gate even if enabled", func(t *testing.T) {
		group := profitControlTestGroup(groupID, 0.3, 0)
		group.Platform = PlatformAnthropic
		require.Nil(t, svc.resolveOpenAIProfitControlGate(profitControlTestCtx(group), &groupID))
	})

	t.Run("grok group routed through openai handler installs gate", func(t *testing.T) {
		group := profitControlTestGroup(groupID, 0.3, 0.05)
		group.Platform = PlatformGrok
		group.RateMultiplier = 0.5
		gate := svc.resolveOpenAIProfitControlGate(profitControlTestCtx(group), &groupID)
		require.NotNil(t, gate)
		require.Equal(t, PlatformGrok, gate.platform)
		require.InDelta(t, 0.5*(1-0.35), gate.threshold, 1e-12)
	})

	t.Run("ctx group id mismatch without snapshot yields no gate", func(t *testing.T) {
		group := profitControlTestGroup(groupID+1, 0.3, 0)
		require.Nil(t, svc.resolveOpenAIProfitControlGate(profitControlTestCtx(group), &groupID))
	})

	t.Run("threshold composes margin and buffer from downstream rate", func(t *testing.T) {
		group := profitControlTestGroup(groupID, 0.3, 0.05)
		group.RateMultiplier = 2.0
		gate := svc.resolveOpenAIProfitControlGate(profitControlTestCtx(group), &groupID)
		require.NotNil(t, gate)
		require.InDelta(t, 2.0*(1-0.35), gate.threshold, 1e-12)
		require.Equal(t, PlatformOpenAI, gate.platform)
		require.False(t, gate.pricingAt.IsZero())
		require.Equal(t, groupID, gate.groupID)
	})

	t.Run("threshold applies peak factor exactly like billing", func(t *testing.T) {
		group := profitControlTestGroup(groupID, 0.5, 0)
		group.SubscriptionType = SubscriptionTypeSubscription
		group.PeakRateEnabled = true
		group.PeakStart = "00:00"
		group.PeakEnd = "23:59"
		group.PeakRateMultiplier = 3.0
		gate := svc.resolveOpenAIProfitControlGate(profitControlTestCtx(group), &groupID)
		require.NotNil(t, gate)
		expected := group.RateMultiplier * group.PeakMultiplierAt(timezone.Now()) * 0.5
		require.InDelta(t, expected, gate.threshold, 1e-9)
		require.Equal(t, PlatformOpenAI, gate.platform)
	})
}

func TestOpenAIProfitControlVetoReason(t *testing.T) {
	now := time.Now()
	gateCtx := func(threshold float64) context.Context {
		return context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, &openAIProfitControlGate{
			threshold: threshold,
			pricingAt: now,
		})
	}

	t.Run("no gate admits everything", func(t *testing.T) {
		vetoed, reason := openAIProfitControlVetoReason(context.Background(), upstreamCostTestOAuthAccount(1))
		require.False(t, vetoed)
		require.Empty(t, reason)
	})

	t.Run("fresh rate below threshold admits", func(t *testing.T) {
		account := profitControlTestAccountWithRate(upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 99, now.Add(-time.Minute), 30*time.Minute), 0.5)
		vetoed, _ := openAIProfitControlVetoReason(gateCtx(0.7), account)
		require.False(t, vetoed)
	})

	t.Run("rate exactly at threshold admits via epsilon", func(t *testing.T) {
		account := profitControlTestAccountWithRate(upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 99, now.Add(-time.Minute), 30*time.Minute), 0.7)
		vetoed, _ := openAIProfitControlVetoReason(gateCtx(0.7), account)
		require.False(t, vetoed)
	})

	t.Run("rate within float noise above threshold admits", func(t *testing.T) {
		account := profitControlTestAccountWithRate(upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 99, now.Add(-time.Minute), 30*time.Minute), 0.7+1e-12)
		vetoed, _ := openAIProfitControlVetoReason(gateCtx(0.7), account)
		require.False(t, vetoed)
	})

	t.Run("rate above threshold is vetoed", func(t *testing.T) {
		account := profitControlTestAccountWithRate(upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.1, now.Add(-time.Minute), 30*time.Minute), 0.8)
		vetoed, reason := openAIProfitControlVetoReason(gateCtx(0.7), account)
		require.True(t, vetoed)
		require.Equal(t, openAIProfitFilterReasonThreshold, reason)
	})

	t.Run("zero threshold only admits free upstream", func(t *testing.T) {
		free := profitControlTestAccountWithRate(upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 99, now.Add(-time.Minute), 30*time.Minute), 0)
		vetoed, _ := openAIProfitControlVetoReason(gateCtx(0), free)
		require.False(t, vetoed)
		paid := profitControlTestAccountWithRate(upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0, now.Add(-time.Minute), 30*time.Minute), 0.01)
		vetoed, reason := openAIProfitControlVetoReason(gateCtx(0), paid)
		require.True(t, vetoed)
		require.Equal(t, openAIProfitFilterReasonThreshold, reason)
	})

	t.Run("missing account rate is invalid", func(t *testing.T) {
		vetoed, reason := openAIProfitControlVetoReason(gateCtx(0.7), upstreamCostTestOAuthAccount(1))
		require.True(t, vetoed)
		require.Equal(t, openAIProfitFilterReasonInvalidAccountRate, reason)
	})

	t.Run("oauth account with manual rate is priceable", func(t *testing.T) {
		account := profitControlTestAccountWithRate(upstreamCostTestOAuthAccount(1), 0.2)
		vetoed, _ := openAIProfitControlVetoReason(gateCtx(0.7), account)
		require.False(t, vetoed)
	})

	t.Run("stale probe does not affect manual account rate", func(t *testing.T) {
		account := profitControlTestAccountWithRate(upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 99, now.Add(-3*time.Hour), 30*time.Minute), 0.1)
		vetoed, _ := openAIProfitControlVetoReason(gateCtx(0.7), account)
		require.False(t, vetoed)
	})

	t.Run("negative and non-finite rates are invalid", func(t *testing.T) {
		for _, rate := range []float64{-1, math.NaN(), math.Inf(1)} {
			account := profitControlTestAccountWithRate(upstreamCostTestOAuthAccount(1), rate)
			vetoed, reason := openAIProfitControlVetoReason(gateCtx(0.7), account)
			require.True(t, vetoed)
			require.Equal(t, openAIProfitFilterReasonInvalidAccountRate, reason)
		}
	})
}

func TestProfitControlSchedulerFiltersCandidates(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	now := time.Now()
	cheap := upstreamCostTestAccount(1, UpstreamBillingProbeStatusOK, 0.3, now.Add(-time.Minute), 30*time.Minute)
	expensive := upstreamCostTestAccount(2, UpstreamBillingProbeStatusOK, 0.8, now.Add(-time.Minute), 30*time.Minute)
	oauth := upstreamCostTestOAuthAccount(3)
	profitControlTestAccountWithRate(cheap, 0.3)
	profitControlTestAccountWithRate(expensive, 0.8)
	for _, account := range []*Account{cheap, expensive, oauth} {
		account.Status = StatusActive
		account.Schedulable = true
		account.Concurrency = 5
	}
	cache := &upstreamCostTrackingConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{
		cheap.ID:     {AccountID: cheap.ID},
		expensive.ID: {AccountID: expensive.ID},
		oauth.ID:     {AccountID: oauth.ID},
	}}
	cfg := &config.Config{}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: []Account{*cheap, *expensive, *oauth}},
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(cache),
	}
	groupID := int64(7)

	t.Run("unprofitable and invalid-rate accounts never win", func(t *testing.T) {
		// margin 0.5 → 阈值 0.5：expensive(0.8) 超阈值、oauth 倍率缺失，仅 cheap 可选。
		ctx := profitControlTestCtx(profitControlTestGroup(groupID, 0.5, 0))
		for i := 0; i < 5; i++ {
			selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, cheap.ID, selection.Account.ID)
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		}
	})

	t.Run("all excluded surfaces standard no-available error with profit reasons", func(t *testing.T) {
		// margin+buffer 0.8 → 阈值 0.2：cheap/expensive 超阈值，oauth 倍率非法。
		ctx := profitControlTestCtx(profitControlTestGroup(groupID, 0.7, 0.1))
		selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
		require.Nil(t, selection)
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrNoAvailableAccounts))
		require.Contains(t, err.Error(), openAIProfitFilterReasonThreshold+"=2")
		require.Contains(t, err.Error(), openAIProfitFilterReasonInvalidAccountRate+"=1")
	})

	t.Run("manually rated oauth account is admitted", func(t *testing.T) {
		// 阈值 0.2 排除两个 API Key；OAuth 手工倍率 0.1 可参与调度。
		profitControlTestAccountWithRate(oauth, 0.1)
		svc.accountRepo = schedulerTestOpenAIAccountRepo{accounts: []Account{*cheap, *expensive, *oauth}}
		ctx := profitControlTestCtx(profitControlTestGroup(groupID, 0.7, 0.1))
		selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.Equal(t, oauth.ID, selection.Account.ID)
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
	})

	t.Run("gate disabled keeps official behavior", func(t *testing.T) {
		group := profitControlTestGroup(groupID, 0.7, 0.1)
		group.ProfitControlEnabled = false
		selection, _, err := svc.SelectAccountWithScheduler(profitControlTestCtx(group), &groupID, "", "", "gpt-test", nil, OpenAIUpstreamTransportAny, false)
		require.NoError(t, err)
		require.NotNil(t, selection)
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
	})
}

func TestValidateProfitControlConfig(t *testing.T) {
	require.NoError(t, ValidateProfitControlConfig(PlatformAnthropic, false, 0, 0))
	for _, platform := range []string{PlatformOpenAI, PlatformAnthropic, PlatformGemini, PlatformGrok, PlatformAntigravity} {
		require.NoError(t, ValidateProfitControlConfig(platform, true, 0.3, 0.05))
		require.NoError(t, ValidateProfitControlConfig(platform, true, 0, 0))
	}

	require.Error(t, ValidateProfitControlConfig(PlatformComposite, true, 0.3, 0))
	require.Error(t, ValidateProfitControlConfig(PlatformOpenAI, true, -0.1, 0))
	require.Error(t, ValidateProfitControlConfig(PlatformOpenAI, true, 1.0, 0))
	require.Error(t, ValidateProfitControlConfig(PlatformOpenAI, true, 0, 1.0))
	require.Error(t, ValidateProfitControlConfig(PlatformOpenAI, true, 0.6, 0.4))
}

func TestNormalizeProfitControlConfig(t *testing.T) {
	t.Run("unsupported platform resets everything", func(t *testing.T) {
		enabled, margin, buffer := NormalizeProfitControlConfig(PlatformComposite, true, 0.3, 0.1)
		require.False(t, enabled)
		require.Zero(t, margin)
		require.Zero(t, buffer)
	})

	t.Run("all five platforms retain configuration", func(t *testing.T) {
		for _, platform := range []string{PlatformOpenAI, PlatformAnthropic, PlatformGemini, PlatformGrok, PlatformAntigravity} {
			enabled, margin, buffer := NormalizeProfitControlConfig(platform, true, 0.3, 0.1)
			require.True(t, enabled)
			require.InDelta(t, 0.3, margin, 1e-12)
			require.InDelta(t, 0.1, buffer, 1e-12)
		}
	})

	t.Run("openai disabled keeps legal values and cleans dirty ones", func(t *testing.T) {
		enabled, margin, buffer := NormalizeProfitControlConfig(PlatformOpenAI, false, 0.3, 0.05)
		require.False(t, enabled)
		require.InDelta(t, 0.3, margin, 1e-12)
		require.InDelta(t, 0.05, buffer, 1e-12)

		_, margin, buffer = NormalizeProfitControlConfig(PlatformOpenAI, false, -1, 1.5)
		require.Zero(t, margin)
		require.Zero(t, buffer)
	})

	t.Run("openai enabled passes through for validation", func(t *testing.T) {
		enabled, margin, buffer := NormalizeProfitControlConfig(PlatformOpenAI, true, 0.3, 0.05)
		require.True(t, enabled)
		require.InDelta(t, 0.3, margin, 1e-12)
		require.InDelta(t, 0.05, buffer, 1e-12)
	})
}
