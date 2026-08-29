//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAccountUserBillingPricingEligibility(t *testing.T) {
	rate := 1.8
	tests := []struct {
		name     string
		account  *Account
		eligible bool
	}{
		{
			name: "OpenAI API key",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				UserBillingRateMultiplier: &rate},
			eligible: true,
		},
		{
			name: "OpenAI OAuth",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				UserBillingRateMultiplier: &rate},
		},
		{
			name: "non-OpenAI API key",
			account: &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
				UserBillingRateMultiplier: &rate},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.eligible, tt.account.IsUserBillingPricingEligible())
			_, enabled := tt.account.UserBillingMultiplier()
			require.Equal(t, tt.eligible, enabled)
		})
	}
}

func TestResolveAccountUserBillingMultiplierPriority(t *testing.T) {
	rate := 1.8
	account := &Account{
		Platform:                  PlatformOpenAI,
		Type:                      AccountTypeAPIKey,
		UserBillingRateMultiplier: &rate,
	}

	require.InDelta(t, 2.5, resolveAccountUserBillingMultiplier(account, UserGroupRateResolution{
		Multiplier: 2.5, UserOverride: true,
	}), 1e-12, "personal multiplier must win")
	require.InDelta(t, 1.8, resolveAccountUserBillingMultiplier(account, UserGroupRateResolution{
		Multiplier: 1.2,
	}), 1e-12, "account multiplier must win over group")

	account.Type = AccountTypeOAuth
	require.InDelta(t, 1.2, resolveAccountUserBillingMultiplier(account, UserGroupRateResolution{
		Multiplier: 1.2,
	}), 1e-12, "ineligible account must retain group pricing")
}

func TestModelPricingResolverAccountExactPriceOnlyForOpenAIAPIKey(t *testing.T) {
	accountInput := 7e-6
	groupInput := 3e-6
	group := &Group{ModelPricing: []ChannelModelPricing{{
		Platform: PlatformOpenAI, Models: []string{"gpt-test"}, BillingMode: BillingModeToken,
		InputPrice: &groupInput,
	}}}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		UserBillingModelPricing: []ChannelModelPricing{{
			Platform: PlatformOpenAI, Models: []string{"gpt-test"}, BillingMode: BillingModeToken,
			InputPrice: &accountInput,
		}},
	}
	resolver := NewModelPricingResolver(nil, &BillingService{})

	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "GPT-TEST", Group: group, Account: account})
	require.Equal(t, PricingSourceAccount, resolved.Source)
	require.InDelta(t, accountInput, resolved.BasePricing.InputPricePerToken, 1e-12)

	resolved = resolver.Resolve(context.Background(), PricingInput{Model: "not-configured", Group: group, Account: account})
	require.NotEqual(t, PricingSourceAccount, resolved.Source, "unmatched model must fall back to the existing chain")

	account.Type = AccountTypeOAuth
	resolved = resolver.Resolve(context.Background(), PricingInput{Model: "gpt-test", Group: group, Account: account})
	require.Equal(t, PricingSourceGroup, resolved.Source)
	require.InDelta(t, groupInput, resolved.BasePricing.InputPricePerToken, 1e-12)
}

func TestNormalizeAccountUserBillingPricingRejectsOtherAccountTypes(t *testing.T) {
	price := 1e-6
	pricing := []ChannelModelPricing{{Models: []string{"gpt-test"}, InputPrice: &price}}

	_, err := normalizeAccountUserBillingModelPricing(PlatformOpenAI, AccountTypeOAuth, pricing)
	require.Error(t, err)
	_, err = normalizeAccountUserBillingModelPricing(PlatformAnthropic, AccountTypeAPIKey, pricing)
	require.Error(t, err)

	normalized, err := normalizeAccountUserBillingModelPricing(PlatformOpenAI, AccountTypeAPIKey, []ChannelModelPricing{{
		ID: 9, ChannelID: 10, Models: []string{" ", " gpt-test "}, InputPrice: &price,
	}})
	require.NoError(t, err)
	require.Equal(t, int64(0), normalized[0].ID)
	require.Equal(t, int64(0), normalized[0].ChannelID)
	require.Equal(t, PlatformOpenAI, normalized[0].Platform)
	require.Equal(t, []string{"gpt-test"}, normalized[0].Models)
}

func TestOpenAIUsageCostUsesAccountModelPriceAndMultiplier(t *testing.T) {
	accountInput := 5e-6
	accountRate := 2.0
	groupID := int64(42)
	account := &Account{
		Platform:                  PlatformOpenAI,
		Type:                      AccountTypeAPIKey,
		UserBillingRateMultiplier: &accountRate,
		UserBillingModelPricing: []ChannelModelPricing{{
			Platform: PlatformOpenAI, Models: []string{"gpt-account-priced"}, BillingMode: BillingModeToken,
			InputPrice: &accountInput,
		}},
	}
	billing := &BillingService{}
	svc := &OpenAIGatewayService{
		billingService: billing,
		resolver:       NewModelPricingResolver(nil, billing),
	}
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformOpenAI}}

	cost, err := svc.calculateOpenAIRecordUsageCost(
		context.Background(), &OpenAIForwardResult{}, apiKey, account,
		[]string{"gpt-account-priced"}, accountRate, 1, 1, 1,
		UsageTokens{InputTokens: 1000}, "", boolPtr(false), time.Time{},
	)
	require.NoError(t, err)
	require.InDelta(t, 0.01, cost.ActualCost, 1e-12)
}

func TestGenericGatewayUsageCostUsesOpenAIAPIKeyAccountPrice(t *testing.T) {
	accountInput := 4e-6
	groupID := int64(43)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		UserBillingModelPricing: []ChannelModelPricing{{
			Platform: PlatformOpenAI, Models: []string{"gpt-compatible"}, BillingMode: BillingModeToken,
			InputPrice: &accountInput,
		}},
	}
	billing := &BillingService{}
	svc := &GatewayService{
		billingService: billing,
		resolver:       NewModelPricingResolver(nil, billing),
	}
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformOpenAI}}

	cost := svc.calculateRecordUsageCost(
		context.Background(),
		&ForwardResult{Usage: ClaudeUsage{InputTokens: 1000}},
		apiKey,
		account,
		"gpt-compatible",
		1,
		1,
		time.Time{},
		&recordUsageOpts{},
	)
	require.NotNil(t, cost)
	require.InDelta(t, 0.004, cost.ActualCost, 1e-12)
}

func TestProfitControlUsesAccountUserBillingMultiplierUnlessPersonalOverride(t *testing.T) {
	upstreamRate := 1.5
	accountRate := 2.0
	account := &Account{
		Platform:                  PlatformOpenAI,
		Type:                      AccountTypeAPIKey,
		RateMultiplier:            &upstreamRate,
		UserBillingRateMultiplier: &accountRate,
	}
	gate := &openAIProfitControlGate{
		threshold:           1,
		accountPricingAware: true,
		groupRate:           UserGroupRateResolution{Multiplier: 1},
		peakMultiplier:      1,
		marginFactor:        1,
	}
	ctx := context.WithValue(context.Background(), openAIProfitControlGateCtxKey{}, gate)

	vetoed, _ := openAIProfitControlVetoReason(ctx, account)
	require.False(t, vetoed, "account multiplier raises the matching downstream threshold")

	gate.groupRate = UserGroupRateResolution{Multiplier: 1, UserOverride: true}
	vetoed, reason := openAIProfitControlVetoReason(ctx, account)
	require.True(t, vetoed, "personal multiplier must suppress the account multiplier")
	require.Equal(t, openAIProfitFilterReasonThreshold, reason)
}
