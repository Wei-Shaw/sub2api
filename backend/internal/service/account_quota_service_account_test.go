package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountSupportsAccountQuota(t *testing.T) {
	tests := []struct {
		accountType string
		want        bool
	}{
		{AccountTypeAPIKey, true},
		{AccountTypeBedrock, true},
		{AccountTypeServiceAccount, true},
		{AccountTypeOAuth, false},
		{AccountTypeSetupToken, false},
		{AccountTypeUpstream, false},
	}
	for _, tt := range tests {
		t.Run(tt.accountType, func(t *testing.T) {
			account := &Account{Type: tt.accountType}
			require.Equal(t, tt.want, account.SupportsAccountQuota())
		})
	}
}

// 配额专用判定放开 service_account，不能连带放开池模式等其它 IsAPIKeyOrBedrock 特性。
func TestServiceAccountQuotaDoesNotWidenPoolMode(t *testing.T) {
	account := &Account{
		Platform:    PlatformGemini,
		Type:        AccountTypeServiceAccount,
		Credentials: map[string]any{"pool_mode": true},
	}
	require.True(t, account.SupportsAccountQuota())
	require.False(t, account.IsAPIKeyOrBedrock())
	require.False(t, account.IsPoolMode())
}

func TestShouldUpdateAccountQuota(t *testing.T) {
	tests := []struct {
		name      string
		account   *Account
		totalCost float64
		want      bool
	}{
		{
			name:      "apikey without limit does not accumulate",
			account:   &Account{Type: AccountTypeAPIKey, Extra: map[string]any{}},
			totalCost: 1.5,
			want:      false,
		},
		{
			name:      "apikey with total limit accumulates",
			account:   &Account{Type: AccountTypeAPIKey, Extra: map[string]any{"quota_limit": 100.0}},
			totalCost: 1.5,
			want:      true,
		},
		{
			name:      "bedrock with daily limit accumulates",
			account:   &Account{Type: AccountTypeBedrock, Extra: map[string]any{"quota_daily_limit": 10.0}},
			totalCost: 1.5,
			want:      true,
		},
		{
			name:      "service_account without limit does not accumulate",
			account:   &Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Extra: map[string]any{"quota_used": 30.0}},
			totalCost: 1.5,
			want:      false,
		},
		{
			name:      "service_account with limit accumulates",
			account:   &Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Extra: map[string]any{"quota_weekly_limit": 50.0}},
			totalCost: 1.5,
			want:      true,
		},
		{
			name:      "service_account zero cost does not accumulate",
			account:   &Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Extra: map[string]any{"quota_limit": 100.0}},
			totalCost: 0,
			want:      false,
		},
		{
			name:      "oauth with limit is not quota eligible",
			account:   &Account{Type: AccountTypeOAuth, Extra: map[string]any{"quota_limit": 100.0}},
			totalCost: 1.5,
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &postUsageBillingParams{
				Cost:    &CostBreakdown{TotalCost: tt.totalCost},
				Account: tt.account,
			}
			require.Equal(t, tt.want, p.shouldUpdateAccountQuota())
		})
	}
}

func TestBuildUsageBillingCommandAccountQuotaCostForServiceAccount(t *testing.T) {
	build := func(account *Account) *UsageBillingCommand {
		return buildUsageBillingCommand("req-sa-quota", nil, &postUsageBillingParams{
			Cost:                  &CostBreakdown{TotalCost: 2, ActualCost: 3},
			User:                  &User{ID: 7},
			APIKey:                &APIKey{ID: 13},
			Account:               account,
			AccountRateMultiplier: 1.5,
		})
	}

	// service_account 设了上限：AccountQuotaCost = TotalCost × 账号倍率
	cmd := build(&Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeServiceAccount, Extra: map[string]any{"quota_limit": 100.0}})
	require.NotNil(t, cmd)
	require.Equal(t, AccountTypeServiceAccount, cmd.AccountType)
	require.InDelta(t, 3.0, cmd.AccountQuotaCost, 1e-9)

	// service_account 不设上限：与 apikey 一致不累加
	cmd = build(&Account{ID: 10, Platform: PlatformGemini, Type: AccountTypeServiceAccount})
	require.NotNil(t, cmd)
	require.Zero(t, cmd.AccountQuotaCost)
}

type accountQuotaIncrementRepoStub struct {
	AccountRepository
	calls []float64
}

func (s *accountQuotaIncrementRepoStub) IncrementQuotaUsed(_ context.Context, _ int64, amount float64) error {
	s.calls = append(s.calls, amount)
	return nil
}

func TestPostUsageBillingLegacyAccumulatesServiceAccountQuota(t *testing.T) {
	repo := &accountQuotaIncrementRepoStub{}
	postUsageBilling(context.Background(), &postUsageBillingParams{
		Cost:                  &CostBreakdown{TotalCost: 2},
		User:                  &User{ID: 7},
		APIKey:                &APIKey{ID: 13},
		Account:               &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeServiceAccount, Extra: map[string]any{"quota_limit": 100.0}},
		AccountRateMultiplier: 1.25,
	}, &billingDeps{accountRepo: repo})

	require.Len(t, repo.calls, 1)
	require.InDelta(t, 2.5, repo.calls[0], 1e-9)
}

type settingGetValueCounter struct {
	SettingRepository
	calls int
}

func (s *settingGetValueCounter) GetValue(context.Context, string) (string, error) {
	s.calls++
	return "false", nil
}

// 未设任何上限的账号没有可跨越的阈值，不应逐请求读取通知设置。
func TestNotifyAccountQuotaSkipsAccountWithoutLimit(t *testing.T) {
	tests := []struct {
		name      string
		account   *Account
		wantCalls int
	}{
		{
			name:      "service_account without limit",
			account:   &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeServiceAccount},
			wantCalls: 0,
		},
		{
			name:      "service_account with total limit",
			account:   &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeServiceAccount, Extra: map[string]any{"quota_limit": 100.0}},
			wantCalls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &settingGetValueCounter{}
			notifyAccountQuota(&postUsageBillingParams{
				Cost:                  &CostBreakdown{TotalCost: 2},
				Account:               tt.account,
				AccountRateMultiplier: 1,
			}, &billingDeps{
				balanceNotifyService: NewBalanceNotifyService(NewEmailService(settings, nil), settings, nil),
			}, nil)
			require.Equal(t, tt.wantCalls, settings.calls)
		})
	}
}

// 账号已删除时计数被跳过、无递增后状态，不应回退到快照做阈值判定。
func TestNotifyAccountQuotaSkipsWhenQuotaIncrementSkipped(t *testing.T) {
	settings := &settingGetValueCounter{}
	notifyAccountQuota(&postUsageBillingParams{
		Cost:                  &CostBreakdown{TotalCost: 2},
		Account:               &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeServiceAccount, Extra: map[string]any{"quota_limit": 100.0}},
		AccountRateMultiplier: 1,
	}, &billingDeps{
		balanceNotifyService: NewBalanceNotifyService(NewEmailService(settings, nil), settings, nil),
	}, &UsageBillingApplyResult{Applied: true})
	require.Zero(t, settings.calls)
}
