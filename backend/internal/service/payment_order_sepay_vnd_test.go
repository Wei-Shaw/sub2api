package service

import (
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
