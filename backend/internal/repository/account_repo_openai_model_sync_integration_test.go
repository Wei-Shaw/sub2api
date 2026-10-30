//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIModelSyncConcurrentWorkersCommitOnlyOneResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repo := newAccountRepositoryWithSQL(testEntClient(t), integrationDB, nil)
	account := mustCreateAccount(t, testEntClient(t), &service.Account{
		Name: "model-sync-workers", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "keep"},
		Extra:       map[string]any{service.OpenAIModelAutoSyncEnabledExtraKey: true},
	})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM scheduler_outbox WHERE account_id = $1`, account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM accounts WHERE id = $1`, account.ID)
	})
	loaded, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, model := range []string{"gpt-first", "gpt-second"} {
		go func(model string) {
			<-start
			results <- repo.CommitOpenAIModelSync(ctx, loaded, map[string]string{model: model}, &service.OpenAIModelAutoSyncState{Status: "success", ManagedModels: []string{model}})
		}(model)
	}
	close(start)
	first, second := <-results, <-results
	if first == nil {
		require.ErrorIs(t, second, service.ErrOpenAIModelAutoSyncChanged)
	} else {
		require.ErrorIs(t, first, service.ErrOpenAIModelAutoSyncChanged)
		require.NoError(t, second)
	}
	got, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	managed := got.Extra[service.OpenAIModelAutoSyncStateExtraKey].(map[string]any)["managed_models"].([]any)
	require.Len(t, managed, 1)
	winner := managed[0].(string)
	require.Contains(t, []string{"gpt-first", "gpt-second"}, winner)
	require.Equal(t, map[string]any{winner: winner}, got.Credentials["model_mapping"])
	var events int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM scheduler_outbox WHERE account_id = $1`, account.ID).Scan(&events))
	require.Equal(t, 1, events)
}

type openAIModelSyncSnapshotCache struct {
	service.SchedulerCache
	set func(context.Context, *service.Account) error
}

func (c *openAIModelSyncSnapshotCache) SetAccount(ctx context.Context, account *service.Account) error {
	return c.set(ctx, account)
}

func TestOpenAIModelSyncRollbackAndPostCommitSnapshot(t *testing.T) {
	ctx := context.Background()
	account := mustCreateAccount(t, testEntClient(t), &service.Account{
		Name: "model-sync-rollback", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "keep", "model_mapping": map[string]any{"manual": "target"}},
		Extra:       map[string]any{service.OpenAIModelAutoSyncEnabledExtraKey: true},
	})
	functionName := fmt.Sprintf("fail_model_sync_outbox_%d", account.ID)
	triggerName := functionName + "_trigger"
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON scheduler_outbox", triggerName))
		_, _ = integrationDB.ExecContext(ctx, fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", functionName))
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM scheduler_outbox WHERE account_id = $1`, account.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE id = $1`, account.ID)
	})
	_, err := integrationDB.ExecContext(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.account_id = %d THEN RAISE EXCEPTION 'forced model sync outbox failure'; END IF;
			RETURN NEW;
		END;
		$$`, functionName, account.ID))
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, fmt.Sprintf("CREATE TRIGGER %s BEFORE INSERT ON scheduler_outbox FOR EACH ROW EXECUTE FUNCTION %s()", triggerName, functionName))
	require.NoError(t, err)
	var published *service.Account
	cache := &openAIModelSyncSnapshotCache{set: func(ctx context.Context, snapshot *service.Account) error {
		// A fresh transaction can lock the row and see the durable event here;
		// publishing before the writer commits would fail this check.
		tx, err := integrationDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.ExecContext(ctx, `SET LOCAL lock_timeout = '1s'`)
		require.NoError(t, err)
		var id int64
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id = $1 FOR NO KEY UPDATE`, account.ID).Scan(&id))
		var events int
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM scheduler_outbox WHERE account_id = $1`, account.ID).Scan(&events))
		require.Equal(t, 1, events)
		published = snapshot
		return nil
	}}
	repo := newAccountRepositoryWithSQL(testEntClient(t), integrationDB, cache)
	loaded, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	state := &service.OpenAIModelAutoSyncState{Status: "success", ManagedModels: []string{"gpt-test"}}
	mapping := map[string]string{"manual": "target", "gpt-test": "gpt-test"}
	err = repo.CommitOpenAIModelSync(ctx, loaded, mapping, state)
	require.ErrorContains(t, err, "forced model sync outbox failure")
	require.Nil(t, published)
	got, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, loaded.Credentials, got.Credentials)
	require.Equal(t, loaded.Extra, got.Extra)
	var events int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM scheduler_outbox WHERE account_id = $1`, account.ID).Scan(&events))
	require.Zero(t, events)
	_, err = integrationDB.ExecContext(ctx, fmt.Sprintf("DROP TRIGGER %s ON scheduler_outbox", triggerName))
	require.NoError(t, err)
	require.NoError(t, repo.CommitOpenAIModelSync(ctx, loaded, mapping, state))
	require.NotNil(t, published)
	require.Equal(t, map[string]any{"manual": "target", "gpt-test": "gpt-test"}, published.Credentials["model_mapping"])
	require.Equal(t, "success", published.Extra[service.OpenAIModelAutoSyncStateExtraKey].(map[string]any)["status"])
}

func TestOpenAIModelSyncRejectsChangedObservation(t *testing.T) {
	for _, tc := range []struct{ name, mutation string }{
		{"token", `UPDATE accounts SET credentials = credentials || '{"refresh_token":"new"}' WHERE id = $1`},
		{"mapping", `UPDATE accounts SET credentials = credentials || '{"model_mapping":{"manual":"edited"}}' WHERE id = $1`},
		{"platform", `UPDATE accounts SET platform = 'grok' WHERE id = $1`},
		{"type", `UPDATE accounts SET type = 'apikey' WHERE id = $1`},
		{"proxy assignment", `UPDATE accounts SET proxy_id = NULL WHERE id = $1`},
		{"proxy host", `UPDATE proxies SET host = 'changed.example' WHERE id = (SELECT proxy_id FROM accounts WHERE id = $1)`},
		{"proxy credentials", `UPDATE proxies SET password = 'new' WHERE id = (SELECT proxy_id FROM accounts WHERE id = $1)`},
		{"proxy status", `UPDATE proxies SET status = 'disabled' WHERE id = (SELECT proxy_id FROM accounts WHERE id = $1)`},
		{"prior state", `UPDATE accounts SET extra = extra || '{"openai_model_auto_sync":{"status":"newer"}}' WHERE id = $1`},
		{"disabled", `UPDATE accounts SET extra = extra || '{"openai_model_auto_sync_enabled":false}' WHERE id = $1`},
		{"missing flag", `UPDATE accounts SET extra = extra - 'openai_model_auto_sync_enabled' WHERE id = $1`},
		{"inactive", `UPDATE accounts SET status = 'disabled' WHERE id = $1`},
		{"expired", `UPDATE accounts SET expires_at = NOW() - INTERVAL '1 second' WHERE id = $1`},
		{"shadow", `UPDATE accounts SET parent_account_id = (SELECT min(id) FROM accounts WHERE name = 'model-sync-parent'), quota_dimension = 'spark' WHERE id = $1`},
		{"deleted", `UPDATE accounts SET deleted_at = NOW() WHERE id = $1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			tx := testEntTx(t)
			repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
			mustCreateAccount(t, tx.Client(), &service.Account{Name: "model-sync-parent", Platform: service.PlatformOpenAI})
			proxy := mustCreateProxy(t, tx.Client(), &service.Proxy{Name: "sync-proxy"})
			account := mustCreateAccount(t, tx.Client(), &service.Account{
				Name: "model-sync-cas", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, ProxyID: &proxy.ID,
				Credentials: map[string]any{"access_token": "keep", "refresh_token": "keep", "model_mapping": map[string]any{"manual": "target"}},
				Extra:       map[string]any{service.OpenAIModelAutoSyncEnabledExtraKey: true},
			})
			loaded, err := repo.GetByID(ctx, account.ID)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, tc.mutation, account.ID)
			require.NoError(t, err)
			var before, after []byte
			require.NoError(t, scanSingleRow(ctx, tx, `SELECT jsonb_build_object('credentials', credentials, 'extra', extra) FROM accounts WHERE id = $1`, []any{account.ID}, &before))
			err = repo.CommitOpenAIModelSync(ctx, loaded, map[string]string{"gpt-test": "gpt-test"}, &service.OpenAIModelAutoSyncState{Status: "success"})
			require.ErrorIs(t, err, service.ErrOpenAIModelAutoSyncChanged)
			require.NoError(t, scanSingleRow(ctx, tx, `SELECT jsonb_build_object('credentials', credentials, 'extra', extra) FROM accounts WHERE id = $1`, []any{account.ID}, &after))
			require.JSONEq(t, string(before), string(after))
			var events int
			require.NoError(t, scanSingleRow(ctx, tx, `SELECT count(*) FROM scheduler_outbox WHERE account_id = $1`, []any{account.ID}, &events))
			require.Zero(t, events)
		})
	}
}

