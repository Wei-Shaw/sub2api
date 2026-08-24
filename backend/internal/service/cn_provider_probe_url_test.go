package service

// CN 供应商探测端点 URL 安全策略回归测试（review B4）：
// 配额/余额探测不得绕过 security.url_allowlist——base_url 衍生的探测端点
// 必须先过运营者策略，被拒绝时不得发起任何上游请求（API key 不出站）。

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func cnProbeAllowlistConfig(hosts ...string) *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:       true,
				UpstreamHosts: hosts,
			},
		},
	}
}

func TestCNValidateProbeURL_AllowlistPolicy(t *testing.T) {
	cfg := cnProbeAllowlistConfig("api.moonshot.cn", "api.deepseek.com")

	// 白名单内主机放行（保留完整路径）。
	ok, err := cnValidateProbeURL(cfg, "https://api.moonshot.cn/v1/users/me/balance")
	require.NoError(t, err)
	require.Equal(t, "https://api.moonshot.cn/v1/users/me/balance", ok)

	// 白名单外主机拒绝。
	_, err = cnValidateProbeURL(cfg, "https://relay.attacker.example/v1/usages")
	require.Error(t, err)
	require.Contains(t, err.Error(), "rejected by URL security policy")

	// 私网主机拒绝（内网探测面）。
	_, err = cnValidateProbeURL(cfg, "http://169.254.169.254/latest/meta-data")
	require.Error(t, err)

	// 白名单关闭：仅格式校验，任意 https 主机放行。
	formatOnly, err := cnValidateProbeURL(&config.Config{
		Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}},
	}, "https://relay.attacker.example/v1/usages")
	require.NoError(t, err)
	require.Equal(t, "https://relay.attacker.example/v1/usages", formatOnly)
}

// recordingHTTPUpstream 断言探测被策略拒绝时没有任何上游请求发出。
type recordingHTTPUpstream struct{ calls int }

func (u *recordingHTTPUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.calls++
	return nil, context.DeadlineExceeded
}

func (u *recordingHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.calls++
	return nil, context.DeadlineExceeded
}

type fakeCNProbeAccountRepo struct {
	AccountRepository
	account *Account
}

func (r *fakeCNProbeAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	return r.account, nil
}

// kimi coding 账号的 base_url 指向中转（含 api.kimi.com/coding 路径段即可被识别
// 为 kimi coding plan）→ 衍生额度端点落在中转主机上，白名单未列名必须拒绝。
func TestCNProviderQuotaService_RejectsURLBlockedByPolicy(t *testing.T) {
	repo := &fakeCNProbeAccountRepo{account: &Account{
		ID: 1, Platform: PlatformKimi, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{
			"account_mode": "coding",
			"api_key":      "sk-test",
			"base_url":     "https://relay.attacker.example/api.kimi.com/coding",
		},
	}}
	upstream := &recordingHTTPUpstream{}
	svc := NewCNProviderQuotaService(repo, nil, upstream, cnProbeAllowlistConfig("api.kimi.com"))

	_, err := svc.QueryUsage(context.Background(), 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "CN_QUOTA_URL_REJECTED")
	require.Zero(t, upstream.calls, "probe must not issue any upstream request when URL policy rejects the target")
}

// deepseek payg 账号自定义 base_url → 余额端点落在中转主机上，必须先过策略。
func TestCNProviderBalanceService_RejectsURLBlockedByPolicy(t *testing.T) {
	repo := &fakeCNProbeAccountRepo{account: &Account{
		ID: 2, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{
			"account_mode": "payg",
			"api_key":      "sk-test",
			"base_url":     "https://relay.attacker.example",
		},
	}}
	upstream := &recordingHTTPUpstream{}
	svc := NewCNProviderBalanceService(repo, nil, upstream, cnProbeAllowlistConfig("api.deepseek.com"))

	_, err := svc.QueryBalance(context.Background(), 2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "CN_BALANCE_URL_REJECTED")
	require.Zero(t, upstream.calls, "probe must not issue any upstream request when URL policy rejects the target")
}

// 白名单包含官方主机的正常路径：URL 校验通过后才发出上游请求（此处允许到达
// httpUpstream 层即视为通过校验，不发真实网络）。
func TestCNProviderBalanceService_OfficialHostPassesValidation(t *testing.T) {
	repo := &fakeCNProbeAccountRepo{account: &Account{
		ID: 3, Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{
			"account_mode": "payg",
			"api_key":      "sk-test",
		},
	}}
	upstream := &recordingHTTPUpstream{}
	svc := NewCNProviderBalanceService(repo, nil, upstream, cnProbeAllowlistConfig("api.deepseek.com"))

	_, _ = svc.QueryBalance(context.Background(), 3)
	require.Equal(t, 1, upstream.calls, "official host must pass URL policy and reach the upstream layer")
}
