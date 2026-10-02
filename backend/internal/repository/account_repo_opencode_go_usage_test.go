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
	// OpenCode 分支必须出现在 Ollama 分支之前，且 eligible 判定按新谓词同时覆盖
	// opencode_go 平台（Go 订阅）与挂载白名单平台 + opencode 基址。
	require.Contains(t, query, "platform = 'opencode_go'")
	require.Contains(t, query, "platform IN ("+opencodeGoUsageMountPlatformsSQL+")")
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
	// 第一个 WHEN 分支是 OpenCode 快照失效分支（代理变化），其 eligible 判定必须
	// 覆盖 opencode_go 平台与挂载白名单的 opencode 基址，使 Ollama 行无法命中。
	require.Contains(t, firstWhen, "platform = 'opencode_go'")
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

// F6 回归：SQL 正则与 service.isOpenCodeGoBaseURL 对齐，接受显式默认端口 :443 与
// 两个官方基址变体（CC/Responses 的 /zen/go/v1、Anthropic 的 /zen/go）。该正则
// 同时用于 eligible 判定与身份清理，Go 侧接受而 SQL 侧拒绝会导致漏清/漏组。
func TestOpenCodeGoBaseURLRegexSQLAcceptsDefaultPort443(t *testing.T) {
	re := regexp.MustCompile(opencodeGoBaseURLRegexSQL)
	for _, url := range []string{
		"https://opencode.ai/zen/go/v1",
		"https://opencode.ai/zen/go/v1/",
		"https://opencode.ai:443/zen/go/v1",
		"https://opencode.ai:443/zen/go/v1/",
		"https://opencode.ai/zen/go",
		"https://opencode.ai/zen/go/",
		"https://opencode.ai:443/zen/go",
		"HTTPS://OPENCODE.AI:443/ZEN/GO/V1",
	} {
		require.True(t, re.MatchString(url), "SQL regex must accept %s", url)
	}
	for _, url := range []string{
		"https://opencode.ai:444/zen/go/v1",
		"https://opencode.ai/v1",
		"https://ollama.com/zen/go/v1",
		"https://opencode.ai/zen/go/v1?x=1",
		// zen 基址必须拒绝：按量付费无订阅配额窗口
		"https://opencode.ai/zen",
		"https://opencode.ai/zen/",
		"https://opencode.ai/zen/v1",
		"https://opencode.ai/zen/v1/",
		// 非官方 scheme / 子域 / 路径前缀拼接
		"http://opencode.ai/zen/go/v1",
		"https://www.opencode.ai/zen/go/v1",
		"https://opencode.ai/zen/gov1",
	} {
		require.False(t, re.MatchString(url), "SQL regex must reject %s", url)
	}
}

// SQL 挂载平台白名单常量与 service 判定必须互为镜像：对每个挂载候选平台，常量里
// 的成员关系都要与 IsOpenCodeGoUsageAccount（apikey + 官方 OpenCode Go 基址）一致，
// 防止两侧平台列表各自漂移。opencode_go 走平台 + Go 订阅分支而非挂载白名单：
// 不得出现在白名单常量里，且其资格由 account_mode 决定，在此一并钉死。
func TestOpenCodeGoUsagePlatformWhitelistMatchesServicePredicate(t *testing.T) {
	matches := regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(opencodeGoUsageMountPlatformsSQL, -1)
	sqlPlatforms := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		sqlPlatforms[match[1]] = struct{}{}
	}
	require.Len(t, sqlPlatforms, 6)
	for _, platform := range []string{
		service.PlatformOpenAI, service.PlatformAnthropic,
		service.PlatformKimi, service.PlatformZhipu, service.PlatformDeepseek, service.PlatformMiniMax,
		service.PlatformGemini, service.PlatformGrok, service.PlatformAntigravity,
		service.PlatformComposite, "kiro",
	} {
		account := openCodeGoUsageRepositoryAccount()
		account.Platform = platform
		_, inSQL := sqlPlatforms[platform]
		require.Equal(t, inSQL, service.IsOpenCodeGoUsageAccount(account), platform)
	}
	// opencode_go 不在挂载白名单里，资格来自平台 + Go 订阅分支。
	_, opencodeInSQL := sqlPlatforms[service.PlatformOpenCodeGo]
	require.False(t, opencodeInSQL)
	goAccount := openCodeGoUsageRepositoryAccount()
	goAccount.Platform = service.PlatformOpenCodeGo
	require.True(t, service.IsOpenCodeGoUsageAccount(goAccount))
	zenAccount := openCodeGoUsageRepositoryAccount()
	zenAccount.Platform = service.PlatformOpenCodeGo
	zenAccount.Credentials["account_mode"] = service.AccountModeZen
	require.False(t, service.IsOpenCodeGoUsageAccount(zenAccount))
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

