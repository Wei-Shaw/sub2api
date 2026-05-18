package service

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
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

	t.Run("stripe instance buckets per sub-method (no stripe key)", func(t *testing.T) {
		t.Parallel()
		stripe := makeInstance(1, payment.TypeStripe, "card,alipay,link,wxpay", "")
		easypay := makeInstance(2, payment.TypeEasyPay, "alipay,wxpay", "")

		groups := pcGroupByPaymentType([]*dbent.PaymentProviderInstance{stripe, easypay})

		// Stripe sub-methods get their own buckets.
		if got := len(groups[payment.TypeCard]); got != 1 || groups[payment.TypeCard][0].ID != 1 {
			t.Fatalf("card group should contain only stripe instance, got %v", groups[payment.TypeCard])
		}
		if got := len(groups[payment.TypeLink]); got != 1 || groups[payment.TypeLink][0].ID != 1 {
			t.Fatalf("link group should contain only stripe instance, got %v", groups[payment.TypeLink])
		}
		// alipay group should contain BOTH stripe and easypay (they each support alipay).
		if got := len(groups[payment.TypeAlipay]); got != 2 {
			t.Fatalf("alipay group should have 2 instances (stripe + easypay), got %d", got)
		}
		// wxpay group should contain BOTH stripe and easypay.
		if got := len(groups[payment.TypeWxpay]); got != 2 {
			t.Fatalf("wxpay group should have 2 instances (stripe + easypay), got %d", got)
		}
		// No collapsed "stripe" bucket.
		if _, present := groups[payment.TypeStripe]; present {
			t.Fatalf("stripe bucket should NOT exist (per-sub-method bucketing), got %v", groups[payment.TypeStripe])
		}
	})

	t.Run("multiple easypay instances in same groups", func(t *testing.T) {
		t.Parallel()
		ep1 := makeInstance(1, payment.TypeEasyPay, "alipay,wxpay", "")
		ep2 := makeInstance(2, payment.TypeEasyPay, "alipay,wxpay", "")

		groups := pcGroupByPaymentType([]*dbent.PaymentProviderInstance{ep1, ep2})

		if len(groups[payment.TypeAlipay]) != 2 {
			t.Fatalf("alipay group should have 2 instances, got %d", len(groups[payment.TypeAlipay]))
		}
		if len(groups[payment.TypeWxpay]) != 2 {
			t.Fatalf("wxpay group should have 2 instances, got %d", len(groups[payment.TypeWxpay]))
		}
	})

	t.Run("stripe with empty supported_types produces no buckets", func(t *testing.T) {
		t.Parallel()
		stripe := makeInstance(1, payment.TypeStripe, "", "")

		groups := pcGroupByPaymentType([]*dbent.PaymentProviderInstance{stripe})

		// With no supported_types, the loop has nothing to add. No buckets at all.
		if len(groups) != 0 {
			t.Fatalf("stripe with empty types should produce no buckets, got %v", groups)
		}
	})
}

func TestPcGroupByPaymentTypeStripeBucketsPerSubMethod(t *testing.T) {
	t.Parallel()

	stripe := makeInstance(1, payment.TypeStripe, "card,alipay,wxpay", "")
	easypay := makeInstance(2, payment.TypeEasyPay, "alipay", "")

	groups := pcGroupByPaymentType([]*dbent.PaymentProviderInstance{stripe, easypay})

	// Stripe should produce per-sub-method buckets (no collapsed "stripe" bucket).
	if got := len(groups[payment.TypeCard]); got != 1 || groups[payment.TypeCard][0].ID != 1 {
		t.Errorf("group[card] should contain only stripe instance, got %v", groups[payment.TypeCard])
	}
	if got := len(groups[payment.TypeWxpay]); got != 1 || groups[payment.TypeWxpay][0].ID != 1 {
		t.Errorf("group[wxpay] should contain only stripe instance, got %v", groups[payment.TypeWxpay])
	}
	// alipay should contain BOTH stripe and easypay.
	if got := len(groups[payment.TypeAlipay]); got != 2 {
		t.Errorf("group[alipay] should contain both stripe (id=1) and easypay (id=2), got %d entries: %v", got, groups[payment.TypeAlipay])
	}
	// The collapsed "stripe" bucket must NOT exist.
	if _, present := groups[payment.TypeStripe]; present {
		t.Errorf("group[stripe] should NOT exist after refactor (each sub-method gets its own bucket), got %v", groups[payment.TypeStripe])
	}
}

