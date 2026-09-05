package service

// ForAccount 直传入口（P2-6）的校验回归测试：
// QueryUsageForAccount / QueryBalanceForAccount 接受已加载的 *Account，
// 但必须复用与 ID 入口相同的加载后校验——直传不能绕过平台/模式检查，
// 且校验在 singleflight 之前完成（无效账号不得发起任何上游请求）。

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func codingAccount(platform string) *Account {
	return &Account{
		ID: 1, Platform: platform, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"account_mode": AccountModeCoding, "api_key": "sk-test"},
	}
}

func paygAccount(platform string) *Account {
	return &Account{
		ID: 2, Platform: platform, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"account_mode": AccountModePayG, "api_key": "sk-test"},
	}
}

func requireReason(t *testing.T, err error, reason string) {
	t.Helper()
	require.Error(t, err)
	var appErr *infraerrors.ApplicationError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, reason, appErr.Reason)
}

func TestValidateCodingPlanAccount_Matrix(t *testing.T) {
	cases := []struct {
		name       string
		account    *Account
		wantReason string
	}{
		{name: "nil", account: nil, wantReason: "CN_QUOTA_ACCOUNT_NOT_FOUND"},
		{name: "non cn provider", account: &Account{ID: 3, Platform: PlatformAnthropic}, wantReason: "CN_QUOTA_INVALID_PLATFORM"},
		{name: "payg has no quota endpoint", account: paygAccount(PlatformKimi), wantReason: "CN_QUOTA_NOT_CODING_PLAN"},
		{name: "kimi coding ok", account: codingAccount(PlatformKimi)},
		{name: "zhipu coding ok", account: codingAccount(PlatformZhipu)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCodingPlanAccount(tc.account)
			if tc.wantReason == "" {
				require.NoError(t, err)
				return
			}
			requireReason(t, err, tc.wantReason)
		})
	}
}

func TestValidatePayGAccount_Matrix(t *testing.T) {
	cases := []struct {
		name       string
		account    *Account
		wantReason string
	}{
		{name: "nil", account: nil, wantReason: "CN_BALANCE_ACCOUNT_NOT_FOUND"},
		{name: "non cn provider", account: &Account{ID: 3, Platform: PlatformAnthropic}, wantReason: "CN_BALANCE_INVALID_PLATFORM"},
		{name: "coding has no balance endpoint", account: codingAccount(PlatformKimi), wantReason: "CN_BALANCE_CODING_PLAN"},
		{name: "kimi payg ok", account: paygAccount(PlatformKimi)},
		{name: "deepseek payg ok", account: paygAccount(PlatformDeepseek)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePayGAccount(tc.account)
			if tc.wantReason == "" {
				require.NoError(t, err)
				return
			}
			requireReason(t, err, tc.wantReason)
		})
	}
}

// 直传入口的校验在 singleflight/上游请求之前：无效账号必须零出站请求。
func TestCNProviderQuotaService_QueryUsageForAccount_RejectsInvalidAccount(t *testing.T) {
	repo := &fakeCNProbeAccountRepo{}
	upstream := &recordingHTTPUpstream{}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

	_, err := svc.QueryUsageForAccount(context.Background(), paygAccount(PlatformKimi))
	requireReason(t, err, "CN_QUOTA_NOT_CODING_PLAN")
	require.Zero(t, upstream.calls)

	_, err = svc.QueryUsageForAccount(context.Background(), nil)
	requireReason(t, err, "CN_QUOTA_ACCOUNT_NOT_FOUND")
	require.Zero(t, upstream.calls)
}

func TestCNProviderBalanceService_QueryBalanceForAccount_RejectsInvalidAccount(t *testing.T) {
	repo := &fakeCNProbeAccountRepo{}
	upstream := &recordingHTTPUpstream{}
	svc := NewCNProviderBalanceService(repo, nil, upstream, nil)

	_, err := svc.QueryBalanceForAccount(context.Background(), codingAccount(PlatformKimi))
	requireReason(t, err, "CN_BALANCE_CODING_PLAN")
	require.Zero(t, upstream.calls)

	_, err = svc.QueryBalanceForAccount(context.Background(), &Account{ID: 9, Platform: PlatformAnthropic})
	requireReason(t, err, "CN_BALANCE_INVALID_PLATFORM")
	require.Zero(t, upstream.calls)
}

// ID 入口与 ForAccount 入口对同一账号的行为一致（loadCodingPlanAccount 的
// 加载后校验 = validateCodingPlanAccount；余额侧对称）。
func TestCNProviderServices_IDEntryAppliesSameValidation(t *testing.T) {
	repo := &fakeCNProbeAccountRepo{account: paygAccount(PlatformKimi)}
	upstream := &recordingHTTPUpstream{}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)

	_, err := svc.QueryUsage(context.Background(), 2)
	requireReason(t, err, "CN_QUOTA_NOT_CODING_PLAN")
	require.Zero(t, upstream.calls)
}
