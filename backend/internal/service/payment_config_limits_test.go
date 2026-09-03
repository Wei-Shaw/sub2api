package service

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestUnionFloat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		agg         float64
		limited     bool
		val         float64
		wantMin     bool
		wantAgg     float64
		wantLimited bool
	}{
		{"first non-zero value", 0, true, 5, true, 5, true},
		{"lower min replaces", 10, true, 3, true, 3, true},
		{"higher min does not replace", 3, true, 10, true, 3, true},
		{"higher max replaces", 10, true, 20, false, 20, true},
		{"lower max does not replace", 20, true, 10, false, 20, true},
		{"zero value makes unlimited", 5, true, 0, true, 5, false},
		{"already unlimited stays unlimited", 5, false, 10, true, 5, false},
		{"zero on first call", 0, true, 0, true, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotAgg, gotLimited := unionFloat(tt.agg, tt.limited, tt.val, tt.wantMin)
			if gotAgg != tt.wantAgg || gotLimited != tt.wantLimited {
				t.Fatalf("unionFloat(%v, %v, %v, %v) = (%v, %v), want (%v, %v)",
					tt.agg, tt.limited, tt.val, tt.wantMin,
					gotAgg, gotLimited, tt.wantAgg, tt.wantLimited)
			}
		})
	}
}

func makeInstance(id int64, providerKey, supportedTypes, limits string) *dbent.PaymentProviderInstance {
	return &dbent.PaymentProviderInstance{
		ID:             id,
		ProviderKey:    providerKey,
		SupportedTypes: supportedTypes,
		Limits:         limits,
		Enabled:        true,
	}
}

func TestPcAggregateMethodLimits(t *testing.T) {
	t.Parallel()

	t.Run("single instance with limits", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, "easypay", "alipay,wxpay",
			`{"alipay":{"singleMin":2,"singleMax":14},"wxpay":{"singleMin":1,"singleMax":12}}`)
		ml := pcAggregateMethodLimits("alipay", []*dbent.PaymentProviderInstance{inst})
		if ml.SingleMin != 2 || ml.SingleMax != 14 {
			t.Fatalf("alipay limits = min:%v max:%v, want min:2 max:14", ml.SingleMin, ml.SingleMax)
		}
	})

	t.Run("two instances union takes widest range", func(t *testing.T) {
		t.Parallel()
		inst1 := makeInstance(1, "easypay", "alipay,wxpay",
			`{"alipay":{"singleMin":5,"singleMax":100}}`)
		inst2 := makeInstance(2, "easypay", "alipay,wxpay",
			`{"alipay":{"singleMin":2,"singleMax":200}}`)
		ml := pcAggregateMethodLimits("alipay", []*dbent.PaymentProviderInstance{inst1, inst2})
		if ml.SingleMin != 2 {
			t.Fatalf("SingleMin = %v, want 2 (lowest floor)", ml.SingleMin)
		}
		if ml.SingleMax != 200 {
			t.Fatalf("SingleMax = %v, want 200 (highest ceiling)", ml.SingleMax)
		}
	})

	t.Run("one instance unlimited makes aggregate unlimited", func(t *testing.T) {
		t.Parallel()
		inst1 := makeInstance(1, "easypay", "wxpay",
			`{"wxpay":{"singleMin":3,"singleMax":10}}`)
		inst2 := makeInstance(2, "easypay", "wxpay", "") // no limits = unlimited
		ml := pcAggregateMethodLimits("wxpay", []*dbent.PaymentProviderInstance{inst1, inst2})
		if ml.SingleMin != 0 || ml.SingleMax != 0 {
			t.Fatalf("limits = min:%v max:%v, want min:0 max:0 (unlimited)", ml.SingleMin, ml.SingleMax)
		}
	})

	t.Run("one field unlimited others limited", func(t *testing.T) {
		t.Parallel()
		inst1 := makeInstance(1, "easypay", "alipay",
			`{"alipay":{"singleMin":5,"singleMax":100}}`)
		inst2 := makeInstance(2, "easypay", "alipay",
			`{"alipay":{"singleMin":3,"singleMax":0}}`) // singleMax=0 = unlimited
		ml := pcAggregateMethodLimits("alipay", []*dbent.PaymentProviderInstance{inst1, inst2})
		if ml.SingleMin != 3 {
			t.Fatalf("SingleMin = %v, want 3 (lowest floor)", ml.SingleMin)
		}
		if ml.SingleMax != 0 {
			t.Fatalf("SingleMax = %v, want 0 (unlimited)", ml.SingleMax)
		}
	})

	t.Run("empty instances returns zeros", func(t *testing.T) {
		t.Parallel()
		ml := pcAggregateMethodLimits("alipay", nil)
		if ml.SingleMin != 0 || ml.SingleMax != 0 || ml.DailyLimit != 0 {
			t.Fatalf("empty instances should return all zeros, got %+v", ml)
		}
	})

	t.Run("invalid JSON treated as unlimited", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, "easypay", "alipay", `{invalid json}`)
		ml := pcAggregateMethodLimits("alipay", []*dbent.PaymentProviderInstance{inst})
		if ml.SingleMin != 0 || ml.SingleMax != 0 {
			t.Fatalf("invalid JSON should be treated as unlimited, got %+v", ml)
		}
	})

	t.Run("type not in limits JSON treated as unlimited", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, "easypay", "alipay,wxpay",
			`{"wxpay":{"singleMin":1,"singleMax":10}}`) // only wxpay, no alipay
		ml := pcAggregateMethodLimits("alipay", []*dbent.PaymentProviderInstance{inst})
		if ml.SingleMin != 0 || ml.SingleMax != 0 {
			t.Fatalf("missing type should be treated as unlimited, got %+v", ml)
		}
	})

	t.Run("daily limit aggregation", func(t *testing.T) {
		t.Parallel()
		inst1 := makeInstance(1, "easypay", "alipay",
			`{"alipay":{"singleMin":1,"singleMax":100,"dailyLimit":500}}`)
		inst2 := makeInstance(2, "easypay", "alipay",
			`{"alipay":{"singleMin":2,"singleMax":200,"dailyLimit":1000}}`)
		ml := pcAggregateMethodLimits("alipay", []*dbent.PaymentProviderInstance{inst1, inst2})
		if ml.DailyLimit != 1000 {
			t.Fatalf("DailyLimit = %v, want 1000 (highest cap)", ml.DailyLimit)
		}
	})
}

