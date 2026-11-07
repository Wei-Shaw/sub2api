//go:build integration

package repository

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUsageIncremental_OutOfOrderCommitLateInsertRollbackAndPagination(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newUsageLogRepositoryWithSQL(client, integrationDB)
	user := mustCreateUser(t, client, &service.User{Email: uuid.NewString() + "@example.com"})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-" + uuid.NewString(), Name: "export"})
	account := mustCreateAccount(t, client, &service.Account{Name: uuid.NewString()})
	t.Cleanup(func() { _, _ = integrationDB.Exec("DELETE FROM usage_logs WHERE user_id = $1", user.ID) })
	var boundary uint64
	require.NoError(t, integrationDB.QueryRow("SELECT pg_current_xact_id()::text").Scan(&boundary))
	cursor := service.UsageCursor{TransactionID: boundary, ID: math.MaxInt64}
	newLog := func() *service.UsageLog {
		return &service.UsageLog{UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID, RequestID: uuid.NewString(), Model: "test-model", CreatedAt: time.Now().Add(-48 * time.Hour)}
	}
	// Allocate a lower transaction/row ID, then let another transaction commit first.
	older, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = older.Rollback() }()
	first := newLog()
	inserted, err := repo.createSingle(ctx, older, first)
	require.NoError(t, err)
	require.True(t, inserted)
	newer := newLog()
	inserted, err = repo.createSingle(ctx, integrationDB, newer)
	require.NoError(t, err)
	require.True(t, inserted)
	blocked, err := repo.ListIncremental(ctx, cursor, 100)
	require.NoError(t, err)
	require.Empty(t, blocked.Records)
	require.Equal(t, cursor, blocked.NextCursor)
	// This late row has an old request timestamp and an ID larger than newer's ID,
	// but shares first's older transaction. It must sort before newer in the feed.
	late := newLog()
	inserted, err = repo.createSingle(ctx, older, late)
	require.NoError(t, err)
	require.True(t, inserted)
	require.NoError(t, older.Commit())
	require.Less(t, first.ID, newer.ID)
	require.Less(t, newer.ID, late.ID)
	expected := []int64{first.ID, late.ID, newer.ID}
	for i, id := range expected {
		page, err := repo.ListIncremental(ctx, cursor, 1)
		require.NoError(t, err)
		require.Len(t, page.Records, 1)
		require.Equal(t, id, page.Records[0].ID)
		require.Equal(t, i < len(expected)-1, page.HasMore)
		cursor = page.NextCursor
	}
	// Duplicate request writes are discarded by the existing unique constraint.
	inserted, err = repo.createSingle(ctx, integrationDB, newer)
	require.NoError(t, err)
	require.False(t, inserted)
	rolledBack, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	discarded := newLog()
	_, err = repo.createSingle(ctx, rolledBack, discarded)
	require.NoError(t, err)
	require.NoError(t, rolledBack.Rollback())
	afterRollback := newLog()
	_, err = repo.createSingle(ctx, integrationDB, afterRollback)
	require.NoError(t, err)
	page, err := repo.ListIncremental(ctx, cursor, 1)
	require.NoError(t, err)
	require.Len(t, page.Records, 1)
	require.Equal(t, afterRollback.ID, page.Records[0].ID)
	empty, err := repo.ListIncremental(ctx, page.NextCursor, 10)
	require.NoError(t, err)
	require.Empty(t, empty.Records)
	require.False(t, empty.HasMore)
	require.Equal(t, page.NextCursor, empty.NextCursor)
}

func TestUsageIncremental_MigrationLegacyRows(t *testing.T) {
	tx := testTx(t)
	_, err := tx.Exec(`CREATE TEMP TABLE legacy_usage (id BIGSERIAL PRIMARY KEY);
 INSERT INTO legacy_usage DEFAULT VALUES`)
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("246_usage_log_ingestion_transaction.sql")
	require.NoError(t, err)
	sql := strings.ReplaceAll(string(migration), "usage_logs", "legacy_usage")
	for range 2 {
		_, err = tx.Exec(sql)
		require.NoError(t, err)
	}
	_, err = tx.Exec("INSERT INTO legacy_usage DEFAULT VALUES")
	require.NoError(t, err)
	rows, err := tx.Query("SELECT ingestion_txid::text FROM legacy_usage ORDER BY id")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var values []string
	for rows.Next() {
		var value string
		require.NoError(t, rows.Scan(&value))
		values = append(values, value)
	}
	require.NoError(t, rows.Err())
	require.Len(t, values, 2)
	require.Equal(t, "0", values[0])
	require.NotEqual(t, "0", values[1])
}
