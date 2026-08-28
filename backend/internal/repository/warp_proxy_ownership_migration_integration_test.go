//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestWarpProxyOwnershipMigrationSelectiveBackfillAndLiveUniqueness(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	migrationSQL, err := dbmigrations.FS.ReadFile("194_warp_proxy_ownership.sql")
	require.NoError(t, err)

	suffix := time.Now().UnixNano()
	managedGroupName := fmt.Sprintf("warp-owned-%d", suffix)
	manualGroupName := fmt.Sprintf("warp-manual-%d", suffix)
	var managedGroupID, manualGroupID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO proxy_groups (name, description)
VALUES ($1, 'Cloudflare WARP proxy pool (auto-managed by warp-gateway sync)')
RETURNING id`, managedGroupName).Scan(&managedGroupID))
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO proxy_groups (name, description)
VALUES ($1, 'operator managed')
RETURNING id`, manualGroupName).Scan(&manualGroupID))

	insertProxy := func(name string, groupID int64, deleted bool) int64 {
		t.Helper()
		var id int64
		deletedExpr := "NULL"
		if deleted {
			deletedExpr = "NOW()"
		}
		query := fmt.Sprintf(`
INSERT INTO proxies (name, protocol, host, port, status, group_id, deleted_at)
VALUES ($1, 'socks5', '127.0.0.1', 41000, 'active', $2, %s)
RETURNING id`, deletedExpr)
		require.NoError(t, tx.QueryRowContext(ctx, query, name, groupID).Scan(&id))
		return id
	}

	eligibleID := insertProxy(fmt.Sprintf("warp-eligible-%d", suffix), managedGroupID, false)
	nameOnlyID := insertProxy(fmt.Sprintf("warp-name-only-%d", suffix), manualGroupID, false)
	groupOnlyID := insertProxy(fmt.Sprintf("manual-in-warp-group-%d", suffix), managedGroupID, false)
	deletedID := insertProxy(fmt.Sprintf("warp-deleted-%d", suffix), managedGroupID, true)

	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err, "migration must be idempotent")

	for _, tc := range []struct {
		id      int64
		managed bool
	}{
		{id: eligibleID, managed: true},
		{id: nameOnlyID},
		{id: groupOnlyID},
		{id: deletedID},
	} {
		var managedBy sql.NullString
		require.NoError(t, tx.QueryRowContext(ctx, `SELECT managed_by FROM proxies WHERE id = $1`, tc.id).Scan(&managedBy))
		require.Equal(t, tc.managed, managedBy.Valid, "proxy id=%d managed_by=%q", tc.id, managedBy.String)
	}

	externalID := fmt.Sprintf("external-%d", suffix)
	_, err = tx.ExecContext(ctx, `UPDATE proxies SET external_id = $1 WHERE id = $2`, externalID, eligibleID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "SAVEPOINT warp_duplicate")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
INSERT INTO proxies (name, protocol, host, port, status, managed_by, external_id)
VALUES ($1, 'socks5', '127.0.0.1', 41001, 'active', 'warp-gateway', $2)`, fmt.Sprintf("warp-duplicate-%d", suffix), externalID)
	require.Error(t, err, "two live proxies must not share an external owner id")
	_, rollbackErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT warp_duplicate")
	require.NoError(t, rollbackErr)

	_, err = tx.ExecContext(ctx, `
INSERT INTO proxies (name, protocol, host, port, status, managed_by, external_id, deleted_at)
VALUES ($1, 'socks5', '127.0.0.1', 41002, 'active', 'warp-gateway', $2, NOW())`, fmt.Sprintf("warp-deleted-reuse-%d", suffix), externalID)
	require.NoError(t, err, "soft-deleted proxies may reuse external_id")
}
