//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// --- provider 放行（M-1 的 service 层一半） ---

func TestOllamaCloudMonitorProviderAllowlists(t *testing.T) {
	require.NoError(t, validateProvider(MonitorProviderOllamaCloud))
	require.NoError(t, validateAPIMode(MonitorProviderOllamaCloud, ""))
	require.NoError(t, validateAPIMode(MonitorProviderOllamaCloud, MonitorAPIModeChatCompletions))
	// responses 探活仍仅 openai（与所有非 openai provider 一致）。
	require.ErrorIs(t, validateAPIMode(MonitorProviderOllamaCloud, MonitorAPIModeResponses), ErrChannelMonitorInvalidAPIMode)

	// ollama_cloud 官方有 OpenAI 兼容 Chat Completions → 三种模式全部放行
	//（对比 antigravity 只允许 quota）。
	require.True(t, providerSupportsProbe(MonitorProviderOllamaCloud))
	require.NoError(t, validateCheckMode(MonitorProviderOllamaCloud, MonitorCheckModeProbe))
	require.NoError(t, validateCheckMode(MonitorProviderOllamaCloud, MonitorCheckModeQuota))
	require.NoError(t, validateCheckMode(MonitorProviderOllamaCloud, MonitorCheckModeQuotaProbe))
}

// 行为钉：opencode_go 在 handler binding 里放行、但 service 层 monitorProviders
// 没有（upstream 有单独修复，本分支尚未合入）——钉住现状，防止被误认为本批回归。
func TestOpenCodeGoMonitorProviderStillMissingFromServiceAllowlist(t *testing.T) {
	require.ErrorIs(t, validateProvider("opencode_go"), ErrChannelMonitorInvalidProvider)
}

// --- 探活 adapter ---

func TestOllamaCloudMonitorAdapterProbesOpenAIChatCompletions(t *testing.T) {
	h := &openAICaptureHandler{}
	endpoint := setupFakeOpenAI(t, h)

	res := runCheckForModel(context.Background(), MonitorProviderOllamaCloud, endpoint+"/v1", "sk-ollama", "gpt-oss:120b-cloud", nil)

	require.Equal(t, MonitorStatusOperational, res.Status)
	require.Equal(t, providerOpenAIPath, h.lastPath)
	require.Equal(t, "gpt-oss:120b-cloud", h.lastBody["model"])
	require.Equal(t, "Bearer sk-ollama", h.lastHeaders.Get("Authorization"))
}

func TestOllamaCloudMonitorMergeDenyListProtectsChallenge(t *testing.T) {
	adapter, apiMode, ok := providerAdapterFor(MonitorProviderOllamaCloud, "")
	require.True(t, ok)
	require.Equal(t, MonitorAPIModeChatCompletions, apiMode)

	body, err := buildRequestBody(adapter, MonitorProviderOllamaCloud, apiMode, "gpt-oss:120b-cloud", "2+2=?", &CheckOptions{
		BodyOverrideMode: MonitorBodyOverrideModeMerge,
		BodyOverride:     map[string]any{"model": "evil", "temperature": 0.5},
	})
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	require.Equal(t, "gpt-oss:120b-cloud", parsed["model"], "model 命中黑名单不得被覆盖")
	require.InDelta(t, 0.5, parsed["temperature"], 0.0001, "非黑名单 key 应正常合并")
}

// --- quota 取数（C29） ---

func ollamaCloudQuotaTestAccount(id int64, extra map[string]any) *Account {
	return &Account{
		ID:          id,
		Name:        "ollama-quota-test",
		Platform:    domain.PlatformOllamaCloud,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-ollama",
			"base_url": "https://ollama.com/v1",
		},
		Extra: extra,
	}
}

func ollamaCloudUsageSnapshotExtra(status string, fetchedAt time.Time, data *OllamaCloudUsageData, lastError string) map[string]any {
	snapshot := map[string]any{
		"status":          status,
		"last_attempt_at": fetchedAt,
		"next_refresh_at": fetchedAt,
		"fetched_at":      fetchedAt,
	}
	if data != nil {
		snapshot["data"] = data
	}
	if lastError != "" {
		snapshot["last_error"] = lastError
	}
	return map[string]any{
		OllamaCloudUsageSnapshotExtraKey: snapshot,
	}
}

func TestQuotaFetcher_OllamaCloudOkSnapshotMapsTiersAndBalance(t *testing.T) {
	fetcher, usage, cnQuota, cnBalance, accounts := newQuotaFetcherTestSetup(t)
	fetchedAt := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	accounts.accounts[31] = ollamaCloudQuotaTestAccount(31, ollamaCloudUsageSnapshotExtra(
		OllamaCloudUsageStatusOK, fetchedAt,
		&OllamaCloudUsageData{
			Plan:     "pro",
			FiveHour: &OllamaCloudUsageWindow{UsedPercent: 42.5},
			SevenDay: &OllamaCloudUsageWindow{UsedPercent: 10},
			Balance:  "$9.50",
		},
		"",
	))

	snapshot := fetcher.Fetch(context.Background(), 31)

	require.True(t, snapshot.Success)
	require.Equal(t, "ollama_quota", snapshot.Source)
	require.Equal(t, "pro", snapshot.PlanLevel)
	require.False(t, snapshot.CredentialInvalid)
	require.Empty(t, snapshot.Error)
	// 快照是被动抓取的历史值：FetchedAt 沿用真实抓取时间。
	require.True(t, fetchedAt.Equal(snapshot.FetchedAt))

	require.Len(t, snapshot.Tiers, 2)
	require.Equal(t, "5h", snapshot.Tiers[0].Window)
	require.InDelta(t, 42.5, snapshot.Tiers[0].UsedPercent, 0.001)
	require.Equal(t, "7d", snapshot.Tiers[1].Window)
	require.InDelta(t, 10, snapshot.Tiers[1].UsedPercent, 0.001)

	require.NotNil(t, snapshot.Balance)
	require.InDelta(t, 9.5, *snapshot.Balance, 0.001)
	require.Equal(t, "USD", snapshot.Currency)

	require.Equal(t, 0, usage.getCalls(), "ollama 走快照读取，不得打海外 usage 服务")
	require.Equal(t, 0, cnQuota.calls)
	require.Equal(t, 0, cnBalance.calls)
	require.Equal(t, MonitorStatusOperational, deriveQuotaCheckResult(snapshot, "quota", time.Now()).Status)
}

