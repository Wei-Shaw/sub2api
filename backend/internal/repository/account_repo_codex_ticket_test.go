package repository

import (
	"context"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLockAndMergeAccountExtraPreservesLatestCodexTicket(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	mock.ExpectQuery(`(?s)SELECT.*FOR NO KEY UPDATE`).
		WithArgs(int64(41), service.PlatformOpenAI, service.AccountTypeOAuth, `{"access_token":"test"}`, nil).
		WillReturnRows(sqlmock.NewRows([]string{"identity_unchanged", "ollama_group_unchanged", "ollama_proxy_unchanged", "enabled", "rate_sync_enabled", "snapshot", "ollama_session", "ollama_auto", "ollama_snapshot", "current_extra"}).
			AddRow(true, false, true, nil, nil, nil, nil, nil, nil, []byte(`{"codex_turn_ticket:model":{"state":"latest-database-ticket"},"old_admin_setting":true}`)))
	account := &service.Account{ID: 41, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Credentials: map[string]any{"access_token": "test"}, Extra: map[string]any{"codex_turn_ticket:model": map[string]any{"state": "stale"}, "codex_turn_ticket:injected": map[string]any{"state": "spoofed"}, "new_admin_setting": true}}
	extra, err := lockAndMergeAccountProbeExtra(context.Background(), client, account, nil, nil)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"state": "latest-database-ticket"}, extra["codex_turn_ticket:model"])
	require.NotContains(t, extra, "codex_turn_ticket:injected")
	require.NotContains(t, extra, "old_admin_setting")
	require.Equal(t, true, extra["new_admin_setting"])
	require.NoError(t, mock.ExpectationsWereMet())
}
