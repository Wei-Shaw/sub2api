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

func encryptValidWebhookGPMPayConfig(t *testing.T, suffix string) string {
	t.Helper()

	return encryptWebhookProviderConfig(t, map[string]string{
		"apiToken":      "gpm_token_" + suffix,
		"webhookSecret": "whsec_" + suffix,
		"bankBin":       "970422",
		"accountNumber": "0123456789",
	})
}

func TestGetOrderProviderInstanceResolvesUniqueLegacyProviderKey(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeGPMPay).
		SetName("gpmpay-a").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{"secretKey": "sk_test_legacy_provider_key"})).
		SetSupportedTypes(payment.TypeGPMPayBankTransfer).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	providerKey := payment.TypeGPMPay
	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeGPMPay,
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
		SetProviderKey(payment.TypeGPMPay).
		SetName("gpmpay-a").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeGPMPayBankTransfer).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeGPMPay).
		SetName("gpmpay-a").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeGPMPayBankTransfer).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeGPMPayBankTransfer,
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
		SetProviderKey(payment.TypeGPMPay).
		SetName("gpmpay-disabled-legacy").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeGPMPayBankTransfer).
		SetEnabled(false).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeGPMPay).
		SetName("gpmpay-enabled-current").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeGPMPayBankTransfer).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	providerKey := payment.TypeGPMPay
	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeGPMPay,
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
		SetProviderKey(payment.TypeGPMPay).
		SetName("gpmpay-snapshot").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{"secretKey": "sk_snapshot"})).
		SetSupportedTypes(payment.TypeGPMPayBankTransfer).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order := &dbent.PaymentOrder{
		ID:          42,
		PaymentType: payment.TypeGPMPay,
		ProviderSnapshot: map[string]any{
			"schema_version":       1,
			"provider_instance_id": strconv.FormatInt(inst.ID, 10),
			"provider_key":         payment.TypeGPMPay,
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
		SetProviderKey(payment.TypeGPMPay).
		SetName("gpmpay-legacy-fallback").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{"secretKey": "sk_legacy"})).
		SetSupportedTypes(payment.TypeGPMPayBankTransfer).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order := &dbent.PaymentOrder{
		ID:          43,
		PaymentType: payment.TypeGPMPay,
		ProviderSnapshot: map[string]any{
			"schema_version":       1,
			"provider_instance_id": "999999",
			"provider_key":         payment.TypeGPMPay,
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
	gpmpayConfigA := encryptValidWebhookGPMPayConfig(t, "a")
	gpmpayConfigB := encryptValidWebhookGPMPayConfig(t, "b")
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeGPMPay).
		SetName("gpmpay-a").
		SetConfig(gpmpayConfigA).
		SetSupportedTypes(payment.TypeGPMPayBankTransfer).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeGPMPay).
		SetName("gpmpay-b").
		SetConfig(gpmpayConfigB).
		SetSupportedTypes(payment.TypeGPMPayBankTransfer).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient:       client,
		loadBalancer:    newWebhookProviderTestLoadBalancer(client),
		registry:        payment.NewRegistry(),
		providersLoaded: true,
	}

	_, err = svc.GetWebhookProviders(ctx, payment.TypeGPMPay, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "ambiguous")
}

func TestGetWebhookProvidersRejectAmbiguousFallbackWithoutOrderReference(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeGPMPay).
		SetName("gpmpay-a").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeGPMPayBankTransfer).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeGPMPay).
		SetName("gpmpay-b").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeGPMPayBankTransfer).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient:       client,
		registry:        payment.NewRegistry(),
		providersLoaded: true,
	}

	_, err = svc.GetWebhookProviders(ctx, payment.TypeGPMPay, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "ambiguous")
}

func TestGetWebhookProviderAllowsSingleInstanceRegistryFallback(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeGPMPay).
		SetName("gpmpay-a").
		SetConfig("{}").
		SetSupportedTypes(payment.TypeGPMPayBankTransfer).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	registry := payment.NewRegistry()
	registry.Register(webhookProviderTestDouble{
		key:   payment.TypeGPMPay,
		types: []payment.PaymentType{payment.TypeGPMPayBankTransfer},
	})

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		providersLoaded: true,
	}

	providers, err := svc.GetWebhookProviders(ctx, payment.TypeGPMPay, "")
	require.NoError(t, err)
	require.Len(t, providers, 1)
	prov := providers[0]
	require.Equal(t, payment.TypeGPMPay, prov.ProviderKey())
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
		SetPaymentType(payment.TypeGPMPayBankTransfer).
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
		key:   payment.TypeGPMPay,
		types: []payment.PaymentType{payment.TypeGPMPayBankTransfer},
	})

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		providersLoaded: true,
	}

	_, err = svc.GetWebhookProviders(ctx, payment.TypeGPMPay, "sub2_test_pinned_order")
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

	gpmpayConfigA := encryptValidWebhookGPMPayConfig(t, "snapshot-a")
	gpmpayConfigB := encryptValidWebhookGPMPayConfig(t, "snapshot-b")
	instA, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeGPMPay).
		SetName("gpmpay-snapshot-a").
		SetConfig(gpmpayConfigA).
		SetSupportedTypes(payment.TypeGPMPayBankTransfer).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeGPMPay).
		SetName("gpmpay-snapshot-b").
		SetConfig(gpmpayConfigB).
		SetSupportedTypes(payment.TypeGPMPayBankTransfer).
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
		SetPaymentType(payment.TypeGPMPayBankTransfer).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderSnapshot(map[string]any{
			"schema_version":       1,
			"provider_instance_id": strconv.FormatInt(instA.ID, 10),
			"provider_key":         payment.TypeGPMPay,
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

	providers, err := svc.GetWebhookProviders(ctx, payment.TypeGPMPay, "sub2_test_snapshot_webhook_order")
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.Equal(t, payment.TypeGPMPay, providers[0].ProviderKey())
}
