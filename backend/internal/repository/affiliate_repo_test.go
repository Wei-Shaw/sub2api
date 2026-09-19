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
