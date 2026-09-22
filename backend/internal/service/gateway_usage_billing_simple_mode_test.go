package service

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildUsageBillingCommandSimpleModeOnlyChargesAPIKeyWindows(t *testing.T) {
	apiKey := &APIKey{ID: 13, Quota: 100, RateLimit5h: 10}
	user := &User{ID: 7}
	account := &Account{ID: 9, Type: AccountTypeAPIKey}
	p := &postUsageBillingParams{
		Cost:                       &CostBreakdown{ActualCost: 3.25, TotalCost: 2.5},
		User:                       user,
		APIKey:                     apiKey,
		Account:                    account,
		APIKeyService:              &apiKeyQuotaUpdaterStub{},
		SimpleModeKeyRateLimitOnly: true,
	}

	cmd := buildUsageBillingCommand("simple-req", nil, p)
	require.NotNil(t, cmd)
	require.Zero(t, cmd.BalanceCost)
	require.Zero(t, cmd.SubscriptionCost)
	require.Zero(t, cmd.APIKeyQuotaCost)
	require.Zero(t, cmd.AccountQuotaCost)
	require.Equal(t, 3.25, cmd.APIKeyRateLimitCost)
}

type apiKeyQuotaUpdaterStub struct{}

func (apiKeyQuotaUpdaterStub) UpdateQuotaUsed(context.Context, int64, float64) error { return nil }
func (apiKeyQuotaUpdaterStub) UpdateRateLimitUsage(context.Context, int64, float64) error {
	return nil
}

type simpleModeUsageBillingRepoStub struct {
	UsageBillingRepository
	seen map[string]struct{}
	cmds []*UsageBillingCommand
}

func (s *simpleModeUsageBillingRepoStub) Apply(_ context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	s.cmds = append(s.cmds, cmd)
	if s.seen == nil {
		s.seen = make(map[string]struct{})
	}
	key := cmd.RequestID + ":" + strconv.FormatInt(cmd.APIKeyID, 10)
	if _, ok := s.seen[key]; ok {
		return &UsageBillingApplyResult{Applied: false}, nil
	}
	s.seen[key] = struct{}{}
	return &UsageBillingApplyResult{Applied: true}, nil
}

func TestApplyUsageBillingSimpleModeDeduplicatesWithoutBalanceEffects(t *testing.T) {
	repo := &simpleModeUsageBillingRepoStub{}
	p := &postUsageBillingParams{
		Cost:                       &CostBreakdown{ActualCost: 3.25, TotalCost: 3.25},
		User:                       &User{ID: 7, Balance: 0},
		APIKey:                     &APIKey{ID: 13, Quota: 100, RateLimit5h: 10},
		Account:                    &Account{ID: 9, Type: AccountTypeAPIKey},
		APIKeyService:              &apiKeyQuotaUpdaterStub{},
		SimpleModeKeyRateLimitOnly: true,
	}

	first, err := applyUsageBilling(context.Background(), "simple-req", nil, p, &billingDeps{deferredService: &DeferredService{}}, repo)
	require.NoError(t, err)
	require.True(t, first)
	second, err := applyUsageBilling(context.Background(), "simple-req", nil, p, &billingDeps{deferredService: &DeferredService{}}, repo)
	require.NoError(t, err)
	require.False(t, second)
	require.Len(t, repo.cmds, 2)
	for _, cmd := range repo.cmds {
		require.Zero(t, cmd.BalanceCost)
		require.Zero(t, cmd.SubscriptionCost)
		require.Zero(t, cmd.APIKeyQuotaCost)
		require.Zero(t, cmd.AccountQuotaCost)
		require.Equal(t, 3.25, cmd.APIKeyRateLimitCost)
	}
}

func TestApplyUsageBillingSimpleModeRejectsMissingRepository(t *testing.T) {
	p := &postUsageBillingParams{
		Cost:                       &CostBreakdown{ActualCost: 1},
		User:                       &User{ID: 7},
		APIKey:                     &APIKey{ID: 13, RateLimit5h: 10},
		Account:                    &Account{ID: 9},
		SimpleModeKeyRateLimitOnly: true,
	}
	_, err := applyUsageBilling(context.Background(), "simple-req", nil, p, &billingDeps{deferredService: &DeferredService{}}, nil)
	require.ErrorIs(t, err, ErrSimpleModeKeyRateLimitBillingUnavailable)
}
