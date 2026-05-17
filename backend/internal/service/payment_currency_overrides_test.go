package service

import (
	"math"
	"strings"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestNormalizeCurrencyOverrides(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]float64
		want  int
	}{
		{"nil input", nil, 0},
		{"empty input", map[string]float64{}, 0},
		{"valid entries", map[string]float64{"vnd": 190000, "cny": 50}, 2},
		{"removes negative", map[string]float64{"VND": -1}, 0},
		{"removes NaN", map[string]float64{"VND": math.NaN()}, 0},
		{"removes Inf", map[string]float64{"VND": math.Inf(1)}, 0},
		{"removes short key", map[string]float64{"VN": 100}, 0},
		{"removes long key", map[string]float64{"VNDD": 100}, 0},
		{"uppercases keys", map[string]float64{"vnd": 190000}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeCurrencyOverrides(tt.input)
			if tt.want == 0 {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if len(got) != tt.want {
				t.Fatalf("len = %d, want %d", len(got), tt.want)
			}
			for k := range got {
				if k != strings.ToUpper(k) {
					t.Fatalf("key %q not uppercased", k)
				}
			}
		})
	}
}

func TestResolveCurrencyOverride(t *testing.T) {
	overrides := map[string]float64{"VND": 190000, "CNY": 50}

	amount, ok := resolveCurrencyOverride(overrides, "VND")
	if !ok || amount != 190000 {
		t.Fatalf("VND: got %v/%v, want 190000/true", amount, ok)
	}

	amount, ok = resolveCurrencyOverride(overrides, "cny")
	if !ok || amount != 50 {
		t.Fatalf("cny: got %v/%v, want 50/true", amount, ok)
	}

	amount, ok = resolveCurrencyOverride(overrides, "KRW")
	if ok {
		t.Fatalf("KRW: got %v/%v, want 0/false", amount, ok)
	}

	amount, ok = resolveCurrencyOverride(nil, "VND")
	if ok {
		t.Fatalf("nil map: got %v/%v, want 0/false", amount, ok)
	}
}

func TestComputeCreateOrderAmountsSubscriptionWithCurrencyOverride(t *testing.T) {
	cfg := paymentCurrencyTestConfig()
	req := CreateOrderRequest{
		PaymentCurrency: "VND",
		OrderType:       payment.OrderTypeSubscription,
	}
	plan := &dbent.SubscriptionPlan{
		Price:             10,
		CurrencyOverrides: map[string]float64{"VND": 190000},
	}

	got, err := computeCreateOrderAmounts(req, cfg, plan, fixedFXTestTime())
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !paymentOrderCurrencyAmountEqual(got.LedgerAmount, 10.00) {
		t.Fatalf("LedgerAmount = %v, want 10.00", got.LedgerAmount)
	}
	if got.PaymentAmount != 190000 {
		t.Fatalf("PaymentAmount = %v, want 190000 (override)", got.PaymentAmount)
	}
}

func TestComputeCreateOrderAmountsSubscriptionOverrideFallbackToFX(t *testing.T) {
	cfg := paymentCurrencyTestConfig()
	req := CreateOrderRequest{
		PaymentCurrency: "VND",
		OrderType:       payment.OrderTypeSubscription,
	}
	plan := &dbent.SubscriptionPlan{
		Price:             10,
		CurrencyOverrides: map[string]float64{"CNY": 50},
	}

	got, err := computeCreateOrderAmounts(req, cfg, plan, fixedFXTestTime())
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got.PaymentAmount != 255000 {
		t.Fatalf("PaymentAmount = %v, want 255000 (FX fallback)", got.PaymentAmount)
	}
}

func TestComputeCreateOrderAmountsBalancePackageWithCurrencyOverride(t *testing.T) {
	cfg := paymentCurrencyTestConfig()
	cfg.BalancePackages = []BalanceRechargePackage{
		{
			ID:                "pkg-vnd",
			AmountLedger:      10,
			CurrencyOverrides: map[string]float64{"VND": 190000},
		},
	}
	req := CreateOrderRequest{
		PaymentCurrency:  "VND",
		OrderType:        payment.OrderTypeBalance,
		BalancePackageID: "pkg-vnd",
	}

	got, err := computeCreateOrderAmounts(req, cfg, nil, fixedFXTestTime())
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !paymentOrderCurrencyAmountEqual(got.LedgerAmount, 10.00) {
		t.Fatalf("LedgerAmount = %v, want 10.00", got.LedgerAmount)
	}
	if got.PaymentAmount != 190000 {
		t.Fatalf("PaymentAmount = %v, want 190000 (override)", got.PaymentAmount)
	}
}
