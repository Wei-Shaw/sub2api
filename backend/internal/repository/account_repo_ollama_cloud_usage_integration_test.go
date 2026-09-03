//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestListDueOllamaCloudUsageAccountsOrderingLimitAndProxyHydration(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	proxy := mustCreateProxy(t, tx.Client(), &service.Proxy{
		Name: "ollama-due-proxy", Protocol: "http", Host: "127.0.0.1", Port: 3128,
		Username: "user", Password: "pass", Status: service.StatusActive,
	})

	createAccount := func(name, baseURL string, proxyID *int64, snapshot map[string]any, lastUsed *time.Time) *service.Account {
		t.Helper()
		extra := map[string]any{
			service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
			service.OllamaCloudUsageAutoRefreshExtraKey: true,
		}
		if snapshot != nil {
			extra[service.OllamaCloudUsageSnapshotExtraKey] = snapshot
		}
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name: name, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": name, "base_url": baseURL},
			Extra:       extra, ProxyID: proxyID, LastUsedAt: lastUsed,
		})
	}

	uppercasePath := createAccount("ollama-uppercase-path", "https://ollama.com/V1", nil, nil, nil)
	missingSnapshot := createAccount("ollama-due-missing", "HTTPS://WWW.OLLAMA.COM:443/v1", &proxy.ID, nil, nil)
	fetched := now.Add(-2 * time.Hour)
	activity := now.Add(-5 * time.Minute)
	due := createAccount("ollama-due-activity", "https://ollama.com", nil, map[string]any{
		"status":          service.OllamaCloudUsageStatusOK,
		"fetched_at":      fetched.UTC().Format(time.RFC3339Nano),
		"last_attempt_at": fetched.UTC().Format(time.RFC3339Nano),
		"next_refresh_at": fetched.Add(time.Hour).UTC().Format(time.RFC3339Nano),
	}, &activity)
	// Success snapshot without newer activity must not be listed.
	_ = createAccount("ollama-not-due-idle", "https://ollama.com", nil, map[string]any{
		"status":          service.OllamaCloudUsageStatusOK,
		"fetched_at":      now.Add(-time.Hour).UTC().Format(time.RFC3339Nano),
		"last_attempt_at": now.Add(-time.Hour).UTC().Format(time.RFC3339Nano),
		"next_refresh_at": now.Add(-time.Minute).UTC().Format(time.RFC3339Nano),
	}, nil)
	_ = createAccount("ollama-ineligible", "https://ollama.com.evil.test", nil, nil, nil)

	accounts, err := repo.ListDueOllamaCloudUsageAccounts(ctx, now, time.Minute, time.Hour, 2)

	require.NoError(t, err)
	require.Len(t, accounts, 2)
	require.Equal(t, missingSnapshot.ID, accounts[0].ID)
	require.Equal(t, due.ID, accounts[1].ID)
	require.NotNil(t, accounts[1].LastUsedAt)
	require.WithinDuration(t, activity.UTC(), accounts[1].LastUsedAt.UTC(), time.Second)
	require.NotContains(t, accountIDs(accounts), uppercasePath.ID)
	require.NotNil(t, accounts[0].Proxy)
	require.Equal(t, proxy.ID, accounts[0].Proxy.ID)
	require.Equal(t, proxy.URL(), accounts[0].Proxy.URL())
}

