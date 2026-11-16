package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Opt-in local PostgreSQL verification. All inserted rows are rolled back.
func TestUsageProxyPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("SUB2API_USAGE_TEST_DSN")
	if dsn == "" {
		t.Skip("SUB2API_USAGE_TEST_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	var userID, keyID, accountID int64
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT user_id,api_key_id,account_id FROM usage_logs LIMIT 1").Scan(&userID, &keyID, &accountID))
	r := newUsageLogRepositoryWithSQL(nil, tx)
	now := time.Now().UTC()
	for _, mode := range []string{"single", "batch", "best effort", "no result"} {
		log := &service.UsageLog{UserID: userID, APIKeyID: keyID, AccountID: accountID, RequestID: fmt.Sprintf("proxy-test-%d-%s", now.UnixNano(), mode), Model: "proxy-test", ProxySnapshot: &service.UsageProxySnapshot{ID: 42, Name: "snapshot-original"}}
		prepared := prepareUsageLogInsert(log)
		switch mode {
		case "single":
			_, err = r.Create(ctx, log)
		case "batch":
			q, args := buildUsageLogBatchInsertQuery([]string{"test"}, map[string]usageLogInsertPrepared{"test": prepared})
			var payload []byte
			err = tx.QueryRowContext(ctx, q, args...).Scan(&payload)
		case "best effort":
			q, args := buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared})
			_, err = tx.ExecContext(ctx, q, args...)
		case "no result":
			err = execUsageLogInsertNoResult(ctx, tx, prepared)
		}
		require.NoError(t, err, mode)
		var id int64
		require.NoError(t, tx.QueryRowContext(ctx, "SELECT id FROM usage_logs WHERE request_id=$1 AND api_key_id=$2", log.RequestID, keyID).Scan(&id))
		loaded, err := r.GetByID(ctx, id)
		require.NoError(t, err, mode)
		require.Equal(t, "snapshot-original", loaded.ProxySnapshot.Name)
		require.EqualValues(t, 42, loaded.ProxySnapshot.ID)
	}
}
