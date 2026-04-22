//go:build unit

package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

const webhookProviderTestEncryptionKey = "REDACTED"

type webhookProviderTestDouble struct {
	key   string
	types []payment.PaymentType
REDACTED

func (p webhookProviderTestDouble) Name() string                          { return p.key REDACTED
func (p webhookProviderTestDouble) ProviderKey() string                   { return p.key REDACTED
func (p webhookProviderTestDouble) SupportedTypes() []payment.PaymentType { return p.types REDACTED
func (p webhookProviderTestDouble) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	panic("unexpected call")
REDACTED
func (p webhookProviderTestDouble) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	panic("unexpected call")
REDACTED
func (p webhookProviderTestDouble) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	panic("unexpected call")
REDACTED
func (p webhookProviderTestDouble) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	panic("unexpected call")
REDACTED

func encryptWebhookProviderConfig(t *testing.T, config map[string]string) string {
REDACTED

	data, err := json.Marshal(config)
REDACTED

	encrypted, err := payment.Encrypt(string(data), []byte(webhookProviderTestEncryptionKey))
REDACTED
	return encrypted
REDACTED

func newWebhookProviderTestLoadBalancer(client *dbent.Client) payment.LoadBalancer {
	return payment.NewDefaultLoadBalancer(client, []byte(webhookProviderTestEncryptionKey))
REDACTED

func encryptValidWebhookWxpayConfig(t *testing.T, suffix string) string {
REDACTED

	key, err := rsa.GenerateKey(rand.Reader, 2048)
REDACTED

	privDER, err := x509.MarshalPKCS8PrivateKey(key)
REDACTED
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
REDACTED

	return encryptWebhookProviderConfig(t, map[string]string{
		"appId":       "wx-app-" + suffix,
		"mchId":       "mch-" + suffix,
		"privateKey":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDERREDACTED)),
		"apiV3Key":    webhookProviderTestEncryptionKey,
		"publicKey":   string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDERREDACTED)),
		"publicKeyId": "public-key-id-" + suffix,
		"certSerial":  "cert-serial-" + suffix,
REDACTED)
REDACTED

func TestGetOrderProviderInstanceResolvesUniqueLegacyProviderKey(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-a").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{"secretKey": "sk_test_legacy_provider_key"REDACTED)).
		SetSupportedTypes("stripe").
		SetEnabled(true).
		Save(ctx)
REDACTED

	providerKey := payment.TypeStripe
	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeStripe,
		ProviderKey: &providerKey,
REDACTED

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
REDACTED

	got, err := svc.getOrderProviderInstance(ctx, order)
REDACTED
	require.NotNil(t, got)
	require.Equal(t, inst.ID, got.ID)
REDACTED

func TestGetOrderProviderInstanceResolvesUniqueLegacyPaymentType(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("wxpay-a").
		SetConfig("{REDACTED").
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		Save(ctx)
REDACTED

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeWxpayDirect,
REDACTED

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
REDACTED

	got, err := svc.getOrderProviderInstance(ctx, order)
REDACTED
	require.NotNil(t, got)
	require.Equal(t, inst.ID, got.ID)
REDACTED

func TestGetOrderProviderInstanceLeavesAmbiguousLegacyOrderUnresolved(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeEasyPay).
		SetName("easypay-a").
		SetConfig("{REDACTED").
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		Save(ctx)
REDACTED
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("wxpay-a").
		SetConfig("{REDACTED").
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		Save(ctx)
REDACTED

	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeWxpay,
REDACTED

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
REDACTED

	got, err := svc.getOrderProviderInstance(ctx, order)
REDACTED
	require.Nil(t, got)
REDACTED

func TestGetOrderProviderInstanceLeavesLegacyProviderKeyUnresolvedWhenHistoricalInstancesConflict(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-disabled-legacy").
		SetConfig("{REDACTED").
		SetSupportedTypes("stripe").
		SetEnabled(false).
		Save(ctx)
REDACTED
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-enabled-current").
		SetConfig("{REDACTED").
		SetSupportedTypes("stripe").
		SetEnabled(true).
		Save(ctx)
REDACTED

	providerKey := payment.TypeStripe
	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeStripe,
		ProviderKey: &providerKey,
REDACTED

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
REDACTED

	got, err := svc.getOrderProviderInstance(ctx, order)
REDACTED
	require.Nil(t, got)
REDACTED

func TestGetOrderProviderInstanceLeavesProviderKeyMatchUnresolvedWhenTypeNotSupported(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("wxpay-only").
		SetConfig("{REDACTED").
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		Save(ctx)
REDACTED

	providerKey := payment.TypeWxpay
	order := &dbent.PaymentOrder{
		PaymentType: payment.TypeAlipayDirect,
		ProviderKey: &providerKey,
REDACTED

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
REDACTED

	got, err := svc.getOrderProviderInstance(ctx, order)
REDACTED
	require.Nil(t, got)
REDACTED

func TestGetOrderProviderInstanceUsesProviderSnapshotWhenPinnedColumnMissing(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-snapshot").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{"secretKey": "sk_snapshot"REDACTED)).
		SetSupportedTypes("stripe").
		SetEnabled(true).
		Save(ctx)
REDACTED

	order := &dbent.PaymentOrder{
		ID:          42,
		PaymentType: payment.TypeStripe,
		ProviderSnapshot: map[string]any{
			"schema_version":       1,
			"provider_instance_id": strconv.FormatInt(inst.ID, 10),
			"provider_key":         payment.TypeStripe,
	REDACTED,
REDACTED

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
REDACTED

	got, err := svc.getOrderProviderInstance(ctx, order)
REDACTED
	require.NotNil(t, got)
	require.Equal(t, inst.ID, got.ID)
REDACTED

func TestGetOrderProviderInstanceRejectsMissingSnapshotInstanceWithoutLegacyFallback(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-legacy-fallback").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{"secretKey": "sk_legacy"REDACTED)).
		SetSupportedTypes("stripe").
		SetEnabled(true).
		Save(ctx)
REDACTED

	order := &dbent.PaymentOrder{
		ID:          43,
		PaymentType: payment.TypeStripe,
		ProviderSnapshot: map[string]any{
			"schema_version":       1,
			"provider_instance_id": "999999",
			"provider_key":         payment.TypeStripe,
	REDACTED,
REDACTED

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
REDACTED

	got, err := svc.getOrderProviderInstance(ctx, order)
	require.Nil(t, got)
REDACTED
	require.Contains(t, err.Error(), "provider snapshot instance 999999 is missing")
REDACTED

func TestGetWebhookProviderRejectsAmbiguousRegistryFallback(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	wxpayConfigA := encryptValidWebhookWxpayConfig(t, "a")
	wxpayConfigB := encryptValidWebhookWxpayConfig(t, "b")
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("wxpay-a").
		SetConfig(wxpayConfigA).
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		Save(ctx)
REDACTED
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("wxpay-b").
		SetConfig(wxpayConfigB).
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		Save(ctx)
REDACTED

	svc := &PaymentService{
		entClient:       client,
		loadBalancer:    newWebhookProviderTestLoadBalancer(client),
		registry:        payment.NewRegistry(),
		providersLoaded: true,
REDACTED

	providers, err := svc.GetWebhookProviders(ctx, payment.TypeWxpay, "")
REDACTED
	require.Len(t, providers, 2)
REDACTED

func TestGetWebhookProvidersRejectAmbiguousFallbackForNonWxpay(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-a").
		SetConfig("{REDACTED").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		Save(ctx)
REDACTED
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-b").
		SetConfig("{REDACTED").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		Save(ctx)
REDACTED

	svc := &PaymentService{
		entClient:       client,
		registry:        payment.NewRegistry(),
		providersLoaded: true,
REDACTED

	_, err = svc.GetWebhookProviders(ctx, payment.TypeAlipay, "")
REDACTED
	require.Contains(t, err.Error(), "ambiguous")
REDACTED

func TestGetWebhookProviderAllowsSingleInstanceRegistryFallback(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName("stripe-a").
		SetConfig("{REDACTED").
		SetSupportedTypes("stripe").
		SetEnabled(true).
		Save(ctx)
REDACTED

	registry := payment.NewRegistry()
	registry.Register(webhookProviderTestDouble{
		key:   payment.TypeStripe,
		types: []payment.PaymentType{payment.TypeStripeREDACTED,
REDACTED)

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		providersLoaded: true,
REDACTED

	providers, err := svc.GetWebhookProviders(ctx, payment.TypeStripe, "")
REDACTED
	require.Len(t, providers, 1)
	prov := providers[0]
	require.Equal(t, payment.TypeStripe, prov.ProviderKey())
REDACTED

func TestGetWebhookProviderRejectsRegistryFallbackForPinnedOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("webhook@example.com").
		SetPasswordHash("hash").
		SetUsername("webhook").
		Save(ctx)
REDACTED

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
		SetPaymentType(payment.TypeWxpay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(pinnedInstanceID).
		Save(ctx)
REDACTED

	registry := payment.NewRegistry()
	registry.Register(webhookProviderTestDouble{
		key:   payment.TypeWxpay,
		types: []payment.PaymentType{payment.TypeWxpayREDACTED,
REDACTED)

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		providersLoaded: true,
REDACTED

	_, err = svc.GetWebhookProviders(ctx, payment.TypeWxpay, "sub2_test_pinned_order")
REDACTED
	require.Contains(t, err.Error(), "provider instance")
REDACTED

func TestGetWebhookProviderUsesProviderSnapshotBeforeWxpayFallback(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("snapshot-webhook@example.com").
		SetPasswordHash("hash").
		SetUsername("snapshot-webhook").
		Save(ctx)
REDACTED

	wxpayConfigA := encryptValidWebhookWxpayConfig(t, "snapshot-a")
	wxpayConfigB := encryptValidWebhookWxpayConfig(t, "snapshot-b")
	instA, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("wxpay-snapshot-a").
		SetConfig(wxpayConfigA).
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		Save(ctx)
REDACTED
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("wxpay-snapshot-b").
		SetConfig(wxpayConfigB).
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		Save(ctx)
REDACTED

	_, err = client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(66).
		SetPayAmount(66).
		SetFeeRate(0).
		SetRechargeCode("SNAPSHOT-WEBHOOK").
		SetOutTradeNo("sub2_test_snapshot_webhook_order").
		SetPaymentType(payment.TypeWxpay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderSnapshot(map[string]any{
			"schema_version":       1,
			"provider_instance_id": strconv.FormatInt(instA.ID, 10),
			"provider_key":         payment.TypeWxpay,
			"payment_mode":         "native",
	REDACTED).
		Save(ctx)
REDACTED

	svc := &PaymentService{
		entClient:       client,
		loadBalancer:    newWebhookProviderTestLoadBalancer(client),
		registry:        payment.NewRegistry(),
		providersLoaded: true,
REDACTED

	providers, err := svc.GetWebhookProviders(ctx, payment.TypeWxpay, "sub2_test_snapshot_webhook_order")
REDACTED
	require.Len(t, providers, 1)
	require.Equal(t, payment.TypeWxpay, providers[0].ProviderKey())
REDACTED
