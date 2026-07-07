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
REDACTED{
		{"first non-zero value", 0, true, 5, true, 5, trueREDACTED,
		{"lower min replaces", 10, true, 3, true, 3, trueREDACTED,
		{"higher min does not replace", 3, true, 10, true, 3, trueREDACTED,
		{"higher max replaces", 10, true, 20, false, 20, trueREDACTED,
		{"lower max does not replace", 20, true, 10, false, 20, trueREDACTED,
		{"zero value makes unlimited", 5, true, 0, true, 5, falseREDACTED,
		{"already unlimited stays unlimited", 5, false, 10, true, 5, falseREDACTED,
		{"zero on first call", 0, true, 0, true, 0, falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotAgg, gotLimited := unionFloat(tt.agg, tt.limited, tt.val, tt.wantMin)
			if gotAgg != tt.wantAgg || gotLimited != tt.wantLimited {
				t.Fatalf("unionFloat(%v, %v, %v, %v) = (%v, %v), want (%v, %v)",
					tt.agg, tt.limited, tt.val, tt.wantMin,
					gotAgg, gotLimited, tt.wantAgg, tt.wantLimited)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func makeInstance(id int64, providerKey, supportedTypes, limits string) *dbent.PaymentProviderInstance {
	return &dbent.PaymentProviderInstance{
		ID:             id,
		ProviderKey:    providerKey,
		SupportedTypes: supportedTypes,
		Limits:         limits,
		Enabled:        true,
REDACTED
REDACTED

func TestPcAggregateMethodLimits(t *testing.T) {
	t.Parallel()

	t.Run("single instance with limits", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, "easypay", "alipay,wxpay",
			`{"alipay":{"singleMin":2,"singleMax":14REDACTED,"wxpay":{"singleMin":1,"singleMax":12REDACTEDREDACTED`)
		ml := pcAggregateMethodLimits("alipay", []*dbent.PaymentProviderInstance{instREDACTED)
		if ml.SingleMin != 2 || ml.SingleMax != 14 {
			t.Fatalf("alipay limits = min:%v max:%v, want min:2 max:14", ml.SingleMin, ml.SingleMax)
	REDACTED
REDACTED)

	t.Run("two instances union takes widest range", func(t *testing.T) {
		t.Parallel()
		inst1 := makeInstance(1, "easypay", "alipay,wxpay",
			`{"alipay":{"singleMin":5,"singleMax":100REDACTEDREDACTED`)
		inst2 := makeInstance(2, "easypay", "alipay,wxpay",
			`{"alipay":{"singleMin":2,"singleMax":200REDACTEDREDACTED`)
		ml := pcAggregateMethodLimits("alipay", []*dbent.PaymentProviderInstance{inst1, inst2REDACTED)
		if ml.SingleMin != 2 {
			t.Fatalf("SingleMin = %v, want 2 (lowest floor)", ml.SingleMin)
	REDACTED
		if ml.SingleMax != 200 {
			t.Fatalf("SingleMax = %v, want 200 (highest ceiling)", ml.SingleMax)
	REDACTED
REDACTED)

	t.Run("one instance unlimited makes aggregate unlimited", func(t *testing.T) {
		t.Parallel()
		inst1 := makeInstance(1, "easypay", "wxpay",
			`{"wxpay":{"singleMin":3,"singleMax":10REDACTEDREDACTED`)
		inst2 := makeInstance(2, "easypay", "wxpay", "") // no limits = unlimited
		ml := pcAggregateMethodLimits("wxpay", []*dbent.PaymentProviderInstance{inst1, inst2REDACTED)
		if ml.SingleMin != 0 || ml.SingleMax != 0 {
			t.Fatalf("limits = min:%v max:%v, want min:0 max:0 (unlimited)", ml.SingleMin, ml.SingleMax)
	REDACTED
REDACTED)

	t.Run("one field unlimited others limited", func(t *testing.T) {
		t.Parallel()
		inst1 := makeInstance(1, "easypay", "alipay",
			`{"alipay":{"singleMin":5,"singleMax":100REDACTEDREDACTED`)
		inst2 := makeInstance(2, "easypay", "alipay",
			`{"alipay":{"singleMin":3,"singleMax":0REDACTEDREDACTED`) // singleMax=0 = unlimited
		ml := pcAggregateMethodLimits("alipay", []*dbent.PaymentProviderInstance{inst1, inst2REDACTED)
		if ml.SingleMin != 3 {
			t.Fatalf("SingleMin = %v, want 3 (lowest floor)", ml.SingleMin)
	REDACTED
		if ml.SingleMax != 0 {
			t.Fatalf("SingleMax = %v, want 0 (unlimited)", ml.SingleMax)
	REDACTED
REDACTED)

	t.Run("empty instances returns zeros", func(t *testing.T) {
		t.Parallel()
		ml := pcAggregateMethodLimits("alipay", nil)
		if ml.SingleMin != 0 || ml.SingleMax != 0 || ml.DailyLimit != 0 {
			t.Fatalf("empty instances should return all zeros, got %+v", ml)
	REDACTED
REDACTED)

	t.Run("invalid JSON treated as unlimited", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, "easypay", "alipay", `{invalid jsonREDACTED`)
		ml := pcAggregateMethodLimits("alipay", []*dbent.PaymentProviderInstance{instREDACTED)
		if ml.SingleMin != 0 || ml.SingleMax != 0 {
			t.Fatalf("invalid JSON should be treated as unlimited, got %+v", ml)
	REDACTED
REDACTED)

	t.Run("type not in limits JSON treated as unlimited", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, "easypay", "alipay,wxpay",
			`{"wxpay":{"singleMin":1,"singleMax":10REDACTEDREDACTED`) // only wxpay, no alipay
		ml := pcAggregateMethodLimits("alipay", []*dbent.PaymentProviderInstance{instREDACTED)
		if ml.SingleMin != 0 || ml.SingleMax != 0 {
			t.Fatalf("missing type should be treated as unlimited, got %+v", ml)
	REDACTED
REDACTED)

	t.Run("daily limit aggregation", func(t *testing.T) {
		t.Parallel()
		inst1 := makeInstance(1, "easypay", "alipay",
			`{"alipay":{"singleMin":1,"singleMax":100,"dailyLimit":500REDACTEDREDACTED`)
		inst2 := makeInstance(2, "easypay", "alipay",
			`{"alipay":{"singleMin":2,"singleMax":200,"dailyLimit":1000REDACTEDREDACTED`)
		ml := pcAggregateMethodLimits("alipay", []*dbent.PaymentProviderInstance{inst1, inst2REDACTED)
		if ml.DailyLimit != 1000 {
			t.Fatalf("DailyLimit = %v, want 1000 (highest cap)", ml.DailyLimit)
	REDACTED
REDACTED)
REDACTED

func TestPcGroupByPaymentType(t *testing.T) {
	t.Parallel()

	t.Run("stripe instance maps all types to stripe group", func(t *testing.T) {
		t.Parallel()
		stripe := makeInstance(1, payment.TypeStripe, "card,alipay,link,wxpay", "")
		easypay := makeInstance(2, payment.TypeEasyPay, "alipay,wxpay", "")

		groups := pcGroupByPaymentType([]*dbent.PaymentProviderInstance{stripe, easypayREDACTED)

		// Stripe instance should only be in "stripe" group
		if len(groups[payment.TypeStripe]) != 1 || groups[payment.TypeStripe][0].ID != 1 {
			t.Fatalf("stripe group should contain only stripe instance, got %v", groups[payment.TypeStripe])
	REDACTED
		// alipay group should only contain easypay, NOT stripe
		if len(groups[payment.TypeAlipay]) != 1 || groups[payment.TypeAlipay][0].ID != 2 {
			t.Fatalf("alipay group should contain only easypay instance, got %v", groups[payment.TypeAlipay])
	REDACTED
		// wxpay group should only contain easypay, NOT stripe
		if len(groups[payment.TypeWxpay]) != 1 || groups[payment.TypeWxpay][0].ID != 2 {
			t.Fatalf("wxpay group should contain only easypay instance, got %v", groups[payment.TypeWxpay])
	REDACTED
REDACTED)

	t.Run("multiple easypay instances in same groups", func(t *testing.T) {
		t.Parallel()
		ep1 := makeInstance(1, payment.TypeEasyPay, "alipay,wxpay", "")
		ep2 := makeInstance(2, payment.TypeEasyPay, "alipay,wxpay", "")

		groups := pcGroupByPaymentType([]*dbent.PaymentProviderInstance{ep1, ep2REDACTED)

		if len(groups[payment.TypeAlipay]) != 2 {
			t.Fatalf("alipay group should have 2 instances, got %d", len(groups[payment.TypeAlipay]))
	REDACTED
		if len(groups[payment.TypeWxpay]) != 2 {
			t.Fatalf("wxpay group should have 2 instances, got %d", len(groups[payment.TypeWxpay]))
	REDACTED
REDACTED)

	t.Run("stripe with no supported types still in stripe group", func(t *testing.T) {
		t.Parallel()
		stripe := makeInstance(1, payment.TypeStripe, "", "")

		groups := pcGroupByPaymentType([]*dbent.PaymentProviderInstance{stripeREDACTED)

		if len(groups[payment.TypeStripe]) != 1 {
			t.Fatalf("stripe with empty types should still be in stripe group, got %v", groups)
	REDACTED
REDACTED)
REDACTED

func TestPcAggregateMethodCurrency(t *testing.T) {
	t.Parallel()

	svc := &PaymentConfigService{REDACTED
	stripe := makeInstance(1, payment.TypeStripe, payment.TypeStripe, "")
	stripe.Config = `{"currency":"hkd"REDACTED`
	currency, ok := svc.pcAggregateMethodCurrency([]*dbent.PaymentProviderInstance{stripeREDACTED)
	require.True(t, ok)
	require.Equal(t, "HKD", currency)

	airwallex := makeInstance(2, payment.TypeAirwallex, payment.TypeAirwallex, "")
	airwallex.Config = `{"currency":"usd"REDACTED`
	currency, ok = svc.pcAggregateMethodCurrency([]*dbent.PaymentProviderInstance{stripe, airwallexREDACTED)
	require.False(t, ok)
	require.Empty(t, currency)

	easypay := makeInstance(3, payment.TypeEasyPay, payment.TypeAlipay, "")
	currency, ok = svc.pcAggregateMethodCurrency([]*dbent.PaymentProviderInstance{easypayREDACTED)
	require.True(t, ok)
	require.Equal(t, payment.DefaultPaymentCurrency, currency)
REDACTED

func TestGetAvailableMethodLimitsOmitsMixedCurrencyMethod(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("Stripe HKD").
		SetConfig(`{"currency":"HKD"REDACTED`).
		SetSupportedTypes("card,link").
		SetEnabled(true).
		Save(ctx)
REDACTED

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("Stripe USD").
		SetConfig(`{"currency":"USD"REDACTED`).
		SetSupportedTypes("card,link").
		SetEnabled(true).
		Save(ctx)
REDACTED

	svc := &PaymentConfigService{entClient: clientREDACTED
	resp, err := svc.GetAvailableMethodLimits(ctx)
REDACTED
	require.NotContains(t, resp.Methods, payment.TypeStripe)

	_, err = svc.ValidateMethodCurrencyConsistency(ctx, payment.TypeStripe)
REDACTED
	appErr := infraerrors.FromError(err)
	require.Equal(t, "PAYMENT_METHOD_CURRENCY_CONFLICT", appErr.Reason)
REDACTED

func TestGetAvailableMethodLimitsIncludesEasyPayCustomMethodDisplayName(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeEasyPay).
		SetName("EasyPay Custom").
		SetConfig(`{"customMethods":"[{\"type\":\"ldc\",\"upstreamType\":\"ldc\",\"displayName\":\"LDC Pay\"REDACTED]"REDACTED`).
		SetSupportedTypes("alipay,wxpay,ldc").
		SetEnabled(true).
		Save(ctx)
REDACTED

	svc := &PaymentConfigService{entClient: clientREDACTED
	resp, err := svc.GetAvailableMethodLimits(ctx)
REDACTED

	limits, ok := resp.Methods["ldc"]
	require.True(t, ok, "expected custom EasyPay method limits to be visible")
	require.Equal(t, "LDC Pay", limits.DisplayName)
REDACTED

func TestPcComputeGlobalRange(t *testing.T) {
	t.Parallel()

	t.Run("all methods have limits", func(t *testing.T) {
		t.Parallel()
		methods := map[string]MethodLimits{
			"alipay": {SingleMin: 2, SingleMax: 14REDACTED,
			"wxpay":  {SingleMin: 1, SingleMax: 12REDACTED,
			"stripe": {SingleMin: 5, SingleMax: 100REDACTED,
	REDACTED
		gMin, gMax := pcComputeGlobalRange(methods)
		if gMin != 1 {
			t.Fatalf("global min = %v, want 1 (lowest floor)", gMin)
	REDACTED
		if gMax != 100 {
			t.Fatalf("global max = %v, want 100 (highest ceiling)", gMax)
	REDACTED
REDACTED)

	t.Run("one method unlimited makes global unlimited", func(t *testing.T) {
		t.Parallel()
		methods := map[string]MethodLimits{
			"alipay": {SingleMin: 2, SingleMax: 14REDACTED,
			"stripe": {SingleMin: 0, SingleMax: 0REDACTED, // unlimited
	REDACTED
		gMin, gMax := pcComputeGlobalRange(methods)
		if gMin != 0 {
			t.Fatalf("global min = %v, want 0 (unlimited)", gMin)
	REDACTED
		if gMax != 0 {
			t.Fatalf("global max = %v, want 0 (unlimited)", gMax)
	REDACTED
REDACTED)

	t.Run("empty methods returns zeros", func(t *testing.T) {
		t.Parallel()
		gMin, gMax := pcComputeGlobalRange(map[string]MethodLimits{REDACTED)
		if gMin != 0 || gMax != 0 {
			t.Fatalf("empty methods should return (0, 0), got (%v, %v)", gMin, gMax)
	REDACTED
REDACTED)

	t.Run("only min unlimited", func(t *testing.T) {
		t.Parallel()
		methods := map[string]MethodLimits{
			"alipay": {SingleMin: 0, SingleMax: 100REDACTED,
			"wxpay":  {SingleMin: 5, SingleMax: 50REDACTED,
	REDACTED
		gMin, gMax := pcComputeGlobalRange(methods)
		if gMin != 0 {
			t.Fatalf("global min = %v, want 0 (unlimited)", gMin)
	REDACTED
		if gMax != 100 {
			t.Fatalf("global max = %v, want 100", gMax)
	REDACTED
REDACTED)
REDACTED

func TestPcInstanceTypeLimits(t *testing.T) {
	t.Parallel()

	t.Run("empty limits string returns false", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, "easypay", "alipay", "")
		_, ok := pcInstanceTypeLimits(inst, "alipay")
		if ok {
			t.Fatal("expected ok=false for empty limits")
	REDACTED
REDACTED)

	t.Run("type found returns correct values", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, "easypay", "alipay",
			`{"alipay":{"singleMin":2,"singleMax":14,"dailyLimit":500REDACTEDREDACTED`)
		cl, ok := pcInstanceTypeLimits(inst, "alipay")
		if !ok {
			t.Fatal("expected ok=true")
	REDACTED
		if cl.SingleMin != 2 || cl.SingleMax != 14 || cl.DailyLimit != 500 {
			t.Fatalf("limits = %+v, want min:2 max:14 daily:500", cl)
	REDACTED
REDACTED)

	t.Run("type not found returns false", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, "easypay", "alipay",
			`{"wxpay":{"singleMin":1REDACTEDREDACTED`)
		_, ok := pcInstanceTypeLimits(inst, "alipay")
		if ok {
			t.Fatal("expected ok=false for missing type")
	REDACTED
REDACTED)

	t.Run("invalid JSON returns false", func(t *testing.T) {
		t.Parallel()
		inst := makeInstance(1, "easypay", "alipay", `{bad jsonREDACTED`)
		_, ok := pcInstanceTypeLimits(inst, "alipay")
		if ok {
			t.Fatal("expected ok=false for invalid JSON")
	REDACTED
REDACTED)
REDACTED

func TestGetAvailableMethodLimitsUsesConfiguredVisibleMethodSource(t *testing.T) {
	tests := []struct {
		name                string
		sourceSetting       string
		wantAlipaySingleMin float64
		wantAlipaySingleMax float64
		wantGlobalMin       float64
		wantGlobalMax       float64
REDACTED{
		{
			name:                "official source",
			sourceSetting:       VisibleMethodSourceOfficialAlipay,
			wantAlipaySingleMin: 10,
			wantAlipaySingleMax: 100,
			wantGlobalMin:       10,
			wantGlobalMax:       300,
	REDACTED,
		{
			name:                "easypay source",
			sourceSetting:       VisibleMethodSourceEasyPayAlipay,
			wantAlipaySingleMin: 20,
			wantAlipaySingleMax: 200,
			wantGlobalMin:       20,
			wantGlobalMax:       300,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)

			_, err := client.PaymentProviderInstance.Create().
				SetProviderKey(payment.TypeAlipay).
				SetName("Official Alipay").
				SetConfig("{REDACTED").
				SetSupportedTypes("alipay").
				SetLimits(`{"alipay":{"singleMin":10,"singleMax":100REDACTEDREDACTED`).
				SetEnabled(true).
				Save(ctx)
			if err != nil {
				t.Fatalf("create official alipay instance: %v", err)
		REDACTED
			_, err = client.PaymentProviderInstance.Create().
				SetProviderKey(payment.TypeEasyPay).
				SetName("EasyPay Alipay").
				SetConfig("{REDACTED").
				SetSupportedTypes("alipay").
				SetLimits(`{"alipay":{"singleMin":20,"singleMax":200REDACTEDREDACTED`).
				SetEnabled(true).
				Save(ctx)
			if err != nil {
				t.Fatalf("create easypay alipay instance: %v", err)
		REDACTED
			_, err = client.PaymentProviderInstance.Create().
				SetProviderKey(payment.TypeWxpay).
				SetName("Official WeChat").
				SetConfig("{REDACTED").
				SetSupportedTypes("wxpay").
				SetLimits(`{"wxpay":{"singleMin":30,"singleMax":300REDACTEDREDACTED`).
				SetEnabled(true).
				Save(ctx)
			if err != nil {
				t.Fatalf("create official wxpay instance: %v", err)
		REDACTED

			svc := &PaymentConfigService{
				entClient: client,
				settingRepo: &paymentConfigSettingRepoStub{
					values: map[string]string{
						SettingPaymentVisibleMethodAlipaySource: tt.sourceSetting,
				REDACTED,
			REDACTED,
		REDACTED

			resp, err := svc.GetAvailableMethodLimits(ctx)
			if err != nil {
				t.Fatalf("GetAvailableMethodLimits returned error: %v", err)
		REDACTED

			alipayLimits, ok := resp.Methods[payment.TypeAlipay]
			if !ok {
				t.Fatalf("expected alipay limits to remain visible, got %v", resp.Methods)
		REDACTED
			if alipayLimits.SingleMin != tt.wantAlipaySingleMin || alipayLimits.SingleMax != tt.wantAlipaySingleMax {
				t.Fatalf("alipay limits = %+v, want min=%v max=%v", alipayLimits, tt.wantAlipaySingleMin, tt.wantAlipaySingleMax)
		REDACTED

			wxpayLimits, ok := resp.Methods[payment.TypeWxpay]
			if !ok {
				t.Fatalf("expected wxpay limits to remain visible, got %v", resp.Methods)
		REDACTED
			if wxpayLimits.SingleMin != 30 || wxpayLimits.SingleMax != 300 {
				t.Fatalf("wxpay limits = %+v, want official-only min=30 max=300", wxpayLimits)
		REDACTED
			if resp.GlobalMin != tt.wantGlobalMin || resp.GlobalMax != tt.wantGlobalMax {
				t.Fatalf("global range = (%v, %v), want (%v, %v)", resp.GlobalMin, resp.GlobalMax, tt.wantGlobalMin, tt.wantGlobalMax)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestGetAvailableMethodLimitsPreservesLegacyCrossProviderBehaviorWhenVisibleMethodSourceMissing(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("Official Alipay").
		SetConfig("{REDACTED").
		SetSupportedTypes("alipay").
		SetLimits(`{"alipay":{"singleMin":10,"singleMax":100REDACTEDREDACTED`).
		SetEnabled(true).
		Save(ctx)
REDACTED

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeEasyPay).
		SetName("EasyPay Mixed").
		SetConfig("{REDACTED").
		SetSupportedTypes("alipay,wxpay").
		SetLimits(`{"alipay":{"singleMin":20,"singleMax":200REDACTED,"wxpay":{"singleMin":40,"singleMax":400REDACTEDREDACTED`).
		SetEnabled(true).
		Save(ctx)
REDACTED

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("Official WeChat").
		SetConfig("{REDACTED").
		SetSupportedTypes("wxpay").
		SetLimits(`{"wxpay":{"singleMin":30,"singleMax":300REDACTEDREDACTED`).
		SetEnabled(true).
		Save(ctx)
REDACTED

	svc := &PaymentConfigService{
		entClient:   client,
		settingRepo: &paymentConfigSettingRepoStub{values: map[string]string{REDACTEDREDACTED,
REDACTED

	resp, err := svc.GetAvailableMethodLimits(ctx)
REDACTED

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
REDACTED
