package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpstreamBillingProbeDeepSeekBalancePersistsWithoutSyncingRate(t *testing.T) {
	configuredRate := 0.75
	account := &Account{
		ID:             5272,
		Platform:       PlatformOpenAI,
		Type:           AccountTypeAPIKey,
		Status:         StatusActive,
		Concurrency:    2,
		RateMultiplier: &configuredRate,
		Credentials: map[string]any{
			"api_key":  "sk-deepseek-secret",
			"base_url": "https://api.deepseek.com/v1",
		},
		Extra: map[string]any{
			UpstreamBillingProbeEnabledExtraKey:    true,
			UpstreamBillingRateSyncEnabledExtraKey: true,
		},
	}
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"is_available":true,
			"balance_infos":[
				{"currency":"CNY","total_balance":"40.2400","granted_balance":"0.00","topped_up_balance":"40.2400"},
				{"currency":"USD","total_balance":"5.10","granted_balance":"1.10","topped_up_balance":"4.00"}
			]
		}`)),
	}}
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})
	fixedNow := time.Date(2026, time.August, 4, 8, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, UpstreamBillingProbeStatusOK, snapshot.Status)
	require.Equal(t, "deepseek.user_balance", snapshot.Data["object"])
	require.Equal(t, "deepseek", snapshot.Data["provider"])
	require.Equal(t, true, snapshot.Data["is_available"])
	require.NotContains(t, snapshot.Data, "resolved_rate_multiplier")
	require.Nil(t, snapshot.SyncedRateMultiplier)

	balances, ok := snapshot.Data["balance_infos"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, balances, 2)
	require.Equal(t, "40.2400", balances[0]["total_balance"])
	require.Equal(t, "USD", balances[1]["currency"])

	require.Equal(t, "https://api.deepseek.com/user/balance", upstream.lastReq.URL.String())
	require.Equal(t, http.MethodGet, upstream.lastReq.Method)
	require.Equal(t, "Bearer sk-deepseek-secret", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
	require.Equal(t, configuredRate, *account.RateMultiplier)

	persisted := decodeUpstreamBillingProbeSnapshot(account.Extra)
	require.NotNil(t, persisted)
	require.Equal(t, UpstreamBillingProbeStatusOK, persisted.Status)
	encoded, err := json.Marshal(persisted)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "sk-deepseek-secret")
}

func TestParseDeepSeekBalanceResponseAcceptsUnavailableAndPreservesDecimals(t *testing.T) {
	data, err := parseDeepSeekBalanceResponse([]byte(`{
		"is_available":false,
		"balance_infos":[
			{"currency":"cny","total_balance":"0.00000001","granted_balance":"0","topped_up_balance":"0.00000001"}
		]
	}`))
	require.NoError(t, err)
	require.Equal(t, false, data["is_available"])
	balances, ok := data["balance_infos"].([]map[string]any)
	require.True(t, ok)
	require.Equal(t, "CNY", balances[0]["currency"])
	require.Equal(t, "0.00000001", balances[0]["total_balance"])

	data, err = parseDeepSeekBalanceResponse([]byte(`{"is_available":false,"balance_infos":[]}`))
	require.NoError(t, err)
	require.Empty(t, data["balance_infos"])
}

func TestParseDeepSeekBalanceResponseRejectsInvalidPayloads(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed json", body: `{"is_available":`},
		{name: "missing availability", body: `{"balance_infos":[]}`},
		{name: "available without balances", body: `{"is_available":true,"balance_infos":[]}`},
		{name: "negative balance", body: `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"-1","granted_balance":"0","topped_up_balance":"0"}]}`},
		{name: "exponent balance", body: `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"1e3","granted_balance":"0","topped_up_balance":"1000"}]}`},
		{name: "non decimal balance", body: `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"NaN","granted_balance":"0","topped_up_balance":"0"}]}`},
		{name: "invalid currency", body: `{"is_available":true,"balance_infos":[{"currency":"CNY<script>","total_balance":"1","granted_balance":"0","topped_up_balance":"1"}]}`},
		{name: "duplicate currency", body: `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"1","granted_balance":"0","topped_up_balance":"1"},{"currency":"cny","total_balance":"2","granted_balance":"0","topped_up_balance":"2"}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseDeepSeekBalanceResponse([]byte(tt.body))
			require.Error(t, err)
		})
	}
}

func TestUpstreamBillingProbeDeepSeekHTTPFailuresAreSanitized(t *testing.T) {
	for _, statusCode := range []int{
		http.StatusUnauthorized,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusNotFound,
	} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			account := &Account{
				ID:       int64(6000 + statusCode),
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Status:   StatusActive,
				Credentials: map[string]any{
					"api_key":  "sk-must-not-leak",
					"base_url": "https://api.deepseek.com",
				},
			}
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: statusCode,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"upstream failure"}}`)),
			}}
			svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{})

			snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
			require.NoError(t, err)
			require.Equal(t, statusCode, snapshot.HTTPStatus)
			if statusCode == http.StatusNotFound {
				require.Equal(t, UpstreamBillingProbeStatusUnsupported, snapshot.Status)
				require.Equal(t, "unsupported", snapshot.LastError)
			} else {
				require.Equal(t, UpstreamBillingProbeStatusFailed, snapshot.Status)
				require.Equal(t, "http_error", snapshot.LastError)
			}
			encoded, marshalErr := json.Marshal(snapshot)
			require.NoError(t, marshalErr)
			require.NotContains(t, string(encoded), "sk-must-not-leak")
		})
	}
}

func TestDeepSeekBillingProbeTargetAndURLAreStrict(t *testing.T) {
	require.True(t, upstreamBillingProbeTargetIsDeepSeekOfficialAPI("https://api.deepseek.com"))
	require.True(t, upstreamBillingProbeTargetIsDeepSeekOfficialAPI("HTTPS://API.DEEPSEEK.COM.:443/v1"))
	require.False(t, upstreamBillingProbeTargetIsDeepSeekOfficialAPI("https://deepseek.com"))
	require.False(t, upstreamBillingProbeTargetIsDeepSeekOfficialAPI("https://api.deepseek.com.evil.example"))
	require.False(t, upstreamBillingProbeTargetIsDeepSeekOfficialAPI("https://notdeepseek.com"))

	require.Equal(t,
		"https://api.deepseek.com/user/balance",
		buildDeepSeekBalanceURL("https://api.deepseek.com/v1?ignored=true#fragment"),
	)
}
