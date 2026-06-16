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

	require.Contains(t, content, "JOIN payment_orders po ON po.id = ual.source_order_id")
	require.Contains(t, content, "ual.amount::double precision")
	require.Contains(t, content, "ual.balance_after::double precision")
	require.NotContains(t, content, "parseAffiliateRebateAmount")
	require.NotContains(t, content, `"current_balance": "u.balance"`)
}

func TestCountPaidInviteesSQLCountsQualifiedInvitees(t *testing.T) {
	query := strings.Join(strings.Fields(countPaidInviteesSQL), " ")

	require.Contains(t, query, "payment_orders")
	require.Contains(t, query, "redeem_codes")
	require.Contains(t, query, "COUNT(DISTINCT ua.user_id)")
	require.Contains(t, query, "po.status IN ('PAID', 'RECHARGING', 'COMPLETED')")
	require.Contains(t, query, "po.order_type IN ('balance', 'subscription')")
	require.Contains(t, query, "rc.type = 'subscription'")
	require.Contains(t, query, "rc.status = 'used'")
	require.Contains(t, query, "rc.value > 0")
}