// TestListDueOllamaCloudUsageAccountsParsesAllRFC3339Precisions pins the SQL
// timestamp parse path across the sub-second precisions and zone spellings that
// actually reach the database.
//
// Each fixture stores a fetched_at only two minutes old with activity 30s later,
// so a correctly parsed row is NOT due (debounce and the min fetch interval both
// place it in the future). A row whose timestamp fails to parse becomes NULL and
// falls into the fail-open branch, which makes it due. Asserting on absence is
// therefore what makes this test able to fail:
//
//   - Go writes UTC times, i.e. the "Z" designator. jsonpath .datetime() only
//     accepts "Z" from PostgreSQL 17 on, so without the Z -> +00:00 rewrite in
//     ollamaCloudUsageParseRFC3339SQL every fixture here goes due on 14-16.
//   - 7/8/9 sub-second digits exceed the microsecond resolution .datetime()
//     allows and must be truncated first.
//
// Run against the oldest supported server to exercise the version-sensitive path:
//
//	SUB2API_TEST_POSTGRES_IMAGE=postgres:15-alpine go test -tags integration ./internal/repository/
func TestListDueOllamaCloudUsageAccountsParsesAllRFC3339Precisions(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Date(2026, time.July, 22, 14, 0, 0, 0, time.UTC)
	activity := now.Add(-30 * time.Second)

	// All three spell the same instant, now-2m, with different precision/zone.
	notDue := map[string]string{
		"nano-z":         "2026-07-22T13:58:00.123456789Z",
		"eight-positive": "2026-07-22T14:58:00.12345678+01:00",
		"seven-negative": "2026-07-22T11:58:00.1234567-02:00",
	}
	for name, fetchedAt := range notDue {
		_ = mustCreateAccount(t, tx.Client(), &service.Account{
			Name: "ollama-precision-" + name, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "precision-" + name, "base_url": "https://ollama.com"},
			Extra: map[string]any{
				service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
				service.OllamaCloudUsageAutoRefreshExtraKey: true,
				service.OllamaCloudUsageSnapshotExtraKey: map[string]any{
					"status":          service.OllamaCloudUsageStatusOK,
					"fetched_at":      fetchedAt,
					"last_attempt_at": fetchedAt,
				},
			},
			LastUsedAt: &activity,
		})
	}

	// Guards against a vacuous pass: an genuinely due row must still come back.
	staleFetched := now.Add(-2 * time.Hour)
	due := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "ollama-precision-due", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "precision-due", "base_url": "https://ollama.com"},
		Extra: map[string]any{
			service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
			service.OllamaCloudUsageAutoRefreshExtraKey: true,
			service.OllamaCloudUsageSnapshotExtraKey: map[string]any{
				"status":          service.OllamaCloudUsageStatusOK,
				"fetched_at":      staleFetched.UTC().Format(time.RFC3339Nano),
				"last_attempt_at": staleFetched.UTC().Format(time.RFC3339Nano),
			},
		},
		LastUsedAt: &activity,
	})

	accounts, err := repo.ListDueOllamaCloudUsageAccounts(ctx, now, time.Minute, time.Hour, 10)

	require.NoError(t, err)
	ids := accountIDs(accounts)
	require.Contains(t, ids, due.ID, "a stale snapshot with fresh activity must be due")
	require.Len(t, ids, 1,
		"only the stale group may be due; extra rows mean a timestamp failed to parse and fell into the fail-open branch")
}

func TestListDueOllamaCloudUsageAccountsUsesGroupMaxLastUsedAndFailsOpen(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Date(2026, time.July, 22, 14, 0, 0, 0, time.UTC)
	fetched := now.Add(-30 * time.Minute)
	older := now.Add(-10 * time.Minute)
	newer := now.Add(-2 * time.Minute)

	leader := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "ollama-group-leader", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "shared-key", "base_url": "https://ollama.com"},
		Extra: map[string]any{
			service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
			service.OllamaCloudUsageAutoRefreshExtraKey: true,
			service.OllamaCloudUsageSnapshotExtraKey: map[string]any{
				"status":          service.OllamaCloudUsageStatusOK,
				"fetched_at":      fetched.UTC().Format(time.RFC3339Nano),
				"last_attempt_at": fetched.UTC().Format(time.RFC3339Nano),
				"next_refresh_at": fetched.Add(time.Hour).UTC().Format(time.RFC3339Nano),
			},
		},
		LastUsedAt: &older,
	})
	_ = mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "ollama-group-sibling", Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "shared-key", "base_url": "https://www.ollama.com/v1"},
		LastUsedAt:  &newer,
	})
	invalid := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "ollama-invalid-snapshot", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "invalid-key", "base_url": "https://ollama.com"},
		Extra: map[string]any{
			service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
			service.OllamaCloudUsageAutoRefreshExtraKey: true,
			service.OllamaCloudUsageSnapshotExtraKey: map[string]any{
				"status": service.OllamaCloudUsageStatusOK, "fetched_at": "2026-02-30T09:00:00.123456789Z",
			},
		},
	})
	idle := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "ollama-idle-ok", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "idle-key", "base_url": "https://ollama.com"},
		Extra: map[string]any{
			service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
			service.OllamaCloudUsageAutoRefreshExtraKey: true,
			service.OllamaCloudUsageSnapshotExtraKey: map[string]any{
				"status":          service.OllamaCloudUsageStatusOK,
				"fetched_at":      fetched.UTC().Format(time.RFC3339Nano),
				"last_attempt_at": fetched.UTC().Format(time.RFC3339Nano),
				"next_refresh_at": fetched.Add(time.Hour).UTC().Format(time.RFC3339Nano),
			},
		},
	})

	accounts, err := repo.ListDueOllamaCloudUsageAccounts(ctx, now, time.Minute, time.Hour, 10)

	require.NoError(t, err, "invalid stored values must not abort the query")
	ids := accountIDs(accounts)
	require.Contains(t, ids, invalid.ID)
	require.Contains(t, ids, leader.ID)
	require.NotContains(t, ids, idle.ID)
	for _, account := range accounts {
		if account.ID == leader.ID {
			require.NotNil(t, account.LastUsedAt)
			require.WithinDuration(t, newer.UTC(), account.LastUsedAt.UTC(), time.Second,
				"group MAX(last_used_at) must come from the sibling")
		}
	}
}

