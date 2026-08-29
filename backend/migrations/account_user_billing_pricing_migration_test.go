package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountUserBillingPricingMigration(t *testing.T) {
	content, err := FS.ReadFile("232_add_account_user_billing_pricing.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS user_billing_rate_multiplier DECIMAL(12,6)")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS user_billing_model_pricing JSONB")
	require.Contains(t, sql, "conname = 'accounts_user_billing_rate_multiplier_positive' AND conrelid = 'accounts'::regclass")
	require.Contains(t, sql, "CHECK (user_billing_rate_multiplier IS NULL OR user_billing_rate_multiplier > 0)")
}
