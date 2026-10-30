package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 账号列表页固定传 lite=1，lite 投影（AccountListItemFromAccount）必须与完整
// Account DTO 一样携带 opencode_go_usage，否则前端用量单元格会退回「余额 --」。
func TestAccountListItemFromAccount_CarriesOpenCodeGoUsage(t *testing.T) {
	now := time.Now().UTC()
	// 生产账号 164 的形态：deepseek 平台 + OpenCode Go 官方基址 + apikey
	eligible := &service.Account{
		ID: 164, Name: "DeepSeek OpenCode", Platform: service.PlatformDeepseek, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://opencode.ai/zen/go/v1", "api_key": "test-key"},
		Extra: map[string]any{
			service.OpenCodeGoUsageAutoRefreshExtraKey: true,
			service.OpenCodeGoUsageSnapshotExtraKey: &service.OpenCodeGoUsageSnapshot{
				Status:        service.OpenCodeGoUsageStatusOK,
				Data:          &service.OpenCodeGoUsageData{Rolling: service.OpenCodeGoUsageWindow{Status: "ok", Percent: 6}},
				LastAttemptAt: now, NextRefreshAt: now.Add(time.Hour),
			},
		},
		Status: service.StatusActive,
	}

	item := AccountListItemFromAccount(AccountFromServiceShallow(eligible))
	require.NotNil(t, item.OpenCodeGoUsage)
	require.True(t, item.OpenCodeGoUsage.Eligible)
	require.True(t, item.OpenCodeGoUsage.AutoRefreshEnabled)
	require.NotNil(t, item.OpenCodeGoUsage.Snapshot)
	require.NotNil(t, item.OpenCodeGoUsage.Snapshot.Data)
	require.Equal(t, 6.0, item.OpenCodeGoUsage.Snapshot.Data.Rolling.Percent)

	raw, err := json.Marshal(item)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"opencode_go_usage"`)
	require.Contains(t, string(raw), `"eligible":true`)

	// 对照：base_url 指向 api.deepseek.com 的普通 deepseek 账号不参与用量窗口
	ineligible := &service.Account{
		ID: 165, Name: "DeepSeek Direct", Platform: service.PlatformDeepseek, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://api.deepseek.com", "api_key": "test-key-2"},
		Status:      service.StatusActive,
	}

	plainItem := AccountListItemFromAccount(AccountFromServiceShallow(ineligible))
	require.Nil(t, plainItem.OpenCodeGoUsage)

	plainRaw, err := json.Marshal(plainItem)
	require.NoError(t, err)
	require.NotContains(t, string(plainRaw), `"opencode_go_usage"`)
}
