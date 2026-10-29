package repository

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAffiliateUserOverviewSQLIncludesMaturedFrozenQuota(t *testing.T) {
	query := strings.Join(strings.Fields(affiliateUserOverviewSQL), " ")

	require.Contains(t, query, "ua.aff_quota + COALESCE(matured.matured_frozen_quota, 0)")
	require.Contains(t, query, "frozen_until <= NOW()")
}

func TestAffiliateRecordQueriesUseLedgerAuditFields(t *testing.T) {
	source, err := os.ReadFile("affiliate_repo.go")
	require.NoError(t, err)
	content := string(source)

	require.Contains(t, content, "LEFT JOIN payment_orders po ON po.id = ual.source_order_id")
	require.Contains(t, content, "ual.amount::double precision")
	require.Contains(t, content, "ual.balance_after::double precision")
	require.NotContains(t, content, "parseAffiliateRebateAmount")
	require.NotContains(t, content, `"current_balance": "u.balance"`)
}

// TestAffiliateRebateRecordsQueryKeepsNonOrderAccruals 锁定返利记录列出全部
// accrue 流水：订单与被邀请人均为 LEFT JOIN，且不按 source_order_id 过滤，
// 兑换码、管理员充值来源的返利才会出现在明细里。
func TestAffiliateRebateRecordsQueryKeepsNonOrderAccruals(t *testing.T) {
	source, err := os.ReadFile("affiliate_repo.go")
	require.NoError(t, err)
	content := string(source)

	require.Contains(t, content, "LEFT JOIN payment_orders po ON po.id = ual.source_order_id")
	require.Contains(t, content, "LEFT JOIN users invitee ON invitee.id = ual.source_user_id")
	require.NotContains(t, content, "\nJOIN payment_orders po ON po.id = ual.source_order_id")
	require.NotContains(t, content, "\nJOIN users invitee ON invitee.id = ual.source_user_id")
	require.NotContains(t, content, "AND ual.source_order_id IS NOT NULL")
}

// TestAffiliateTransferRecordsQueryIncludesOfflineWithdrawals 锁定提取记录同时
// 列出转入余额与线下提现两类额度流出。
func TestAffiliateTransferRecordsQueryIncludesOfflineWithdrawals(t *testing.T) {
	source, err := os.ReadFile("affiliate_repo.go")
	require.NoError(t, err)
	content := string(source)

	require.Contains(t, content, "WHERE ual.action IN ('transfer', 'withdraw')")
}

// TestAffiliateWithdrawClaimsOperationBeforeDeducting 锁定线下提现的幂等形态：
// 同一事务内先按 operation_id 唯一约束写入占位流水，冲突时不扣减，
// 且唯一约束由迁移建立。
func TestAffiliateWithdrawClaimsOperationBeforeDeducting(t *testing.T) {
	source, err := os.ReadFile("affiliate_repo.go")
	require.NoError(t, err)
	content := string(source)

	require.Contains(t, content, "ON CONFLICT (operation_id) WHERE operation_id IS NOT NULL DO NOTHING")
	withdraw := content[strings.Index(content, "func (r *affiliateRepository) WithdrawQuota("):]
	withdraw = withdraw[:strings.Index(withdraw, "\n}\n")]
	claimAt := strings.Index(withdraw, "claimAffiliateWithdrawLedger(")
	deductAt := strings.Index(withdraw, "SET aff_quota = aff_quota - $1")
	require.Positive(t, claimAt)
	require.Greater(t, deductAt, claimAt, "operation claim must precede the quota deduction")

	migration, err := os.ReadFile("../../migrations/240_affiliate_ledger_operation_id.sql")
	require.NoError(t, err)
	require.Contains(t, string(migration), "CREATE UNIQUE INDEX IF NOT EXISTS idx_user_affiliate_ledger_operation_id")
	require.Contains(t, string(migration), "WHERE operation_id IS NOT NULL")
}
