package dto

import (
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
	"github.com/stretchr/testify/require"
)

func TestUserWeb3DepositAmountsMarshalAsStrings(t *testing.T) {
	t.Parallel()

	amounts, err := web3deposit.ConvertUSDT0Amount(big.NewInt(1_234_567))
	require.NoError(t, err)

	payload, err := json.Marshal(NewUserWeb3DepositAmounts(amounts, true))
	require.NoError(t, err)
	require.JSONEq(t, `{
		"token_amount": "1.234567",
		"credited_amount": "1.23456700"
	}`, string(payload))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.IsType(t, "", decoded["token_amount"])
	require.IsType(t, "", decoded["credited_amount"])
}

func TestUserWeb3DepositAmountsMarshalPendingCreditAsNull(t *testing.T) {
	t.Parallel()

	amounts, err := web3deposit.ConvertUSDT0Amount(big.NewInt(1_000_000))
	require.NoError(t, err)

	payload, err := json.Marshal(NewUserWeb3DepositAmounts(amounts, false))
	require.NoError(t, err)
	require.JSONEq(t, `{
		"token_amount": "1.000000",
		"credited_amount": null
	}`, string(payload))
}

func TestAdminWeb3DepositAmountsPreserveRawUint256String(t *testing.T) {
	t.Parallel()

	maximumUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	amounts, err := web3deposit.ConvertUSDT0Amount(big.NewInt(1))
	require.NoError(t, err)
	dtoAmounts, err := NewAdminWeb3DepositAmounts(maximumUint256, amounts, false)
	require.NoError(t, err)

	payload, err := json.Marshal(dtoAmounts)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"raw_amount": "115792089237316195423570985008687907853269984665640564039457584007913129639935",
		"token_amount": "0.000001",
		"credited_amount": null
	}`, string(payload))
	require.NotContains(t, string(payload), "e+")
}

func TestAdminWeb3DepositAmountsRequireRawAmount(t *testing.T) {
	t.Parallel()

	amounts, err := web3deposit.ConvertUSDT0Amount(big.NewInt(1))
	require.NoError(t, err)
	_, err = NewAdminWeb3DepositAmounts(nil, amounts, false)
	require.True(t, errors.Is(err, web3deposit.ErrRawAmountRequired))
}

func TestWeb3DepositAmountsPreserveMaximumPlatformValue(t *testing.T) {
	t.Parallel()

	rawAmount := new(big.Int).SetUint64(999_999_999_999_999_999)
	amounts, err := web3deposit.ConvertUSDT0Amount(rawAmount)
	require.NoError(t, err)
	dtoAmounts, err := NewAdminWeb3DepositAmounts(rawAmount, amounts, true)
	require.NoError(t, err)

	payload, err := json.Marshal(dtoAmounts)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"raw_amount": "999999999999999999",
		"token_amount": "999999999999.999999",
		"credited_amount": "999999999999.99999900"
	}`, string(payload))
}
