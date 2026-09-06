//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// openCodeGoManagedExtra 构造同时携带探测 / Ollama / OpenCode 受管键的 extra，
// 用于验证各身份清理分支只清自己该清的键、不跨身份误清。
func openCodeGoManagedExtra(now time.Time) map[string]any {
	return map[string]any{
		service.UpstreamBillingProbeExtraKey:        map[string]any{"status": "ok"},
		service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
		service.OllamaCloudUsageAutoRefreshExtraKey: true,
		service.OllamaCloudUsageSnapshotExtraKey: map[string]any{
			"status": service.OllamaCloudUsageStatusOK, "last_attempt_at": now, "next_refresh_at": now.Add(time.Hour),
		},
		service.OpenCodeGoUsageAutoRefreshExtraKey: true,
		service.OpenCodeGoUsageSnapshotExtraKey: map[string]any{
			"status": service.OpenCodeGoUsageStatusOK, "last_attempt_at": now, "next_refresh_at": now.Add(time.Hour),
		},
	}
}

// F1：混合 OpenCode + openai Ollama + anthropic Ollama 行的批量代理变化与
// 代理+key 变化，验证 OpenCode/Ollama CASE 分支互斥、各自只清自己该清的键。
// 回归点：OpenCode eligible 判定若不含旧行 opencode base URL，OpenAI+Ollama 行
// 在代理变化时会先命中 OpenCode 分支而遮蔽 Ollama 快照清理。
func TestBulkUpdateOpenCodeGoMixedPlatformIdentityCleanup(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Now().UTC()

	create := func(name, platform, baseURL string, extra map[string]any) *service.Account {
		t.Helper()
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name: name, Platform: platform, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "shared-key", "base_url": baseURL},
			Extra:       extra,
		})
	}
	ollamaExtra := func() map[string]any {
		return map[string]any{
			service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
			service.OllamaCloudUsageAutoRefreshExtraKey: true,
			service.OllamaCloudUsageSnapshotExtraKey: map[string]any{
				"status": service.OllamaCloudUsageStatusOK, "last_attempt_at": now, "next_refresh_at": now.Add(time.Hour),
			},
		}
	}
	opencode := create("mixed-opencode", service.PlatformOpenAI, "https://opencode.ai/zen/go/v1", map[string]any{
		service.OpenCodeGoUsageAutoRefreshExtraKey: true,
		service.OpenCodeGoUsageSnapshotExtraKey: map[string]any{
			"status": service.OpenCodeGoUsageStatusOK, "last_attempt_at": now, "next_refresh_at": now.Add(time.Hour),
		},
	})
	openaiOllama := create("mixed-openai-ollama", service.PlatformOpenAI, "https://ollama.com", ollamaExtra())
	anthropicOllama := create("mixed-anthropic-ollama", service.PlatformAnthropic, "https://ollama.com", ollamaExtra())
	proxy := mustCreateProxy(t, tx.Client(), &service.Proxy{
		Name: "mixed-proxy", Protocol: "http", Host: "127.0.0.1", Port: 3128,
		Username: "user", Password: "pass", Status: service.StatusActive,
	})
	proxy2 := mustCreateProxy(t, tx.Client(), &service.Proxy{
		Name: "mixed-proxy-2", Protocol: "http", Host: "127.0.0.2", Port: 3128,
		Username: "user", Password: "pass", Status: service.StatusActive,
	})

	// 只改 proxy：OpenCode 行清快照留开关；Ollama 行清快照留 session/开关。
	rows, err := repo.BulkUpdate(ctx, []int64{opencode.ID, openaiOllama.ID, anthropicOllama.ID}, service.AccountBulkUpdate{
		ProxyID: &proxy.ID,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), rows)

	opencodeLoaded, err := repo.GetByID(ctx, opencode.ID)
	require.NoError(t, err)
	require.Equal(t, true, opencodeLoaded.Extra[service.OpenCodeGoUsageAutoRefreshExtraKey])
	require.NotContains(t, opencodeLoaded.Extra, service.OpenCodeGoUsageSnapshotExtraKey)

	for _, id := range []int64{openaiOllama.ID, anthropicOllama.ID} {
		loaded, err := repo.GetByID(ctx, id)
		require.NoError(t, err)
		require.Equal(t, "cipher:wos-session=fixture", loaded.Extra[service.OllamaCloudUsageSessionExtraKey])
		require.Equal(t, true, loaded.Extra[service.OllamaCloudUsageAutoRefreshExtraKey])
		require.NotContains(t, loaded.Extra, service.OllamaCloudUsageSnapshotExtraKey,
			"OpenAI+Ollama 行的快照必须由 Ollama 分支清理，不能被 OpenCode 分支遮蔽")
	}

	// 同时改 proxy + api_key：OpenCode 行清开关+快照；Ollama 行清 session/开关/快照。
	rows, err = repo.BulkUpdate(ctx, []int64{opencode.ID, openaiOllama.ID, anthropicOllama.ID}, service.AccountBulkUpdate{
		ProxyID:     &proxy2.ID,
		Credentials: map[string]any{"api_key": "rotated-key"},
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), rows)

	opencodeLoaded, err = repo.GetByID(ctx, opencode.ID)
	require.NoError(t, err)
	require.NotContains(t, opencodeLoaded.Extra, service.OpenCodeGoUsageAutoRefreshExtraKey)
	require.NotContains(t, opencodeLoaded.Extra, service.OpenCodeGoUsageSnapshotExtraKey)

	for _, id := range []int64{openaiOllama.ID, anthropicOllama.ID} {
		loaded, err := repo.GetByID(ctx, id)
		require.NoError(t, err)
		require.NotContains(t, loaded.Extra, service.OllamaCloudUsageSessionExtraKey)
		require.NotContains(t, loaded.Extra, service.OllamaCloudUsageAutoRefreshExtraKey)
		require.NotContains(t, loaded.Extra, service.OllamaCloudUsageSnapshotExtraKey)
	}
}