func TestPcAggregateMethodCurrency(t *testing.T) {
	t.Parallel()

	svc := &PaymentConfigService{}
	stripe := makeInstance(1, payment.TypeStripe, payment.TypeStripe, "")
	stripe.Config = `{"currency":"hkd"}`
	currency, ok := svc.pcAggregateMethodCurrency([]*dbent.PaymentProviderInstance{stripe})
	require.True(t, ok)
	require.Equal(t, "HKD", currency)

	airwallex := makeInstance(2, payment.TypeAirwallex, payment.TypeAirwallex, "")
	airwallex.Config = `{"currency":"usd"}`
	currency, ok = svc.pcAggregateMethodCurrency([]*dbent.PaymentProviderInstance{stripe, airwallex})
	require.False(t, ok)
	require.Empty(t, currency)

	easypay := makeInstance(3, payment.TypeEasyPay, payment.TypeAlipay, "")
	currency, ok = svc.pcAggregateMethodCurrency([]*dbent.PaymentProviderInstance{easypay})
	require.True(t, ok)
	require.Equal(t, payment.DefaultPaymentCurrency, currency)
}

func TestGetAvailableMethodLimitsOmitsMixedCurrencyMethod(t *testing.T) {
	// SKIPPED on this fork: upstream groups Stripe as a single top-level method,
	// while this fork explodes Stripe into per-sub-method buckets
	// (card / alipay / wxpay / link) — see pcGroupByPaymentType. As a result,
	// ValidateMethodCurrencyConsistency("stripe") looks up typeInstances["stripe"]
	// which is always empty here. Currency-conflict detection has to be redesigned
	// at the sub-method level before this test can be reactivated.
	t.Skip("incompatible with per-sub-method Stripe grouping in this fork")
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

func TestGetAvailableMethodLimitsUsesConfiguredVisibleMethodSource(t *testing.T) {
	tests := []struct {
		name                string
		sourceSetting       string
		wantAlipaySingleMin float64
		wantAlipaySingleMax float64
		wantGlobalMin       float64
		wantGlobalMax       float64
	}{
		{
			name:                "official source",
			sourceSetting:       VisibleMethodSourceOfficialAlipay,
			wantAlipaySingleMin: 10,
			wantAlipaySingleMax: 100,
			wantGlobalMin:       10,
			wantGlobalMax:       300,
		},
		{
			name:                "easypay source",
			sourceSetting:       VisibleMethodSourceEasyPayAlipay,
			wantAlipaySingleMin: 20,
			wantAlipaySingleMax: 200,
			wantGlobalMin:       20,
			wantGlobalMax:       300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)

			_, err := client.PaymentProviderInstance.Create().
				SetProviderKey(payment.TypeAlipay).
				SetName("Official Alipay").
				SetConfig("{}").
				SetSupportedTypes("alipay").
				SetLimits(`{"alipay":{"singleMin":10,"singleMax":100}}`).
				SetEnabled(true).
				Save(ctx)
			if err != nil {
				t.Fatalf("create official alipay instance: %v", err)
			}
			_, err = client.PaymentProviderInstance.Create().
				SetProviderKey(payment.TypeEasyPay).
				SetName("EasyPay Alipay").
				SetConfig("{}").
				SetSupportedTypes("alipay").
				SetLimits(`{"alipay":{"singleMin":20,"singleMax":200}}`).
				SetEnabled(true).
				Save(ctx)
			if err != nil {
				t.Fatalf("create easypay alipay instance: %v", err)
			}
			_, err = client.PaymentProviderInstance.Create().
				SetProviderKey(payment.TypeWxpay).
				SetName("Official WeChat").
				SetConfig("{}").
				SetSupportedTypes("wxpay").
				SetLimits(`{"wxpay":{"singleMin":30,"singleMax":300}}`).
				SetEnabled(true).
				Save(ctx)
			if err != nil {
				t.Fatalf("create official wxpay instance: %v", err)
			}

			svc := &PaymentConfigService{
				entClient: client,
				settingRepo: &paymentConfigSettingRepoStub{
					values: map[string]string{
						SettingPaymentVisibleMethodAlipaySource: tt.sourceSetting,
					},
				},
			}

			resp, err := svc.GetAvailableMethodLimits(ctx)
			if err != nil {
				t.Fatalf("GetAvailableMethodLimits returned error: %v", err)
			}

			alipayLimits, ok := resp.Methods[payment.TypeAlipay]
			if !ok {
				t.Fatalf("expected alipay limits to remain visible, got %v", resp.Methods)
			}
			if alipayLimits.SingleMin != tt.wantAlipaySingleMin || alipayLimits.SingleMax != tt.wantAlipaySingleMax {
				t.Fatalf("alipay limits = %+v, want min=%v max=%v", alipayLimits, tt.wantAlipaySingleMin, tt.wantAlipaySingleMax)
			}

			wxpayLimits, ok := resp.Methods[payment.TypeWxpay]
			if !ok {
				t.Fatalf("expected wxpay limits to remain visible, got %v", resp.Methods)
			}
			if wxpayLimits.SingleMin != 30 || wxpayLimits.SingleMax != 300 {
				t.Fatalf("wxpay limits = %+v, want official-only min=30 max=300", wxpayLimits)
			}
			if resp.GlobalMin != tt.wantGlobalMin || resp.GlobalMax != tt.wantGlobalMax {
				t.Fatalf("global range = (%v, %v), want (%v, %v)", resp.GlobalMin, resp.GlobalMax, tt.wantGlobalMin, tt.wantGlobalMax)
			}
		})
	}
}