func TestLockAndMergeAccountProbeExtraCoalescesNullableOllamaGroupIdentity(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "ordinary-openai-without-base-url", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-no-base-url"},
		Extra:       map[string]any{service.UpstreamBillingProbeEnabledExtraKey: true},
	})
	loaded, err := newAccountRepositoryWithSQL(tx.Client(), tx, nil).GetByID(ctx, account.ID)
	require.NoError(t, err)

	merged, err := lockAndMergeAccountProbeExtra(ctx, tx.Client(), loaded, nil, nil)

	require.NoError(t, err, "a NULL Ollama eligibility expression must scan as false")
	require.NotContains(t, merged, service.OllamaCloudUsageSessionExtraKey)
	require.Equal(t, true, merged[service.UpstreamBillingProbeEnabledExtraKey])
}

func TestOllamaCloudUsageGroupWritesAreAtomicAcrossPlatformsAndURLVariants(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	create := func(name, platform, apiKey, baseURL string) *service.Account {
		t.Helper()
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name: name, Platform: platform, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": apiKey, "base_url": baseURL},
			Extra:       map[string]any{},
		})
	}
	first := create("ollama-group-openai", service.PlatformOpenAI, "shared-key", "https://ollama.com")
	second := create("ollama-group-anthropic", service.PlatformAnthropic, "shared-key", "HTTPS://WWW.OLLAMA.COM:443/v1")
	different := create("ollama-group-different", service.PlatformOpenAI, "different-key", "https://ollama.com")

	require.NoError(t, repo.SaveOllamaCloudUsageSession(ctx, first, "cipher:shared", false))
	for _, id := range []int64{first.ID, second.ID} {
		account, err := repo.GetByID(ctx, id)
		require.NoError(t, err)
		require.Equal(t, "cipher:shared", account.Extra[service.OllamaCloudUsageSessionExtraKey])
		require.Equal(t, false, account.Extra[service.OllamaCloudUsageAutoRefreshExtraKey])
	}
	differentLoaded, err := repo.GetByID(ctx, different.ID)
	require.NoError(t, err)
	require.NotContains(t, differentLoaded.Extra, service.OllamaCloudUsageSessionExtraKey)

	secondLoaded, err := repo.GetByID(ctx, second.ID)
	require.NoError(t, err)
	require.NoError(t, repo.SetOllamaCloudUsageAutoRefresh(ctx, secondLoaded, true))
	firstLoaded, err := repo.GetByID(ctx, first.ID)
	require.NoError(t, err)
	secondLoaded, err = repo.GetByID(ctx, second.ID)
	require.NoError(t, err)
	require.Equal(t, true, firstLoaded.Extra[service.OllamaCloudUsageAutoRefreshExtraKey])
	require.Equal(t, true, secondLoaded.Extra[service.OllamaCloudUsageAutoRefreshExtraKey])

	now := time.Now().UTC()
	snapshot := &service.OllamaCloudUsageSnapshot{
		Status: service.OllamaCloudUsageStatusOK, LastAttemptAt: now, NextRefreshAt: now.Add(time.Hour),
	}
	require.NoError(t, repo.UpdateOllamaCloudUsageSnapshot(ctx, firstLoaded, snapshot))
	secondLoaded, err = repo.GetByID(ctx, second.ID)
	require.NoError(t, err)
	require.Equal(t, service.OllamaCloudUsageStatusOK,
		secondLoaded.Extra[service.OllamaCloudUsageSnapshotExtraKey].(map[string]any)["status"])

	staleSecond := secondLoaded
	require.NoError(t, repo.UpdateCredentials(ctx, second.ID, map[string]any{
		"api_key": "rotated-key", "base_url": "https://ollama.com",
	}))
	require.ErrorIs(t, repo.DisableOllamaCloudUsageAutoRefresh(ctx, staleSecond), service.ErrOllamaCloudUsageIdentityChanged)
	firstLoaded, err = repo.GetByID(ctx, first.ID)
	require.NoError(t, err)
	secondLoaded, err = repo.GetByID(ctx, second.ID)
	require.NoError(t, err)
	require.Equal(t, "cipher:shared", firstLoaded.Extra[service.OllamaCloudUsageSessionExtraKey])
	require.Equal(t, true, firstLoaded.Extra[service.OllamaCloudUsageAutoRefreshExtraKey])
	require.NotContains(t, secondLoaded.Extra, service.OllamaCloudUsageSessionExtraKey)
	require.NotContains(t, secondLoaded.Extra, service.OllamaCloudUsageAutoRefreshExtraKey)

	require.NoError(t, repo.DeleteOllamaCloudUsageSession(ctx, firstLoaded))
	firstLoaded, err = repo.GetByID(ctx, first.ID)
	require.NoError(t, err)
	require.NotContains(t, firstLoaded.Extra, service.OllamaCloudUsageSessionExtraKey)
}

