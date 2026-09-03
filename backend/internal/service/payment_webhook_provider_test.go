//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

const webhookProviderTestEncryptionKey = "0123456789abcdef0123456789abcdef"

type webhookProviderTestDouble struct {
	key   string
	types []payment.PaymentType
}

func (p webhookProviderTestDouble) Name() string                          { return p.key }
func (p webhookProviderTestDouble) ProviderKey() string                   { return p.key }
func (p webhookProviderTestDouble) SupportedTypes() []payment.PaymentType { return p.types }
func (p webhookProviderTestDouble) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	panic("unexpected call")
}
func (p webhookProviderTestDouble) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	panic("unexpected call")
}
func (p webhookProviderTestDouble) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	panic("unexpected call")
}

func encryptWebhookProviderConfig(t *testing.T, config map[string]string) string {
	t.Helper()

	data, err := json.Marshal(config)
	require.NoError(t, err)

	encrypted, err := payment.Encrypt(string(data), []byte(webhookProviderTestEncryptionKey))
	require.NoError(t, err)
	return encrypted
}

func newWebhookProviderTestLoadBalancer(client *dbent.Client) payment.LoadBalancer {
	return payment.NewDefaultLoadBalancer(client, []byte(webhookProviderTestEncryptionKey))
}

func encryptValidWebhookSePayConfig(t *testing.T, suffix string) string {
	t.Helper()

	return encryptWebhookProviderConfig(t, map[string]string{
		"merchantId": "MERCHANT_" + suffix,
		"secretKey":  "sk_test_" + suffix,
		"env":        "sandbox",
		"currency":   "VND",
	})
}

func TestGetOrderProviderInstanceResolvesUniqueLegacyProviderKey(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("sepay-a").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{"secretKey": "sk_test_legacy_provider_key"})).
		SetSupportedTypes(payment.TypeSePayCard).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	providerKey := payment.TypeSePay
	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeSePay,
		ProviderKey: &providerKey,
	}

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
	}

	got, err := svc.getOrderProviderInstance(ctx, order)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, inst.ID, got.ID)
}

func TestGetOrderProviderInstanceLeavesAmbiguousLegacyOrderUnresolved(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("sepay-a").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeSePayNapas).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("sepay-a").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeSePayNapas).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeSePayNapas,
	}

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
	}

	got, err := svc.getOrderProviderInstance(ctx, order)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestGetOrderProviderInstanceLeavesLegacyProviderKeyUnresolvedWhenHistoricalInstancesConflict(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("sepay-disabled-legacy").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeSePayCard).
		SetEnabled(false).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("sepay-enabled-current").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeSePayCard).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	providerKey := payment.TypeSePay
	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeSePay,
		ProviderKey: &providerKey,
	}

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
	}

	got, err := svc.getOrderProviderInstance(ctx, order)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestGetOrderProviderInstanceUsesProviderSnapshotWhenPinnedColumnMissing(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("sepay-snapshot").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{"secretKey": "sk_snapshot"})).
		SetSupportedTypes(payment.TypeSePayCard).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order := &dbent.PaymentOrder{
		ID:          42,
		PaymentType: payment.TypeSePay,
		ProviderSnapshot: map[string]any{
			"schema_version":       1,
			"provider_instance_id": strconv.FormatInt(inst.ID, 10),
			"provider_key":         payment.TypeSePay,
		},
	}

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
	}

	got, err := svc.getOrderProviderInstance(ctx, order)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, inst.ID, got.ID)
}

func TestGetOrderProviderInstanceRejectsMissingSnapshotInstanceWithoutLegacyFallback(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("sepay-legacy-fallback").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{"secretKey": "sk_legacy"})).
		SetSupportedTypes(payment.TypeSePayCard).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order := &dbent.PaymentOrder{
		ID:          43,
		PaymentType: payment.TypeSePay,
		ProviderSnapshot: map[string]any{
			"schema_version":       1,
			"provider_instance_id": "999999",
			"provider_key":         payment.TypeSePay,
		},
	}

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
	}

	got, err := svc.getOrderProviderInstance(ctx, order)
	require.Nil(t, got)
	require.Error(t, err)
	require.Contains(t, err.Error(), "provider snapshot instance 999999 is missing")
}