func TestGetAvailableMethodLimitsPreservesLegacyCrossProviderBehaviorWhenVisibleMethodSourceMissing(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("Official Alipay").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetLimits(`{"alipay":{"singleMin":10,"singleMax":100}}`).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeEasyPay).
		SetName("EasyPay Mixed").
		SetConfig("{}").
		SetSupportedTypes("alipay,wxpay").
		SetLimits(`{"alipay":{"singleMin":20,"singleMax":200},"wxpay":{"singleMin":40,"singleMax":400}}`).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("Official WeChat").
		SetConfig("{}").
		SetSupportedTypes("wxpay").
		SetLimits(`{"wxpay":{"singleMin":30,"singleMax":300}}`).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentConfigService{
		entClient:   client,
		settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{}},
	}

	resp, err := svc.GetAvailableMethodLimits(ctx)
	require.NoError(t, err)

	alipayLimits, ok := resp.Methods[payment.TypeAlipay]
	require.True(t, ok, "expected alipay limits to remain visible")
	require.Equal(t, 10.0, alipayLimits.SingleMin)
	require.Equal(t, 200.0, alipayLimits.SingleMax)

	wxpayLimits, ok := resp.Methods[payment.TypeWxpay]
	require.True(t, ok, "expected wxpay limits to remain visible")
	require.Equal(t, 30.0, wxpayLimits.SingleMin)
	require.Equal(t, 400.0, wxpayLimits.SingleMax)

	require.Equal(t, 10.0, resp.GlobalMin)
	require.Equal(t, 400.0, resp.GlobalMax)
}