func TestConcurrentOllamaCloudUsageSaveAndDeleteSerializeGroupState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := testEntClient(t)
	repo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	suffix := time.Now().UnixNano()
	apiKey := fmt.Sprintf("ollama-concurrent-%d", suffix)
	create := func(platform string) *service.Account {
		t.Helper()
		return mustCreateAccount(t, client, &service.Account{
			Name: fmt.Sprintf("%s-%s", apiKey, platform), Platform: platform, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": apiKey, "base_url": "https://ollama.com"},
			Extra: map[string]any{
				service.OllamaCloudUsageSessionExtraKey:     "cipher:initial",
				service.OllamaCloudUsageAutoRefreshExtraKey: true,
			},
		})
	}
	first := create(service.PlatformOpenAI)
	second := create(service.PlatformAnthropic)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id IN ($1, $2)", first.ID, second.ID)
	})
	anchor, err := repo.GetByID(ctx, first.ID)
	require.NoError(t, err)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		errs <- repo.SaveOllamaCloudUsageSession(ctx, anchor, "cipher:replacement", true)
	}()
	go func() {
		defer wg.Done()
		<-start
		errs <- repo.DeleteOllamaCloudUsageSession(ctx, anchor)
	}()
	close(start)
	wg.Wait()
	close(errs)
	for writeErr := range errs {
		require.NoError(t, writeErr)
	}

	firstLoaded, err := repo.GetByID(ctx, first.ID)
	require.NoError(t, err)
	secondLoaded, err := repo.GetByID(ctx, second.ID)
	require.NoError(t, err)
	managedState := func(account *service.Account) map[string]any {
		state := make(map[string]any)
		for _, key := range []string{
			service.OllamaCloudUsageSessionExtraKey,
			service.OllamaCloudUsageAutoRefreshExtraKey,
			service.OllamaCloudUsageSnapshotExtraKey,
		} {
			if value, ok := account.Extra[key]; ok {
				state[key] = value
			}
		}
		return state
	}
	firstState := managedState(firstLoaded)
	require.Equal(t, firstState, managedState(secondLoaded), "a serialized last commit must own the whole group")
	if len(firstState) > 0 {
		require.Equal(t, "cipher:replacement", firstState[service.OllamaCloudUsageSessionExtraKey])
		require.Equal(t, true, firstState[service.OllamaCloudUsageAutoRefreshExtraKey])
		require.NotContains(t, firstState, service.OllamaCloudUsageSnapshotExtraKey)
	}
}

func accountIDs(accounts []service.Account) []int64 {
	ids := make([]int64, len(accounts))
	for index := range accounts {
		ids[index] = accounts[index].ID
	}
	return ids
}

