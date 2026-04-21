//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type paymentResumeLookupProvider struct {
	queryCount int
REDACTED

func (p *paymentResumeLookupProvider) Name() string { return "resume-lookup-provider" REDACTED

func (p *paymentResumeLookupProvider) ProviderKey() string { return payment.TypeAlipay REDACTED

func (p *paymentResumeLookupProvider) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeAlipayREDACTED
REDACTED

func (p *paymentResumeLookupProvider) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	panic("unexpected call")
REDACTED

func (p *paymentResumeLookupProvider) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	p.queryCount++
	return &payment.QueryOrderResponse{Status: payment.ProviderStatusPendingREDACTED, nil
REDACTED

func (p *paymentResumeLookupProvider) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	panic("unexpected call")
REDACTED

func (p *paymentResumeLookupProvider) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	panic("unexpected call")
REDACTED

func TestGetPublicOrderByResumeTokenReturnsMatchingOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("resume@example.com").
		SetPasswordHash("hash").
		SetUsername("resume-user").
		Save(ctx)
REDACTED

	instanceID := "12"
	providerKey := payment.TypeEasyPay
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("RESUME-ORDER").
		SetOutTradeNo("sub2_resume_lookup").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-1").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instanceID).
		SetProviderKey(providerKey).
		Save(ctx)
REDACTED

	resumeSvc := NewPaymentResumeService([]byte("REDACTED"))
	token, err := resumeSvc.CreateToken(ResumeTokenClaims{
		OrderID:            order.ID,
		UserID:             user.ID,
		ProviderInstanceID: instanceID,
		ProviderKey:        providerKey,
		PaymentType:        payment.TypeAlipay,
		CanonicalReturnURL: "https://app.example.com/payment/result",
REDACTED)
REDACTED

	svc := &PaymentService{
		entClient:     client,
		resumeService: resumeSvc,
REDACTED

	got, err := svc.GetPublicOrderByResumeToken(ctx, token)
REDACTED
	require.Equal(t, order.ID, got.ID)
REDACTED

func TestGetPublicOrderByResumeTokenRejectsSnapshotMismatch(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("resume-mismatch@example.com").
		SetPasswordHash("hash").
		SetUsername("resume-mismatch-user").
		Save(ctx)
REDACTED

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("RESUME-MISMATCH").
		SetOutTradeNo("sub2_resume_lookup_mismatch").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-2").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID("12").
		SetProviderKey(payment.TypeEasyPay).
		Save(ctx)
REDACTED

	resumeSvc := NewPaymentResumeService([]byte("REDACTED"))
	token, err := resumeSvc.CreateToken(ResumeTokenClaims{
		OrderID:            order.ID,
		UserID:             user.ID,
		ProviderInstanceID: "99",
		ProviderKey:        payment.TypeEasyPay,
		PaymentType:        payment.TypeAlipay,
		CanonicalReturnURL: "https://app.example.com/payment/result",
REDACTED)
REDACTED

	svc := &PaymentService{
		entClient:     client,
		resumeService: resumeSvc,
REDACTED

	_, err = svc.GetPublicOrderByResumeToken(ctx, token)
REDACTED
	require.Contains(t, err.Error(), "resume token")
REDACTED

func TestGetPublicOrderByResumeTokenChecksUpstreamForPendingOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("resume-refresh@example.com").
		SetPasswordHash("hash").
		SetUsername("resume-refresh-user").
		Save(ctx)
REDACTED

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("RESUME-PENDING").
		SetOutTradeNo("sub2_resume_lookup_pending").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-pending").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
REDACTED

	resumeSvc := NewPaymentResumeService([]byte("REDACTED"))
	token, err := resumeSvc.CreateToken(ResumeTokenClaims{
		OrderID:            order.ID,
		UserID:             user.ID,
		PaymentType:        payment.TypeAlipay,
		CanonicalReturnURL: "https://app.example.com/payment/result",
REDACTED)
REDACTED

	registry := payment.NewRegistry()
	provider := &paymentResumeLookupProvider{REDACTED
	registry.Register(provider)

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		resumeService:   resumeSvc,
		providersLoaded: true,
REDACTED

	got, err := svc.GetPublicOrderByResumeToken(ctx, token)
REDACTED
	require.Equal(t, order.ID, got.ID)
	require.Equal(t, 1, provider.queryCount)
REDACTED

func TestVerifyOrderPublicDoesNotCheckUpstreamForPendingOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("public-verify@example.com").
		SetPasswordHash("hash").
		SetUsername("public-verify-user").
		Save(ctx)
REDACTED

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("PUBLIC-VERIFY").
		SetOutTradeNo("sub2_public_verify_pending").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-public-verify").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
REDACTED

	registry := payment.NewRegistry()
	provider := &paymentResumeLookupProvider{REDACTED
	registry.Register(provider)

	svc := &PaymentService{
		entClient:       client,
		registry:        registry,
		providersLoaded: true,
REDACTED

	got, err := svc.VerifyOrderPublic(ctx, order.OutTradeNo)
REDACTED
	require.Equal(t, order.ID, got.ID)
	require.Equal(t, 0, provider.queryCount)
REDACTED
