package web3deposit

import (
	"errors"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertUSDT0AmountExactValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rawAmount string
		want      string
	}{
		{name: "smallest unit", rawAmount: "1", want: "0.000001"},
		{name: "one token", rawAmount: "1000000", want: "1.000000"},
		{name: "fractional token", rawAmount: "1234567", want: "1.234567"},
		{name: "maximum platform amount", rawAmount: "999999999999999999", want: "999999999999.999999"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			rawAmount, ok := new(big.Int).SetString(test.rawAmount, 10)
			require.True(t, ok)

			amounts, err := ConvertUSDT0Amount(rawAmount)
			require.NoError(t, err)
			require.Equal(t, test.want, amounts.TokenAmount.StringFixed(USDT0Decimals))
			require.Equal(t, test.want, amounts.CreditedAmount.StringFixed(USDT0Decimals))
			require.True(t, amounts.TokenAmount.Equal(amounts.CreditedAmount))
		})
	}
}

func TestConvertUSDT0AmountRejectsInvalidRawAmounts(t *testing.T) {
	t.Parallel()

	maximumUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	aboveUint256 := new(big.Int).Add(maximumUint256, big.NewInt(1))
	tests := []struct {
		name      string
		rawAmount *big.Int
		wantErr   error
	}{
		{name: "nil", wantErr: ErrRawAmountRequired},
		{name: "negative", rawAmount: big.NewInt(-1), wantErr: ErrRawAmountNegative},
		{name: "zero", rawAmount: new(big.Int), wantErr: ErrRawAmountZero},
		{name: "above uint256", rawAmount: aboveUint256, wantErr: ErrRawAmountExceedsUint256},
		{name: "maximum uint256 overflows platform", rawAmount: maximumUint256, wantErr: ErrCreditedAmountOverflow},
		{name: "one unit above platform range", rawAmount: new(big.Int).SetUint64(1_000_000_000_000_000_000), wantErr: ErrCreditedAmountOverflow},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := ConvertUSDT0Amount(test.rawAmount)
			require.True(t, errors.Is(err, test.wantErr), err)
		})
	}
}

func TestConvertUSDT0AmountDoesNotRetainRawAmount(t *testing.T) {
	t.Parallel()

	rawAmount := big.NewInt(1_234_567)
	amounts, err := ConvertUSDT0Amount(rawAmount)
	require.NoError(t, err)

	rawAmount.SetInt64(1)
	require.Equal(t, "1.234567", amounts.TokenAmount.StringFixed(USDT0Decimals))
	require.Equal(t, "1.234567", amounts.CreditedAmount.StringFixed(USDT0Decimals))
}
