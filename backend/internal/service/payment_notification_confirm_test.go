//go:build unit

package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type paymentConfirmTestProvider struct {
	paymentFulfillmentTestProvider
	err   error
	calls *int
}

func (p paymentConfirmTestProvider) ConfirmNotification(_ context.Context, n *payment.PaymentNotification) error {
	*p.calls++
	if p.err == nil && n.TradeNo == "" {
		n.TradeNo = "upstream-trade"
	}
	return p.err
}

func TestConfirmNotificationUpstreamRecordsAuditOnFailure(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().
		SetEmail("confirm-upstream@example.com").
		SetPasswordHash("hash").
		SetUsername("confirm-upstream-user").
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(10).
		SetPayAmount(10).
		SetFeeRate(0).
		SetRechargeCode("PAY-CONFIRM-UPSTREAM").
		SetOutTradeNo("sub2_confirm_upstream").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	calls := 0
	prov := paymentConfirmTestProvider{
		paymentFulfillmentTestProvider: paymentFulfillmentTestProvider{key: payment.TypeEasyPay},
		err:                            errors.New("easypay confirm: upstream reports order sub2_confirm_upstream as not paid"),
		calls:                          &calls,
	}
	n := &payment.PaymentNotification{OrderID: order.OutTradeNo, TradeNo: "T-unconfirmed", Amount: 10, Status: payment.NotificationStatusSuccess}

	err = svc.ConfirmNotificationUpstream(ctx, prov, n)
	require.Error(t, err)
	require.Equal(t, 1, calls)

	count, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("PAYMENT_UPSTREAM_UNCONFIRMED")).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, reloaded.Status)
}

func TestConfirmNotificationUpstreamPassThrough(t *testing.T) {
	svc := &PaymentService{}
	n := &payment.PaymentNotification{OrderID: "sub2_x", Amount: 10, Status: payment.NotificationStatusSuccess}

	// Providers without the confirmer capability keep their existing flow.
	require.NoError(t, svc.ConfirmNotificationUpstream(context.Background(), paymentFulfillmentTestProvider{key: payment.TypeStripe}, n))

	calls := 0
	prov := paymentConfirmTestProvider{paymentFulfillmentTestProvider: paymentFulfillmentTestProvider{key: payment.TypeEasyPay}, calls: &calls}

	// Non-success notifications are never fulfilled, so they are not queried.
	failed := &payment.PaymentNotification{OrderID: "sub2_x", Status: payment.ProviderStatusFailed}
	require.NoError(t, svc.ConfirmNotificationUpstream(context.Background(), prov, failed))
	require.Equal(t, 0, calls)

	require.NoError(t, svc.ConfirmNotificationUpstream(context.Background(), prov, n))
	require.Equal(t, 1, calls)
	require.Equal(t, "upstream-trade", n.TradeNo)
}
