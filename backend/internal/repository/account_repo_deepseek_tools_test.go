//go:build unit

package repository

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDeepseekToolsAccountEditUsesLockedBase(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stored any
	}{
		{"changed base clears stale form marker", nil}, {"unchanged base retains current probe", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			defer client.Close()
			mock.ExpectQuery(`(?s)CASE WHEN platform = 'deepseek'.*credentials -> 'base_url' IS DISTINCT FROM \$4::jsonb -> 'base_url'.*credentials -> 'api_base_urls' IS DISTINCT FROM.*FOR NO KEY UPDATE`).
				WillReturnRows(sqlmock.NewRows(openCodeGoMergeMockColumns()).
					AddRow(false, false, true, nil, nil, nil, nil, nil, nil, false, nil, nil, nil, nil, nil, tc.stored))
			account := &service.Account{ID: 7, Platform: service.PlatformDeepseek, Type: service.AccountTypeAPIKey,
				Credentials: map[string]any{"base_url": "https://relay.test"}, Extra: map[string]any{"deepseek_tools_supported": true, "deepseek_tools_mode": "auto"}}
			extra, err := lockAndMergeAccountProbeExtra(context.Background(), client, account, service.AccountBillingSettingsUpdate{})
			require.NoError(t, err)
			marker, exists := extra["deepseek_tools_supported"]
			require.Equal(t, tc.stored != nil, exists)
			require.Equal(t, tc.stored, marker)
			require.Equal(t, "auto", extra["deepseek_tools_mode"])
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDeepseekToolsBulkEditInvalidatesBaseInSameWrite(t *testing.T) {
	for _, key := range []string{"base_url", "api_protocol", "api_base_urls", "account_mode", "api_key"} {
		t.Run(key, func(t *testing.T) {
			exec := &recordingSQLExecutor{result: rowsAffectedResult(2)}
			repo := newAccountRepositoryWithSQL(nil, exec, nil)
			_, err := repo.BulkUpdate(context.Background(), []int64{1, 2}, service.AccountBulkUpdate{
				Credentials: map[string]any{key: "edited"}, Extra: map[string]any{"deepseek_tools_supported": true},
			})
			require.NoError(t, err)
			require.NotEmpty(t, exec.execQueries)
			query := normalizeSQLWhitespace(exec.execQueries[0])
			if key == "api_key" {
				require.NotContains(t, query, "deepseek_tools_supported")
				return
			}
			require.Contains(t, query, "credentials = COALESCE(credentials, '{}'::jsonb) || $1::jsonb")
			require.Contains(t, query, "CASE WHEN platform = 'deepseek'")
			require.Contains(t, query, "credentials -> '"+key+"' IS DISTINCT FROM (COALESCE(credentials, '{}'::jsonb) || $1::jsonb) -> '"+key+"'")
			require.Contains(t, query, "THEN ARRAY['deepseek_tools_supported'] ELSE ARRAY[]::text[] END")
		})
	}
}

func TestDeepseekToolsLateProbeCannotOverwriteEditedBase(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	defer client.Close()
	repo := newAccountRepositoryWithSQL(client, nil, nil)
	account := &service.Account{ID: 7, Credentials: map[string]any{"base_url": "https://old.test"}}
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE accounts SET extra = .*WHERE id = \$2.*platform = 'deepseek'.*credentials = \$3::jsonb AND proxy_id IS NOT DISTINCT FROM \$4`).
		WithArgs(`{"deepseek_tools_supported":true}`, int64(7), `{"base_url":"https://old.test"}`, nil).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	require.NoError(t, repo.UpdateDeepseekToolsSupport(context.Background(), account, new(true)))
	require.NoError(t, mock.ExpectationsWereMet())
}
