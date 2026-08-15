package service

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestCalculateSubscriptionGatewayBaseAmountVND(t *testing.T) {
	cfg := &PaymentConfig{SubscriptionUSDToVNDRate: 25000}
	cases := []struct {
		name     string
		cfg      *PaymentConfig
		currency string
		amount   float64
		want     float64
	}{
		{"vnd rate applies", cfg, payment.CurrencyVND, 9.9, 247500},
		{"vnd rate zero keeps price", &PaymentConfig{}, payment.CurrencyVND, 9.9, 9.9},
		{"cny unaffected", &PaymentConfig{SubscriptionUSDToCNYRate: 7.2}, payment.DefaultPaymentCurrency, 10, 72},
		{"other currency untouched", cfg, "USD", 9.9, 9.9},
		{"nil cfg safe", nil, payment.CurrencyVND, 9.9, 9.9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := calculateSubscriptionGatewayBaseAmount(tc.amount, tc.cfg, tc.currency); got != tc.want {
				t.Fatalf("= %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCreateOrderPayAmountForOrderTypeVND(t *testing.T) {
	cfg := &PaymentConfig{SubscriptionUSDToVNDRate: 25000}
	str, amt, err := calculateCreateOrderPayAmountForOrderType(9.9, 0, payment.CurrencyVND, payment.OrderTypeSubscription, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if str != "247500" || amt != 247500 {
		t.Fatalf("str=%q amt=%v, want 247500", str, amt)
	}
}

func TestCalculateRechargeCreditedBalanceVND(t *testing.T) {
	cases := []struct {
		name          string
		payAmount     float64
		currency      string
		cfg           *PaymentConfig
		want          float64
		wantErrSubstr string
	}{
		{"vnd divides by rate", 250000, payment.CurrencyVND, &PaymentConfig{SubscriptionUSDToVNDRate: 25000}, 10, ""},
		{"multiplier composes on vnd", 250000, payment.CurrencyVND, &PaymentConfig{SubscriptionUSDToVNDRate: 25000, BalanceRechargeMultiplier: 0.5}, 5, ""},
		{"vnd without rate rejected", 250000, payment.CurrencyVND, &PaymentConfig{}, 0, "RECHARGE_VND_RATE_REQUIRED"},
		{"cny keeps multiplier-only", 100, payment.DefaultPaymentCurrency, &PaymentConfig{BalanceRechargeMultiplier: 0.14}, 14, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := calculateRechargeCreditedBalance(tc.payAmount, tc.currency, tc.cfg)
			if tc.wantErrSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrSubstr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("= %v, want %v", got, tc.want)
			}
		})
	}
}
