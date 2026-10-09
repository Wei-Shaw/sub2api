package service

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func relayTestID(id int64) *int64 { return &id }

func TestRelayPrivateGroupIsOwnerOnly(t *testing.T) {
	g := &Group{ID: 9, OwnerUserID: relayTestID(1), IsExclusive: true}
	require.True(t, g.CanUse(&User{ID: 1}))
	require.False(t, g.CanUse(&User{ID: 2, AllowedGroups: []int64{9}}))
	require.False(t, g.CanUse(&User{ID: 2, Role: RoleAdmin}))
	require.False(t, g.CanUse(nil))
	public := &Group{ID: 10, SubscriptionType: SubscriptionTypeStandard}
	require.True(t, public.CanUse(&User{ID: 2}))
	require.False(t, public.CanUse(&User{ID: 2, RestrictPublicGroups: true}))
}

func TestRelayPrivateBillingKeepsUsageAndAccountCostWithoutChargingUser(t *testing.T) {
	user := &User{ID: 1}
	group := &Group{ID: 9, OwnerUserID: relayTestID(1)}
	key := &APIKey{ID: 10, UserID: 1, GroupID: relayTestID(9), Group: group, Quota: 100}
	cost := &CostBreakdown{TotalCost: 12, ActualCost: 7}
	log := &UsageLog{ActualCost: 7, TotalCost: 12, RateMultiplier: 1}
	p := &postUsageBillingParams{User: user, APIKey: key, Account: &Account{ID: 11, OwnerUserID: relayTestID(1)}, Cost: cost}
	cmd := buildUsageBillingCommand("private-call", log, p)
	require.Zero(t, cmd.BalanceCost)
	require.Zero(t, cmd.SubscriptionCost)
	require.Zero(t, cmd.APIKeyQuotaCost)
	require.Zero(t, cmd.APIKeyRateLimitCost)
	require.Nil(t, cmd.SupplierUserID)
	require.Zero(t, log.ActualCost)
	require.Equal(t, 12.0, log.TotalCost)
	require.Equal(t, 7.0, cost.ActualCost, "do not mutate the caller's cost snapshot")
}

func TestRelayPublicSelfUseChargesAndAccruesIncome(t *testing.T) {
	group := &Group{ID: 9, SubscriptionType: SubscriptionTypeStandard, SupplierRate: 0.9}
	p := &postUsageBillingParams{User: &User{ID: 1}, APIKey: &APIKey{ID: 10, GroupID: relayTestID(9), Group: group},
		Account: &Account{ID: 11, OwnerUserID: relayTestID(1)}, Cost: &CostBreakdown{ActualCost: 1, TotalCost: 1}}
	cmd := buildUsageBillingCommand("public-self", nil, p)
	require.Equal(t, 1.0, cmd.BalanceCost)
	require.Equal(t, int64(1), *cmd.SupplierUserID)
	require.Equal(t, int64(9), *cmd.GroupID)
	require.Equal(t, 0.9, cmd.SupplierRate)
	p.Account.OwnerUserID = relayTestID(2)
	other := buildUsageBillingCommand("public-other", nil, p)
	require.Equal(t, 1.0, other.BalanceCost)
	require.Equal(t, int64(2), *other.SupplierUserID)
}

func TestRelaySelfUseBalanceNotificationUsesNetWalletChange(t *testing.T) {
	final := 9.9
	p := &postUsageBillingParams{User: &User{ID: 1}, Cost: &CostBreakdown{ActualCost: 1}}
	result := &UsageBillingApplyResult{NewBalance: &final, SupplierUserID: relayTestID(1), SupplierAmount: 0.9}
	require.InDelta(t, 10, resolveOldBalance(p, result), 1e-8)
	require.InDelta(t, 0.1, balanceCostAfterSupplierCredit(p, result), 1e-8)
}

func TestRelayCredentialsRejectUnsafeURLsAndAdminFields(t *testing.T) {
	for _, raw := range []string{"http://localhost", "http://127.0.0.1", "http://10.0.0.1", "http://[::1]", "file:///etc/passwd", "https://user:pass@example.com", "https://example.com?secret=x"} {
		t.Run(raw, func(t *testing.T) {
			require.Error(t, ValidateRelayCredentials(AccountTypeAPIKey, map[string]any{"base_url": raw}))
		})
	}
	require.NoError(t, ValidateRelayCredentials(AccountTypeAPIKey, map[string]any{"base_url": "https://upstream.example.com/v1", "api_key": "secret"}))
	require.Error(t, ValidateRelayCredentials(AccountTypeAPIKey, map[string]any{"proxy_url": "http://localhost"}))
	require.Error(t, ValidateRelayCredentials(AccountTypeAPIKey, map[string]any{"api_base_urls": map[string]any{"responses": "http://127.0.0.1"}}))
}

func TestRelayUpstreamRequestsEnforcePublicHostsAndDisableRedirects(t *testing.T) {
	req := httptest.NewRequest("POST", "https://upstream.example.com", nil)
	RestrictRelayUpstreamRequest(req, &Account{OwnerUserID: relayTestID(1)})
	require.True(t, HTTPUpstreamPublicHostsOnly(req.Context()))
	require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))
	adminReq := httptest.NewRequest("POST", "http://localhost", nil)
	RestrictRelayUpstreamRequest(adminReq, &Account{})
	require.False(t, HTTPUpstreamPublicHostsOnly(adminReq.Context()))
}

type relayAccountRepoStub struct {
	AccountRepository
	account *Account
}

func (r relayAccountRepoStub) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func TestRelayCannotManageAnotherUsersAccount(t *testing.T) {
	s := &RelayService{accounts: relayAccountRepoStub{account: &Account{ID: 8, OwnerUserID: relayTestID(1)}}}
	_, err := s.OwnedAccount(context.Background(), 2, 8)
	require.ErrorIs(t, err, ErrAccountNotFound)
	require.ErrorIs(t, s.DeleteAccount(context.Background(), 2, 8), ErrAccountNotFound)
	_, err = s.UpdateAccount(context.Background(), 2, 8, &UpdateAccountInput{Name: "stolen"})
	require.ErrorIs(t, err, ErrAccountNotFound)
	a, err := s.OwnedAccount(context.Background(), 1, 8)
	require.NoError(t, err)
	require.Equal(t, int64(8), a.ID)
}

func TestRelayOwnedAccountsNeverEnterUngroupedScheduling(t *testing.T) {
	owned := Account{ID: 1, OwnerUserID: relayTestID(2)}
	admin := Account{ID: 2}
	filtered := excludeUngroupedRelayAccounts([]Account{owned, admin}, nil)
	require.Len(t, filtered, 1)
	require.Equal(t, int64(2), filtered[0].ID)
	require.False(t, openAIStickyAccountMatchesGroup(&owned, nil))
	require.False(t, (&GatewayService{}).isAccountInGroup(&owned, nil))
	owned.GroupIDs = []int64{9}
	require.Len(t, excludeUngroupedRelayAccounts([]Account{owned, admin}, relayTestID(9)), 2)
	require.Len(t, excludeUngroupedRelayAccounts([]Account{owned, admin}, relayTestID(10)), 1)
}

func TestRelayDisabledAccountImportStaysDisabled(t *testing.T) {
	a, err := buildAccountForCreate(&CreateAccountInput{Name: "paused upstream", OwnerUserID: relayTestID(7), Status: StatusDisabled,
		Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test"}, Concurrency: 3}, nil)
	require.NoError(t, err)
	require.Equal(t, StatusDisabled, a.Status)
	require.False(t, a.IsSchedulable())
	require.Equal(t, int64(7), *a.OwnerUserID)
}
