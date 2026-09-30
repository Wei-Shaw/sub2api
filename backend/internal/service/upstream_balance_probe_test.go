package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeBalanceProbeSource(t *testing.T) {
	require.Equal(t, BalanceProbeSourceSub2API, NormalizeBalanceProbeSource(" Sub2API "))
	require.Equal(t, BalanceProbeSourceNewAPI, NormalizeBalanceProbeSource("newapi"))
	require.Equal(t, "", NormalizeBalanceProbeSource("auto"))
	require.Equal(t, "", NormalizeBalanceProbeSource(""))
}

func TestParseSub2APIUsageBalanceWallet(t *testing.T) {
	balance, currency, available, ok := parseSub2APIUsageBalance([]byte(
		`{"mode":"unrestricted","remaining":12.5,"unit":"USD","balance":12.5}`,
	))
	require.True(t, ok)
	require.True(t, available)
	require.Equal(t, 12.5, balance)
	require.Equal(t, "USD", currency)
}

func TestParseSub2APIUsageBalanceQuota(t *testing.T) {
	balance, currency, available, ok := parseSub2APIUsageBalance([]byte(
		`{"mode":"quota_limited","quota":{"limit":100,"used":40,"remaining":60,"unit":"USD"}}`,
	))
	require.True(t, ok)
	require.True(t, available)
	require.Equal(t, 60.0, balance)
	require.Equal(t, "USD", currency)
}

func TestParseUpstreamBalanceBodyNewAPI(t *testing.T) {
	balance, currency, available, ok := parseUpstreamBalanceBody(BalanceProbeSourceNewAPI, []byte(
		`{"code":true,"data":{"remaining":87.01,"unit":"USD","is_active":true}}`,
	))
	require.True(t, ok)
	require.True(t, available)
	require.Equal(t, 87.01, balance)
	require.Equal(t, "USD", currency)
}