func TestOpenAIModelSyncPreservesCredentialsAndUnrelatedExtra(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "model-sync-merge", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "keep", "refresh_token": "refresh", "expires_at": float64(1800000000),
			"custom": map[string]any{"nested": true}, "model_mapping": map[string]any{"manual": "target"},
		},
		Extra: map[string]any{service.OpenAIModelAutoSyncEnabledExtraKey: true, "custom": map[string]any{"nested": true}},
	})
	loaded, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `UPDATE accounts SET extra = extra || '{"concurrent":"keep"}' WHERE id = $1`, account.ID)
	require.NoError(t, err)
	mapping := map[string]string{"manual": "target", "gpt-test": "gpt-test"}
	require.NoError(t, repo.CommitOpenAIModelSync(ctx, loaded, mapping, &service.OpenAIModelAutoSyncState{Status: "success", ManagedModels: []string{"gpt-test"}}))
	got, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	wantCredentials := copyJSONMap(loaded.Credentials)
	wantCredentials["model_mapping"] = map[string]any{"manual": "target", "gpt-test": "gpt-test"}
	require.Equal(t, wantCredentials, got.Credentials)
	require.Equal(t, "keep", got.Extra["concurrent"])
	require.Equal(t, map[string]any{"nested": true}, got.Extra["custom"])
	preserved := copyJSONMap(got.Credentials)
	require.NoError(t, repo.CommitOpenAIModelSync(ctx, got, nil, &service.OpenAIModelAutoSyncState{Status: "error", ErrorCode: "fetch_failed"}))
	got, err = repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, preserved, got.Credentials)
	require.Equal(t, "error", got.Extra[service.OpenAIModelAutoSyncStateExtraKey].(map[string]any)["status"])
	var events int
	require.NoError(t, scanSingleRow(ctx, tx, `SELECT count(*) FROM scheduler_outbox WHERE account_id = $1`, []any{account.ID}, &events))
	// Multiple changes may coalesce into one durable scheduler invalidation.
	require.Equal(t, 1, events)
}

