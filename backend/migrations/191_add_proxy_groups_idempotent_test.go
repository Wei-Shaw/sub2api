//go:build unit

package migrations_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMigration191ProxyGroupsIsIdempotentSQL 静态验收：191 迁移全文使用 IF NOT EXISTS，
// 可安全重复执行（真正的 DB 双跑仍依赖部署环境；此处锁住 SQL 幂等写法）。
func TestMigration191ProxyGroupsIsIdempotentSQL(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	path := filepath.Join(filepath.Dir(file), "191_add_proxy_groups.sql")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	sql := string(raw)

	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS proxy_groups")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS proxy_groups_status_idx")
	require.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS proxy_groups_name_alive_uidx")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS group_id")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS proxy_group_id")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS proxies_group_id_idx")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS accounts_proxy_group_id_idx")

	// 禁止非幂等的裸 CREATE TABLE / CREATE INDEX（不含 IF NOT EXISTS）
	for _, line := range strings.Split(sql, "\n") {
		trim := strings.TrimSpace(line)
		upper := strings.ToUpper(trim)
		if strings.HasPrefix(upper, "CREATE TABLE ") && !strings.Contains(upper, "IF NOT EXISTS") {
			t.Fatalf("non-idempotent CREATE TABLE: %s", trim)
		}
		if strings.HasPrefix(upper, "CREATE INDEX ") && !strings.Contains(upper, "IF NOT EXISTS") {
			t.Fatalf("non-idempotent CREATE INDEX: %s", trim)
		}
		if strings.HasPrefix(upper, "CREATE UNIQUE INDEX ") && !strings.Contains(upper, "IF NOT EXISTS") {
			t.Fatalf("non-idempotent CREATE UNIQUE INDEX: %s", trim)
		}
		if strings.HasPrefix(upper, "ALTER TABLE ") && strings.Contains(upper, "ADD COLUMN ") && !strings.Contains(upper, "IF NOT EXISTS") {
			t.Fatalf("non-idempotent ADD COLUMN: %s", trim)
		}
	}
}
