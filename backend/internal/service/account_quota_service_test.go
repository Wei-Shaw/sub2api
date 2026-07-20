package service

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type accountQuotaHTTPStub struct {
	response       string
	statusResponse string
	request        *http.Request
}

type accountQuotaHTTPSequenceStub struct {
	responses []*http.Response
	requests  []*http.Request
}

func (s *accountQuotaHTTPSequenceStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	if req.URL.Path == "/api/status" {
		return newAccountQuotaStatusResponse(`{"quota_display_type":"USD"}`), nil
	}
	s.requests = append(s.requests, req)
	response := s.responses[len(s.requests)-1]
	if response.Header == nil {
		response.Header = make(http.Header)
	}
	if response.Body == nil {
		response.Body = io.NopCloser(strings.NewReader(""))
	}
	return response, nil
}

func (s *accountQuotaHTTPSequenceStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func (s *accountQuotaHTTPStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	if req.URL.Path == "/api/status" {
		statusResponse := s.statusResponse
		if statusResponse == "" {
			statusResponse = `{"quota_display_type":"USD","quota_per_unit":500000}`
		}
		return newAccountQuotaStatusResponse(statusResponse), nil
	}
	s.request = req
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(s.response)), Header: make(http.Header)}, nil
}

func newAccountQuotaStatusResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func (s *accountQuotaHTTPStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestNewAPIQuotaProviderUsesCurrentKeyWithoutUserID(t *testing.T) {
	client := &accountQuotaHTTPStub{response: `{"code":true,"data":{"total_granted":50000000,"total_used":0,"total_available":50000000,"unlimited_quota":false,"expires_at":1893456000,"group":"vip"}}`}
	provider := &newAPIQuotaProvider{client: client}
	account := &Account{ID: 7, Concurrency: 2}

	result, err := provider.Fetch(t.Context(), AccountQuotaFetchInput{Account: account, BaseURL: "https://new-api.example/v1", APIKey: "sk-current"})

	require.NoError(t, err)
	require.Equal(t, "https://new-api.example/api/usage/token/", client.request.URL.String())
	require.Equal(t, "Bearer sk-current", client.request.Header.Get("Authorization"))
	require.Empty(t, client.request.Header.Get("New-Api-User"))
	require.Empty(t, client.request.URL.Query().Get("user_id"))
	require.Len(t, result.Metrics, 1)
	require.Equal(t, "USD", result.Metrics[0].Unit)
	require.Equal(t, float64(100), *result.Metrics[0].Remaining)
	require.NotNil(t, result.KeyExpiresAt)
	require.Equal(t, "vip", result.Plan.Name)
}

func TestNewAPIQuotaProviderFollowsSameOriginLegacyRedirect(t *testing.T) {
	firstRedirectHeader := make(http.Header)
	firstRedirectHeader.Set("Location", "/api/usage/token")
	secondRedirectHeader := make(http.Header)
	secondRedirectHeader.Set("Location", "/api/usage/token/")
	client := &accountQuotaHTTPSequenceStub{responses: []*http.Response{
		{StatusCode: http.StatusMovedPermanently, Header: firstRedirectHeader},
		{StatusCode: http.StatusPermanentRedirect, Header: secondRedirectHeader},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":true,"data":{"total_granted":100,"total_used":20,"total_available":80,"user_id":42}}`))},
	}}
	provider := &newAPIQuotaProvider{client: client}

	result, err := provider.Fetch(t.Context(), AccountQuotaFetchInput{Account: &Account{ID: 8}, BaseURL: "https://old-new-api.example/v1/", APIKey: "sk-current", Config: map[string]any{"user_id": "42", "quota_per_usd": 1}})

	require.NoError(t, err)
	require.Len(t, client.requests, 3)
	require.Equal(t, "https://old-new-api.example/api/usage/token/", client.requests[0].URL.String())
	require.Equal(t, "https://old-new-api.example/api/usage/token", client.requests[1].URL.String())
	require.Equal(t, "https://old-new-api.example/api/usage/token/", client.requests[2].URL.String())
	for _, request := range client.requests {
		require.Equal(t, "Bearer sk-current", request.Header.Get("Authorization"))
		require.Equal(t, "42", request.Header.Get("New-Api-User"))
	}
	require.Equal(t, float64(80), *result.Metrics[0].Remaining)
	require.Equal(t, "42", result.SuggestedConfig["user_id"])
}

func TestNewAPIQuotaProviderShowsAccountBalanceForUnlimitedKey(t *testing.T) {
	client := &accountQuotaHTTPSequenceStub{responses: []*http.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":true,"data":{"total_granted":0,"total_used":0,"total_available":0,"unlimited_quota":true}}`))},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"hard_limit_usd":150}}`))},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"total_usage":2500}`))},
	}}
	provider := &newAPIQuotaProvider{client: client}

	result, err := provider.Fetch(t.Context(), AccountQuotaFetchInput{Account: &Account{ID: 10}, BaseURL: "https://new-api.example/v1", APIKey: "sk-current"})

	require.NoError(t, err)
	require.Len(t, client.requests, 3)
	require.Equal(t, "https://new-api.example/v1/dashboard/billing/subscription", client.requests[1].URL.String())
	require.Equal(t, "https://new-api.example/v1/dashboard/billing/usage", client.requests[2].URL.Scheme+"://"+client.requests[2].URL.Host+client.requests[2].URL.Path)
	require.Len(t, result.Metrics, 1)
	require.Equal(t, "balance", result.Metrics[0].Key)
	require.Equal(t, "USD", result.Metrics[0].Unit)
	require.Equal(t, float64(125), *result.Metrics[0].Remaining)
	require.False(t, result.Metrics[0].Unlimited)
}

