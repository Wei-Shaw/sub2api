package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration202ExpandsWeb3DepositTokenAmountForUint256(t *testing.T) {
	content, err := FS.ReadFile("202_web3_deposit_token_amount_uint256.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "ALTER COLUMN token_amount TYPE NUMERIC(78,6)")
	require.Contains(t, sql, "USING token_amount::NUMERIC(78,6)")
}
