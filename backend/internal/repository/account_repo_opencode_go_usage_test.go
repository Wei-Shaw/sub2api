package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func openCodeGoUsageRepositoryAccount() *service.Account {
	previousAttempt := time.Date(2026, time.August, 15, 8, 0, 0, 0, time.UTC)
	return &service.Account{
		ID:          17,
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "key", "base_url": "https://opencode.ai/zen/go/v1"},
		Extra: map[string]any{
			service.OpenCodeGoUsageAutoRefreshExtraKey: true,
			service.OpenCodeGoUsageSnapshotExtraKey: &service.OpenCodeGoUsageSnapshot{
				Status:        service.OpenCodeGoUsageStatusOK,
				LastAttemptAt: previousAttempt,
				NextRefreshAt: previousAttempt.Add(time.Hour),
			},
		},
	}
}

func TestUpdateOpenCodeGoUsageSnapshotWritesSnapshotOnly(t *testing.T) {
	client, mock := newOllamaCloudUsageRepositoryTestClient(t)
	account := openCodeGoUsageRepositoryAccount()
	previousSnapshotJSON, err := json.Marshal(account.Extra[service.OpenCodeGoUsageSnapshotExtraKey])
	require.NoError(t, err)

	attemptedAt := time.Date(2026, time.August, 15, 9, 0, 0, 0, time.UTC)
	snapshot := &service.OpenCodeGoUsageSnapshot{
		Status:        service.OpenCodeGoUsageStatusOK,
		LastAttemptAt: attemptedAt,
		NextRefreshAt: attemptedAt.Add(time.Hour),
	}
	expectedPayload, err := json.Marshal(map[string]any{
		service.OpenCodeGoUsageSnapshotExtraKey: snapshot,
	})
	require.NoError(t, err)
	require.NotContains(t, string(expectedPayload), service.OpenCodeGoUsageAutoRefreshExtraKey)

	credentials, err := json.Marshal(normalizeJSONMap(account.Credentials))
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("SELECT")+`.*`+regexp.QuoteMeta("FOR NO KEY UPDATE")).
		WithArgs("key", account.ID, account.Platform, account.Type, string(credentials), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "anchor_matches", "auto_refresh", "snapshot"}).
			AddRow(account.ID, true, `true`, string(previousSnapshotJSON)))
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts")).
		WithArgs(string(expectedPayload), "key", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	repo := newAccountRepositoryWithSQL(client, nil, nil)

	err = repo.UpdateOpenCodeGoUsageSnapshot(context.Background(), account, snapshot)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// lockAndMergeAccountProbeExtra 的 SELECT 现在多出三列（opencode 组身份 / 开关 / 快照），
// 供通用 Update 路径做 OpenCode 受管键的原子回填。
func openCodeGoMergeMockColumns() []string {
	return []string{
		"identity_unchanged", "ollama_group_unchanged", "ollama_proxy_unchanged",
		"enabled", "rate_sync_enabled", "snapshot",
		"ollama_session", "ollama_auto", "ollama_snapshot",
		"opencode_group_unchanged", "opencode_auto", "opencode_snapshot",
	}
}

func openCodeGoSnapshotJSON() string {
	previousAttempt := time.Date(2026, time.August, 15, 8, 0, 0, 0, time.UTC)
	raw, err := json.Marshal(&service.OpenCodeGoUsageSnapshot{
		Status:        service.OpenCodeGoUsageStatusOK,
		LastAttemptAt: previousAttempt,
		NextRefreshAt: previousAttempt.Add(time.Hour),
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func TestLockAndMergeAccountProbeExtraPreservesOpenCodeGoManagedState(t *testing.T) {
	tests := []struct {
		name              string
		account           *service.Account
		groupUnchanged    bool
		proxyUnchanged    bool
		databaseAuto      string
		databaseSnapshot  string
		inputExtra        map[string]any
		wantAuto          any // nil 表示键不存在
		wantSnapshot      any // nil 表示键不存在
		wantSnapshotCheck bool
	}{
		{
			name:             "identity unchanged restores both managed keys over forged input",
			account:          openCodeGoUsageRepositoryAccount(),
			groupUnchanged:   true,
			proxyUnchanged:   true,
			databaseAuto:     "true",
			databaseSnapshot: openCodeGoSnapshotJSON(),
			inputExtra: map[string]any{
				service.OpenCodeGoUsageAutoRefreshExtraKey: false,
				service.OpenCodeGoUsageSnapshotExtraKey:    map[string]any{"status": "forged"},
			},
			wantAuto:          true,
			wantSnapshotCheck: true,
		},
		{
			name:             "identity unchanged keeps disabled switch from database",
			account:          openCodeGoUsageRepositoryAccount(),
			groupUnchanged:   true,
			proxyUnchanged:   true,
			databaseAuto:     "false",
			databaseSnapshot: openCodeGoSnapshotJSON(),
			inputExtra: map[string]any{
				service.OpenCodeGoUsageAutoRefreshExtraKey: true,
			},
			wantAuto:          false,
			wantSnapshotCheck: true,
		},
		{
			name:             "api key changed clears both managed keys",
			account:          openCodeGoUsageRepositoryAccount(),
			groupUnchanged:   false,
			proxyUnchanged:   true,
			databaseAuto:     "true",
			databaseSnapshot: openCodeGoSnapshotJSON(),
			inputExtra: map[string]any{
				service.OpenCodeGoUsageAutoRefreshExtraKey: true,
				service.OpenCodeGoUsageSnapshotExtraKey:    map[string]any{"status": "stale"},
			},
		},
		{
			name:             "base url no longer eligible clears both managed keys",
			account:          openCodeGoUsageRepositoryAccount(),
			groupUnchanged:   false,
			proxyUnchanged:   true,
			databaseAuto:     "true",
			databaseSnapshot: openCodeGoSnapshotJSON(),
			inputExtra:       map[string]any{"custom": "value"},
		},
		{
			name:             "proxy changed invalidates snapshot but keeps auto-refresh switch",
			account:          openCodeGoUsageRepositoryAccount(),
			groupUnchanged:   true,
			proxyUnchanged:   false,
			databaseAuto:     "true",
			databaseSnapshot: openCodeGoSnapshotJSON(),
			inputExtra:       map[string]any{"custom": "value"},
			wantAuto:         true,
		},
		{
			name: "no longer eligible account never inherits managed keys",
			account: func() *service.Account {
				account := openCodeGoUsageRepositoryAccount()
				account.Type = service.AccountTypeOAuth
				return account
			}(),
			groupUnchanged:   false,
			proxyUnchanged:   true,
			databaseAuto:     "true",
			databaseSnapshot: openCodeGoSnapshotJSON(),
			inputExtra: map[string]any{
				service.OpenCodeGoUsageAutoRefreshExtraKey: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, mock := newOllamaCloudUsageRepositoryTestClient(t)
			credentials, err := json.Marshal(normalizeJSONMap(tt.account.Credentials))
			require.NoError(t, err)
			mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("SELECT")+`.*`+regexp.QuoteMeta("FOR NO KEY UPDATE")).
				WithArgs(tt.account.ID, tt.account.Platform, tt.account.Type, string(credentials), nil).
				WillReturnRows(sqlmock.NewRows(openCodeGoMergeMockColumns()).
					AddRow(false, false, tt.proxyUnchanged, nil, nil, nil, nil, nil, nil, tt.groupUnchanged, tt.databaseAuto, tt.databaseSnapshot))

			got, err := lockAndMergeAccountProbeExtra(context.Background(), client, tt.account, nil, nil)
			require.NoError(t, err)
			if tt.wantAuto == nil {
				require.NotContains(t, got, service.OpenCodeGoUsageAutoRefreshExtraKey)
			} else {
				require.Equal(t, tt.wantAuto, got[service.OpenCodeGoUsageAutoRefreshExtraKey])
			}
			if tt.wantSnapshotCheck {
				snapshot, ok := got[service.OpenCodeGoUsageSnapshotExtraKey].(map[string]any)
				require.True(t, ok)
				require.Equal(t, service.OpenCodeGoUsageStatusOK, snapshot["status"])
			} else if tt.wantSnapshot == nil {
				require.NotContains(t, got, service.OpenCodeGoUsageSnapshotExtraKey)
			} else {
				require.Equal(t, tt.wantSnapshot, got[service.OpenCodeGoUsageSnapshotExtraKey])
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUpdateCredentialsOpenCodeGoIdentityChangeClearsManagedExtra(t *testing.T) {
	client, mock := newOllamaCloudUsageRepositoryTestClient(t)
	mock.ExpectBegin()
	// opencode 清理分支必须文本上先于 ollama 分支出现，否则 opencode 行的
	// api_key/base_url 变化会被先求值的 Ollama 分支遮蔽。
	mock.ExpectExec(`(?s)UPDATE accounts.*- 'opencode_go_usage_auto_refresh'.*- 'opencode_go_usage_snapshot'.*- 'ollama_cloud_usage_session'`).
		WithArgs(`{"api_key":"new-key","base_url":"https://opencode.ai/zen/go/v1"}`, int64(17)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(17), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	repo := newAccountRepositoryWithSQL(client, nil, nil)

	err := repo.UpdateCredentials(context.Background(), 17, map[string]any{
		"api_key": "new-key", "base_url": "https://opencode.ai/zen/go/v1",
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// opencode → ollama 的 base_url 跨域变化同样必须清除 opencode 受管键（不再 eligible）。
func TestUpdateCredentialsOpenCodeGoToOllamaCrossOverClearsManagedExtra(t *testing.T) {
	client, mock := newOllamaCloudUsageRepositoryTestClient(t)
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts.*opencode_go_usage_auto_refresh.*opencode_go_usage_snapshot`).
		WithArgs(`{"api_key":"same-key","base_url":"https://ollama.com/v1"}`, int64(17)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(17), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	repo := newAccountRepositoryWithSQL(client, nil, nil)

	err := repo.UpdateCredentials(context.Background(), 17, map[string]any{
		"api_key": "same-key", "base_url": "https://ollama.com/v1",
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// OpenCode 清理分支必须带顶层 credentials DISTINCT 守卫：凭证未变化时不能误清受管键。
func TestUpdateCredentialsOpenCodeGoCleanupRequiresChangedCredentials(t *testing.T) {
	client, mock := newOllamaCloudUsageRepositoryTestClient(t)
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts.*CASE.*AND credentials IS DISTINCT FROM \$1::jsonb`).
		WithArgs(`{"api_key":"same-key","base_url":"https://relay.example.com/v1"}`, int64(17)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(17), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	repo := newAccountRepositoryWithSQL(client, nil, nil)

	err := repo.UpdateCredentials(context.Background(), 17, map[string]any{
		"api_key": "same-key", "base_url": "https://relay.example.com/v1",
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBulkUpdateOpenCodeGoIdentityCleanupIsValueConditional(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	_, err := repo.BulkUpdate(context.Background(), []int64{17}, service.AccountBulkUpdate{
		Credentials: map[string]any{"api_key": "new-key"},
	})

	require.NoError(t, err)
	require.NotEmpty(t, exec.execQueries)
	query := normalizeSQLWhitespace(exec.execQueries[0])
	// OpenCode 分支必须出现在 Ollama 分支之前，且只在旧行是 opencode base URL 时触发。
	require.Contains(t, query, "platform = 'openai' AND type = 'apikey'")
	require.Contains(t, query, "- 'opencode_go_usage_auto_refresh' - 'opencode_go_usage_snapshot'")
	opencodeBranch := strings.Index(query, "opencode_go_usage_auto_refresh")
	ollamaBranch := strings.Index(query, "ollama_cloud_usage_auto_refresh")
	require.NotEqual(t, -1, opencodeBranch)
	require.NotEqual(t, -1, ollamaBranch)
	require.Less(t, opencodeBranch, ollamaBranch)
}

// F1 回归：OpenCode eligible 判定必须包含旧行 opencode base URL，否则 OpenAI+Ollama
// 行在代理变化时会先命中 OpenCode 分支而遮蔽 Ollama 快照清理。这里断言 OpenCode 分支
// 的 WHEN 携带 opencode 正则（与 Ollama 正则互斥），真实行为由 integration 测试覆盖。
func TestBulkUpdateOpenCodeGoEligiblePredicateIncludesBaseURL(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	proxyID := int64(9)
	_, err := repo.BulkUpdate(context.Background(), []int64{17}, service.AccountBulkUpdate{
		ProxyID: &proxyID,
	})

	require.NoError(t, err)
	require.NotEmpty(t, exec.execQueries)
	query := normalizeSQLWhitespace(exec.execQueries[0])
	// 第一个 WHEN 分支是 OpenCode 快照失效分支（代理变化），其 eligible 判定必须
	// 包含 opencode base URL 正则，使 OpenAI+Ollama 行无法命中该分支。
	caseStart := strings.Index(query, "CASE")
	firstThen := strings.Index(query, "THEN")
	require.NotEqual(t, -1, caseStart)
	require.NotEqual(t, -1, firstThen)
	require.Less(t, caseStart, firstThen)
	firstWhen := query[caseStart:firstThen]
	require.Contains(t, firstWhen, "[oO][pP][eE][nN][cC][oO][dD][eE]")
	require.Contains(t, firstWhen, "credentials ->> 'base_url'")
	require.NotContains(t, firstWhen, "[oO][lL][lL][aA][mM][aA]")
}

// F3 延伸：BulkUpdate 的 opencode base_url 变化子句同样必须 NULL-safe——新 base_url
// 缺失/为 null 时 regex(NULL) 为 NULL，NOT NULL 仍为 NULL 会令 WHEN 不命中而残留
// OpenCode 状态；IS NOT TRUE 把 NULL 视为不匹配。
func TestBulkUpdateOpenCodeGoBaseURLClauseIsNullSafe(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	_, err := repo.BulkUpdate(context.Background(), []int64{17}, service.AccountBulkUpdate{
		Credentials: map[string]any{"base_url": nil},
	})

	require.NoError(t, err)
	require.NotEmpty(t, exec.execQueries)
	query := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, query, "IS NOT TRUE")
	require.NotContains(t, query, "AND NOT btrim(credentials ->> 'base_url')")
}

// F6 回归：SQL 正则与 service.isOpenCodeGoBaseURL 对齐，接受显式默认端口 :443。
// 该正则同时用于 eligible 判定与身份清理，Go 侧接受而 SQL 侧拒绝会导致漏清/漏组。
func TestOpenCodeGoBaseURLRegexSQLAcceptsDefaultPort443(t *testing.T) {
	re := regexp.MustCompile(opencodeGoBaseURLRegexSQL)
	for _, url := range []string{
		"https://opencode.ai/zen/go/v1",
		"https://opencode.ai/zen/go/v1/",
		"https://opencode.ai:443/zen/go/v1",
		"https://opencode.ai:443/zen/go/v1/",
		"HTTPS://OPENCODE.AI:443/ZEN/GO/V1",
	} {
		require.True(t, re.MatchString(url), "SQL regex must accept %s", url)
	}
	for _, url := range []string{
		"https://opencode.ai:444/zen/go/v1",
		"https://opencode.ai/v1",
		"https://ollama.com/zen/go/v1",
		"https://opencode.ai/zen/go/v1?x=1",
	} {
		require.False(t, re.MatchString(url), "SQL regex must reject %s", url)
	}
}

func TestBulkUpdateOpenCodeGoProxyChangeClearsSnapshotOnly(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	proxyID := int64(9)
	_, err := repo.BulkUpdate(context.Background(), []int64{17}, service.AccountBulkUpdate{
		ProxyID: &proxyID,
	})

	require.NoError(t, err)
	require.NotEmpty(t, exec.execQueries)
	query := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, query, "- 'opencode_go_usage_snapshot'")
	require.NotContains(t, query, "- 'opencode_go_usage_auto_refresh'")
}

func TestInvalidateProxyProbeSnapshotsClearsOpenCodeGoSnapshot(t *testing.T) {
	client, mock := newOllamaCloudUsageRepositoryTestClient(t)
	mock.ExpectQuery(`(?s)UPDATE accounts.*opencode_go_usage_snapshot.*RETURNING id`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(17)))

	ids, err := invalidateProxyProbeSnapshots(context.Background(), client, 9)

	require.NoError(t, err)
	require.Equal(t, []int64{17}, ids)
	require.NoError(t, mock.ExpectationsWereMet())
}
