//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

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

func TestGetWebhookProviderRejectsAmbiguousRegistryFallback(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("wxpay-a").
		SetConfig("{REDACTED").
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		Save(ctx)
REDACTED
	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeWxpay).
		SetName("wxpay-b").
		SetConfig("{REDACTED").
		SetSupportedTypes("wxpay").
		SetEnabled(true).
		Save(ctx)
REDACTED

	svc := &PaymentService{
		entClient:       client,
		registry:        payment.NewRegistry(),
		providersLoaded: true,
REDACTED

	_, err = svc.GetWebhookProvider(ctx, payment.TypeWxpay, "")
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

	prov, err := svc.GetWebhookProvider(ctx, payment.TypeStripe, "")
REDACTED
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

	_, err = svc.GetWebhookProvider(ctx, payment.TypeWxpay, "sub2_test_pinned_order")
REDACTED
	require.Contains(t, err.Error(), "provider instance")
REDACTED