func TestPcGroupByPaymentType(t *testing.T) {
	t.Parallel()

	t.Run("instance is grouped under each supported method", func(t *testing.T) {
		t.Parallel()
		bank := makeInstance(1, payment.TypeSePay, payment.TypeSePayBankTransfer+","+payment.TypeSePayCard, "")
		napas := makeInstance(2, payment.TypeSePay, payment.TypeSePayNapas, "")

		groups := pcGroupByPaymentType([]*dbent.PaymentProviderInstance{bank, napas})

		if len(groups[payment.TypeSePayBankTransfer]) != 1 || groups[payment.TypeSePayBankTransfer][0].ID != 1 {
			t.Fatalf("bank transfer group should contain only instance 1, got %v", groups[payment.TypeSePayBankTransfer])
		}
		if len(groups[payment.TypeSePayCard]) != 1 || groups[payment.TypeSePayCard][0].ID != 1 {
			t.Fatalf("card group should contain only instance 1, got %v", groups[payment.TypeSePayCard])
		}
		if len(groups[payment.TypeSePayNapas]) != 1 || groups[payment.TypeSePayNapas][0].ID != 2 {
			t.Fatalf("napas group should contain only instance 2, got %v", groups[payment.TypeSePayNapas])
		}
	})

	t.Run("multiple instances share the same method groups", func(t *testing.T) {
		t.Parallel()
		first := makeInstance(1, payment.TypeSePay, payment.TypeSePayBankTransfer+","+payment.TypeSePayNapas, "")
		second := makeInstance(2, payment.TypeSePay, payment.TypeSePayBankTransfer+","+payment.TypeSePayNapas, "")

		groups := pcGroupByPaymentType([]*dbent.PaymentProviderInstance{first, second})

		if len(groups[payment.TypeSePayBankTransfer]) != 2 {
			t.Fatalf("bank transfer group should have 2 instances, got %d", len(groups[payment.TypeSePayBankTransfer]))
		}
		if len(groups[payment.TypeSePayNapas]) != 2 {
			t.Fatalf("napas group should have 2 instances, got %d", len(groups[payment.TypeSePayNapas]))
		}
	})

	t.Run("instance with no supported types is grouped under nothing", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, payment.TypeSePay, "", "")

		groups := pcGroupByPaymentType([]*dbent.PaymentProviderInstance{inst})

		if len(groups) != 0 {
			t.Fatalf("instance without supported types should not be grouped, got %v", groups)
		}
	})
}

func TestPcAggregateMethodCurrency(t *testing.T) {
	t.Parallel()

	svc := &PaymentConfigService{}
	stripe := makeInstance(1, payment.TypeSePay, payment.TypeSePay, "")
	stripe.Config = `{"currency":"hkd"}`
	currency, ok := svc.pcAggregateMethodCurrency([]*dbent.PaymentProviderInstance{stripe})
	require.True(t, ok)
	require.Equal(t, "HKD", currency)

	airwallex := makeInstance(2, payment.TypeSePay, payment.TypeSePay, "")
	airwallex.Config = `{"currency":"usd"}`
	currency, ok = svc.pcAggregateMethodCurrency([]*dbent.PaymentProviderInstance{stripe, airwallex})
	require.False(t, ok)
	require.Empty(t, currency)

	easypay := makeInstance(3, payment.TypeSePay, payment.TypeSePayBankTransfer, "")
	currency, ok = svc.pcAggregateMethodCurrency([]*dbent.PaymentProviderInstance{easypay})
	require.True(t, ok)
	require.Equal(t, payment.DefaultPaymentCurrency, currency)
}

