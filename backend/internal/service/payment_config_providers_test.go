//go:build unit

package service

import (
	"context"
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
		name        string
		providerKey string
		instName    string
		types       string
		wantErr     bool
	}{
		{name: "gpmpay instance", providerKey: payment.TypeGPMPay, instName: "GPM Pay", types: payment.TypeGPMPayBankTransfer},
		{name: "nowpayments instance", providerKey: payment.TypeNowPayments, instName: "NOWPayments", types: payment.TypeNowPaymentsCrypto},
		{name: "empty supported types is allowed", providerKey: payment.TypeGPMPay, instName: "GPM Pay", types: ""},
		{name: "blank name is rejected", providerKey: payment.TypeGPMPay, instName: "  ", types: payment.TypeGPMPayBankTransfer, wantErr: true},
		{name: "removed sepay provider key is rejected", providerKey: "sepay", instName: "SePay", types: "sepay_bank_transfer", wantErr: true},
		{name: "removed provider key is rejected", providerKey: "stripe", instName: "Stripe", types: "stripe", wantErr: true},
		{name: "unknown provider key is rejected", providerKey: "nosuchgateway", instName: "X", types: "", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateProviderRequest(tc.providerKey, tc.instName, tc.types)
			if tc.wantErr {
				require.Error(t, err)
				assert.Equal(t, "VALIDATION_ERROR", infraerrors.Reason(err))
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestIsSensitiveProviderConfigField(t *testing.T) {
	t.Parallel()

	// The API token and webhook secret are the credentials a GPM Pay instance
	// holds; they must never be echoed back by the admin GET API. Everything
	// else is identity configuration the admin needs to see in order to edit
	// the instance.
	assert.True(t, isSensitiveProviderConfigField(payment.TypeGPMPay, "apiToken"))
	assert.True(t, isSensitiveProviderConfigField(payment.TypeGPMPay, "WEBHOOKSECRET"))
	assert.False(t, isSensitiveProviderConfigField(payment.TypeGPMPay, "bankBin"))
	assert.False(t, isSensitiveProviderConfigField(payment.TypeGPMPay, "accountNumber"))
	assert.True(t, isSensitiveProviderConfigField(payment.TypeNowPayments, "ipnSecretKey"))
	assert.False(t, isSensitiveProviderConfigField(payment.TypeNowPayments, "currency"))
	assert.False(t, isSensitiveProviderConfigField("unknown", "apiToken"))
}

func TestUpdateProviderInstancePersistsEnabledAndSupportedTypes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{
		entClient:     client,
		encryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	}

	instance, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey:    payment.TypeGPMPay,
		Name:           "gpmpay-instance",
		Config:         validGPMPayProviderConfig(t),
		SupportedTypes: []string{},
		Enabled:        false,
	})
	require.NoError(t, err)

	updated, err := svc.UpdateProviderInstance(ctx, int64(instance.ID), UpdateProviderInstanceRequest{
		Enabled:        boolPtrValue(true),
		SupportedTypes: []string{payment.TypeGPMPayBankTransfer},
	})
	require.NoError(t, err)
	assert.True(t, updated.Enabled)
	assert.Equal(t, payment.TypeGPMPayBankTransfer, updated.SupportedTypes)
}

func TestUpdateProviderInstanceRejectsProtectedConfigChangesWhilePendingOrders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		updateConfig map[string]string
		fieldName    string
		wantValue    string
	}{
		{name: "webhookSecret", updateConfig: map[string]string{"webhookSecret": "whsec_updated"}, fieldName: "webhookSecret", wantValue: "whsec_test_123"},
		{name: "bankBin", updateConfig: map[string]string{"bankBin": "970436"}, fieldName: "bankBin", wantValue: "970422"},
		{name: "accountNumber", updateConfig: map[string]string{"accountNumber": "9999999999"}, fieldName: "accountNumber", wantValue: "0123456789"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			svc := &PaymentConfigService{
				entClient:     client,
				encryptionKey: []byte("0123456789abcdef0123456789abcdef"),
			}

			instance, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
				ProviderKey:    payment.TypeGPMPay,
				Name:           "protected-config-instance",
				Config:         validGPMPayProviderConfig(t),
				SupportedTypes: []string{payment.TypeGPMPayBankTransfer},
				Enabled:        true,
			})
			require.NoError(t, err)
			createPendingProviderConfigOrder(t, ctx, client, instance)

			_, err = svc.UpdateProviderInstance(ctx, int64(instance.ID), UpdateProviderInstanceRequest{
				Config: tc.updateConfig,
			})
			require.Error(t, err)
			assert.Equal(t, "PENDING_ORDERS", infraerrors.Reason(err))

			reloaded, err := client.PaymentProviderInstance.Get(ctx, int64(instance.ID))
			require.NoError(t, err)
			stored, err := svc.decryptConfig(reloaded.Config)
			require.NoError(t, err)
			assert.Equal(t, tc.wantValue, stored[tc.fieldName])
		})
	}
}

