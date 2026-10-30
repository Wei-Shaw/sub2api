package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// service_account 与 apikey 同语义：设了总额度才输出 quota_limit / quota_used。
func TestAccountFromServiceShallow_ServiceAccountQuota(t *testing.T) {
	t.Run("no limit omits quota fields", func(t *testing.T) {
		out := AccountFromServiceShallow(&service.Account{
			ID:       1,
			Platform: service.PlatformGemini,
			Type:     service.AccountTypeServiceAccount,
			Extra:    map[string]any{"quota_used": 12.5},
		})
		require.Nil(t, out.QuotaUsed)
		require.Nil(t, out.QuotaLimit)
		require.Nil(t, out.QuotaDailyLimit)
		require.Nil(t, out.QuotaWeeklyLimit)

		payload, err := json.Marshal(out)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(payload, &decoded))
		require.NotContains(t, decoded, "quota_used")
		require.NotContains(t, decoded, "quota_limit")
	})

	t.Run("with limits outputs limits and usage", func(t *testing.T) {
		out := AccountFromServiceShallow(&service.Account{
			ID:       3,
			Platform: service.PlatformGemini,
			Type:     service.AccountTypeServiceAccount,
			Extra: map[string]any{
				"quota_used":        30.0,
				"quota_limit":       100.0,
				"quota_daily_limit": 10.0,
			},
		})
		require.NotNil(t, out.QuotaLimit)
		require.InDelta(t, 100.0, *out.QuotaLimit, 1e-9)
		require.NotNil(t, out.QuotaUsed)
		require.InDelta(t, 30.0, *out.QuotaUsed, 1e-9)
		require.NotNil(t, out.QuotaDailyLimit)
		require.NotNil(t, out.QuotaDailyUsed)
	})

	t.Run("lite list item keeps quota fields", func(t *testing.T) {
		item := AccountListItemFromAccount(AccountFromServiceShallow(&service.Account{
			ID:       4,
			Platform: service.PlatformGemini,
			Type:     service.AccountTypeServiceAccount,
			Extra:    map[string]any{"quota_used": 7.25, "quota_limit": 290.0},
		}))
		payload, err := json.Marshal(item)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(payload, &decoded))
		require.InDelta(t, 7.25, decoded["quota_used"], 1e-9)
		require.InDelta(t, 290.0, decoded["quota_limit"], 1e-9)
	})
}

// apikey 维持原语义：未设总额度时不输出 quota_used。
func TestAccountFromServiceShallow_APIKeyWithoutLimitOmitsQuotaUsed(t *testing.T) {
	out := AccountFromServiceShallow(&service.Account{
		ID:    5,
		Type:  service.AccountTypeAPIKey,
		Extra: map[string]any{"quota_used": 5.0},
	})
	require.Nil(t, out.QuotaUsed)
	require.Nil(t, out.QuotaLimit)
}
