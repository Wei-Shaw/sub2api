//go:build unit

package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type latePaymentTestProvider struct {
	key         string
	refundsLate bool
	refundErr   error
	refunds     []payment.RefundRequest
}

func (p *latePaymentTestProvider) Name() string        { return "late-payment-test" }
func (p *latePaymentTestProvider) ProviderKey() string { return p.key }
func (p *latePaymentTestProvider) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{p.key}
}
func (p *latePaymentTestProvider) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	panic("unexpected call")
}
func (p *latePaymentTestProvider) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	panic("unexpected call")
}
func (p *latePaymentTestProvider) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	panic("unexpected call")
}
func (p *latePaymentTestProvider) Refund(_ context.Context, req payment.RefundRequest) (*payment.RefundResponse, error) {
	p.refunds = append(p.refunds, req)
	if p.refundErr != nil {
		return nil, p.refundErr
	}
	return &payment.RefundResponse{RefundID: "tran_late", Status: payment.ProviderStatusSuccess}, nil
}
func (p *latePaymentTestProvider) RefundsLatePayments() bool { return p.refundsLate }

func createExpiredBeyondGraceOrder(t *testing.T, client *dbent.Client, providerKey, outTradeNo string) *dbent.PaymentOrder {
	t.Helper()
	ctx := context.Background()
	user, err := client.User.Create().
		SetEmail(outTradeNo + "@example.com").
		SetPasswordHash("hash").
		SetUsername(outTradeNo).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(3).
		SetPayAmount(3).
		SetFeeRate(0).
		SetRechargeCode("LATE-" + outTradeNo).
		SetOutTradeNo(outTradeNo).
		SetPaymentType(providerKey).
		SetPaymentTradeNo("ch_created").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusExpired).
		SetExpiresAt(time.Now().Add(-time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)
	// 过期时间早于宽限期，toPaid 不会再把订单找回。
	_, err = client.PaymentOrder.UpdateOneID(order.ID).
		SetUpdatedAt(time.Now().Add(-(paymentGraceMinutes + 10) * time.Minute)).
		Save(ctx)
	require.NoError(t, err)
	return order
}

func newLatePaymentTestService(client *dbent.Client, prov payment.Provider) *PaymentService {
	registry := payment.NewRegistry()
	registry.Register(prov)
	return &PaymentService{entClient: client, registry: registry, providersLoaded: true}
}

func auditActions(t *testing.T, client *dbent.Client, orderID int64) []string {
	t.Helper()
	logs, err := client.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(orderID, 10))).All(context.Background())
	require.NoError(t, err)
	actions := make([]string, 0, len(logs))
	for _, l := range logs {
		actions = append(actions, l.Action)
	}
	return actions
}

func TestLatePaymentAfterGraceIsRefundedForLatePaymentRefunders(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)
	order := createExpiredBeyondGraceOrder(t, client, payment.TypeCreem, "sub2_late_creem")
	prov := &latePaymentTestProvider{key: payment.TypeCreem, refundsLate: true}
	svc := newLatePaymentTestService(client, prov)

	err := svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		TradeNo: "ch_paid_late",
		OrderID: order.OutTradeNo,
		Amount:  3,
		Status:  payment.NotificationStatusSuccess,
	}, payment.TypeCreem)
	require.NoError(t, err)

	require.Len(t, prov.refunds, 1)
	require.Equal(t, "ch_paid_late", prov.refunds[0].TradeNo)
	require.Equal(t, order.OutTradeNo, prov.refunds[0].OrderID)
	require.Equal(t, "3.00", prov.refunds[0].Amount)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusExpired, reloaded.Status, "a refunded late payment must not credit the order")
	require.Contains(t, auditActions(t, client, order.ID), "LATE_PAYMENT_REFUNDED")
}

func TestLatePaymentRefundFailureIsReturnedSoTheWebhookIsRetried(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)
	order := createExpiredBeyondGraceOrder(t, client, payment.TypeCreem, "sub2_late_creem_fail")
	prov := &latePaymentTestProvider{key: payment.TypeCreem, refundsLate: true, refundErr: errors.New("upstream down")}
	svc := newLatePaymentTestService(client, prov)

	err := svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		TradeNo: "ch_paid_late",
		OrderID: order.OutTradeNo,
		Amount:  3,
		Status:  payment.NotificationStatusSuccess,
	}, payment.TypeCreem)
	require.ErrorContains(t, err, "upstream down")
	require.Contains(t, auditActions(t, client, order.ID), "LATE_PAYMENT_REFUND_FAILED")
}

func TestLatePaymentAfterGraceIsNotRefundedForOtherProviders(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)
	order := createExpiredBeyondGraceOrder(t, client, payment.TypeAirwallex, "sub2_late_airwallex")
	prov := &latePaymentTestProvider{key: payment.TypeAirwallex, refundsLate: false}
	svc := newLatePaymentTestService(client, prov)

	err := svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		TradeNo: "int_paid_late",
		OrderID: order.OutTradeNo,
		Amount:  3,
		Status:  payment.NotificationStatusSuccess,
	}, payment.TypeAirwallex)
	require.NoError(t, err)
	require.Empty(t, prov.refunds)
	actions := auditActions(t, client, order.ID)
	require.Contains(t, actions, "PAYMENT_AFTER_EXPIRY")
	require.NotContains(t, actions, "LATE_PAYMENT_REFUNDED")
}