// UpdateCredentials 的 CASE 求值顺序是正确性依赖而非防御：Ollama 分支的 WHEN 是
// 宽守卫（NOT(ollamaMatch(old) AND ollamaMatch(new))，旧行不匹配 ollama.com 基址
// 时恒真，且不含 opencode.ai 正则），而两侧平台白名单完全相同，因此挂载行
// （白名单平台 + 官方 OpenCode Go 基址）会同时满足两分支的 WHEN。若把 Ollama
// 分支前移，挂载行的 api_key 变化会先命中 Ollama 分支，OpenCode THEN 才清除的
// opencode_go_usage_snapshot / opencode_go_usage_auto_refresh 残留，陈旧快照跟着
// 新 api_key 走，跨 key 组污染。本测试用正则钉死 OpenCode 分支的 WHEN 标记
// （platform = 'opencode_go'，只出现在 OpenCode 分支）文本上先于 Ollama 分支的
// WHEN 标记（ollama.com 基址正则片段）。
func TestUpdateCredentialsOpenCodeBranchPrecedesOllamaBranch(t *testing.T) {
	client, mock := newOllamaCloudUsageRepositoryTestClient(t)
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE accounts.*platform = 'opencode_go'.*\[oO\]\[lL\]\[lL\]\[aA\]\[mM\]\[aA\]`).
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

// SQL 侧「默认 Go」语义的常量级护栏：opencode_go 平台行 account_mode 缺失/为
// JSON null 时，COALESCE 兜底必须落 true（视为 Go 订阅，与 GetOpenCodeAccountMode
// 的默认兼容逻辑一致）。若误改成 false，存量 opencode_go 账号在 SQL 侧集体失去
// 资格（组查询漏行、身份清理漏清、RunDue 自动刷新停摆），而 Go 侧仍判合格，
// 两侧不一致；唯一的行为级覆盖在需要 Docker 的 integration 测试，无 Docker 的
// unit 通道此前完全拦不住，这里以文本形态钉死该 COALESCE 表达式。
func TestOpenCodeGoUsageEligibleSQLDefaultsMissingAccountModeToGo(t *testing.T) {
	require.Contains(t, opencodeGoUsageEligibleSQL,
		"COALESCE(btrim(credentials ->> 'account_mode') <> 'zen', true)")
}

// Go 侧「默认 Go」契约的表驱动钉死：account_mode 只有 trim 后恰好等于 "zen" 才是
// Zen，其余取值一律判 Go。SQL 侧（opencodeGoUsageEligibleSQL）以
// COALESCE(btrim(...) <> 'zen', true) 声明同一语义：btrim 只去空格，与本表全部
// 取值在 Go 侧 strings.TrimSpace 下的结论一致（空白字符集差异见常量注释）。
func TestOpenCodeAccountModeOnlyTrimmedZenIsZen(t *testing.T) {
	tests := []struct {
		name        string
		accountMode func(map[string]any)
		wantMode    string
		wantGoPlan  bool
	}{
		{name: "键缺失", accountMode: func(map[string]any) {}, wantMode: service.AccountModeGo, wantGoPlan: true},
		{name: "JSON null", accountMode: func(c map[string]any) { c["account_mode"] = nil }, wantMode: service.AccountModeGo, wantGoPlan: true},
		{name: "空串", accountMode: func(c map[string]any) { c["account_mode"] = "" }, wantMode: service.AccountModeGo, wantGoPlan: true},
		{name: "go", accountMode: func(c map[string]any) { c["account_mode"] = service.AccountModeGo }, wantMode: service.AccountModeGo, wantGoPlan: true},
		{name: "zen", accountMode: func(c map[string]any) { c["account_mode"] = service.AccountModeZen }, wantMode: service.AccountModeZen, wantGoPlan: false},
		{name: "前后空格的 zen", accountMode: func(c map[string]any) { c["account_mode"] = " zen " }, wantMode: service.AccountModeZen, wantGoPlan: false},
		{name: "大小写不同的 ZEN", accountMode: func(c map[string]any) { c["account_mode"] = "ZEN" }, wantMode: service.AccountModeGo, wantGoPlan: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &service.Account{
				ID:          17,
				Platform:    service.PlatformOpenCodeGo,
				Type:        service.AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "key"},
			}
			tt.accountMode(account.Credentials)
			require.Equal(t, tt.wantMode, account.GetOpenCodeAccountMode())
			require.Equal(t, tt.wantGoPlan, account.IsOpenCodeGoPlan())
			require.Equal(t, !tt.wantGoPlan, account.IsOpenCodeZen())
			// opencode_go 平台行的用量资格即 Go 订阅判定，与 SQL 侧
			// opencodeGoUsageEligibleSQL 的「默认 Go、仅 trim 后等于 zen 排除」
			// 互为镜像。
			require.Equal(t, tt.wantGoPlan, service.IsOpenCodeGoUsageAccount(account))
		})
	}
}
