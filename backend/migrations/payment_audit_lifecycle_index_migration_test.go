package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 迁移 251 把 payment_audit_logs 的唯一约束收窄到“一单只发生一次”的动作。
// 这里锁定三点：只覆盖三个认领动作、仍然使用 CONCURRENTLY、旧的全量唯一索引被删除。
func TestPaymentAuditLifecycleUniqueIndexMigration(t *testing.T) {
	content, err := FS.ReadFile("251_payment_audit_logs_lifecycle_unique_index_notx.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql,
		"CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_payment_audit_logs_order_action_once_uniq ON payment_audit_logs (order_id, action) WHERE action IN ('SUBSCRIPTION_ASSIGNED', 'AFFILIATE_REBATE_APPLIED', 'AFFILIATE_REBATE_SKIPPED')")
	require.Contains(t, sql, "DROP INDEX CONCURRENTLY IF EXISTS idx_payment_audit_logs_order_action_uniq")
	require.NotContains(t, sql,
		"CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_payment_audit_logs_order_action_uniq",
		"旧名字不能再被建成全量唯一索引")
}
