package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func normalizedMigrationSQL(t *testing.T, name string) string {
	t.Helper()
	content, err := FS.ReadFile(name)
	require.NoError(t, err)
	return strings.Join(strings.Fields(string(content)), " ")
}

func TestAffiliateRebateSourcesMigrationAvoidsHistoricalGuessingAndBlockingIndexes(t *testing.T) {
	coreSQL := normalizedMigrationSQL(t, "231_affiliate_rebate_sources.sql")
	require.Contains(t, coreSQL, "ADD COLUMN IF NOT EXISTS source_type VARCHAR(32) NULL")
	require.Contains(t, coreSQL, "ADD COLUMN IF NOT EXISTS base_amount DECIMAL(20,8) NULL")
	require.Contains(t, coreSQL, "ADD COLUMN IF NOT EXISTS source_redeem_code_id BIGINT NULL")
	require.NotContains(t, strings.ToUpper(coreSQL), "UPDATE USER_AFFILIATE_LEDGER")
	require.NotContains(t, strings.ToUpper(coreSQL), "CREATE INDEX")
	require.NotContains(t, coreSQL, "INTERVAL '10 minutes'")

	indexSQL := normalizedMigrationSQL(t, "231_affiliate_rebate_sources_indexes_notx.sql")
	require.Contains(t, indexSQL, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_affiliate_ledger_source_type_created_at")
	require.Contains(t, indexSQL, "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_user_affiliate_ledger_accrue_order_uniq")
	require.Contains(t, indexSQL, "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_user_affiliate_ledger_accrue_redeem_code_uniq")

	constraintSQL := normalizedMigrationSQL(t, "232_affiliate_rebate_source_constraints.sql")
	require.Contains(t, constraintSQL, "checksum = 'ceb508efbf81877a891a95fe6688cb3287462c2552e1a5c8a8254be9328d6806'")
	require.Contains(t, constraintSQL, "source_type = 'admin_recharge'")
	require.Contains(t, constraintSQL, "source_type = 'legacy_unknown'")
	require.Contains(t, constraintSQL, "FOREIGN KEY (source_order_id) REFERENCES payment_orders(id) ON DELETE RESTRICT NOT VALID")
	require.Contains(t, constraintSQL, "FOREIGN KEY (source_redeem_code_id) REFERENCES redeem_codes(id) ON DELETE RESTRICT NOT VALID")
	require.Contains(t, constraintSQL, "source_type IN ('balance_redeem_code', 'admin_recharge')")
	require.Contains(t, constraintSQL, "source_redeem_code_id IS NOT NULL")
	require.Contains(t, constraintSQL, ") NOT VALID")
	require.NotContains(t, constraintSQL, "INTERVAL '10 minutes'")
	require.NotContains(t, strings.ToUpper(constraintSQL), "DELETE FROM USER_AFFILIATE_LEDGER")
}