// 平台白名单放开后，官方 ollama.com key 挂在国产 OpenAI 兼容平台下同样进用量
// 窗口：组写入跨 kimi/zhipu/deepseek 共享，白名单外平台（gemini）不得被卷入；
// 组身份守卫（lockAndMerge）与 due 列表也必须识别 CN 平台的行。
func TestOllamaCloudUsageEligibilityExtendsToCNOpenAICompatPlatforms(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Now().UTC()
	activity := now.Add(-time.Minute)
	create := func(name, platform, baseURL string) *service.Account {
		t.Helper()
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name: name, Platform: platform, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "cn-shared-key", "base_url": baseURL},
			Extra:       map[string]any{}, LastUsedAt: &activity,
		})
	}
	kimi := create("ollama-cn-kimi", service.PlatformKimi, "https://ollama.com")
	zhipu := create("ollama-cn-zhipu", service.PlatformZhipu, "HTTPS://WWW.OLLAMA.COM:443/v1")
	deepseek := create("ollama-cn-deepseek", service.PlatformDeepseek, "https://ollama.com/v1")
	gemini := create("ollama-cn-gemini", service.PlatformGemini, "https://ollama.com")

	require.NoError(t, repo.SaveOllamaCloudUsageSession(ctx, kimi, "cipher:cn-shared", true))
	for _, id := range []int64{kimi.ID, zhipu.ID, deepseek.ID} {
		account, err := repo.GetByID(ctx, id)
		require.NoError(t, err)
		require.Equal(t, "cipher:cn-shared", account.Extra[service.OllamaCloudUsageSessionExtraKey], account.Name)
		require.Equal(t, true, account.Extra[service.OllamaCloudUsageAutoRefreshExtraKey], account.Name)
	}
	geminiLoaded, err := repo.GetByID(ctx, gemini.ID)
	require.NoError(t, err)
	require.NotContains(t, geminiLoaded.Extra, service.OllamaCloudUsageSessionExtraKey,
		"用量窗口不随 base_url 放开到白名单外平台")

	// lockAndMerge 组身份守卫：CN 行凭证未变时必须保留 ollama 托管键。
	kimiLoaded, err := repo.GetByID(ctx, kimi.ID)
	require.NoError(t, err)
	merged, err := lockAndMergeAccountProbeExtra(ctx, tx.Client(), kimiLoaded, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "cipher:cn-shared", merged[service.OllamaCloudUsageSessionExtraKey])
	require.Equal(t, true, merged[service.OllamaCloudUsageAutoRefreshExtraKey])

	// due 列表识别 CN 行：给同 key 组挂上过期快照（fetched 2h 前、活动更新），
	// 每个 api_key 只返回一行，且必须来自 CN 组员。
	staleFetched := now.Add(-2 * time.Hour)
	deepseekLoaded, err := repo.GetByID(ctx, deepseek.ID)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateOllamaCloudUsageSnapshot(ctx, deepseekLoaded, &service.OllamaCloudUsageSnapshot{
		Status:        service.OllamaCloudUsageStatusOK,
		FetchedAt:     &staleFetched,
		LastAttemptAt: staleFetched,
	}))

	due, err := repo.ListDueOllamaCloudUsageAccounts(ctx, now, time.Minute, time.Hour, 10)

	require.NoError(t, err)
	require.Len(t, due, 1, "同 key 跨 CN 平台组按组去重后只应有一行 due")
	require.Contains(t, []int64{kimi.ID, zhipu.ID, deepseek.ID}, due[0].ID,
		"due 行必须来自 CN 平台的 ollama 组员")
	require.NotNil(t, due[0].LastUsedAt)
}

// 语义等价性端到端：普通（非 ollama）kimi apikey 账号改凭证落进 Ollama 分支后，
// probe 快照仍被清理，开关键与其它 extra 键不受影响。
func TestUpdateCredentialsPlainCNAPIKeyAccountCleanupIsSemanticallyEquivalent(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "plain-kimi-apikey", Platform: service.PlatformKimi, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-moonshot", "base_url": "https://api.moonshot.cn"},
		Extra: map[string]any{
			service.UpstreamBillingProbeExtraKey:        map[string]any{"status": "ok"},
			service.UpstreamBillingProbeEnabledExtraKey: true,
			"custom_note": "keep-me",
		},
	})

	require.NoError(t, repo.UpdateCredentials(ctx, account.ID, map[string]any{
		"api_key": "sk-moonshot-rotated", "base_url": "https://api.moonshot.cn",
	}))

	loaded, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotContains(t, loaded.Extra, service.UpstreamBillingProbeExtraKey,
		"probe 快照仍必须被清理")
	require.Equal(t, true, loaded.Extra[service.UpstreamBillingProbeEnabledExtraKey],
		"探测开关键不受清理影响")
	require.Equal(t, "keep-me", loaded.Extra["custom_note"], "其它 extra 键不得误伤")
	require.NotContains(t, loaded.Extra, service.OllamaCloudUsageSessionExtraKey)
}