func TestNewAPIQuotaProviderRejectsUnlimitedBillingSentinelWithoutAccessTokenFallback(t *testing.T) {
	client := &accountQuotaHTTPSequenceStub{responses: []*http.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":true,"data":{"total_granted":0,"total_used":0,"total_available":0,"unlimited_quota":true}}`))},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"hard_limit_usd":100000000}`))},
	}}
	provider := &newAPIQuotaProvider{client: client}

	_, err := provider.Fetch(t.Context(), AccountQuotaFetchInput{
		Account: &Account{ID: 13}, BaseURL: "https://new-api.example/v1", APIKey: "sk-current",
	})

	require.ErrorContains(t, err, "API Key")
	require.Len(t, client.requests, 2)
}

func TestNewAPIQuotaProviderUsesUpstreamDisplayUnit(t *testing.T) {
	tests := []struct {
		name           string
		statusResponse string
		expectedUnit   string
		expectedValue  float64
	}{
		{name: "USD", statusResponse: `{"quota_display_type":"USD","quota_per_unit":500000}`, expectedUnit: "USD", expectedValue: 1},
		{name: "CNY", statusResponse: `{"quota_display_type":"CNY","quota_per_unit":500000,"usd_exchange_rate":7.3}`, expectedUnit: "CNY", expectedValue: 7.3},
		{name: "tokens", statusResponse: `{"quota_display_type":"TOKENS","quota_per_unit":500000}`, expectedUnit: "tokens", expectedValue: 500000},
		{name: "custom", statusResponse: `{"quota_display_type":"CUSTOM","quota_per_unit":500000,"custom_currency_exchange_rate":2.5,"custom_currency_symbol":"credits"}`, expectedUnit: "credits", expectedValue: 2.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &accountQuotaHTTPStub{
				response:       `{"code":true,"data":{"total_granted":500000,"total_used":0,"total_available":500000,"unlimited_quota":false}}`,
				statusResponse: tt.statusResponse,
			}
			provider := &newAPIQuotaProvider{client: client}

			result, err := provider.Fetch(t.Context(), AccountQuotaFetchInput{Account: &Account{ID: 15}, BaseURL: "https://new-api.example/v1", APIKey: "sk-current"})

			require.NoError(t, err)
			require.Equal(t, tt.expectedUnit, result.Metrics[0].Unit)
			require.InDelta(t, tt.expectedValue, *result.Metrics[0].Remaining, 0.001)
		})
	}
}

func TestNewAPIQuotaProviderUsesLargeWalletBalanceWithoutAccessToken(t *testing.T) {
	client := &accountQuotaHTTPSequenceStub{responses: []*http.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":true,"data":{"total_granted":0,"total_used":0,"total_available":0,"unlimited_quota":true}}`))},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"hard_limit_usd":1000000111300.9163}`))},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"total_usage":1159614.7278}`))},
	}}
	provider := &newAPIQuotaProvider{client: client}

	result, err := provider.Fetch(t.Context(), AccountQuotaFetchInput{
		Account: &Account{ID: 14}, BaseURL: "https://new-api.example/v1", APIKey: "sk-current",
	})

	require.NoError(t, err)
	require.Len(t, client.requests, 3)
	require.Equal(t, "https://new-api.example/v1/dashboard/billing/subscription", client.requests[1].URL.String())
	require.Equal(t, "/v1/dashboard/billing/usage", client.requests[2].URL.Path)
	require.InDelta(t, float64(1000000099704.769), *result.Metrics[0].Remaining, 0.001)
}

