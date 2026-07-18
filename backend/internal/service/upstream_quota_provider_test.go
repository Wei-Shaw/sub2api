package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpstreamQuotaProviderRegistry(t *testing.T) {
	registry := newUpstreamQuotaProviderRegistry()
	for _, name := range []string{UpstreamQuotaSub2API, UpstreamQuotaNewAPI} {
		provider, ok := registry.Get(name)
		require.True(t, ok)
		require.Equal(t, name, provider.Name())
	}
	_, ok := registry.Get("unknown")
	require.False(t, ok)
}

func TestSub2APIQuotaProviderParse(t *testing.T) {
	quota, err := (sub2APIQuotaProvider{}).Parse([]byte(`{
		"mode":"quota_limited",
		"quota":{"limit":100,"used":23.5,"remaining":76.5,"unit":"USD"}
	}`))
	require.NoError(t, err)
	require.Equal(t, 100.0, quota.Limit)
	require.Equal(t, 23.5, quota.Used)
}

func TestSub2APIQuotaProviderRejectsUnlimitedResponse(t *testing.T) {
	_, err := (sub2APIQuotaProvider{}).Parse([]byte(`{"mode":"unrestricted","balance":20}`))
	require.ErrorContains(t, err, "limited API key quota")
}

func TestNewAPIQuotaProviderParseAndNormalize(t *testing.T) {
	quota, err := (newAPIQuotaProvider{}).Parse([]byte(`{
		"code":true,
		"data":{"object":"token_usage","total_granted":1000000,"total_used":125000,"total_available":875000,"unlimited_quota":false}
	}`))
	require.NoError(t, err)
	require.Equal(t, 2.0, quota.Limit)
	require.Equal(t, 0.25, quota.Used)
}

func TestNewAPIQuotaProviderRejectsUnlimitedQuota(t *testing.T) {
	_, err := (newAPIQuotaProvider{}).Parse([]byte(`{"code":true,"data":{"unlimited_quota":true}}`))
	require.ErrorContains(t, err, "limited token quota")
}

func TestValidateUpstreamQuotaProviderConfig(t *testing.T) {
	extra := map[string]any{
		UpstreamQuotaProviderExtraKey: "NEW-API",
		"quota_limit":                 1.0,
	}
	require.NoError(t, ValidateUpstreamQuotaProviderConfig(AccountTypeAPIKey, extra))
	require.Equal(t, UpstreamQuotaNewAPI, extra[UpstreamQuotaProviderExtraKey])

	require.Error(t, ValidateUpstreamQuotaProviderConfig(AccountTypeOAuth, extra))
	require.Error(t, ValidateUpstreamQuotaProviderConfig(AccountTypeAPIKey, map[string]any{
		UpstreamQuotaProviderExtraKey: UpstreamQuotaSub2API,
	}))
}