func TestGetWebhookProviderRejectsAmbiguousRegistryFallbackForMultipleInstances(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	sepayConfigA := encryptValidWebhookSePayConfig(t, "a")
	sepayConfigB := encryptValidWebhookSePayConfig(t, "b")
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("sepay-a").
		SetConfig(sepayConfigA).
		SetSupportedTypes(payment.TypeSePayNapas).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("sepay-b").
		SetConfig(sepayConfigB).
		SetSupportedTypes(payment.TypeSePayNapas).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient:       client,
		loadBalancer:    newWebhookProviderTestLoadBalancer(client),
		registry:        payment.NewRegistry(),
		providersLoaded: true,
	}

	_, err = svc.GetWebhookProviders(ctx, payment.TypeSePay, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "ambiguous")
}

func TestGetWebhookProvidersRejectAmbiguousFallbackWithoutOrderReference(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("sepay-a").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeSePayBankTransfer).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("sepay-b").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeSePayBankTransfer).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient:       client,
		registry:        payment.NewRegistry(),
		providersLoaded: true,
	}

	_, err = svc.GetWebhookProviders(ctx, payment.TypeSePay, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "ambiguous")
}

func TestGetWebhookProviderAllowsSingleInstanceRegistryFallback(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("sepay-a").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeSePayCard).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	registry := payment.NewRegistry()
	registry.Register(webhookProviderTestDouble{
		key:   payment.TypeSePay,
		types: []payment.PaymentType{payment.TypeSePayCard},
	})

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		providersLoaded: true,
	}

	providers, err := svc.GetWebhookProviders(ctx, payment.TypeSePay, "")
	require.NoError(t, err)
	require.Len(t, providers, 1)
	prov := providers[0]
	require.Equal(t, payment.TypeSePay, prov.ProviderKey())
}

func TestGetWebhookProviderRejectsRegistryFallbackForPinnedOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("webhook@example.com").
		SetPasswordHash("hash").
		SetUsername("webhook").
		Save(ctx)
	require.NoError(t, err)

	pinnedInstanceID := "999"
	_, err = client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("TEST-RECHARGE").
		SetOutTradeNo("sub2_test_pinned_order").
		SetPaymentType(payment.TypeSePayNapas).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(pinnedInstanceID).
		Save(ctx)
	require.NoError(t, err)

	registry := payment.NewRegistry()
	registry.Register(webhookProviderTestDouble{
		key:   payment.TypeSePay,
		types: []payment.PaymentType{payment.TypeSePayNapas},
	})

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		providersLoaded: true,
	}

	_, err = svc.GetWebhookProviders(ctx, payment.TypeSePay, "sub2_test_pinned_order")
	require.Error(t, err)
	require.Contains(t, err.Error(), "provider instance")
}

func TestGetWebhookProviderUsesProviderSnapshotBeforeRegistryFallback(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("snapshot-webhook@example.com").
		SetPasswordHash("hash").
		SetUsername("snapshot-webhook").
		Save(ctx)
	require.NoError(t, err)

	sepayConfigA := encryptValidWebhookSePayConfig(t, "snapshot-a")
	sepayConfigB := encryptValidWebhookSePayConfig(t, "snapshot-b")
	instA, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("sepay-snapshot-a").
		SetConfig(sepayConfigA).
		SetSupportedTypes(payment.TypeSePayNapas).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeSePay).
		SetName("sepay-snapshot-b").
		SetConfig(sepayConfigB).
		SetSupportedTypes(payment.TypeSePayNapas).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(66).
		SetPayAmount(66).
		SetFeeRate(0).
		SetRechargeCode("SNAPSHOT-WEBHOOK").
		SetOutTradeNo("sub2_test_snapshot_webhook_order").
		SetPaymentType(payment.TypeSePayNapas).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderSnapshot(map[string]any{
			"schema_version":       1,
			"provider_instance_id": strconv.FormatInt(instA.ID, 10),
			"provider_key":         payment.TypeSePay,
			"payment_mode":         "native",
		}).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient:       client,
		loadBalancer:    newWebhookProviderTestLoadBalancer(client),
		registry:        payment.NewRegistry(),
		providersLoaded: true,
	}

	providers, err := svc.GetWebhookProviders(ctx, payment.TypeSePay, "sub2_test_snapshot_webhook_order")
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.Equal(t, payment.TypeSePay, providers[0].ProviderKey())
}
