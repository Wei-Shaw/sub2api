package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// service_account 不设上限也始终输出顶层 quota_used（外部系统依赖此契约读取账号累计费用）。
func TestAccountFromServiceShallow_ServiceAccountAlwaysOutputsQuotaUsed(t *testing.T) {
	t.Run("no limit and never accumulated outputs zero", func(t *testing.T) {
		out := AccountFromServiceShallow(&service.Account{
			ID:       1,
			Platform: service.PlatformGemini,
			Type:     service.AccountTypeServiceAccount,
		})
		require.NotNil(t, out.QuotaUsed)
		require.Zero(t, *out.QuotaUsed)
		require.Nil(t, out.QuotaLimit)
		require.Nil(t, out.QuotaDailyLimit)
		require.Nil(t, out.QuotaWeeklyLimit)

		payload, err := json.Marshal(out)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(payload, &decoded))
		require.Contains(t, decoded, "quota_used")
		require.EqualValues(t, 0, decoded["quota_used"])
		require.NotContains(t, decoded, "quota_limit")
	})

	t.Run("no limit outputs accumulated quota_used", func(t *testing.T) {
		out := AccountFromServiceShallow(&service.Account{
			ID:       2,
			Platform: service.PlatformGemini,
			Type:     service.AccountTypeServiceAccount,
			Extra:    map[string]any{"quota_used": 12.5},
		})
		require.NotNil(t, out.QuotaUsed)
		require.InDelta(t, 12.5, *out.QuotaUsed, 1e-9)
		require.Nil(t, out.QuotaLimit)
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

	t.Run("lite list item keeps quota_used", func(t *testing.T) {
		item := AccountListItemFromAccount(AccountFromServiceShallow(&service.Account{
			ID:       4,
			Platform: service.PlatformGemini,
			Type:     service.AccountTypeServiceAccount,
			Extra:    map[string]any{"quota_used": 7.25},
		}))
		payload, err := json.Marshal(item)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(payload, &decoded))
		require.InDelta(t, 7.25, decoded["quota_used"], 1e-9)
		require.NotContains(t, decoded, "quota_limit")
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