func TestNewAPIQuotaProviderDoesNotTreatUnlimitedKeyZeroAsAccountBalance(t *testing.T) {
	client := &accountQuotaHTTPSequenceStub{responses: []*http.Response{
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":true,"data":{"total_granted":0,"total_used":0,"total_available":0,"unlimited_quota":true}}`))},
		{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(`{"message":"not found"}`))},
	}}
	provider := &newAPIQuotaProvider{client: client}

	_, err := provider.Fetch(t.Context(), AccountQuotaFetchInput{Account: &Account{ID: 12}, BaseURL: "https://new-api.example/v1", APIKey: "sk-current"})

	require.ErrorContains(t, err, "账号余额获取失败")
}

func TestMergeSuggestedQuotaConfigOnlyFillsMissingValues(t *testing.T) {
	merged, changed := mergeSuggestedQuotaConfig(map[string]any{"region": "cn"}, map[string]any{"user_id": "42"})
	require.True(t, changed)
	require.Equal(t, map[string]any{"region": "cn", "user_id": "42"}, merged)

	merged, changed = mergeSuggestedQuotaConfig(map[string]any{"user_id": "7"}, map[string]any{"user_id": "42"})
	require.False(t, changed)
	require.Equal(t, "7", merged["user_id"])
}

func TestQuotaFetchRejectsCrossOriginRedirect(t *testing.T) {
	redirectHeader := make(http.Header)
	redirectHeader.Set("Location", "https://other.example/api/usage/token/")
	client := &accountQuotaHTTPSequenceStub{responses: []*http.Response{
		{StatusCode: http.StatusMovedPermanently, Header: redirectHeader},
	}}

	_, err := fetchQuotaJSON(t.Context(), client, AccountQuotaFetchInput{Account: &Account{ID: 8}, APIKey: "secret"}, "https://new-api.example/api/usage/token/")

	require.ErrorContains(t, err, "不受信任")
	require.Len(t, client.requests, 1)
}

func TestSub2APIQuotaProviderParsesSubscriptionWindowsAndExpiry(t *testing.T) {
	client := &accountQuotaHTTPStub{response: `{
		"mode":"unrestricted","planName":"Pro","expires_at":"2030-01-01T00:00:00Z",
		"subscription":{
			"daily_usage_usd":2,"daily_limit_usd":10,"daily_window_start":"2030-01-10T00:00:00Z",
			"weekly_usage_usd":8,"weekly_limit_usd":50,"weekly_window_start":"2030-01-08T00:00:00Z",
			"monthly_usage_usd":20,"monthly_limit_usd":100,"monthly_window_start":"2030-01-01T00:00:00Z",
			"expires_at":"2030-02-01T00:00:00Z"
		}
	}`}
	provider := &sub2APIQuotaProvider{client: client}

	result, err := provider.Fetch(t.Context(), AccountQuotaFetchInput{Account: &Account{ID: 9}, BaseURL: "https://sub2.example", APIKey: "key"})

	require.NoError(t, err)
	require.Equal(t, "https://sub2.example/v1/usage", client.request.URL.String())
	require.Equal(t, "Pro", result.Plan.Name)
	require.NotNil(t, result.Plan.ExpiresAt)
	require.Len(t, result.Metrics, 3)
	require.Equal(t, float64(8), *result.Metrics[0].Remaining)
	require.Equal(t, float64(42), *result.Metrics[1].Remaining)
	require.Equal(t, time.Date(2030, time.January, 11, 0, 0, 0, 0, time.UTC), *result.Metrics[0].ResetAt)
	require.Equal(t, time.Date(2030, time.January, 15, 0, 0, 0, 0, time.UTC), *result.Metrics[1].ResetAt)
	require.Equal(t, time.Date(2030, time.January, 31, 0, 0, 0, 0, time.UTC), *result.Metrics[2].ResetAt)
}

func TestSub2APIQuotaProviderParsesWalletBalanceWithoutCapacityLimit(t *testing.T) {
	client := &accountQuotaHTTPStub{response: `{
		"mode":"unrestricted","planName":"钱包余额","remaining":100,"balance":100,"unit":"USD"
	}`}
	provider := &sub2APIQuotaProvider{client: client}

	result, err := provider.Fetch(t.Context(), AccountQuotaFetchInput{Account: &Account{ID: 11}, BaseURL: "https://sub2.example", APIKey: "key"})

	require.NoError(t, err)
	require.Len(t, result.Metrics, 1)
	require.Equal(t, "total", result.Metrics[0].Key)
	require.Nil(t, result.Metrics[0].Limit)
	require.Nil(t, result.Metrics[0].Used)
	require.Nil(t, result.Metrics[0].Utilization)
	require.Equal(t, float64(100), *result.Metrics[0].Remaining)
}