func TestOllamaCloudUsageCredentialAndBulkUpdatesPreserveManagedStateOnlyWhenSafe(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Now().UTC()
	newAccount := func(name string) *service.Account {
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name: name, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": "old-key", "base_url": "https://ollama.com"},
			Extra: map[string]any{
				service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
				service.OllamaCloudUsageAutoRefreshExtraKey: true,
				service.OllamaCloudUsageSnapshotExtraKey: map[string]any{
					"status": service.OllamaCloudUsageStatusOK, "last_attempt_at": now, "next_refresh_at": now.Add(time.Hour),
				},
			},
		})
	}

	rawAccount := newAccount("ollama-raw-credentials")
	require.NoError(t, repo.UpdateCredentials(ctx, rawAccount.ID, map[string]any{
		"api_key": "old-key", "base_url": "https://ollama.com/V1",
	}))
	rawUpdated, err := repo.GetByID(ctx, rawAccount.ID)
	require.NoError(t, err)
	require.NotContains(t, rawUpdated.Extra, service.OllamaCloudUsageSessionExtraKey)
	require.NotContains(t, rawUpdated.Extra, service.OllamaCloudUsageAutoRefreshExtraKey)
	require.NotContains(t, rawUpdated.Extra, service.OllamaCloudUsageSnapshotExtraKey)

	bulkAccount := newAccount("ollama-bulk-credentials")
	rows, err := repo.BulkUpdate(ctx, []int64{bulkAccount.ID}, service.AccountBulkUpdate{
		Credentials: map[string]any{"base_url": "HTTPS://WWW.OLLAMA.COM:443/v1"},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)
	bulkUnchanged, err := repo.GetByID(ctx, bulkAccount.ID)
	require.NoError(t, err)
	require.Contains(t, bulkUnchanged.Extra, service.OllamaCloudUsageSnapshotExtraKey)

	rows, err = repo.BulkUpdate(ctx, []int64{bulkAccount.ID}, service.AccountBulkUpdate{
		Credentials: map[string]any{"base_url": "https://ollama.com/V1"},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), rows)
	bulkIneligible, err := repo.GetByID(ctx, bulkAccount.ID)
	require.NoError(t, err)
	require.NotContains(t, bulkIneligible.Extra, service.OllamaCloudUsageSessionExtraKey)
	require.NotContains(t, bulkIneligible.Extra, service.OllamaCloudUsageAutoRefreshExtraKey)
	require.NotContains(t, bulkIneligible.Extra, service.OllamaCloudUsageSnapshotExtraKey)
}

func TestProxyIdentityUpdateInvalidatesOllamaSnapshotAndRejectsInFlightCAS(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	accountRepo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	proxyRepo := newProxyRepositoryWithSQL(tx.Client(), tx)
	proxy := mustCreateProxy(t, tx.Client(), &service.Proxy{
		Name: "ollama-identity-proxy", Protocol: "http", Host: "old.example", Port: 8080,
		Username: "old-user", Password: "old-pass", Status: service.StatusActive,
	})
	now := time.Now().UTC()
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "ollama-proxy-account", Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "key", "base_url": "https://ollama.com"},
		ProxyID:     &proxy.ID,
		Extra: map[string]any{
			service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
			service.OllamaCloudUsageAutoRefreshExtraKey: true,
			service.OllamaCloudUsageSnapshotExtraKey: map[string]any{
				"status": service.OllamaCloudUsageStatusOK, "last_attempt_at": now, "next_refresh_at": now.Add(time.Hour),
			},
		},
	})
	inFlight, err := accountRepo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotNil(t, inFlight.Proxy)
	require.Equal(t, "old.example", inFlight.Proxy.Host)

	proxyToUpdate, err := proxyRepo.GetByID(ctx, proxy.ID)
	require.NoError(t, err)
	proxyToUpdate.Host = "new.example"
	require.NoError(t, proxyRepo.Update(ctx, proxyToUpdate))

	got, err := accountRepo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotContains(t, got.Extra, service.OllamaCloudUsageSnapshotExtraKey)
	require.Equal(t, "cipher:wos-session=fixture", got.Extra[service.OllamaCloudUsageSessionExtraKey])
	require.Equal(t, true, got.Extra[service.OllamaCloudUsageAutoRefreshExtraKey])

	err = accountRepo.UpdateOllamaCloudUsageSnapshot(ctx, inFlight, &service.OllamaCloudUsageSnapshot{
		Status: service.OllamaCloudUsageStatusOK, LastAttemptAt: now, NextRefreshAt: now.Add(time.Hour),
	})
	require.ErrorIs(t, err, service.ErrOllamaCloudUsageIdentityChanged)
}