// F2：UpdateCredentials 的 OpenCode 分支必须与通用身份变化不变量一致——
// 清 probe、OpenCode 两键以及不应跨身份残留的 Ollama session/auto/snapshot。
func TestUpdateCredentialsOpenCodeGoClearsAllManagedStateOnIdentityChange(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Now().UTC()

	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "opencode-credentials", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "old-key", "base_url": "https://opencode.ai/zen/go/v1"},
		Extra:       openCodeGoManagedExtra(now),
	})

	require.NoError(t, repo.UpdateCredentials(ctx, account.ID, map[string]any{
		"api_key": "new-key", "base_url": "https://opencode.ai/zen/go/v1",
	}))
	loaded, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotContains(t, loaded.Extra, service.UpstreamBillingProbeExtraKey)
	require.NotContains(t, loaded.Extra, service.OpenCodeGoUsageAutoRefreshExtraKey)
	require.NotContains(t, loaded.Extra, service.OpenCodeGoUsageSnapshotExtraKey)
	require.NotContains(t, loaded.Extra, service.OllamaCloudUsageSessionExtraKey)
	require.NotContains(t, loaded.Extra, service.OllamaCloudUsageAutoRefreshExtraKey)
	require.NotContains(t, loaded.Extra, service.OllamaCloudUsageSnapshotExtraKey)
}

// F3：新 credentials 缺 base_url（NULL）时 OpenCode 分支必须仍命中（NULL-safe），
// 否则 OpenCode 受管状态会残留。api_key 保持不变，专门压 NOT regex(NULL) 路径。
func TestUpdateCredentialsOpenCodeGoMissingBaseURLClearsManagedState(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Now().UTC()

	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "opencode-missing-base-url", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "old-key", "base_url": "https://opencode.ai/zen/go/v1"},
		Extra: map[string]any{
			service.OpenCodeGoUsageAutoRefreshExtraKey: true,
			service.OpenCodeGoUsageSnapshotExtraKey: map[string]any{
				"status": service.OpenCodeGoUsageStatusOK, "last_attempt_at": now, "next_refresh_at": now.Add(time.Hour),
			},
		},
	})

	require.NoError(t, repo.UpdateCredentials(ctx, account.ID, map[string]any{
		"api_key": "old-key",
	}))
	loaded, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotContains(t, loaded.Extra, service.OpenCodeGoUsageAutoRefreshExtraKey)
	require.NotContains(t, loaded.Extra, service.OpenCodeGoUsageSnapshotExtraKey)
}

// F3 延伸：BulkUpdate 移除 base_url（null）时 opencode 受管键必须清除，
// 否则 NOT regex(NULL) 为 NULL 会令 WHEN 不命中而残留。
func TestBulkUpdateOpenCodeGoRemovedBaseURLClearsManagedState(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Now().UTC()

	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "opencode-bulk-remove-base-url", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "old-key", "base_url": "https://opencode.ai/zen/go/v1"},
		Extra: map[string]any{
			service.OpenCodeGoUsageAutoRefreshExtraKey: true,
			service.OpenCodeGoUsageSnapshotExtraKey: map[string]any{
				"status": service.OpenCodeGoUsageStatusOK, "last_attempt_at": now, "next_refresh_at": now.Add(time.Hour),
			},
		},
	})

	rows, err := repo.BulkUpdate(ctx, []int64{account.ID}, service.AccountBulkUpdate{
		Credentials: map[string]any{"base_url": nil},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)
	loaded, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotContains(t, loaded.Extra, service.OpenCodeGoUsageAutoRefreshExtraKey)
	require.NotContains(t, loaded.Extra, service.OpenCodeGoUsageSnapshotExtraKey)
}

