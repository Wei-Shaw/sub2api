//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestPaymentQuoteLocksFXSnapshotForCreateOrderAmounts(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingPaymentEnabled:           "true",
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,VND,CNY",
		SettingManualFXRates:            `{"USD":1,"VND":0.000039215686,"CNY":0.139}`,
		SettingMinRechargeAmount:        "1",
		SettingMaxRechargeAmount:        "1000",
	}}
	configSvc := NewPaymentConfigService(nil, repo, []byte("0123456789abcdef0123456789abcdef"))
	svc := NewPaymentService(nil, nil, nil, nil, nil, configSvc, nil, nil, nil)

	quote, err := svc.CreatePaymentQuote(context.Background(), CreatePaymentQuoteRequest{
		UserID:          42,
		Amount:          255000,
		AmountMode:      PaymentAmountModePayment,
		PaymentCurrency: "vnd",
		PaymentType:     payment.TypeSepay,
		OrderType:       payment.OrderTypeBalance,
	})
	require.NoError(t, err)
	require.NotEmpty(t, quote.QuoteID)
	require.Equal(t, "VND", quote.PaymentCurrency)
	require.Equal(t, "USD", quote.LedgerCurrency)
	require.Equal(t, 255000.0, quote.PaymentAmount)
	require.InDelta(t, 10.0, quote.LedgerAmount, 0.01)
	require.InDelta(t, 0.000039215686, quote.FXRate, 0.000000000001)
	require.True(t, quote.ExpiresAt.After(time.Now()))

	// Simulate an FX update after the quote was issued. The order creation path
	// must continue using the quote's immutable snapshot instead of the new rate.
	repo.values[SettingManualFXRates] = `{"USD":1,"VND":0.00005,"CNY":0.139}`

	req := CreateOrderRequest{
		UserID:          42,
		Amount:          255000,
		AmountMode:      PaymentAmountModePayment,
		PaymentCurrency: "VND",
		QuoteID:         quote.QuoteID,
		PaymentType:     payment.TypeSepay,
		OrderType:       payment.OrderTypeBalance,
	}
	require.NoError(t, svc.applyPaymentQuoteToCreateOrder(&req))
	cfg, err := configSvc.GetPaymentConfig(context.Background())
	require.NoError(t, err)
	amounts, err := computeCreateOrderAmounts(req, cfg, nil, time.Now())
	require.NoError(t, err)
	require.Equal(t, 255000.0, amounts.PaymentAmount)
	require.InDelta(t, 10.0, amounts.LedgerAmount, 0.01)
	require.InDelta(t, 0.000039215686, amounts.FXSnapshot.RatePaymentToLedger, 0.000000000001)
}

func TestPaymentQuoteRejectsMismatchedUser(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingPaymentEnabled:           "true",
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,VND",
		SettingManualFXRates:            `{"USD":1,"VND":0.000039215686}`,
		SettingMinRechargeAmount:        "1",
		SettingMaxRechargeAmount:        "1000",
	}}
	configSvc := NewPaymentConfigService(nil, repo, []byte("0123456789abcdef0123456789abcdef"))
	svc := NewPaymentService(nil, nil, nil, nil, nil, configSvc, nil, nil, nil)

	quote, err := svc.CreatePaymentQuote(context.Background(), CreatePaymentQuoteRequest{
		UserID:          42,
		Amount:          255000,
		AmountMode:      PaymentAmountModePayment,
		PaymentCurrency: "VND",
		PaymentType:     payment.TypeSepay,
		OrderType:       payment.OrderTypeBalance,
	})
	require.NoError(t, err)

	req := CreateOrderRequest{
		UserID:      7,
		QuoteID:     quote.QuoteID,
		PaymentType: payment.TypeSepay,
	}
	err = svc.applyPaymentQuoteToCreateOrder(&req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "payment quote user mismatch")
}

func TestPaymentQuoteRejectsUnsupportedPaymentCurrency(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingPaymentEnabled:           "true",
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,VND,CNY",
		SettingManualFXRates:            `{"USD":1,"VND":0.000039215686,"CNY":0.139}`,
	}}
	configSvc := NewPaymentConfigService(nil, repo, []byte("0123456789abcdef0123456789abcdef"))
	svc := NewPaymentService(nil, nil, nil, nil, nil, configSvc, nil, nil, nil)

	_, err := svc.CreatePaymentQuote(context.Background(), CreatePaymentQuoteRequest{
		UserID:          42,
		Amount:          10,
		AmountMode:      PaymentAmountModePayment,
		PaymentCurrency: "CNY",
		PaymentType:     payment.TypeSepay,
		OrderType:       payment.OrderTypeBalance,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "payment currency is not supported")
}