// 无变化的凭证持久化（如 CRS 同步重放同一凭证）不得触发任何 extra 清理；
// 真实变化仍必须按旧语义清 openai 探测快照。
func TestUpdateCredentialsUnchangedCredentialsPreserveManagedExtra(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)

	probeAccount := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "openai-probe-unchanged", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-probe", "base_url": "https://relay.example.com/v1"},
		Extra: map[string]any{
			service.UpstreamBillingProbeEnabledExtraKey: true,
			service.UpstreamBillingProbeExtraKey:        map[string]any{"status": "ok"},
		},
	})
	require.NoError(t, repo.UpdateCredentials(ctx, probeAccount.ID, map[string]any{
		"api_key": "sk-probe", "base_url": "https://relay.example.com/v1",
	}))
	probeLoaded, err := repo.GetByID(ctx, probeAccount.ID)
	require.NoError(t, err)
	require.Contains(t, probeLoaded.Extra, service.UpstreamBillingProbeExtraKey,
		"unchanged credentials must not clear the probe snapshot")

	now := time.Now().UTC()
	ollamaAccount := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "ollama-unchanged", Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "ollama-key", "base_url": "https://ollama.com"},
		Extra: map[string]any{
			service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
			service.OllamaCloudUsageAutoRefreshExtraKey: true,
			service.OllamaCloudUsageSnapshotExtraKey: map[string]any{
				"status": service.OllamaCloudUsageStatusOK, "last_attempt_at": now, "next_refresh_at": now.Add(time.Hour),
			},
		},
	})
	require.NoError(t, repo.UpdateCredentials(ctx, ollamaAccount.ID, map[string]any{
		"api_key": "ollama-key", "base_url": "https://ollama.com",
	}))
	ollamaLoaded, err := repo.GetByID(ctx, ollamaAccount.ID)
	require.NoError(t, err)
	require.Equal(t, "cipher:wos-session=fixture", ollamaLoaded.Extra[service.OllamaCloudUsageSessionExtraKey])
	require.Equal(t, true, ollamaLoaded.Extra[service.OllamaCloudUsageAutoRefreshExtraKey])
	require.Contains(t, ollamaLoaded.Extra, service.OllamaCloudUsageSnapshotExtraKey)

	require.NoError(t, repo.UpdateCredentials(ctx, probeAccount.ID, map[string]any{
		"api_key": "sk-probe", "base_url": "https://relay.example.org/v1",
	}))
	probeLoaded, err = repo.GetByID(ctx, probeAccount.ID)
	require.NoError(t, err)
	require.NotContains(t, probeLoaded.Extra, service.UpstreamBillingProbeExtraKey,
		"changed credentials must keep clearing the probe snapshot")
}

