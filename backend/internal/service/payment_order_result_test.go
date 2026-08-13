package service

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func TestCalculateCreateOrderPayAmountUsesCurrencyPrecision(t *testing.T) {
	t.Parallel()

	amountStr, amount, err := calculateCreateOrderPayAmount(100, 2.5, "JPY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if amountStr != "103" || amount != 103 {
		t.Fatalf("JPY pay amount = (%q, %v), want (103, 103)", amountStr, amount)
	}

	amountStr, amount, err = calculateCreateOrderPayAmount(12.345, 1, "KWD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if amountStr != "12.469" || amount != 12.469 {
		t.Fatalf("KWD pay amount = (%q, %v), want (12.469, 12.469)", amountStr, amount)
	}
}

func TestCalculateCreateOrderPayAmountForSubscriptionConvertsVNDPriceWhenRateConfigured(t *testing.T) {
	t.Parallel()

	amountStr, amount, err := calculateCreateOrderPayAmountForOrderType(9.99, 0, "VND", payment.OrderTypeSubscription, 26000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if amountStr != "259740" || amount != 259740 {
		t.Fatalf("subscription VND pay amount = (%q, %v), want (259740, 259740)", amountStr, amount)
	}
}

func TestCalculateCreateOrderPayAmountForSubscriptionAppliesFeeAfterVNDConversion(t *testing.T) {
	t.Parallel()

	amountStr, amount, err := calculateCreateOrderPayAmountForOrderType(9.99, 2.5, "VND", payment.OrderTypeSubscription, 26000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2.5% of 259,740 is 6,493.5, rounded up to a whole dong.
	if amountStr != "266234" || amount != 266234 {
		t.Fatalf("subscription VND pay amount with fee = (%q, %v), want (266234, 266234)", amountStr, amount)
	}
}

func TestCalculateCreateOrderPayAmountForSubscriptionKeepsNonVNDPrice(t *testing.T) {
	t.Parallel()

	amountStr, amount, err := calculateCreateOrderPayAmountForOrderType(9.99, 0, "USD", payment.OrderTypeSubscription, 26000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if amountStr != "9.99" || amount != 9.99 {
		t.Fatalf("subscription USD pay amount = (%q, %v), want (9.99, 9.99)", amountStr, amount)
	}
}

// Conversion is opt-in: with no rate configured a dong channel charges the plan
// price as-is. Guessing a rate would be worse than charging the wrong currency
// visibly, so this pins the do-nothing branch.
func TestCalculateCreateOrderPayAmountForSubscriptionKeepsDirectPriceWhenRateDisabled(t *testing.T) {
	t.Parallel()

	amountStr, amount, err := calculateCreateOrderPayAmountForOrderType(10, 0, "VND", payment.OrderTypeSubscription, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if amountStr != "10" || amount != 10 {
		t.Fatalf("subscription VND pay amount without rate = (%q, %v), want (10, 10)", amountStr, amount)
	}
}

// 汇率只作用于订阅订单，余额充值订单不受影响。
func TestCalculateCreateOrderPayAmountForBalanceIgnoresSubscriptionRate(t *testing.T) {
	t.Parallel()

	amountStr, amount, err := calculateCreateOrderPayAmountForOrderType(50, 0, "CNY", payment.OrderTypeBalance, 7.15)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if amountStr != "50.00" || amount != 50 {
		t.Fatalf("balance CNY pay amount = (%q, %v), want (50.00, 50)", amountStr, amount)
	}
}

func TestCalculateCreditedBalanceStillUsesRechargeMultiplier(t *testing.T) {
	t.Parallel()

	got := calculateCreditedBalance(10, 0.14)
	if got != 1.4 {
		t.Fatalf("credited balance = %v, want 1.4", got)
	}

	got = calculateCreditedBalance(5, 10)
	if got != 50 {
		t.Fatalf("credited balance = %v, want 50", got)
	}
}

func TestCalculateCreateOrderPayAmountRejectsFractionalZeroDecimal(t *testing.T) {
	t.Parallel()

	_, _, err := calculateCreateOrderPayAmount(100.5, 0, "JPY")
	if err == nil {
		t.Fatal("expected fractional JPY amount to fail")
	}
	if appErr := infraerrors.FromError(err); appErr.Reason != "INVALID_AMOUNT" {
		t.Fatalf("reason = %q, want INVALID_AMOUNT", appErr.Reason)
	}
}

func TestComputeValidityDaysSupportsSingularAndPluralUnits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		days int
		unit string
		want int
	}{
		{name: "days", days: 1, unit: "days", want: 1},
		{name: "week", days: 1, unit: "week", want: 7},
		{name: "weeks", days: 2, unit: "weeks", want: 14},
		{name: "month", days: 1, unit: "month", want: 30},
		{name: "months", days: 1, unit: "months", want: 30},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := psComputeValidityDays(tt.days, tt.unit); got != tt.want {
				t.Fatalf("psComputeValidityDays(%d, %q) = %d, want %d", tt.days, tt.unit, got, tt.want)
			}
		})
	}
}

func TestBuildPaymentSubjectAppliesAffixToSubscriptionPlanProductName(t *testing.T) {
	t.Parallel()

	svc := &PaymentService{}
	cfg := &PaymentConfig{
		ProductNamePrefix: "PRE",
		ProductNameSuffix: "SUF",
	}
	plan := &dbent.SubscriptionPlan{
		Name:        "Pro Monthly",
		ProductName: "Claude Pro",
	}

	got := svc.buildPaymentSubject(plan, 0, cfg, nil)
	if got != "PRE Claude Pro SUF" {
		t.Fatalf("buildPaymentSubject() = %q, want %q", got, "PRE Claude Pro SUF")
	}
}

func TestBuildPaymentSubjectAppliesAffixToSubscriptionPlanDefaultName(t *testing.T) {
	t.Parallel()

	svc := &PaymentService{}
	cfg := &PaymentConfig{
		ProductNamePrefix: "PRE",
		ProductNameSuffix: "SUF",
	}
	plan := &dbent.SubscriptionPlan{Name: "Team Monthly"}

	got := svc.buildPaymentSubject(plan, 0, cfg, nil)
	if got != "PRE Sub2API Subscription Team Monthly SUF" {
		t.Fatalf("buildPaymentSubject() = %q, want %q", got, "PRE Sub2API Subscription Team Monthly SUF")
	}
}