// F6：SQL eligible 判定与 Go 侧对齐，接受显式默认端口 :443。
func TestOpenCodeGoUsageEligibleSQLAcceptsDefaultPort443(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)

	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "opencode-port-443", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "port-key", "base_url": "https://opencode.ai:443/zen/go/v1"},
		Extra:       map[string]any{},
	})

	// ListOpenCodeGoUsageGroupAccounts 走 opencodeGoUsageEligibleSQL，能解析到
	// :443 行即证明 SQL 正则与 Go 判定一致。
	groups, err := repo.ListOpenCodeGoUsageGroupAccounts(ctx, []*service.Account{account})
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, account.ID, groups[0].ID)
}

// F7：生产故障路径——前端脱敏普通编辑（incoming Extra 不含 OpenCode 受管键）经真实
// repo.Update 走 lockAndMergeAccountProbeExtra 的 SELECT/Scan/UPDATE 后，custom 生效
// 且两个受管键从锁定 DB 行回填；incoming Extra 伪造受管键时以锁定 DB 值为准。
// 旧代码（无 opencode 三列与回填分支）下：脱敏编辑会直接抹掉 DB 里的开关与快照，
// 伪造编辑会把伪造值原样写入，两条断言都会失败。
func TestUpdateOpenCodeGoManagedStateSurvivesDesensitizedEdit(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Now().UTC()

	proxy := mustCreateProxy(t, tx.Client(), &service.Proxy{
		Name: "opencode-edit-proxy", Protocol: "http", Host: "127.0.0.1", Port: 3128,
		Username: "user", Password: "pass", Status: service.StatusActive,
	})
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "opencode-desensitized-edit", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "edit-key", "base_url": "https://opencode.ai/zen/go/v1"},
		ProxyID:     &proxy.ID,
		Extra: map[string]any{
			service.OpenCodeGoUsageAutoRefreshExtraKey: true,
			service.OpenCodeGoUsageSnapshotExtraKey: map[string]any{
				"status": service.OpenCodeGoUsageStatusOK, "last_attempt_at": now, "next_refresh_at": now.Add(time.Hour),
			},
			"custom": "original",
		},
	})

	// 前端脱敏普通编辑：incoming Extra 不含两个受管键，只改 custom；base_url 用
	// :443 + 尾斜杠变体证明 normalized 身份判定（同 ID/platform/type/api_key/proxy）。
	edit, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	edit.Credentials = map[string]any{"api_key": "edit-key", "base_url": "https://opencode.ai:443/zen/go/v1/"}
	edit.Extra = map[string]any{"custom": "edited"}
	require.NoError(t, repo.Update(ctx, edit))

	loaded, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, "edited", loaded.Extra["custom"])
	require.Equal(t, true, loaded.Extra[service.OpenCodeGoUsageAutoRefreshExtraKey],
		"脱敏编辑不得抹掉 auto_refresh")
	snapshot, ok := loaded.Extra[service.OpenCodeGoUsageSnapshotExtraKey].(map[string]any)
	require.True(t, ok)
	require.Equal(t, service.OpenCodeGoUsageStatusOK, snapshot["status"], "脱敏编辑不得抹掉快照")

	// incoming Extra 伪造受管键：锁定 DB 值必须胜出。
	forged, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	forged.Extra = map[string]any{
		service.OpenCodeGoUsageAutoRefreshExtraKey: false,
		service.OpenCodeGoUsageSnapshotExtraKey:    map[string]any{"status": "forged"},
		"custom":                                   "edited-again",
	}
	require.NoError(t, repo.Update(ctx, forged))

	loaded, err = repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, "edited-again", loaded.Extra["custom"])
	require.Equal(t, true, loaded.Extra[service.OpenCodeGoUsageAutoRefreshExtraKey],
		"伪造 auto_refresh 必须以锁定 DB 值为准")
	snapshot, ok = loaded.Extra[service.OpenCodeGoUsageSnapshotExtraKey].(map[string]any)
	require.True(t, ok)
	require.Equal(t, service.OpenCodeGoUsageStatusOK, snapshot["status"], "伪造快照必须以锁定 DB 值为准")
}