func TestGetAvailableMethodLimitsOmitsMixedCurrencyMethod(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("SePay VND").
		SetConfig(`{"currency":"VND"}`).
		SetSupportedTypes(payment.TypeSePayCard).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("SePay USD").
		SetConfig(`{"currency":"USD"}`).
		SetSupportedTypes(payment.TypeSePayCard).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentConfigService{entClient: client}
	resp, err := svc.GetAvailableMethodLimits(ctx)
	require.NoError(t, err)
	require.NotContains(t, resp.Methods, payment.TypeSePayCard)

	_, err = svc.ValidateMethodCurrencyConsistency(ctx, payment.TypeSePayCard)
	require.Error(t, err)
	appErr := infraerrors.FromError(err)
	require.Equal(t, "PAYMENT_METHOD_CURRENCY_CONFLICT", appErr.Reason)
}

func TestPcComputeGlobalRange(t *testing.T) {
	t.Parallel()

	t.Run("all methods have limits", func(t *testing.T) {
		t.Parallel()
		methods := map[string]MethodLimits{
			"alipay": {SingleMin: 2, SingleMax: 14},
			"wxpay":  {SingleMin: 1, SingleMax: 12},
			"stripe": {SingleMin: 5, SingleMax: 100},
		}
		gMin, gMax := pcComputeGlobalRange(methods)
		if gMin != 1 {
			t.Fatalf("global min = %v, want 1 (lowest floor)", gMin)
		}
		if gMax != 100 {
			t.Fatalf("global max = %v, want 100 (highest ceiling)", gMax)
		}
	})

	t.Run("one method unlimited makes global unlimited", func(t *testing.T) {
		t.Parallel()
		methods := map[string]MethodLimits{
			"alipay": {SingleMin: 2, SingleMax: 14},
			"stripe": {SingleMin: 0, SingleMax: 0}, // unlimited
		}
		gMin, gMax := pcComputeGlobalRange(methods)
		if gMin != 0 {
			t.Fatalf("global min = %v, want 0 (unlimited)", gMin)
		}
		if gMax != 0 {
			t.Fatalf("global max = %v, want 0 (unlimited)", gMax)
		}
	})

	t.Run("empty methods returns zeros", func(t *testing.T) {
		t.Parallel()
		gMin, gMax := pcComputeGlobalRange(map[string]MethodLimits{})
		if gMin != 0 || gMax != 0 {
			t.Fatalf("empty methods should return (0, 0), got (%v, %v)", gMin, gMax)
		}
	})

	t.Run("only min unlimited", func(t *testing.T) {
		t.Parallel()
		methods := map[string]MethodLimits{
			"alipay": {SingleMin: 0, SingleMax: 100},
			"wxpay":  {SingleMin: 5, SingleMax: 50},
		}
		gMin, gMax := pcComputeGlobalRange(methods)
		if gMin != 0 {
			t.Fatalf("global min = %v, want 0 (unlimited)", gMin)
		}
		if gMax != 100 {
			t.Fatalf("global max = %v, want 100", gMax)
		}
	})
}

func TestPcInstanceTypeLimits(t *testing.T) {
	t.Parallel()

	t.Run("empty limits string returns false", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, "easypay", "alipay", "")
		_, ok := pcInstanceTypeLimits(inst, "alipay")
		if ok {
			t.Fatal("expected ok=false for empty limits")
		}
	})

	t.Run("type found returns correct values", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, "easypay", "alipay",
			`{"alipay":{"singleMin":2,"singleMax":14,"dailyLimit":500}}`)
		cl, ok := pcInstanceTypeLimits(inst, "alipay")
		if !ok {
			t.Fatal("expected ok=true")
		}
		if cl.SingleMin != 2 || cl.SingleMax != 14 || cl.DailyLimit != 500 {
			t.Fatalf("limits = %+v, want min:2 max:14 daily:500", cl)
		}
	})

	t.Run("type not found returns false", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, "easypay", "alipay",
			`{"wxpay":{"singleMin":1}}`)
		_, ok := pcInstanceTypeLimits(inst, "alipay")
		if ok {
			t.Fatal("expected ok=false for missing type")
		}
	})

	t.Run("invalid JSON returns false", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, "easypay", "alipay", `{bad json}`)
		_, ok := pcInstanceTypeLimits(inst, "alipay")
		if ok {
			t.Fatal("expected ok=false for invalid JSON")
		}
	})
}