func TestLegacySub2APIResetFallbacksFillDailyAndMonthlyCountdowns(t *testing.T) {
	subscription := map[string]any{
		"daily_usage_usd":     2.0,
		"daily_limit_usd":     10.0,
		"monthly_usage_usd":   20.0,
		"monthly_limit_usd":   100.0,
		"weekly_window_start": "2030-01-08T00:00:00Z",
		"expires_at":          "2030-02-01T00:00:00Z",
	}

	applyLegacySub2APIResetFallbacks(subscription, time.Date(2030, time.January, 10, 12, 0, 0, 0, time.UTC))
	result := &AccountQuotaResult{}
	appendSub2APISubscriptionMetric(result, subscription, "daily", "日配额", "day")
	appendSub2APISubscriptionMetric(result, subscription, "monthly", "月配额", "month")

	require.Len(t, result.Metrics, 2)
	require.Equal(t, time.Date(2030, time.January, 11, 0, 0, 0, 0, time.UTC), *result.Metrics[0].ResetAt)
	require.Equal(t, time.Date(2030, time.February, 1, 0, 0, 0, 0, time.UTC), *result.Metrics[1].ResetAt)
}

func TestUpstreamQuotaModeDoesNotEnableLocalQuotaScheduling(t *testing.T) {
	account := &Account{Type: AccountTypeAPIKey, Extra: map[string]any{
		AccountQuotaModeExtraKey: AccountQuotaModeUpstream,
		"quota_limit":            10.0,
		"quota_used":             10.0,
	}}

	require.False(t, account.HasAnyQuotaLimit())
	require.False(t, account.IsQuotaExceeded())
}

func TestValidateAccountQuotaConfigKeepsProviderIDsExtensible(t *testing.T) {
	account := &Account{Type: AccountTypeAPIKey, Extra: map[string]any{
		AccountQuotaModeExtraKey:     AccountQuotaModeUpstream,
		AccountQuotaProviderExtraKey: "Future-Provider",
	}}

	require.NoError(t, validateAccountQuotaConfig(account))
	require.Equal(t, "future-provider", account.Extra[AccountQuotaProviderExtraKey])
}

func TestValidateAccountQuotaConfigNormalizesNewAPIUserID(t *testing.T) {
	account := &Account{Type: AccountTypeAPIKey, Extra: map[string]any{
		AccountQuotaModeExtraKey:     AccountQuotaModeUpstream,
		AccountQuotaProviderExtraKey: "newapi",
		"upstream_quota_config":      map[string]any{"user_id": float64(42), "quota_per_usd": "250000"},
	}}

	require.NoError(t, validateAccountQuotaConfig(account))
	require.Equal(t, "42", account.Extra["upstream_quota_config"].(map[string]any)["user_id"])
	require.Equal(t, float64(250000), account.Extra["upstream_quota_config"].(map[string]any)["quota_per_usd"])
}

func TestValidateAccountQuotaConfigRejectsInvalidNewAPIUserID(t *testing.T) {
	account := &Account{Type: AccountTypeAPIKey, Extra: map[string]any{
		AccountQuotaModeExtraKey:     AccountQuotaModeUpstream,
		AccountQuotaProviderExtraKey: "newapi",
		"upstream_quota_config":      map[string]any{"user_id": -1},
	}}

	require.Error(t, validateAccountQuotaConfig(account))
}

func TestValidateAccountQuotaConfigRejectsInvalidNewAPIQuotaRate(t *testing.T) {
	account := &Account{Type: AccountTypeAPIKey, Extra: map[string]any{
		AccountQuotaModeExtraKey:     AccountQuotaModeUpstream,
		AccountQuotaProviderExtraKey: "newapi",
		"upstream_quota_config":      map[string]any{"quota_per_usd": 0},
	}}

	require.Error(t, validateAccountQuotaConfig(account))
}

func TestValidateAccountQuotaConfigClearsManualQuotaFieldsInUpstreamMode(t *testing.T) {
	account := &Account{Type: AccountTypeAPIKey, Extra: map[string]any{
		AccountQuotaModeExtraKey:     AccountQuotaModeUpstream,
		AccountQuotaProviderExtraKey: "newapi",
		"quota_limit":                10.0,
		"quota_used":                 3.0,
	}}

	require.NoError(t, validateAccountQuotaConfig(account))
	require.NotContains(t, account.Extra, "quota_limit")
	require.NotContains(t, account.Extra, "quota_used")
}
