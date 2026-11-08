//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 配额模式 repo 层集成测试：
//   - Create/GetByID/Update 的 check_mode/account_id 往返
//   - InsertHistoryBatch → ListHistory / ListLatestForMonitorIDs 的 quota JSONB 回读
//     （裸 SQL 列 + scanMonitorQuota），以及探活模式旧行 quota=NULL 的兼容
//
// 注意 channelMonitorRepository 的 GetByID/裸 SQL 走全局 client（不识别 tx ctx），
// 因此本文件用 integrationEntClient 直连 + t.Cleanup 显式清理，不走 testEntTx 回滚。

func TestChannelMonitorQuotaModeRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewChannelMonitorRepository(integrationEntClient, integrationDB)

	account := mustCreateAccount(t, integrationEntClient, &service.Account{
		Name: "quota-linked-kimi", Platform: domain.PlatformKimi, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-kimi", "account_mode": service.AccountModeCoding},
	})
	t.Cleanup(func() {
		_ = integrationEntClient.Account.DeleteOneID(account.ID).Exec(ctx)
	})

	created := &service.ChannelMonitor{
		Name:             "kimi-quota-roundtrip",
		Provider:         service.MonitorProviderKimi,
		APIMode:          service.MonitorAPIModeChatCompletions,
		Endpoint:         "",
		APIKey:           "encrypted-empty",
		PrimaryModel:     "quota",
		Enabled:          true,
		IntervalSeconds:  60,
		CheckMode:        service.MonitorCheckModeQuota,
		AccountID:        &account.ID,
		BodyOverrideMode: service.MonitorBodyOverrideModeOff,
	}
	require.NoError(t, repo.Create(ctx, created))
	t.Cleanup(func() {
		_ = repo.Delete(ctx, created.ID)
	})

	loaded, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, service.MonitorCheckModeQuota, loaded.CheckMode)
	require.NotNil(t, loaded.AccountID)
	require.Equal(t, account.ID, *loaded.AccountID)

	// Update：切换模式并清空关联账号（probe 化）。
	loaded.CheckMode = service.MonitorCheckModeProbe
	loaded.AccountID = nil
	loaded.Endpoint = "https://api.moonshot.cn"
	require.NoError(t, repo.Update(ctx, loaded))

	reloaded, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, service.MonitorCheckModeProbe, reloaded.CheckMode)
	require.Nil(t, reloaded.AccountID)

	// 重新绑定账号。
	reloaded.CheckMode = service.MonitorCheckModeQuotaProbe
	reloaded.AccountID = &account.ID
	require.NoError(t, repo.Update(ctx, reloaded))
	final, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, service.MonitorCheckModeQuotaProbe, final.CheckMode)
	require.NotNil(t, final.AccountID)
}

func TestChannelMonitorHistoryQuotaRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewChannelMonitorRepository(integrationEntClient, integrationDB)

	monitor := &service.ChannelMonitor{
		Name:             "quota-history-roundtrip",
		Provider:         service.MonitorProviderOpenAI,
		APIMode:          service.MonitorAPIModeChatCompletions,
		Endpoint:         "https://api.openai.com",
		APIKey:           "encrypted",
		PrimaryModel:     "gpt-test",
		ExtraModels:      []string{"gpt-extra"},
		Enabled:          true,
		IntervalSeconds:  60,
		BodyOverrideMode: service.MonitorBodyOverrideModeOff,
	}
	require.NoError(t, repo.Create(ctx, monitor))
	t.Cleanup(func() {
		_ = repo.Delete(ctx, monitor.ID) // histories 级联删除
	})

	now := time.Now().UTC()
	rows := []*service.ChannelMonitorHistoryRow{
		{
			MonitorID: monitor.ID, Model: "gpt-test", Status: service.MonitorStatusOperational,
			Message: "ok", CheckedAt: now,
			Quota: &domain.MonitorQuotaSnapshot{
				Source:    "usage",
				Success:   true,
				PlanLevel: "PRO",
				Tiers: []domain.MonitorQuotaTier{
					{Window: "5h", UsedPercent: 42.5, Used: 17, Limit: 40, ResetAt: now.Add(time.Hour).Format(time.RFC3339)},
				},
				FetchedAt: now,
			},
		},
		{
			// 探活模式旧行：无 quota（NULL 兼容）。
			MonitorID: monitor.ID, Model: "gpt-extra", Status: service.MonitorStatusOperational,
			Message: "ok", CheckedAt: now,
		},
	}
	require.NoError(t, repo.InsertHistoryBatch(ctx, rows))

	history, err := repo.ListHistory(ctx, monitor.ID, "", 10)
	require.NoError(t, err)
	require.Len(t, history, 2)

	byModel := map[string]*service.ChannelMonitorHistoryEntry{}
	for _, entry := range history {
		byModel[entry.Model] = entry
	}
	withQuota := byModel["gpt-test"]
	require.NotNil(t, withQuota.Quota)
	require.True(t, withQuota.Quota.Success)
	require.Equal(t, "usage", withQuota.Quota.Source)
	require.Equal(t, "PRO", withQuota.Quota.PlanLevel)
	require.Len(t, withQuota.Quota.Tiers, 1)
	require.Equal(t, "5h", withQuota.Quota.Tiers[0].Window)
	require.InDelta(t, 42.5, withQuota.Quota.Tiers[0].UsedPercent, 0.001)
	require.Nil(t, byModel["gpt-extra"].Quota, "probe rows must read back as NULL quota")

	// 用户视图聚合：主模型最近一行带快照。
	latest, err := repo.ListLatestForMonitorIDs(ctx, []int64{monitor.ID})
	require.NoError(t, err)
	primaryRows := latest[monitor.ID]
	require.NotEmpty(t, primaryRows)
	var primaryQuota *domain.MonitorQuotaSnapshot
	for _, row := range primaryRows {
		if row.Model == "gpt-test" {
			primaryQuota = row.Quota
		}
	}
	require.NotNil(t, primaryQuota, "ListLatestForMonitorIDs must surface quota for the primary model")
	require.True(t, primaryQuota.Success)
}
