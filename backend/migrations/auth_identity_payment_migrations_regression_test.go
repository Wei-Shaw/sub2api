package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration112UsesIdempotentAddColumn(t *testing.T) {
	content, err := FS.ReadFile("112_add_payment_order_provider_key_snapshot.sql")
REDACTED

	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS provider_key VARCHAR(30)")
	require.NotContains(t, sql, "ADD COLUMN provider_key VARCHAR(30);")
REDACTED

func TestMigration118DoesNotForceOverwriteAuthSourceGrantDefaults(t *testing.T) {
	content, err := FS.ReadFile("118_wechat_dual_mode_and_auth_source_defaults.sql")
REDACTED

	sql := string(content)
	require.NotContains(t, sql, "UPDATE settings")
	require.NotContains(t, sql, "SET value = 'false'")
	require.True(t, strings.Contains(sql, "ON CONFLICT (key) DO NOTHING"))
REDACTED

func TestMigration119EnforcesOutTradeNoPartialUniqueIndex(t *testing.T) {
	content, err := FS.ReadFile("119_enforce_payment_orders_out_trade_no_unique.sql")
REDACTED

	sql := string(content)
	require.Contains(t, sql, "DROP INDEX IF EXISTS paymentorder_out_trade_no")
	require.Contains(t, sql, "CREATE UNIQUE INDEX IF NOT EXISTS paymentorder_out_trade_no")
	require.Contains(t, sql, "WHERE out_trade_no <> ''")
REDACTED
