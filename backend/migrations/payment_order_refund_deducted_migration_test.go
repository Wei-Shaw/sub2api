package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPaymentOrderRefundDeductedMigration 守护 238 的两条硬性要求：
//  1. 两个台账列必须是 NOT NULL DEFAULT 0，否则存量订单读出 NULL，
//     prepDeduct 的差额计算会退化成"全额再扣一次"；
//  2. 回填必须只认 REFUND_ROLLBACK_FAILED 审计证据。按状态一刀切回填
//     （例如给所有 REFUND_FAILED 记 refund_amount）会把回滚其实成功的订单
//     误记成已扣，重试时少扣一笔——这是静默的资金错误，必须挡在 CI。
func TestPaymentOrderRefundDeductedMigration(t *testing.T) {
	content, err := FS.ReadFile("238_add_payment_order_refund_deducted.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")

	require.Contains(t, sql,
		"ADD COLUMN IF NOT EXISTS refund_deducted_amount DECIMAL(20,2) NOT NULL DEFAULT 0")
	require.Contains(t, sql,
		"ADD COLUMN IF NOT EXISTS refund_deducted_sub_days INTEGER NOT NULL DEFAULT 0")
	require.Contains(t, sql, "COMMENT ON COLUMN payment_orders.refund_deducted_amount")
	require.Contains(t, sql, "COMMENT ON COLUMN payment_orders.refund_deducted_sub_days")

	// 回填的唯一证据来源。
	require.Contains(t, sql, "l.action = 'REFUND_ROLLBACK_FAILED'")
	// 幂等：只写仍为 0 的行。
	require.Contains(t, sql, "o.refund_deducted_amount = 0")
	require.Contains(t, sql, "o.refund_deducted_sub_days = 0")
	// order_id 是 varchar，非数字值不能让整个 DO 块炸掉。
	require.Contains(t, sql, "l.order_id ~ '^[0-9]+$'")

	// 绝不允许按订单状态一刀切回填。
	require.NotContains(t, sql, "status = 'REFUND_FAILED'")
	require.NotContains(t, sql, "SET refund_deducted_amount = refund_amount")
}
