package service

// ForAccount 直传入口（P2-6）的校验回归测试：
// QueryUsageForAccount / QueryBalanceForAccount 接受已加载的 *Account，
// 但必须复用与 ID 入口相同的加载后校验——直传不能绕过平台/模式检查，
// 且校验在 singleflight 之前完成（无效账号不得发起任何上游请求）。

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type minimaxQuotaTestRepo struct {
	AccountRepository
	updates map[string]any
}

func (r *minimaxQuotaTestRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = updates
	return nil
}

type minimaxQuotaTestUpstream struct {
	HTTPUpstream
	req *http.Request
}

func (u *minimaxQuotaTestUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.req = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"model_remains":[{
				"model_name":"general",
				"current_interval_remaining_percent":98,
				"current_weekly_remaining_percent":95,
				"current_weekly_status":1,
				"end_time":1780329600000,
				"weekly_end_time":1780848000000
			}],
			"base_resp":{"status_code":0,"status_msg":"success"}
		}`)),
	}, nil
}

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
		{name: "minimax coding ok", account: codingAccount(PlatformMiniMax)},
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

func TestCNProviderQuotaService_QueryUsageForAccount_MiniMaxCodingPlan(t *testing.T) {
	repo := &minimaxQuotaTestRepo{}
	upstream := &minimaxQuotaTestUpstream{}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)
	account := codingAccount(PlatformMiniMax)
	account.Credentials["api_key"] = "sk-cp-test"

	result, err := svc.QueryUsageForAccount(context.Background(), account)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.True(t, result.CredentialValid)
	require.True(t, result.Persisted)
	require.Len(t, result.Tiers, 2)
	require.NotNil(t, upstream.req)
	require.Equal(t, "https://api.minimaxi.com/v1/api/openplatform/coding_plan/remains", upstream.req.URL.String())
	require.Equal(t, "Bearer sk-cp-test", upstream.req.Header.Get("Authorization"))
	require.InDelta(t, 2.0, repo.updates["minimax_5h_used_percent"], 1e-9)
	require.InDelta(t, 5.0, repo.updates["minimax_weekly_used_percent"], 1e-9)
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
