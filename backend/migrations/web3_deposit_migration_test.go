package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration196CreatesCompleteWeb3DepositSchema(t *testing.T) {
	content, err := FS.ReadFile("196_web3_deposit.sql")
	require.NoError(t, err)
	sql := string(content)

	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS web3_deposit_wallets",
		"wallet_id VARCHAR(64) NOT NULL UNIQUE",
		"account_path VARCHAR(64) NOT NULL",
		"xpub_fingerprint VARCHAR(64) NOT NULL",
		"next_derivation_index >= 0 AND next_derivation_index <= 2147483648",
		"status IN ('active', 'disabled')",

		"CREATE TABLE IF NOT EXISTS web3_deposit_addresses",
		"user_id BIGINT NOT NULL REFERENCES users(id)",
		"wallet_id VARCHAR(64) NOT NULL REFERENCES web3_deposit_wallets(wallet_id)",
		"UNIQUE (user_id, wallet_id)",
		"UNIQUE (wallet_id, derivation_index)",
		"UNIQUE (normalized_address)",
		"ON web3_deposit_addresses (user_id, created_at DESC)",

		"CREATE TABLE IF NOT EXISTS web3_deposits",
		"deposit_address_id BIGINT NOT NULL REFERENCES web3_deposit_addresses(id)",
		"raw_amount NUMERIC(78,0) NOT NULL",
		"token_amount NUMERIC(78,6) NOT NULL",
		"credited_amount DECIMAL(20,8)",
		"UNIQUE (chain_id, tx_hash, log_index)",
		"ON web3_deposits (status, next_retry_at, id)",
		"ON web3_deposits (user_id, created_at DESC, id DESC)",
		"ON web3_deposits (block_number, id)",
		"ON web3_deposits (deposit_address_id, created_at DESC)",
		"ON web3_deposits (LOWER(tx_hash))",

		"CREATE TABLE IF NOT EXISTS web3_scanner_cursors",
		"scanner_key VARCHAR(128) NOT NULL UNIQUE",
		"UNIQUE (chain_id, token_contract)",
		"last_scanned_block >= scan_start_block",
		"last_finalized_block <= last_scanned_block",
		"ON web3_scanner_cursors (lease_expires_at)",

		"CREATE TABLE IF NOT EXISTS web3_user_balances",
		"UNIQUE (user_id, asset_key)",
		"available_amount = total_deposited - total_transferred",
		"CREATE TABLE IF NOT EXISTS web3_balance_transfers",
		"FOREIGN KEY (web3_balance_id, user_id)",
		"REFERENCES web3_user_balances(id, user_id)",
		"idempotency_key VARCHAR(180) NOT NULL UNIQUE",
		"web3_balance_after = web3_balance_before - amount",
		"user_balance_after = user_balance_before + amount",

		"CREATE TABLE IF NOT EXISTS web3_rescan_jobs",
		"attempt_count INTEGER NOT NULL DEFAULT 0",
		"lease_expires_at TIMESTAMPTZ",
		"'pending', 'running', 'succeeded', 'failed'",

		"INSERT INTO ops_alert_rules",
		"ON CONFLICT (name) DO NOTHING",
		"'web3_rpc_unhealthy'",
		"'web3_scanner_lag_blocks'",
		"'web3_finalizer_lag_blocks'",
		"'web3_manual_review_count'",
		"'web3_credit_failures_total'",
	} {
		require.Contains(t, sql, fragment)
	}

	require.NotContains(t, sql, "next_derivation_index < 2147483648")
	require.NotContains(t, sql, "token_amount NUMERIC(38,18)")
	require.NotContains(t, sql, "FLOAT")
	require.NotContains(t, sql, "DOUBLE")
	require.NotContains(t, sql, "ON DELETE CASCADE")
}
