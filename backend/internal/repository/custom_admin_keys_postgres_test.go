//go:build unit

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 只使用显式指定的隔离测试数据库，每次创建并清理独立 schema。
func scopedAdminKeyTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SCOPED_ADMIN_TEST_DSN")
	if dsn == "" {
		t.Skip("需要独立 SCOPED_ADMIN_TEST_DSN")
	}
	base, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	schema := fmt.Sprintf("scoped_admin_test_%d", time.Now().UnixNano())
	_, err = base.Exec(`CREATE SCHEMA ` + schema)
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn+" search_path="+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		_ = base.Close()
	})
	_, err = db.Exec(`CREATE TABLE users(id BIGINT PRIMARY KEY); INSERT INTO users(id) VALUES(9)`)
	require.NoError(t, err)
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "247_scoped_admin_api_keys.sql"))
	require.NoError(t, err)
	_, err = db.Exec(string(migration))
	require.NoError(t, err)
	return db
}

func TestScopedAdminKeyPostgresDeleteRecordPreservesHistory(t *testing.T) {
	db := scopedAdminKeyTestDB(t)
	repo := NewScopedAdminKeyRepository(db)
	settings := service.NewSettingService(nil, nil)
	settings.SetScopedAdminKeyRepository(repo)
	ctx := context.Background()
	key, item, err := settings.CreateScopedAdminKey(ctx, service.ScopedAdminKeyInput{Name: "待移除记录", Enabled: true}, 9)
	require.NoError(t, err)

	// 使用真实审计表，检查列表移除不会删除或改写历史日志。
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "180_audit_logs.sql"))
	require.NoError(t, err)
	_, err = db.Exec(string(migration))
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO audit_logs(actor_user_id, auth_method, action, extra) VALUES(9, 'admin_api_key', 'admin.accounts.update', jsonb_build_object('admin_key_id', $1::text))`, item.ID)
	require.NoError(t, err)
	var historyBefore, historyAfter string
	require.NoError(t, db.QueryRow(`SELECT row_to_json(audit_logs)::text FROM audit_logs`).Scan(&historyBefore))

	// 未撤销的 Key 即使直接调用接口也不能被隐藏，停用也不能替代撤销。
	require.ErrorIs(t, settings.DeleteScopedAdminKeyRecord(ctx, item.ID), service.ErrScopedAdminKeyNotFound)
	_, err = settings.AuthenticateScopedAdminKey(ctx, key, "192.0.2.1")
	require.NoError(t, err)
	input := item.ScopedAdminKeyInput
	input.Enabled = false
	_, err = settings.UpdateScopedAdminKey(ctx, item.ID, input)
	require.NoError(t, err)
	require.ErrorIs(t, settings.DeleteScopedAdminKeyRecord(ctx, item.ID), service.ErrScopedAdminKeyNotFound)
	require.NoError(t, settings.RevokeScopedAdminKey(ctx, item.ID))
	items, err := settings.ListScopedAdminKeys(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1, "撤销仍保留在列表")

	require.NoError(t, settings.DeleteScopedAdminKeyRecord(ctx, item.ID))
	items, err = settings.ListScopedAdminKeys(ctx)
	require.NoError(t, err)
	require.Empty(t, items, "重新读取列表仍应隐藏")
	stored, err := repo.GetScopedAdminKey(ctx, item.ID)
	require.NoError(t, err, "底层记录保留以供追溯")
	require.NotNil(t, stored.RevokedAt)
	require.Equal(t, item.Name, stored.Name)
	require.NoError(t, db.QueryRow(`SELECT row_to_json(audit_logs)::text FROM audit_logs`).Scan(&historyAfter))
	require.Equal(t, historyBefore, historyAfter)
	_, err = settings.AuthenticateScopedAdminKey(ctx, key, "192.0.2.1")
	require.ErrorIs(t, err, service.ErrScopedAdminKeyInvalid)
	_, err = settings.UpdateScopedAdminKey(ctx, item.ID, input)
	require.ErrorIs(t, err, service.ErrScopedAdminKeyNotFound)
	_, _, err = settings.RotateScopedAdminKey(ctx, item.ID)
	require.ErrorIs(t, err, service.ErrScopedAdminKeyNotFound)
	require.ErrorIs(t, settings.DeleteScopedAdminKeyRecord(ctx, item.ID), service.ErrScopedAdminKeyNotFound)
	require.ErrorIs(t, settings.DeleteScopedAdminKeyRecord(ctx, "不存在的Key"), service.ErrScopedAdminKeyNotFound)
}

func TestScopedAdminKeyPostgresMigrationAndLifecycle(t *testing.T) {
	db := scopedAdminKeyTestDB(t)
	repo := NewScopedAdminKeyRepository(db)
	settings := service.NewSettingService(nil, nil)
	settings.SetScopedAdminKeyRepository(repo)
	ctx := context.Background()
	input := service.ScopedAdminKeyInput{Name: "隔离数据库验收", Enabled: true, AllowedIPs: []string{"192.0.2.0/24"}, Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.read"}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}}}}}
	key, item, err := settings.CreateScopedAdminKey(ctx, input, 9)
	require.NoError(t, err)
	stored, err := repo.GetScopedAdminKey(ctx, item.ID)
	require.NoError(t, err)
	require.NotEqual(t, key, stored.Hash)
	require.Len(t, stored.Hash, 64)
	_, err = settings.AuthenticateScopedAdminKey(ctx, key, "192.0.2.9")
	require.NoError(t, err, "真实 PostgreSQL 时间精度下 CAS 应能成功")
	_, err = settings.AuthenticateScopedAdminKey(ctx, key, "198.51.100.9")
	require.Error(t, err)

	// 读出策略之后发生收权，旧读取不得通过准入 CAS。
	observed, err := repo.GetScopedAdminKey(ctx, item.ID)
	require.NoError(t, err)
	input.Policy = adminaccess.Policy{}
	_, err = settings.UpdateScopedAdminKey(ctx, item.ID, input)
	require.NoError(t, err)
	require.Error(t, repo.TouchScopedAdminKey(ctx, observed.ID, observed.Hash, observed.UpdatedAt, "192.0.2.9", time.Now().UTC()))
	authenticated, err := settings.AuthenticateScopedAdminKey(ctx, key, "192.0.2.9")
	require.NoError(t, err)
	require.False(t, adminaccess.RouteAllowed(authenticated.Policy, "GET", "/api/v1/admin/accounts"))

	rotated, _, err := settings.RotateScopedAdminKey(ctx, item.ID)
	require.NoError(t, err)
	_, err = settings.AuthenticateScopedAdminKey(ctx, key, "192.0.2.9")
	require.Error(t, err)
	_, err = settings.AuthenticateScopedAdminKey(ctx, rotated, "192.0.2.9")
	require.NoError(t, err)
	input.Enabled = false
	_, err = settings.UpdateScopedAdminKey(ctx, item.ID, input)
	require.NoError(t, err)
	_, err = settings.AuthenticateScopedAdminKey(ctx, rotated, "192.0.2.9")
	require.Error(t, err)
	require.NoError(t, settings.RevokeScopedAdminKey(ctx, item.ID))
	input.Enabled = true
	_, err = settings.UpdateScopedAdminKey(ctx, item.ID, input)
	require.ErrorIs(t, err, service.ErrScopedAdminKeyNotFound)
	_, _, err = settings.RotateScopedAdminKey(ctx, item.ID)
	require.ErrorIs(t, err, service.ErrScopedAdminKeyNotFound)
}
