package service

import (
	"math"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestComputeCreateOrderAmountsPaymentModeVNDTopUp(t *testing.T) {
	cfg := paymentCurrencyTestConfig()
	req := CreateOrderRequest{
		Amount:          255000,
		AmountMode:      PaymentAmountModePayment,
		PaymentCurrency: "VND",
		OrderType:       payment.OrderTypeBalance,
	}

	got, err := computeCreateOrderAmounts(req, cfg, nil, fixedFXTestTime())
	if err != nil {
		t.Fatalf("computeCreateOrderAmounts returned error: %v", err)
	}
	if !paymentOrderCurrencyAmountEqual(got.LedgerAmount, 10.00) {
		t.Fatalf("LedgerAmount = %v, want 10.00", got.LedgerAmount)
	}
	if got.PaymentAmount != 255000 {
		t.Fatalf("PaymentAmount = %v, want 255000", got.PaymentAmount)
	}
	if got.FXSnapshot.PaymentCurrency != "VND" || got.FXSnapshot.LedgerCurrency != "USD" {
		t.Fatalf("snapshot currencies = %s/%s, want VND/USD", got.FXSnapshot.PaymentCurrency, got.FXSnapshot.LedgerCurrency)
	}
	if got.FXSnapshot.Source != fxSourceManual {
		t.Fatalf("snapshot source = %q, want %q", got.FXSnapshot.Source, fxSourceManual)
	}
}

func TestComputeCreateOrderAmountsLegacyUSDTopUp(t *testing.T) {
	cfg := paymentCurrencyTestConfig()
	req := CreateOrderRequest{
		Amount:    10,
		OrderType: payment.OrderTypeBalance,
	}

	got, err := computeCreateOrderAmounts(req, cfg, nil, fixedFXTestTime())
	if err != nil {
		t.Fatalf("computeCreateOrderAmounts returned error: %v", err)
	}
	if !paymentOrderCurrencyAmountEqual(got.LedgerAmount, 10.00) {
		t.Fatalf("LedgerAmount = %v, want 10.00", got.LedgerAmount)
	}
	if !paymentOrderCurrencyAmountEqual(got.PaymentAmount, 10.00) {
		t.Fatalf("PaymentAmount = %v, want 10.00", got.PaymentAmount)
	}
	if got.FXSnapshot.PaymentCurrency != "USD" || got.FXSnapshot.Source != fxSourceIdentity {
		t.Fatalf("snapshot = %+v, want USD identity", got.FXSnapshot)
	}
}

func TestComputeCreateOrderAmountsSubscriptionConvertsPlanPriceToPaymentCurrency(t *testing.T) {
	cfg := paymentCurrencyTestConfig()
	req := CreateOrderRequest{
		PaymentCurrency: "VND",
		OrderType:       payment.OrderTypeSubscription,
	}
	plan := &dbent.SubscriptionPlan{Price: 10}

	got, err := computeCreateOrderAmounts(req, cfg, plan, fixedFXTestTime())
	if err != nil {
		t.Fatalf("computeCreateOrderAmounts returned error: %v", err)
	}
	if !paymentOrderCurrencyAmountEqual(got.LedgerAmount, 10.00) {
		t.Fatalf("LedgerAmount = %v, want 10.00", got.LedgerAmount)
	}
	if got.PaymentAmount != 255000 {
		t.Fatalf("PaymentAmount = %v, want 255000", got.PaymentAmount)
	}
}

func TestComputeCreateOrderAmountsMissingFXRateReturnsError(t *testing.T) {
	cfg := paymentCurrencyTestConfig()
	cfg.AllowedPaymentCurrencies = []string{"USD", "KRW"}
	req := CreateOrderRequest{
		Amount:          10000,
		AmountMode:      PaymentAmountModePayment,
		PaymentCurrency: "KRW",
		OrderType:       payment.OrderTypeBalance,
	}

	_, err := computeCreateOrderAmounts(req, cfg, nil, fixedFXTestTime())
	if err == nil {
		t.Fatal("expected missing FX rate error")
	}
	if !strings.Contains(err.Error(), "manual fx rate not found for KRW") {
		t.Fatalf("error = %v, want missing KRW fx rate", err)
	}
}

func TestValidateLedgerAmountLimitsUsesConvertedLedgerAmount(t *testing.T) {
	cfg := paymentCurrencyTestConfig()
	cfg.MinAmount = 5
	cfg.MaxAmount = 20

	if err := validateLedgerAmountLimits(payment.OrderTypeBalance, 10, cfg); err != nil {
		t.Fatalf("expected converted 10 USD to pass limits: %v", err)
	}
	if err := validateLedgerAmountLimits(payment.OrderTypeBalance, 4.99, cfg); err == nil {
		t.Fatal("expected converted amount below min to fail")
	}
	if err := validateLedgerAmountLimits(payment.OrderTypeBalance, 20.01, cfg); err == nil {
		t.Fatal("expected converted amount above max to fail")
	}
}

func TestComputeCreateOrderAmountsInvalidAmountModeIsRejectedForAllOrderTypes(t *testing.T) {
	cfg := paymentCurrencyTestConfig()

	_, err := computeCreateOrderAmounts(CreateOrderRequest{
		Amount:     10,
		AmountMode: "local",
		OrderType:  payment.OrderTypeBalance,
	}, cfg, nil, fixedFXTestTime())
	if err == nil {
		t.Fatal("expected invalid balance amount_mode to fail")
	}

	_, err = computeCreateOrderAmounts(CreateOrderRequest{
		AmountMode: "local",
		OrderType:  payment.OrderTypeSubscription,
	}, cfg, &dbent.SubscriptionPlan{Price: 10}, fixedFXTestTime())
	if err == nil {
		t.Fatal("expected invalid subscription amount_mode to fail")
	}
}

func TestFormatAmountLimitKeepsZeroReadable(t *testing.T) {
	if got := formatAmountLimit(0); got != "0" {
		t.Fatalf("formatAmountLimit(0) = %q, want 0", got)
	}
}

func paymentOrderCurrencyAmountEqual(got, want float64) bool {
	return math.Abs(got-want) < 1e-9
}

func paymentCurrencyTestConfig() *PaymentConfig {
	return &PaymentConfig{
		LedgerCurrency:            "USD",
		AllowedPaymentCurrencies:  []string{"USD", "VND"},
		ManualFXRates:             map[string]float64{"USD": 1, "VND": 10.0 / 255000.0},
		BalanceRechargeMultiplier: 1,
	}
}

func fixedFXTestTime() time.Time {
	return time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC)
}
