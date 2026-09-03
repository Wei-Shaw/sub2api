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
		{name: "sepay instance", providerKey: payment.TypeSePay, instName: "SePay", types: payment.TypeSePayBankTransfer},
		{name: "empty supported types is allowed", providerKey: payment.TypeSePay, instName: "SePay", types: ""},
		{name: "blank name is rejected", providerKey: payment.TypeSePay, instName: "  ", types: payment.TypeSePayCard, wantErr: true},
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

	// The merchant secret is the only credential a SePay instance holds; it
	// must never be echoed back by the admin GET API. Everything else is
	// identity configuration the admin needs to see in order to edit the instance.
	assert.True(t, isSensitiveProviderConfigField(payment.TypeSePay, "secretKey"))
	assert.True(t, isSensitiveProviderConfigField(payment.TypeSePay, "SECRETKEY"))
	assert.False(t, isSensitiveProviderConfigField(payment.TypeSePay, "merchantId"))
	assert.False(t, isSensitiveProviderConfigField(payment.TypeSePay, "env"))
	assert.False(t, isSensitiveProviderConfigField(payment.TypeSePay, "currency"))
	assert.False(t, isSensitiveProviderConfigField("unknown", "secretKey"))
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
		ProviderKey:    payment.TypeSePay,
		Name:           "sepay-instance",
		Config:         validSePayProviderConfig(t),
		SupportedTypes: []string{payment.TypeSePayBankTransfer},
		Enabled:        false,
	})
	require.NoError(t, err)

	updated, err := svc.UpdateProviderInstance(ctx, int64(instance.ID), UpdateProviderInstanceRequest{
		Enabled:        boolPtrValue(true),
		SupportedTypes: []string{payment.TypeSePayBankTransfer, payment.TypeSePayCard},
	})
	require.NoError(t, err)
	assert.True(t, updated.Enabled)
	assert.Equal(t, payment.TypeSePayBankTransfer+","+payment.TypeSePayCard, updated.SupportedTypes)
}

func TestUpdateProviderInstanceRejectsProtectedConfigChangesWhilePendingOrders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		updateConfig map[string]string
		fieldName    string
		wantValue    string
	}{
		{name: "merchantId", updateConfig: map[string]string{"merchantId": "MERCHANT_UPDATED"}, fieldName: "merchantId", wantValue: "MERCHANT_TEST"},
		{name: "secretKey", updateConfig: map[string]string{"secretKey": "sk_test_updated"}, fieldName: "secretKey", wantValue: "sk_test_123"},
		{name: "env", updateConfig: map[string]string{"env": "production"}, fieldName: "env", wantValue: "sandbox"},
		{name: "currency", updateConfig: map[string]string{"currency": "USD"}, fieldName: "currency", wantValue: "VND"},
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
				ProviderKey:    payment.TypeSePay,
				Name:           "protected-config-instance",
				Config:         validSePayProviderConfig(t),
				SupportedTypes: []string{payment.TypeSePayBankTransfer},
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
		ProviderKey:    payment.TypeSePay,
		Name:           "safe-config-instance",
		Config:         validSePayProviderConfig(t),
		SupportedTypes: []string{payment.TypeSePayBankTransfer},
		Enabled:        true,
	})
	require.NoError(t, err)
	createPendingProviderConfigOrder(t, ctx, client, instance)

	// notifyUrl is not part of the merchant identity, so it stays editable even
	// while the instance still has orders in flight.
	updated, err := svc.UpdateProviderInstance(ctx, int64(instance.ID), UpdateProviderInstanceRequest{
		Config: map[string]string{"notifyUrl": "https://merchant.example.com/sepay/notify"},
	})
	require.NoError(t, err)

	stored, err := svc.decryptConfig(updated.Config)
	require.NoError(t, err)
	assert.Equal(t, "https://merchant.example.com/sepay/notify", stored["notifyUrl"])
	assert.Equal(t, "MERCHANT_TEST", stored["merchantId"])
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
		ProviderKey:    payment.TypeSePay,
		Name:           "masked-instance",
		Config:         validSePayProviderConfig(t),
		SupportedTypes: []string{payment.TypeSePayBankTransfer},
		Enabled:        true,
	})
	require.NoError(t, err)

	instances, err := svc.ListProviderInstancesWithConfig(ctx)
	require.NoError(t, err)
	require.Len(t, instances, 1)
	_, hasSecret := instances[0].Config["secretKey"]
	assert.False(t, hasSecret, "secretKey must not leave the server")
	assert.Equal(t, "MERCHANT_TEST", instances[0].Config["merchantId"])
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
	if providerKey == payment.TypeSePay {
		return payment.TypeSePayBankTransfer
	}
	return providerKey
}

func validSePayProviderConfig(t *testing.T) map[string]string {
	t.Helper()

	return map[string]string{
		"merchantId": "MERCHANT_TEST",
		"secretKey":  "sk_test_123",
		"env":        "sandbox",
		"currency":   "VND",
	}
}

func boolPtrValue(v bool) *bool {
	return &v
}
