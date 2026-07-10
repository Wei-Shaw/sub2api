package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMoneyUSDConstructionAndTenDecimalRoundTrip(t *testing.T) {
	money, err := NewMoney("12.1234567890", CurrencyUSD)
	require.NoError(t, err)
	require.Equal(t, CurrencyUSD, money.Currency())
	require.Equal(t, "12.1234567890", money.String())
	require.Equal(t, "12.1234567890", money.Decimal().StringFixed(10))
}

func TestMoneyZeroDecimalUsesLedgerScale(t *testing.T) {
	money, err := NewMoney("42", CurrencyUSD)
	require.NoError(t, err)
	require.Equal(t, "42.0000000000", money.String())
}

func TestMoneyRejectsDifferentCurrencies(t *testing.T) {
	usd := MustUSD("1.0000000000")
	eur, err := NewMoney("1.0000000000", Currency("EUR"))
	require.NoError(t, err)

	_, err = usd.Add(eur)
	require.ErrorContains(t, err, "currency mismatch")

	_, err = usd.Compare(eur)
	require.ErrorContains(t, err, "currency mismatch")
}

func TestMoneyRejectsNegativeReservationInput(t *testing.T) {
	_, err := NewMoney("-0.0000000001", CurrencyUSD)
	require.ErrorContains(t, err, "must not be negative")
}

func TestMoneyArithmeticCompareAndNegativeResult(t *testing.T) {
	left := MustUSD("1.2500000000")
	right := MustUSD("2.5000000000")

	sum, err := left.Add(right)
	require.NoError(t, err)
	require.Equal(t, "3.7500000000", sum.String())

	difference, err := left.Subtract(right)
	require.NoError(t, err)
	require.True(t, difference.IsNegative())
	require.Equal(t, "-1.2500000000", difference.String())

	comparison, err := left.Compare(right)
	require.NoError(t, err)
	require.Equal(t, -1, comparison)
}

func TestMoneyAddRejectsNumericIntegerOverflow(t *testing.T) {
	maximum := MustUSD("9999999999.9999999999")
	smallestUnit := MustUSD("0.0000000001")

	_, err := maximum.Add(smallestUnit)
	require.ErrorContains(t, err, "NUMERIC(20,10)")
}

func TestMoneySubtractRejectsNegativeNumericIntegerOverflow(t *testing.T) {
	zero := MustUSD("0")
	maximum := MustUSD("9999999999.9999999999")
	smallestUnit := MustUSD("0.0000000001")

	negativeMaximum, err := zero.Subtract(maximum)
	require.NoError(t, err)
	require.True(t, negativeMaximum.IsNegative())

	_, err = negativeMaximum.Subtract(smallestUnit)
	require.ErrorContains(t, err, "NUMERIC(20,10)")
}

func TestMoneyJSONAndDatabaseStringsStayDecimal(t *testing.T) {
	money := MustUSD("9876543210.1234567890")

	data, err := json.Marshal(money)
	require.NoError(t, err)
	require.JSONEq(t, `{"amount":"9876543210.1234567890","currency":"USD"}`, string(data))
	require.Equal(t, "9876543210.1234567890", money.String(), "database argument must remain a decimal string")

	var decoded Money
	require.NoError(t, json.Unmarshal(data, &decoded))
	comparison, err := money.Compare(decoded)
	require.NoError(t, err)
	require.Zero(t, comparison)
}

func TestMoneyRejectsInvalidProductionInput(t *testing.T) {
	tests := []struct {
		name     string
		amount   string
		currency Currency
	}{
		{name: "invalid decimal", amount: "not-a-number", currency: CurrencyUSD},
		{name: "too many decimal places", amount: "1.00000000001", currency: CurrencyUSD},
		{name: "invalid currency", amount: "1.0000000000", currency: Currency("usd")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMoney(tt.amount, tt.currency)
			require.Error(t, err)
		})
	}
}