// TestListDueOllamaCloudUsageAccountsSQLDueRulesMatchService proves the SQL
// candidate layer applies debounce / max-wait / failure-backoff before LIMIT,
// matching service.ollamaCloudUsageIsAutoRefreshDue, and that >20 active-but-
// not-yet-due groups cannot starve a truly due max-wait group.
func TestListDueOllamaCloudUsageAccountsSQLDueRulesMatchService(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	debounce := time.Minute
	maxWait := time.Hour

	createOK := func(name string, fetched, lastUsed time.Time) *service.Account {
		t.Helper()
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name: name, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": name, "base_url": "https://ollama.com"},
			Extra: map[string]any{
				service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
				service.OllamaCloudUsageAutoRefreshExtraKey: true,
				service.OllamaCloudUsageSnapshotExtraKey: map[string]any{
					"status":          service.OllamaCloudUsageStatusOK,
					"fetched_at":      fetched.UTC().Format(time.RFC3339Nano),
					"last_attempt_at": fetched.UTC().Format(time.RFC3339Nano),
					"next_refresh_at": fetched.Add(maxWait).UTC().Format(time.RFC3339Nano),
				},
			},
			LastUsedAt: &lastUsed,
		})
	}
	createFailed := func(name string, lastAttempt, lastUsed, nextRefresh time.Time, nextRefreshRaw string) *service.Account {
		t.Helper()
		snapshot := map[string]any{
			"status":          service.OllamaCloudUsageStatusFailed,
			"last_attempt_at": lastAttempt.UTC().Format(time.RFC3339Nano),
			"failure_count":   1,
		}
		if nextRefreshRaw != "" {
			snapshot["next_refresh_at"] = nextRefreshRaw
		} else {
			snapshot["next_refresh_at"] = nextRefresh.UTC().Format(time.RFC3339Nano)
		}
		return mustCreateAccount(t, tx.Client(), &service.Account{
			Name: name, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"api_key": name, "base_url": "https://ollama.com"},
			Extra: map[string]any{
				service.OllamaCloudUsageSessionExtraKey:     "cipher:wos-session=fixture",
				service.OllamaCloudUsageAutoRefreshExtraKey: true,
				service.OllamaCloudUsageSnapshotExtraKey:    snapshot,
			},
			LastUsedAt: &lastUsed,
		})
	}

	// 21 groups with activity after fetch but debounce not elapsed — previously
	// these alone could fill LIMIT 20 every minute and starve true due groups.
	notDueIDs := make(map[int64]struct{}, 21)
	for i := 0; i < 21; i++ {
		// fetched 10m ago, last used 10s ago → due_at = lastUsed+debounce = now+50s (not due)
		acc := createOK(fmt.Sprintf("ollama-not-due-debounce-%02d", i), now.Add(-10*time.Minute), now.Add(-10*time.Second))
		notDueIDs[acc.ID] = struct{}{}
	}

	// Truly due via max-wait: fetched 2h ago, continuous activity 10s ago.
	// due_at = min(now-10s+1m, now-2h+1h) = now-1h → due.
	maxWaitDue := createOK("ollama-due-maxwait", now.Add(-2*time.Hour), now.Add(-10*time.Second))

	// Success debounce elapsed: last used 2m ago with debounce 1m → due.
	debounceDue := createOK("ollama-due-debounce", now.Add(-30*time.Minute), now.Add(-2*time.Minute))

	// Success still within debounce → not due.
	_ = createOK("ollama-not-due-fresh", now.Add(-30*time.Minute), now.Add(-20*time.Second))

	// Failure blocked by next_refresh_at backoff even with new activity.
	_ = createFailed("ollama-fail-backoff", now.Add(-30*time.Minute), now.Add(-2*time.Minute), now.Add(10*time.Minute), "")

	// Failure after backoff with new request → due.
	failDue := createFailed("ollama-fail-due", now.Add(-30*time.Minute), now.Add(-2*time.Minute), now.Add(-time.Minute), "")

	// Invalid next_refresh_at must fail open (not abort query / not block activity due).
	failInvalidNext := createFailed("ollama-fail-invalid-next", now.Add(-30*time.Minute), now.Add(-2*time.Minute), time.Time{}, "not-a-timestamp")

	accounts, err := repo.ListDueOllamaCloudUsageAccounts(ctx, now, debounce, maxWait, 20)
	require.NoError(t, err)

	ids := accountIDs(accounts)
	require.Contains(t, ids, maxWaitDue.ID, "max-wait due group must not be starved by not-yet-due activity groups")
	require.Contains(t, ids, debounceDue.ID, "success debounce elapsed must be due in SQL")
	require.Contains(t, ids, failDue.ID, "failure after backoff with new activity must be due in SQL")
	require.Contains(t, ids, failInvalidNext.ID, "invalid next_refresh_at must fail open to activity due")
	require.LessOrEqual(t, len(accounts), 20)

	// Fixtures below match service.ollamaCloudUsageIsAutoRefreshDue semantics;
	// none of the not-yet-due groups may appear even when they outnumber the limit.
	for _, id := range ids {
		_, isNotDue := notDueIDs[id]
		require.False(t, isNotDue, "not-yet-due debounce group %d must not be returned by SQL LIMIT layer", id)
	}
	require.NotContains(t, ids, int64(0))

	// Explicit not-due names must stay out: fresh success and failure still in backoff.
	for _, account := range accounts {
		require.NotContains(t, account.Name, "not-due")
		require.NotEqual(t, "ollama-fail-backoff", account.Name)
		require.NotEqual(t, "ollama-not-due-fresh", account.Name)
	}
}