// credits 类账号：只有余额没有窗口 → only-balance 降级（余额文本可解析时透出）。
func TestQuotaFetcher_OllamaCloudBalanceOnlySnapshot(t *testing.T) {
	fetcher, _, _, _, accounts := newQuotaFetcherTestSetup(t)
	accounts.accounts[32] = ollamaCloudQuotaTestAccount(32, ollamaCloudUsageSnapshotExtra(
		OllamaCloudUsageStatusOK, time.Now(),
		&OllamaCloudUsageData{Balance: "USD$12"},
		"",
	))

	snapshot := fetcher.Fetch(context.Background(), 32)

	require.True(t, snapshot.Success)
	require.Empty(t, snapshot.Tiers)
	require.NotNil(t, snapshot.Balance)
	require.InDelta(t, 12, *snapshot.Balance, 0.001)
}

// 无法解析的余额文本跳过（余额仅展示，不做停调判定）。
func TestQuotaFetcher_OllamaCloudUnparsableBalanceSkipped(t *testing.T) {
	_, ok := parseOllamaCloudBalanceUSD("")
	require.False(t, ok)
	_, ok = parseOllamaCloudBalanceUSD("$9.50 / $20.00")
	require.False(t, ok)

	value, ok := parseOllamaCloudBalanceUSD("USD$1,234.50")
	require.True(t, ok)
	require.InDelta(t, 1234.5, value, 0.001)
}

// status!=ok（unauthorized/failed）→ 错误快照，带 last_error 与 fetched_at；
// unauthorized 是抓取通道（settings 页会话）过期，不判凭据失效 → 推导 error 而非 failed。
func TestQuotaFetcher_OllamaCloudFailedSnapshotsYieldErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  string
		wantSub string
	}{
		{name: "unauthorized session", status: OllamaCloudUsageStatusUnauthorized, wantSub: "session expired"},
		{name: "scrape failed", status: OllamaCloudUsageStatusFailed, wantSub: "usage page unreachable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fetcher, _, _, _, accounts := newQuotaFetcherTestSetup(t)
			fetchedAt := time.Now().Add(-3 * time.Hour).UTC()
			accounts.accounts[33] = ollamaCloudQuotaTestAccount(33, ollamaCloudUsageSnapshotExtra(
				tc.status, fetchedAt, nil, tc.wantSub,
			))

			snapshot := fetcher.Fetch(context.Background(), 33)

			require.False(t, snapshot.Success)
			require.False(t, snapshot.CredentialInvalid)
			require.Contains(t, snapshot.Error, tc.wantSub)
			require.Contains(t, snapshot.Error, "fetched at")
			require.Contains(t, snapshot.Error, fetchedAt.Format("2006-01-02"))
			require.Equal(t, MonitorStatusError, deriveQuotaCheckResult(snapshot, "quota", time.Now()).Status)
		})
	}
}

// eligible=false（host 门禁：反代域名）→ 带原因码的可解释错误，不是笼统报错。
func TestQuotaFetcher_OllamaCloudIneligibleBaseURLYieldsExplainableError(t *testing.T) {
	fetcher, usage, _, _, accounts := newQuotaFetcherTestSetup(t)
	account := ollamaCloudQuotaTestAccount(34, nil)
	account.Credentials["base_url"] = "https://proxy.example.com/v1"
	accounts.accounts[34] = account

	snapshot := fetcher.Fetch(context.Background(), 34)

	require.False(t, snapshot.Success)
	require.Contains(t, snapshot.Error, "unsupported_base_url")
	require.Equal(t, 0, usage.getCalls())
}

// 尚无快照（未配置会话或还没有请求活动）→ 可解释错误。
func TestQuotaFetcher_OllamaCloudNoSnapshotYieldsExplainableError(t *testing.T) {
	fetcher, _, _, _, accounts := newQuotaFetcherTestSetup(t)
	accounts.accounts[35] = ollamaCloudQuotaTestAccount(35, nil)

	snapshot := fetcher.Fetch(context.Background(), 35)

	require.False(t, snapshot.Success)
	require.Contains(t, snapshot.Error, "no ollama cloud usage snapshot yet")
}

// 回归：opencode_go 仍走 CN quota 路由，kimi coding 仍走 CN quota（kimi 已有
// 独立用例，这里钉 opencode_go 与 ollama 互不串线）。
func TestQuotaFetcher_OpenCodeGoStillRoutesToCNQuota(t *testing.T) {
	fetcher, usage, cnQuota, _, accounts := newQuotaFetcherTestSetup(t)
	accounts.accounts[36] = &Account{ID: 36, Platform: domain.PlatformOpenCodeGo, Type: AccountTypeAPIKey}
	cnQuota.result = &CNProviderQuotaProbeResult{Success: true}

	snapshot := fetcher.Fetch(context.Background(), 36)

	require.True(t, snapshot.Success)
	require.Equal(t, "cn_quota", snapshot.Source)
	require.Equal(t, 1, cnQuota.calls)
	require.Equal(t, 0, usage.getCalls())
}
