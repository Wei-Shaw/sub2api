//go:build unit

package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateProviderRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		providerKey    string
		providerName   string
		supportedTypes string
		wantErr        bool
		errContains    string
REDACTED{
		{
			name:           "valid easypay with types",
			providerKey:    "easypay",
			providerName:   "MyProvider",
			supportedTypes: "alipay,wxpay",
			wantErr:        false,
	REDACTED,
		{
			name:           "valid stripe with empty types",
			providerKey:    "stripe",
			providerName:   "Stripe Provider",
			supportedTypes: "",
			wantErr:        false,
	REDACTED,
		{
			name:           "valid airwallex provider",
			providerKey:    payment.TypeAirwallex,
			providerName:   "Airwallex Provider",
			supportedTypes: payment.TypeAirwallex,
			wantErr:        false,
	REDACTED,
		{
			name:           "valid alipay provider",
			providerKey:    "alipay",
			providerName:   "Alipay Direct",
			supportedTypes: "alipay",
			wantErr:        false,
	REDACTED,
		{
			name:           "valid wxpay provider",
			providerKey:    "wxpay",
			providerName:   "WeChat Pay",
			supportedTypes: "wxpay",
			wantErr:        false,
	REDACTED,
		{
			name:           "invalid provider key",
			providerKey:    "invalid",
			providerName:   "Name",
			supportedTypes: "alipay",
			wantErr:        true,
			errContains:    "invalid provider key",
	REDACTED,
		{
			name:           "empty name",
			providerKey:    "easypay",
			providerName:   "",
			supportedTypes: "alipay",
			wantErr:        true,
			errContains:    "provider name is required",
	REDACTED,
		{
			name:           "whitespace-only name",
			providerKey:    "easypay",
			providerName:   "  ",
			supportedTypes: "alipay",
			wantErr:        true,
			errContains:    "provider name is required",
	REDACTED,
		{
			name:           "tab-only name",
			providerKey:    "easypay",
			providerName:   "\t",
			supportedTypes: "alipay",
			wantErr:        true,
			errContains:    "provider name is required",
	REDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateProviderRequest(tc.providerKey, tc.providerName, tc.supportedTypes)
			if tc.wantErr {
			REDACTED
				assert.Contains(t, err.Error(), tc.errContains)
		REDACTED else {
			REDACTED
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestValidateEasyPayCustomMethods(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		config         map[string]string
		supportedTypes string
		wantErr        string
REDACTED{
		{
			name:           "valid custom methods",
			config:         map[string]string{"customMethods": `[{"type":"ldc","upstreamType":"epay","displayName":"LDC"REDACTED]`REDACTED,
			supportedTypes: "alipay,wxpay,ldc",
	REDACTED,
		{
			name:           "malformed custom methods json",
			config:         map[string]string{"customMethods": `not-json`REDACTED,
			supportedTypes: "alipay,wxpay,ldc",
			wantErr:        "customMethods must be a JSON array",
	REDACTED,
		{
			name:           "missing upstream type",
			config:         map[string]string{"customMethods": `[{"type":"ldc","displayName":"LDC"REDACTED]`REDACTED,
			supportedTypes: "alipay,wxpay,ldc",
			wantErr:        "customMethods upstreamType is required",
	REDACTED,
		{
			name:           "duplicate custom type",
			config:         map[string]string{"customMethods": `[{"type":"ldc","upstreamType":"epay"REDACTED,{"type":"ldc","upstreamType":"epay2"REDACTED]`REDACTED,
			supportedTypes: "alipay,wxpay,ldc",
			wantErr:        "duplicate customMethods type",
	REDACTED,
		{
			name:           "custom type must already be lowercase",
			config:         map[string]string{"customMethods": `[{"type":"LDC","upstreamType":"epay"REDACTED]`REDACTED,
			supportedTypes: "alipay,wxpay,ldc",
			wantErr:        "customMethods type may only contain lowercase letters",
	REDACTED,
		{
			name:           "upstream type must already be lowercase",
			config:         map[string]string{"customMethods": `[{"type":"ldc","upstreamType":"ALIPAY"REDACTED]`REDACTED,
			supportedTypes: "alipay,wxpay,ldc",
			wantErr:        "customMethods upstreamType may only contain lowercase letters",
	REDACTED,
		{
			name:           "custom type uses alipay prefix",
			config:         map[string]string{"customMethods": `[{"type":"alipay_hk","upstreamType":"hkpay"REDACTED]`REDACTED,
			supportedTypes: "alipay,wxpay,alipay_hk",
			wantErr:        "customMethods type cannot start with alipay or wxpay",
	REDACTED,
		{
			name:           "custom type uses wxpay prefix",
			config:         map[string]string{"customMethods": `[{"type":"wxpay_usdt","upstreamType":"usdt"REDACTED]`REDACTED,
			supportedTypes: "alipay,wxpay,wxpay_usdt",
			wantErr:        "customMethods type cannot start with alipay or wxpay",
	REDACTED,
		{
			name:           "supported custom type missing mapping",
			config:         map[string]string{"customMethods": `[{"type":"ldc","upstreamType":"epay"REDACTED]`REDACTED,
			supportedTypes: "alipay,wxpay,ldc,usdt_trc20",
			wantErr:        "supported EasyPay custom type usdt_trc20 has no customMethods mapping",
	REDACTED,
		{
			name:           "supported custom type must already be lowercase",
			config:         map[string]string{"customMethods": `[{"type":"ldc","upstreamType":"epay"REDACTED]`REDACTED,
			supportedTypes: "alipay,wxpay,LDC",
			wantErr:        "supported EasyPay custom type LDC may only contain lowercase letters",
	REDACTED,
REDACTED

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateEasyPayCustomMethods(tc.config, tc.supportedTypes)
			if tc.wantErr == "" {
			REDACTED
				return
		REDACTED
		REDACTED
			require.Contains(t, err.Error(), tc.wantErr)
	REDACTED)
REDACTED
REDACTED

func TestIsSensitiveProviderConfigField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		providerKey string
		field       string
		wantSen     bool
REDACTED{
		// Stripe: publishableKey is public, only secretKey/webhookSecret are secrets
		{"stripe", "secretKey", trueREDACTED,
		{"stripe", "webhookSecret", trueREDACTED,
		{"stripe", "SecretKey", trueREDACTED, // case-insensitive
		{"stripe", "publishableKey", falseREDACTED,
		{"stripe", "currency", falseREDACTED,
		{"stripe", "appId", falseREDACTED,

		// Alipay
		{"alipay", "privateKey", trueREDACTED,
		{"alipay", "publicKey", trueREDACTED,
		{"alipay", "alipayPublicKey", trueREDACTED,
		{"alipay", "appId", falseREDACTED,
		{"alipay", "notifyUrl", falseREDACTED,

		// Wxpay
		{"wxpay", "privateKey", trueREDACTED,
		{"wxpay", "apiV3Key", trueREDACTED,
		{"wxpay", "publicKey", trueREDACTED,
		{"wxpay", "publicKeyId", falseREDACTED,
		{"wxpay", "certSerial", falseREDACTED,
		{"wxpay", "mchId", falseREDACTED,

		// EasyPay
		{"easypay", "pkey", trueREDACTED,
		{"easypay", "pid", falseREDACTED,
		{"easypay", "apiBase", falseREDACTED,

		// Airwallex
		{payment.TypeAirwallex, "apiKey", trueREDACTED,
		{payment.TypeAirwallex, "webhookSecret", trueREDACTED,
		{payment.TypeAirwallex, "clientId", falseREDACTED,
		{payment.TypeAirwallex, "apiBase", falseREDACTED,
		{payment.TypeAirwallex, "accountId", falseREDACTED,
		{payment.TypeAirwallex, "currency", falseREDACTED,

		// Unknown provider: never sensitive
		{"unknown", "secretKey", falseREDACTED,
REDACTED

	for _, tc := range tests {
		tc := tc
		t.Run(tc.providerKey+"/"+tc.field, func(t *testing.T) {
			t.Parallel()

			got := isSensitiveProviderConfigField(tc.providerKey, tc.field)
			assert.Equal(t, tc.wantSen, got, "isSensitiveProviderConfigField(%q, %q)", tc.providerKey, tc.field)
	REDACTED)
REDACTED
REDACTED

func TestJoinTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []string
		want  string
REDACTED{
		{
			name:  "multiple types",
			input: []string{"alipay", "wxpay"REDACTED,
			want:  "alipay,wxpay",
	REDACTED,
		{
			name:  "single type",
			input: []string{"stripe"REDACTED,
			want:  "stripe",
	REDACTED,
		{
			name:  "empty slice",
			input: []string{REDACTED,
			want:  "",
	REDACTED,
		{
			name:  "nil slice",
			input: nil,
			want:  "",
	REDACTED,
		{
			name:  "three types",
			input: []string{"alipay", "wxpay", "stripe"REDACTED,
			want:  "alipay,wxpay,stripe",
	REDACTED,
		{
			name:  "types with spaces are not trimmed",
			input: []string{" alipay ", " wxpay "REDACTED,
			want:  " alipay , wxpay ",
	REDACTED,
REDACTED

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := joinTypes(tc.input)
			assert.Equal(t, tc.want, got)
	REDACTED)
REDACTED
REDACTED

func TestCreateProviderInstanceAllowsVisibleMethodProvidersFromDifferentSources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{
		entClient:     client,
		encryptionKey: []byte("REDACTED"),
REDACTED

	_, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey: "easypay",
		Name:        "EasyPay Alipay",
		Config: map[string]string{
			"pid":       "1001",
			"pkey":      "pkey-1001",
			"apiBase":   "https://pay.example.com",
			"notifyUrl": "https://merchant.example.com/notify",
			"returnUrl": "https://merchant.example.com/return",
	REDACTED,
		SupportedTypes: []string{"alipay"REDACTED,
		Enabled:        true,
REDACTED)
REDACTED

	_, err = svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey:    "alipay",
		Name:           "Official Alipay",
		Config:         map[string]string{"appId": "app-1", "privateKey": "private-key"REDACTED,
		SupportedTypes: []string{"alipay"REDACTED,
		Enabled:        true,
REDACTED)
REDACTED
REDACTED

func TestUpdateProviderInstanceAllowsEnablingVisibleMethodProviderFromDifferentSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{
		entClient:     client,
		encryptionKey: []byte("REDACTED"),
REDACTED

	existing, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey: "easypay",
		Name:        "EasyPay WeChat",
		Config: map[string]string{
			"pid":       "2001",
			"pkey":      "pkey-2001",
			"apiBase":   "https://pay.example.com",
			"notifyUrl": "https://merchant.example.com/notify",
			"returnUrl": "https://merchant.example.com/return",
	REDACTED,
		SupportedTypes: []string{"wxpay"REDACTED,
		Enabled:        true,
REDACTED)
REDACTED
	require.NotNil(t, existing)

	candidate, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey:    "wxpay",
		Name:           "Official WeChat",
		Config:         validWxpayProviderConfig(t),
		SupportedTypes: []string{"wxpay"REDACTED,
		Enabled:        false,
REDACTED)
REDACTED

	_, err = svc.UpdateProviderInstance(ctx, candidate.ID, UpdateProviderInstanceRequest{
		Enabled: boolPtrValue(true),
REDACTED)
REDACTED
REDACTED

func TestUpdateProviderInstancePersistsEnabledAndSupportedTypes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{
		entClient:     client,
		encryptionKey: []byte("REDACTED"),
REDACTED

	instance, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey: "easypay",
		Name:        "EasyPay",
		Config: map[string]string{
			"pid":       "3001",
			"pkey":      "pkey-3001",
			"apiBase":   "https://pay.example.com",
			"notifyUrl": "https://merchant.example.com/notify",
			"returnUrl": "https://merchant.example.com/return",
	REDACTED,
		SupportedTypes: []string{"alipay"REDACTED,
		Enabled:        false,
REDACTED)
REDACTED

	_, err = svc.UpdateProviderInstance(ctx, instance.ID, UpdateProviderInstanceRequest{
		Enabled:        boolPtrValue(true),
		SupportedTypes: []string{"alipay", "wxpay"REDACTED,
REDACTED)
REDACTED

	saved, err := client.PaymentProviderInstance.Get(ctx, instance.ID)
REDACTED
	require.True(t, saved.Enabled)
	require.Equal(t, "alipay,wxpay", saved.SupportedTypes)
REDACTED

func TestUpdateProviderInstanceRejectsProtectedConfigChangesWhilePendingOrders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		providerKey   string
		createConfig  func(*testing.T) map[string]string
		supportedType []string
		updateConfig  map[string]string
		fieldName     string
		wantValue     string
REDACTED{
		{
			name:          "wxpay appId",
			providerKey:   payment.TypeWxpay,
			createConfig:  validWxpayProviderConfig,
			supportedType: []string{payment.TypeWxpayREDACTED,
			updateConfig:  map[string]string{"appId": "wx-app-updated"REDACTED,
			fieldName:     "appId",
			wantValue:     "wx-app-test",
	REDACTED,
		{
			name:          "wxpay mpAppId",
			providerKey:   payment.TypeWxpay,
			createConfig:  validWxpayProviderConfigWithJSAPIAppID,
			supportedType: []string{payment.TypeWxpayREDACTED,
			updateConfig:  map[string]string{"mpAppId": "wx-mp-app-updated"REDACTED,
			fieldName:     "mpAppId",
			wantValue:     "wx-mp-app-test",
	REDACTED,
		{
			name:          "wxpay mchId",
			providerKey:   payment.TypeWxpay,
			createConfig:  validWxpayProviderConfig,
			supportedType: []string{payment.TypeWxpayREDACTED,
			updateConfig:  map[string]string{"mchId": "mch-updated"REDACTED,
			fieldName:     "mchId",
			wantValue:     "mch-test",
	REDACTED,
		{
			name:          "wxpay publicKeyId",
			providerKey:   payment.TypeWxpay,
			createConfig:  validWxpayProviderConfig,
			supportedType: []string{payment.TypeWxpayREDACTED,
			updateConfig:  map[string]string{"publicKeyId": "public-key-id-updated"REDACTED,
			fieldName:     "publicKeyId",
			wantValue:     "public-key-id-test",
	REDACTED,
		{
			name:          "wxpay certSerial",
			providerKey:   payment.TypeWxpay,
			createConfig:  validWxpayProviderConfig,
			supportedType: []string{payment.TypeWxpayREDACTED,
			updateConfig:  map[string]string{"certSerial": "cert-serial-updated"REDACTED,
			fieldName:     "certSerial",
			wantValue:     "cert-serial-test",
	REDACTED,
		{
			name:          "alipay appId",
			providerKey:   payment.TypeAlipay,
			createConfig:  validAlipayProviderConfig,
			supportedType: []string{payment.TypeAlipayREDACTED,
			updateConfig:  map[string]string{"appId": "alipay-app-updated"REDACTED,
			fieldName:     "appId",
			wantValue:     "alipay-app-test",
	REDACTED,
		{
			name:          "easypay pid",
			providerKey:   payment.TypeEasyPay,
			createConfig:  validEasyPayProviderConfig,
			supportedType: []string{payment.TypeAlipayREDACTED,
			updateConfig:  map[string]string{"pid": "pid-updated"REDACTED,
			fieldName:     "pid",
			wantValue:     "pid-test",
	REDACTED,
		{
			name:          "stripe currency",
			providerKey:   payment.TypeStripe,
			createConfig:  validStripeProviderConfig,
			supportedType: []string{payment.TypeStripeREDACTED,
			updateConfig:  map[string]string{"currency": "HKD"REDACTED,
			fieldName:     "currency",
			wantValue:     "CNY",
	REDACTED,
		{
			name:          "airwallex accountId",
			providerKey:   payment.TypeAirwallex,
			createConfig:  validAirwallexProviderConfig,
			supportedType: []string{payment.TypeAirwallexREDACTED,
			updateConfig:  map[string]string{"accountId": "acct-updated"REDACTED,
			fieldName:     "accountId",
			wantValue:     "acct-test",
	REDACTED,
		{
			name:          "airwallex currency",
			providerKey:   payment.TypeAirwallex,
			createConfig:  validAirwallexProviderConfig,
			supportedType: []string{payment.TypeAirwallexREDACTED,
			updateConfig:  map[string]string{"currency": "HKD"REDACTED,
			fieldName:     "currency",
			wantValue:     "CNY",
	REDACTED,
		{
			name:          "airwallex webhookSecret",
			providerKey:   payment.TypeAirwallex,
			createConfig:  validAirwallexProviderConfig,
			supportedType: []string{payment.TypeAirwallexREDACTED,
			updateConfig:  map[string]string{"webhookSecret": "whsec-updated"REDACTED,
			fieldName:     "webhookSecret",
			wantValue:     "whsec-test",
	REDACTED,
REDACTED

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			svc := &PaymentConfigService{
				entClient:     client,
				encryptionKey: []byte("REDACTED"),
		REDACTED

			instance, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
				ProviderKey:    tc.providerKey,
				Name:           "protected-config-instance",
				Config:         tc.createConfig(t),
				SupportedTypes: tc.supportedType,
				Enabled:        true,
		REDACTED)
		REDACTED

			createPendingProviderConfigOrder(t, ctx, client, instance)

			updated, err := svc.UpdateProviderInstance(ctx, instance.ID, UpdateProviderInstanceRequest{
				Config: tc.updateConfig,
		REDACTED)
			require.Nil(t, updated)
		REDACTED
			require.Equal(t, "PENDING_ORDERS", infraerrors.Reason(err))

			saved, err := client.PaymentProviderInstance.Get(ctx, instance.ID)
		REDACTED
			cfg, err := svc.decryptConfig(saved.Config)
		REDACTED
			require.Equal(t, tc.wantValue, cfg[tc.fieldName])
	REDACTED)
REDACTED
REDACTED

func TestUpdateProviderInstanceAllowsSafeConfigChangesWhilePendingOrders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		providerKey   string
		createConfig  func(*testing.T) map[string]string
		supportedType []string
		updateConfig  map[string]string
		fieldName     string
		wantValue     string
REDACTED{
		{
			name:          "wxpay notifyUrl",
			providerKey:   payment.TypeWxpay,
			createConfig:  validWxpayProviderConfig,
			supportedType: []string{payment.TypeWxpayREDACTED,
			updateConfig:  map[string]string{"notifyUrl": "https://merchant.example.com/wxpay/notify-v2"REDACTED,
			fieldName:     "notifyUrl",
			wantValue:     "https://merchant.example.com/wxpay/notify-v2",
	REDACTED,
		{
			name:          "alipay same appId",
			providerKey:   payment.TypeAlipay,
			createConfig:  validAlipayProviderConfig,
			supportedType: []string{payment.TypeAlipayREDACTED,
			updateConfig:  map[string]string{"appId": "alipay-app-test"REDACTED,
			fieldName:     "appId",
			wantValue:     "alipay-app-test",
	REDACTED,
REDACTED

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			svc := &PaymentConfigService{
				entClient:     client,
				encryptionKey: []byte("REDACTED"),
		REDACTED

			instance, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
				ProviderKey:    tc.providerKey,
				Name:           "safe-config-instance",
				Config:         tc.createConfig(t),
				SupportedTypes: tc.supportedType,
				Enabled:        true,
		REDACTED)
		REDACTED

			createPendingProviderConfigOrder(t, ctx, client, instance)

			updated, err := svc.UpdateProviderInstance(ctx, instance.ID, UpdateProviderInstanceRequest{
				Config: tc.updateConfig,
		REDACTED)
		REDACTED
			require.NotNil(t, updated)

			saved, err := client.PaymentProviderInstance.Get(ctx, instance.ID)
		REDACTED
			cfg, err := svc.decryptConfig(saved.Config)
		REDACTED
			require.Equal(t, tc.wantValue, cfg[tc.fieldName])
	REDACTED)
REDACTED
REDACTED

func TestUpdateProviderInstanceClearsAirwallexAccountID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{
		entClient:     client,
		encryptionKey: []byte("REDACTED"),
REDACTED

	instance, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey:    payment.TypeAirwallex,
		Name:           "airwallex-clear-account",
		Config:         validAirwallexProviderConfig(t),
		SupportedTypes: []string{payment.TypeAirwallexREDACTED,
		Enabled:        true,
REDACTED)
REDACTED

	updated, err := svc.UpdateProviderInstance(ctx, instance.ID, UpdateProviderInstanceRequest{
		Config: map[string]string{"accountId": ""REDACTED,
REDACTED)
REDACTED
	require.NotNil(t, updated)

	saved, err := client.PaymentProviderInstance.Get(ctx, instance.ID)
REDACTED
	cfg, err := svc.decryptConfig(saved.Config)
REDACTED
	require.Empty(t, cfg["accountId"])
	require.Equal(t, "client-id-test", cfg["clientId"])
REDACTED

func createPendingProviderConfigOrder(t *testing.T, ctx context.Context, client *dbent.Client, instance *dbent.PaymentProviderInstance) {
REDACTED

	user, err := client.User.Create().
		SetEmail("provider-config-pending@example.com").
		SetPasswordHash("hash").
		SetUsername("provider-config-pending-user").
		Save(ctx)
REDACTED

	instanceID := strconv.FormatInt(instance.ID, 10)
	_, err = client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("PENDING-PROVIDER-CONFIG-" + instanceID).
		SetOutTradeNo("sub2_pending_provider_config_" + instanceID).
		SetPaymentType(providerPendingOrderPaymentType(instance.ProviderKey)).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instanceID).
		SetProviderKey(instance.ProviderKey).
		Save(ctx)
REDACTED
REDACTED

func providerPendingOrderPaymentType(providerKey string) string {
	switch providerKey {
	case payment.TypeWxpay:
		return payment.TypeWxpay
	case payment.TypeAlipay:
		return payment.TypeAlipay
	case payment.TypeAirwallex:
		return payment.TypeAirwallex
	case payment.TypeStripe:
		return payment.TypeStripe
	default:
		return payment.TypeAlipay
REDACTED
REDACTED

func validStripeProviderConfig(t *testing.T) map[string]string {
REDACTED

	return map[string]string{
		"secretKey":      "sk_test_123",
		"publishableKey": "pk_test_123",
		"webhookSecret":  "whsec-test",
		"currency":       "CNY",
REDACTED
REDACTED

func boolPtrValue(v bool) *bool {
	return &v
REDACTED

func validAlipayProviderConfig(t *testing.T) map[string]string {
REDACTED

	return map[string]string{
		"appId":      "alipay-app-test",
		"privateKey": "alipay-private-key-test",
		"notifyUrl":  "https://merchant.example.com/alipay/notify",
		"returnUrl":  "https://merchant.example.com/alipay/return",
REDACTED
REDACTED

func validEasyPayProviderConfig(t *testing.T) map[string]string {
REDACTED

	return map[string]string{
		"pid":       "pid-test",
		"pkey":      "pkey-test",
		"apiBase":   "https://pay.example.com",
		"notifyUrl": "https://merchant.example.com/easypay/notify",
		"returnUrl": "https://merchant.example.com/easypay/return",
REDACTED
REDACTED

func validAirwallexProviderConfig(t *testing.T) map[string]string {
REDACTED

	return map[string]string{
		"clientId":      "client-id-test",
		"apiKey":        "api-key-test",
		"webhookSecret": "whsec-test",
		"apiBase":       "https://api-demo.airwallex.com/api/v1",
		"accountId":     "acct-test",
		"currency":      "CNY",
REDACTED
REDACTED

func validWxpayProviderConfig(t *testing.T) map[string]string {
REDACTED

	key, err := rsa.GenerateKey(rand.Reader, 2048)
REDACTED

	privDER, err := x509.MarshalPKCS8PrivateKey(key)
REDACTED
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
REDACTED

	return map[string]string{
		"appId":       "wx-app-test",
		"mchId":       "mch-test",
		"privateKey":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDERREDACTED)),
		"apiV3Key":    "12345678901234567890123456789012",
		"publicKey":   string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDERREDACTED)),
		"publicKeyId": "public-key-id-test",
		"certSerial":  "cert-serial-test",
REDACTED
REDACTED

func validWxpayProviderConfigWithJSAPIAppID(t *testing.T) map[string]string {
REDACTED

	cfg := validWxpayProviderConfig(t)
	cfg["mpAppId"] = "wx-mp-app-test"
	return cfg
REDACTED
