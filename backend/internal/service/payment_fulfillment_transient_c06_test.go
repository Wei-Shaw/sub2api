//go:build unit

package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

var c06ErrTransientDB = errors.New("pq: server closed the connection unexpectedly")

// c06FailFromNth 从第 n 次（含）PaymentOrder 查询起返回瞬时 DB 错误；armed 之前不计数。
func c06FailFromNth(armed *atomic.Bool, n int32) dbent.Interceptor {
	var calls atomic.Int32
	return dbent.TraverseFunc(func(context.Context, dbent.Query) error {
		if !armed.Load() {
			return nil
		}
		if calls.Add(1) >= n {
			return c06ErrTransientDB
		}
		return nil
	})
}

func c06CreatePendingOrder(t *testing.T, ctx context.Context, client *dbent.Client) *dbent.PaymentOrder {
	t.Helper()
	user, err := client.User.Create().
		SetEmail("c06-transient@example.com").
		SetPasswordHash("hash").
		SetUsername("c06-transient").
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(50).
		SetPayAmount(50).
		SetFeeRate(0).
		SetRechargeCode("C06-TRANSIENT").
		SetOutTradeNo("sub2_c06_transient").
		SetPaymentType(payment.TypeSePayBankTransfer).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)
	return order
}

// 第一次按 out_trade_no 查到订单，confirmPayment 里的 Get 遇到瞬时错误时必须返回错误，
// 让 webhook 回 5xx 触发重试，而不是返回 nil 被 ACK 掉。
func TestC06ConfirmPaymentSurfacesTransientOrderLoadError(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := c06CreatePendingOrder(t, ctx, client)

	var armed atomic.Bool
	client.PaymentOrder.Intercept(c06FailFromNth(&armed, 2))
	armed.Store(true)

	svc := &PaymentService{entClient: client, providersLoaded: true}
	err := svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		TradeNo: "np-123",
		OrderID: order.OutTradeNo,
		Amount:  decimal.NewFromFloat(order.PayAmount),
		Status:  payment.NotificationStatusSuccess,
	}, payment.TypeSePay)
	require.Error(t, err)
	require.ErrorIs(t, err, c06ErrTransientDB)
	require.False(t, errors.Is(err, ErrOrderNotFound))

	armed.Store(false)
	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, reloaded.Status)
}

func TestC06AlreadyProcessedSurfacesTransientReloadError(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := c06CreatePendingOrder(t, ctx, client)

	var armed atomic.Bool
	client.PaymentOrder.Intercept(c06FailFromNth(&armed, 1))
	armed.Store(true)

	svc := &PaymentService{entClient: client, providersLoaded: true}
	err := svc.alreadyProcessed(ctx, order)
	require.Error(t, err)
	require.ErrorIs(t, err, c06ErrTransientDB)
}

// 真正不存在的旧式 sub2_N 订单仍保持原行为：记录日志并返回 nil。
func TestC06ConfirmPaymentStillIgnoresMissingLegacyOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentService{entClient: client, providersLoaded: true}

	err := svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		TradeNo: "np-legacy",
		OrderID: "sub2_987654",
		Amount:  decimal.NewFromInt(10),
		Status:  payment.NotificationStatusSuccess,
	}, payment.TypeSePay)
	require.NoError(t, err)

	err = svc.alreadyProcessed(ctx, &dbent.PaymentOrder{ID: 987654})
	require.NoError(t, err)
}