func TestOpenAIModelSyncManagedStateSurvivesOrdinaryWrites(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := &service.Account{
		Name: "model-sync-managed", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Status: service.StatusActive, Concurrency: 1, Priority: 1, Schedulable: true,
		Credentials: map[string]any{"access_token": "original", "model_mapping": map[string]any{"gpt-test": "gpt-test"}},
		Extra: map[string]any{
			service.OpenAIModelAutoSyncEnabledExtraKey: true,
			service.OpenAIModelAutoSyncStateExtraKey:   map[string]any{"status": "forged"},
			"custom":                                   "keep",
		},
	}
	require.NoError(t, repo.Create(ctx, account))
	loaded, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotContains(t, loaded.Extra, service.OpenAIModelAutoSyncStateExtraKey)

	state := &service.OpenAIModelAutoSyncState{Status: "success", NextSyncUnix: 1800000000, ManagedModels: []string{"gpt-test"}}
	require.NoError(t, repo.CommitOpenAIModelSync(ctx, loaded, nil, state))
	// A stale form may omit, forge, or clear ownership. Turning sync off and
	// refreshing OAuth tokens must not convert previously managed models to manual.
	loaded.Extra[service.OpenAIModelAutoSyncEnabledExtraKey] = false
	loaded.Extra[service.OpenAIModelAutoSyncStateExtraKey] = map[string]any{"status": "stale"}
	loaded.Credentials["access_token"] = "refreshed"
	require.NoError(t, repo.Update(ctx, loaded))
	got, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, "refreshed", got.Credentials["access_token"])
	require.Equal(t, false, got.Extra[service.OpenAIModelAutoSyncEnabledExtraKey])
	require.Equal(t, map[string]any{"status": "success", "next_sync_unix": float64(1800000000), "managed_models": []any{"gpt-test"}}, got.Extra[service.OpenAIModelAutoSyncStateExtraKey])
	preserved := got.Extra[service.OpenAIModelAutoSyncStateExtraKey]

	delete(got.Extra, service.OpenAIModelAutoSyncStateExtraKey)
	require.NoError(t, repo.Update(ctx, got))
	require.NoError(t, repo.UpdateExtra(ctx, account.ID, map[string]any{service.OpenAIModelAutoSyncStateExtraKey: nil, "custom": "edited"}))
	_, err = repo.BulkUpdate(ctx, []int64{account.ID}, service.AccountBulkUpdate{
		Extra:       map[string]any{service.OpenAIModelAutoSyncStateExtraKey: map[string]any{"status": "forged"}},
		Credentials: map[string]any{"refresh_token": "bulk-refresh"},
	})
	require.NoError(t, err)
	require.NoError(t, repo.UpdateCredentials(ctx, account.ID, map[string]any{"access_token": "refreshed-again"}))
	got, err = repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, preserved, got.Extra[service.OpenAIModelAutoSyncStateExtraKey])
	require.Equal(t, "edited", got.Extra["custom"])
}

