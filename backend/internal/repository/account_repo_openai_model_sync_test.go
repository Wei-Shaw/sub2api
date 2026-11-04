package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIModelSyncDueLimitAndQueryErrors(t *testing.T) {
	repo := newAccountRepositoryWithSQL(nil, nil, nil)
	for _, limit := range []int{-1, 0} {
		accounts, err := repo.ListDueOpenAIModelSyncAccounts(context.Background(), time.Now(), limit)
		require.NoError(t, err)
		require.Empty(t, accounts)
	}
	_, err := repo.ListDueOpenAIModelSyncAccounts(context.Background(), time.Now(), 1)
	require.ErrorContains(t, err, "SQL executor not configured")
}

func TestOpenAIModelSyncRejectsInvalidInputs(t *testing.T) {
	repo := newAccountRepositoryWithSQL(nil, nil, nil)
	require.ErrorIs(t, repo.CommitOpenAIModelSync(context.Background(), nil, nil, &service.OpenAIModelAutoSyncState{}), service.ErrAccountNilInput)
	require.ErrorIs(t, repo.CommitOpenAIModelSync(context.Background(), &service.Account{}, nil, nil), service.ErrAccountNilInput)
	for _, account := range []*service.Account{
		{Platform: service.PlatformGrok, Type: service.AccountTypeOAuth},
		{Platform: service.PlatformOpenAI, Type: "upstream"},
		{Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth},
	} {
		require.ErrorIs(t, repo.CommitOpenAIModelSync(context.Background(), account, nil, &service.OpenAIModelAutoSyncState{}), service.ErrOpenAIModelAutoSyncChanged)
	}
}

func TestOpenAIModelSyncUsesCallerTransactionWithoutPublishingSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	mock.ExpectBegin()
	tx, err := client.Tx(context.Background())
	require.NoError(t, err)
	mock.ExpectExec(`(?s)UPDATE accounts.*CASE WHEN \$9::jsonb IS NULL THEN credentials`).
		WithArgs(`{"openai_model_auto_sync":{"status":"error","next_sync_unix":0}}`, int64(17), service.PlatformOpenAI, service.AccountTypeAPIKey, `{"api_key":"keep"}`, nil, "null", "true", nil).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).WillReturnResult(sqlmock.NewResult(1, 1))
	// Any cache call on this empty implementation would panic.
	repo := newAccountRepositoryWithSQL(nil, &recordingSQLExecutor{err: errors.New("must use transaction client")}, &openAIModelSyncUnexpectedCache{})
	account := &service.Account{ID: 17, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "keep"}, Extra: map[string]any{service.OpenAIModelAutoSyncEnabledExtraKey: true}}
	require.NoError(t, repo.CommitOpenAIModelSync(dbent.NewTxContext(context.Background(), tx), account, nil, &service.OpenAIModelAutoSyncState{Status: "error"}))
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

type openAIModelSyncUnexpectedCache struct{ service.SchedulerCache }

func TestOpenAIModelSyncCommitsMappingStateAndOutboxAtomically(t *testing.T) {
	for _, tc := range []struct {
		name      string
		affected  int64
		outboxErr error
		wantErr   error
	}{
		{name: "commit", affected: 1},
		{name: "stale identity or state", wantErr: service.ErrOpenAIModelAutoSyncChanged},
		{name: "outbox failure rolls back", affected: 1, outboxErr: errors.New("outbox failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })
			repo := newAccountRepositoryWithSQL(client, db, nil)
			mock.ExpectBegin()
			mock.ExpectExec(`(?s)UPDATE accounts.*credentials = CASE.*extra = COALESCE.*AND credentials = \$5::jsonb.*AND proxy_id IS NOT DISTINCT FROM \$6.*COALESCE\(extra -> 'openai_model_auto_sync', 'null'::jsonb\) = \$7::jsonb.*COALESCE\(extra -> 'openai_model_auto_sync_enabled', 'null'::jsonb\) = \$8::jsonb.*parent_account_id IS NULL.*deleted_at IS NULL`).
				WithArgs(`{"openai_model_auto_sync":{"status":"success","next_sync_unix":1800000000,"managed_models":["gpt-test"]}}`, int64(17), service.PlatformOpenAI, service.AccountTypeOAuth, `{"access_token":"keep","refresh_token":"also-keep"}`, nil, "null", "true", `{"gpt-test":"gpt-test"}`).
				WillReturnResult(sqlmock.NewResult(0, tc.affected))
			if tc.affected > 0 {
				expect := mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox")).
					WithArgs(service.SchedulerOutboxEventAccountChanged, int64(17), nil, nil, sqlmock.AnyArg())
				if tc.outboxErr != nil {
					expect.WillReturnError(tc.outboxErr)
				} else {
					expect.WillReturnResult(sqlmock.NewResult(1, 1))
				}
			}
			if tc.wantErr != nil || tc.outboxErr != nil {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			account := &service.Account{
				ID: 17, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
				Credentials: map[string]any{"access_token": "keep", "refresh_token": "also-keep"},
				Extra:       map[string]any{service.OpenAIModelAutoSyncEnabledExtraKey: true},
			}
			err = repo.CommitOpenAIModelSync(context.Background(), account, map[string]string{"gpt-test": "gpt-test"}, &service.OpenAIModelAutoSyncState{
				Status: "success", NextSyncUnix: 1800000000, ManagedModels: []string{"gpt-test"},
			})
			if tc.outboxErr != nil {
				require.ErrorIs(t, err, tc.outboxErr)
			} else if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
