//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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
		SettingCurrencyCapabilities:     `{"methods":{"sepay":["VND"]}}`,
	}}
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSepay).
		SetName("Sepay").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeSepay).
		SetEnabled(true).
		Save(context.Background())
	require.NoError(t, err)
	configSvc := NewPaymentConfigService(client, repo, []byte("0123456789abcdef0123456789abcdef"))
	svc := NewPaymentService(nil, nil, nil, nil, nil, configSvc, nil, nil, nil)

	_, err = svc.CreatePaymentQuote(context.Background(), CreatePaymentQuoteRequest{
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

func TestPaymentQuoteDefaultsMissingSingleMethodCurrency(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingPaymentEnabled:           "true",
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,VND,CNY",
		SettingManualFXRates:            `{"USD":1,"VND":0.000039215686,"CNY":0.139}`,
		SettingCurrencyCapabilities:     `{"methods":{"sepay":["VND"]}}`,
		SettingMinRechargeAmount:        "1",
		SettingMaxRechargeAmount:        "1000",
	}}
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSepay).
		SetName("Sepay").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeSepay).
		SetEnabled(true).
		Save(context.Background())
	require.NoError(t, err)
	configSvc := NewPaymentConfigService(client, repo, []byte("0123456789abcdef0123456789abcdef"))
	svc := NewPaymentService(nil, nil, nil, nil, nil, configSvc, nil, nil, nil)

	quote, err := svc.CreatePaymentQuote(context.Background(), CreatePaymentQuoteRequest{
		UserID:      42,
		Amount:      200000,
		AmountMode:  PaymentAmountModePayment,
		PaymentType: payment.TypeSepay,
		OrderType:   payment.OrderTypeBalance,
	})
	require.NoError(t, err)
	require.Equal(t, "VND", quote.PaymentCurrency)
	require.Equal(t, 200000.0, quote.PaymentAmount)
	require.InDelta(t, 7.84, quote.LedgerAmount, 0.01)
}

func TestPaymentQuoteRequiresCurrencyForMultiCurrencyMethod(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingPaymentEnabled:           "true",
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,KRW",
		SettingManualFXRates:            `{"USD":1,"KRW":0.00073}`,
		SettingMinRechargeAmount:        "1",
		SettingMaxRechargeAmount:        "1000",
	}}
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypePaddle).
		SetName("Paddle KRW/USD").
		SetConfig(`{"allowed_payment_currencies":"KRW,USD"}`).
		SetSupportedTypes(payment.TypePaddle).
		SetEnabled(true).
		Save(context.Background())
	require.NoError(t, err)
	configSvc := NewPaymentConfigService(client, repo, []byte("0123456789abcdef0123456789abcdef"))
	svc := NewPaymentService(nil, nil, nil, nil, nil, configSvc, nil, nil, nil)

	_, err = svc.CreatePaymentQuote(context.Background(), CreatePaymentQuoteRequest{
		UserID:      42,
		Amount:      10000,
		AmountMode:  PaymentAmountModePayment,
		PaymentType: payment.TypePaddle,
		OrderType:   payment.OrderTypeBalance,
	})
	require.Error(t, err)
	var appErr *infraerrors.ApplicationError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, "PAYMENT_CURRENCY_REQUIRED", appErr.Reason)
	require.Equal(t, payment.TypePaddle, appErr.Metadata["payment_type"])
	require.Equal(t, "KRW,USD", appErr.Metadata["supported_currencies"])
}

func TestPaymentQuoteSupportsConfiguredProviderCurrency(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingPaymentEnabled:           "true",
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD",
		SettingManualFXRates:            `{"USD":1,"KRW":0.00073}`,
		SettingCurrencyCapabilities:     `{"providers":{"paddle":["KRW","USD"]}}`,
		SettingMinRechargeAmount:        "1",
		SettingMaxRechargeAmount:        "1000",
	}}
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypePaddle).
		SetName("Paddle KRW/USD").
		SetConfig("{}").
		SetSupportedTypes(payment.TypePaddle).
		SetEnabled(true).
		Save(context.Background())
	require.NoError(t, err)
	configSvc := NewPaymentConfigService(client, repo, []byte("0123456789abcdef0123456789abcdef"))
	svc := NewPaymentService(nil, nil, nil, nil, nil, configSvc, nil, nil, nil)

	quote, err := svc.CreatePaymentQuote(context.Background(), CreatePaymentQuoteRequest{
		UserID:          42,
		Amount:          10000,
		AmountMode:      PaymentAmountModePayment,
		PaymentCurrency: "KRW",
		PaymentType:     payment.TypePaddle,
		OrderType:       payment.OrderTypeBalance,
	})
	require.NoError(t, err)
	require.Equal(t, "KRW", quote.PaymentCurrency)
	require.Equal(t, 10000.0, quote.PaymentAmount)
	require.Equal(t, "USD", quote.LedgerCurrency)
	require.InDelta(t, 7.30, quote.LedgerAmount, 0.01)
}

func TestPaymentQuoteReturnsBadRequestWhenFXRateMissing(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingPaymentEnabled:           "true",
		SettingLedgerCurrency:           "USD",
		SettingAllowedPaymentCurrencies: "USD,VND",
		SettingManualFXRates:            `{"USD":1}`,
	}}
	configSvc := NewPaymentConfigService(nil, repo, []byte("0123456789abcdef0123456789abcdef"))
	svc := NewPaymentService(nil, nil, nil, nil, nil, configSvc, nil, nil, nil)

	_, err := svc.CreatePaymentQuote(context.Background(), CreatePaymentQuoteRequest{
		UserID:          42,
		Amount:          200000,
		AmountMode:      PaymentAmountModePayment,
		PaymentCurrency: "VND",
		PaymentType:     payment.TypeSepay,
		OrderType:       payment.OrderTypeBalance,
	})
	require.Error(t, err)
	var appErr *infraerrors.ApplicationError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, "FX_RATE_MISSING", appErr.Reason)
	require.Equal(t, "VND", appErr.Metadata["payment_currency"])
	require.Equal(t, "USD", appErr.Metadata["ledger_currency"])
}