func TestOpenAIModelSyncDueInvalidNumbersAreImmediatelyDue(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	_, err := tx.ExecContext(ctx, `UPDATE accounts SET extra = extra - 'openai_model_auto_sync_enabled'`)
	require.NoError(t, err)
	invalid := []string{`null`, `"1800000000"`, `"junk"`, `true`, `[]`, `{}`, `1.5`, `9223372036854775808`, `-9223372036854775809`, `1e100`}
	var want []int64
	for i, value := range invalid {
		var id int64
		err := scanSingleRow(ctx, tx, `
			INSERT INTO accounts (name, platform, type, status, extra)
			VALUES ($1, 'openai', 'apikey', 'active', $2::jsonb) RETURNING id
		`, []any{fmt.Sprintf("invalid-next-%d", i), `{"openai_model_auto_sync_enabled":true,"openai_model_auto_sync":{"next_sync_unix":` + value + `}}`}, &id)
		require.NoError(t, err)
		want = append(want, id)
	}
	for _, state := range []string{`null`, `[]`, `"invalid-state"`, `{"next_sync_unix":-9223372036854775808}`} {
		var id int64
		err := scanSingleRow(ctx, tx, `
			INSERT INTO accounts (name, platform, type, status, extra)
			VALUES ('invalid-state', 'openai', 'oauth', 'active', $1::jsonb) RETURNING id
		`, []any{`{"openai_model_auto_sync_enabled":true,"openai_model_auto_sync":` + state + `}`}, &id)
		require.NoError(t, err)
		want = append(want, id)
	}
	// A valid int64 future value must not be mistaken for a malformed one.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO accounts (name, platform, type, status, extra)
		VALUES ('int64-max', 'openai', 'oauth', 'active', '{"openai_model_auto_sync_enabled":true,"openai_model_auto_sync":{"next_sync_unix":9223372036854775807}}')
	`)
	require.NoError(t, err)
	accounts, err := repo.ListDueOpenAIModelSyncAccounts(ctx, time.Now(), 100)
	require.NoError(t, err)
	var got []int64
	for _, account := range accounts {
		got = append(got, account.ID)
	}
	require.Equal(t, want, got)
}

func TestOpenAIModelSyncDueFilteringAndOrdering(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	now := time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)
	_, err := tx.ExecContext(ctx, `UPDATE accounts SET extra = extra - 'openai_model_auto_sync_enabled'`)
	require.NoError(t, err)

	insert := func(name, platform, accountType, status, extra string, expiresAt *time.Time, parentID *int64, deletedAt *time.Time) int64 {
		t.Helper()
		var id int64
		err := scanSingleRow(ctx, tx, `
			INSERT INTO accounts (name, platform, type, status, credentials, extra, expires_at, parent_account_id, deleted_at, quota_dimension)
			VALUES ($1, $2, $3, $4, '{"api_key":"keep","model_mapping":{"manual":"target"}}'::jsonb, $5::jsonb, $6, $7, $8, CASE WHEN $7::bigint IS NULL THEN 'global' ELSE 'spark' END)
			RETURNING id
		`, []any{name, platform, accountType, status, extra, expiresAt, parentID, deletedAt}, &id)
		require.NoError(t, err)
		return id
	}
	extraAt := func(unix int64) string {
		return fmt.Sprintf(`{"openai_model_auto_sync_enabled":true,"openai_model_auto_sync":{"next_sync_unix":%d}}`, unix)
	}
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	due := extraAt(now.Unix())
	insert("future", "openai", "oauth", "active", extraAt(future.Unix()), nil, nil, nil)
	latestID := insert("latest", "openai", "oauth", "active", due, &future, nil, nil)
	earliestID := insert("earliest", "openai", "apikey", "active", extraAt(past.Unix()), nil, nil, nil)
	missingID := insert("missing-state", "openai", "oauth", "active", `{"openai_model_auto_sync_enabled":true}`, nil, nil, nil)
	insert("wrong-platform", "grok", "oauth", "active", due, nil, nil, nil)
	insert("wrong-type", "openai", "upstream", "active", due, nil, nil, nil)
	insert("inactive", "openai", "oauth", "disabled", due, nil, nil, nil)
	insert("deleted", "openai", "oauth", "active", due, nil, nil, &past)
	insert("expired", "openai", "oauth", "active", due, &past, nil, nil)
	insert("expires-now", "openai", "oauth", "active", due, &now, nil, nil)
	insert("shadow", "openai", "apikey", "active", due, nil, &earliestID, nil)
	insert("disabled", "openai", "oauth", "active", `{"openai_model_auto_sync_enabled":false}`, nil, nil, nil)
	insert("string-enabled", "openai", "oauth", "active", `{"openai_model_auto_sync_enabled":"true"}`, nil, nil, nil)
	insert("missing-enabled", "openai", "oauth", "active", `{}`, nil, nil, nil)

	accounts, err := repo.ListDueOpenAIModelSyncAccounts(ctx, now, 2)
	require.NoError(t, err)
	require.Len(t, accounts, 2)
	require.Equal(t, missingID, accounts[0].ID)
	require.Equal(t, earliestID, accounts[1].ID)
	require.Equal(t, "keep", accounts[1].Credentials["api_key"])
	require.Equal(t, map[string]any{"manual": "target"}, accounts[1].Credentials["model_mapping"])

	accounts, err = repo.ListDueOpenAIModelSyncAccounts(ctx, now, 20)
	require.NoError(t, err)
	require.Len(t, accounts, 3)
	require.Equal(t, latestID, accounts[2].ID)
	require.Equal(t, service.AccountTypeOAuth, accounts[2].Type)
}
