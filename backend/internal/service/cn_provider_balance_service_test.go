package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type cnBalanceResponseUpstream struct {
	statusCode int
	body       string
}

func (u *cnBalanceResponseUpstream) Do(
	_ *http.Request,
	_ string,
	_ int64,
	_ int,
) (*http.Response, error) {
	return &http.Response{
		StatusCode: u.statusCode,
		Body:       io.NopCloser(strings.NewReader(u.body)),
		Header:     make(http.Header),
	}, nil
}

func (u *cnBalanceResponseUpstream) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

type cnBalanceProbeRepo struct {
	AccountRepository
	account     *Account
	extraWrites []map[string]any
}

func (r *cnBalanceProbeRepo) GetByID(_ context.Context, _ int64) (*Account, error) {
	return r.account, nil
}

func (r *cnBalanceProbeRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.extraWrites = append(r.extraWrites, updates)
	return nil
}

func newDeepSeekBalanceProbeAccount() *Account {
	return &Account{
		ID:       42,
		Platform: PlatformDeepseek,
		Type:     AccountTypeAPIKey,
		Status:   StatusActive,
		Credentials: map[string]any{
			"account_mode": AccountModePayG,
			"api_key":      "sk-test",
			"base_url":     "https://relay.example.com",
		},
	}
}

func TestCNProviderBalanceService_DeepSeekInvalidBalancePayloadDoesNotBecomeZero(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantError string
	}{
		{
			name:      "missing balance infos",
			body:      `{"data":{"models":["deepseek-v4-flash"]}}`,
			wantError: "missing balance_infos",
		},
		{
			name:      "empty balance infos",
			body:      `{"is_available":true,"balance_infos":[]}`,
			wantError: "no valid balance entries",
		},
		{
			name:      "invalid balance value",
			body:      `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"not-a-number"}]}`,
			wantError: "no valid balance entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &cnBalanceProbeRepo{account: newDeepSeekBalanceProbeAccount()}
			upstream := &cnBalanceResponseUpstream{statusCode: http.StatusOK, body: tt.body}
			svc := NewCNProviderBalanceService(repo, nil, upstream, nil)

			result, err := svc.QueryBalance(context.Background(), repo.account.ID)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, result.Success)
			require.Contains(t, result.Error, tt.wantError)
			require.Empty(t, result.Balances)
			require.Empty(t, repo.extraWrites, "invalid relay balance payload must not persist a synthetic zero balance")
		})
	}
}

func TestCNProviderBalanceService_DeepSeekValidZeroBalanceRemainsSuccessful(t *testing.T) {
	repo := &cnBalanceProbeRepo{account: newDeepSeekBalanceProbeAccount()}
	upstream := &cnBalanceResponseUpstream{
		statusCode: http.StatusOK,
		body:       `{"is_available":false,"balance_infos":[{"currency":"CNY","total_balance":"0"}]}`,
	}
	svc := NewCNProviderBalanceService(repo, nil, upstream, nil)

	result, err := svc.QueryBalance(context.Background(), repo.account.ID)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.False(t, result.Available)
	require.Equal(t, "CNY", result.Currency)
	require.Zero(t, result.Balance)
	require.Len(t, result.Balances, 1)
	require.Len(t, repo.extraWrites, 1, "a valid upstream zero balance must still be persisted")
}

type cnBalanceSequenceUpstream struct {
	responses []cnBalanceResponseUpstream
	calls     int
	urls      []string
}

func (u *cnBalanceSequenceUpstream) Do(
	req *http.Request,
	_ string,
	_ int64,
	_ int,
) (*http.Response, error) {
	u.urls = append(u.urls, req.URL.String())
	idx := u.calls
	u.calls++
	if idx >= len(u.responses) {
		idx = len(u.responses) - 1
	}
	r := u.responses[idx]
	return &http.Response{
		StatusCode: r.statusCode,
		Body:       io.NopCloser(strings.NewReader(r.body)),
		Header:     make(http.Header),
	}, nil
}

func (u *cnBalanceSequenceUpstream) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestCNProviderBalanceService_DeepSeekFallsBackToNewAPITokenUsage(t *testing.T) {
	repo := &cnBalanceProbeRepo{account: newDeepSeekBalanceProbeAccount()}
	upstream := &cnBalanceSequenceUpstream{
		responses: []cnBalanceResponseUpstream{
			{statusCode: http.StatusOK, body: `<!doctype html><html><body>spa</body></html>`},
			{statusCode: http.StatusOK, body: `{"code":true,"message":"ok","data":{"object":"token_usage","remaining":87.006556,"unit":"USD","is_active":true,"unlimited_quota":true}}`},
		},
	}
	svc := NewCNProviderBalanceService(repo, nil, upstream, nil)

	result, err := svc.QueryBalance(context.Background(), repo.account.ID)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 87.006556, result.Balance)
	require.Equal(t, "USD", result.Currency)
	require.True(t, result.Available)
	require.Equal(t, 2, upstream.calls)
	require.Contains(t, upstream.urls[0], "/user/balance")
	require.True(t, strings.Contains(upstream.urls[1], "/api/usage/token"), upstream.urls[1])
	require.Len(t, repo.extraWrites, 1)
}

func TestParseNewAPITokenUsageBalance(t *testing.T) {
	entries, available, ok := parseNewAPITokenUsageBalance([]byte(
		`{"code":true,"data":{"remaining":12.5,"unit":"USD","is_active":true}}`,
	))
	require.True(t, ok)
	require.True(t, available)
	require.Equal(t, []CNProviderBalanceEntry{{Currency: "USD", Balance: 12.5}}, entries)

	entries, available, ok = parseNewAPITokenUsageBalance([]byte(
		`{"code":true,"data":{"total_available":500000,"object":"token_usage"}}`,
	))
	require.True(t, ok)
	require.True(t, available)
	require.InDelta(t, 1.0, entries[0].Balance, 1e-9)
	require.Equal(t, "USD", entries[0].Currency)

	_, _, ok = parseNewAPITokenUsageBalance([]byte(`{"success":false}`))
	require.False(t, ok)
}

func TestCNNewAPITokenUsageURL(t *testing.T) {
	account := newDeepSeekBalanceProbeAccount()
	account.Credentials["base_url"] = "https://www.sheapi.top/v1"
	require.Equal(t, "https://www.sheapi.top/api/usage/token/", cnNewAPITokenUsageURL(account))
}