func TestUpdateProviderInstanceAllowsSafeConfigChangesWhilePendingOrders(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{
		entClient:     client,
		encryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	}

	instance, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey:    payment.TypeGPMPay,
		Name:           "safe-config-instance",
		Config:         validGPMPayProviderConfig(t),
		SupportedTypes: []string{payment.TypeGPMPayBankTransfer},
		Enabled:        true,
	})
	require.NoError(t, err)
	createPendingProviderConfigOrder(t, ctx, client, instance)

	// notifyUrl is not part of the merchant identity, so it stays editable even
	// while the instance still has orders in flight.
	updated, err := svc.UpdateProviderInstance(ctx, int64(instance.ID), UpdateProviderInstanceRequest{
		Config: map[string]string{"notifyUrl": "https://merchant.example.com/gpmpay/notify"},
	})
	require.NoError(t, err)

	stored, err := svc.decryptConfig(updated.Config)
	require.NoError(t, err)
	assert.Equal(t, "https://merchant.example.com/gpmpay/notify", stored["notifyUrl"])
	assert.Equal(t, "0123456789", stored["accountNumber"])
}

func TestListProviderInstancesWithConfigMasksSecretKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentConfigService{
		entClient:     client,
		encryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	}

	_, err := svc.CreateProviderInstance(ctx, CreateProviderInstanceRequest{
		ProviderKey:    payment.TypeGPMPay,
		Name:           "masked-instance",
		Config:         validGPMPayProviderConfig(t),
		SupportedTypes: []string{payment.TypeGPMPayBankTransfer},
		Enabled:        true,
	})
	require.NoError(t, err)

	instances, err := svc.ListProviderInstancesWithConfig(ctx)
	require.NoError(t, err)
	require.Len(t, instances, 1)
	_, hasToken := instances[0].Config["apiToken"]
	assert.False(t, hasToken, "apiToken must not leave the server")
	_, hasSecret := instances[0].Config["webhookSecret"]
	assert.False(t, hasSecret, "webhookSecret must not leave the server")
	assert.Equal(t, "0123456789", instances[0].Config["accountNumber"])
}

func createPendingProviderConfigOrder(t *testing.T, ctx context.Context, client *dbent.Client, instance *dbent.PaymentProviderInstance) {
	t.Helper()

	// payment_orders.user_id is a real foreign key, so the order needs an
	// actual user row rather than a hard-coded id.
	user, err := client.User.Create().
		SetEmail("provider-config-pending@example.com").
		SetPasswordHash("hash").
		SetUsername("provider-config-pending-user").
		Save(ctx)
	require.NoError(t, err)

	instanceID := strconv.FormatInt(int64(instance.ID), 10)
	_, err = client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
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
	require.NoError(t, err)
}

func providerPendingOrderPaymentType(providerKey string) string {
	if providerKey == payment.TypeGPMPay {
		return payment.TypeGPMPayBankTransfer
	}
	return providerKey
}

func validGPMPayProviderConfig(t *testing.T) map[string]string {
	t.Helper()

	return map[string]string{
		"apiToken":      "gpm_token_123",
		"webhookSecret": "whsec_test_123",
		"bankBin":       "970422",
		"accountNumber": "0123456789",
	}
}

func boolPtrValue(v bool) *bool {
	return &v
}

func TestEveryShippedGatewayCanBeAdded(t *testing.T) {
	t.Parallel()

	// A gateway that registers, routes and signs correctly but cannot be added
	// in the admin UI is indistinguishable from a gateway that does not exist.
	// This is what a second hand-maintained list of provider keys buys you, so
	// pin the two config maps to the same source of truth as the validator.
	for _, providerKey := range []string{payment.TypeGPMPay, payment.TypeNowPayments} {
		require.True(t, payment.IsProviderKey(providerKey), providerKey)
		require.NoError(t, validateProviderRequest(providerKey, "Instance", ""), providerKey)
		assert.NotEmpty(t, providerSensitiveConfigFields[providerKey], providerKey)
		assert.NotEmpty(t, providerPendingOrderProtectedConfigFields[providerKey], providerKey)
	}

	assert.False(t, payment.IsProviderKey("stripe"))
	assert.False(t, payment.IsProviderKey("sepay"))
	// A payment method is not a gateway: "gpmpay_bank_transfer" must never pass
	// as a provider key just because it starts with one.
	assert.False(t, payment.IsProviderKey(payment.TypeGPMPayBankTransfer))
}
