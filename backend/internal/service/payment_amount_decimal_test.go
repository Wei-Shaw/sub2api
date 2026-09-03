//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// =========================
// L3: 金额跨接口不再走 float64
// =========================

// TestConfirmPaymentRejectsOneCentShortfall 钉住 L3 的核心后果：
// PaymentNotification.Amount 原先是 float64，比对时带 amountToleranceCNY = 0.01 的容差，
// 于是 100.00 的订单收到 99.99 也照样足额履约，PAYMENT_AMOUNT_MISMATCH 永远不会触发。
// 改成 decimal 之后金额全程精确，少付方向零容差。
func TestConfirmPaymentRejectsOneCentShortfall(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusPending, time.Now())
	order, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetAmount(100).
		SetPayAmount(100).
		SetOrderType(payment.OrderTypeBalance).
		ClearPlanID().
		ClearSubscriptionGroupID().
		ClearSubscriptionDays().
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	err = svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		TradeNo: "alipay-trade-99-99",
		OrderID: order.OutTradeNo,
		Amount:  decimal.RequireFromString("99.99"),
		Status:  payment.NotificationStatusSuccess,
	}, payment.TypeAlipay)
	require.ErrorContains(t, err, "amount mismatch")

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, reloaded.Status, "少付一分不得推进订单状态")

	logged, err := client.PaymentAuditLog.Query().All(ctx)
	require.NoError(t, err)
	found := false
	for _, l := range logged {
		if l.Action == "PAYMENT_AMOUNT_MISMATCH" {
			found = true
		}
	}
	require.True(t, found, "少付必须留下 PAYMENT_AMOUNT_MISMATCH 审计")
}

// TestConfirmPaymentAcceptsExactDecimalAmount 精确等额必须照常放行，
// 且不能因为 float64 的表示误差（0.1+0.2 那类）误判成少付。
func TestConfirmPaymentAcceptsExactDecimalAmount(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusPending, time.Now())
	order, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetAmount(0.3).
		SetPayAmount(0.3).
		SetOrderType(payment.OrderTypeBalance).
		ClearPlanID().
		ClearSubscriptionGroupID().
		ClearSubscriptionDays().
		Save(ctx)
	require.NoError(t, err)

	redeemRepo := &redeemCodeRepoStub{codesByCode: map[string]*RedeemCode{
		order.RechargeCode: {
			ID:     301,
			Code:   order.RechargeCode,
			Type:   RedeemTypeBalance,
			Value:  order.Amount,
			Status: StatusUsed,
		},
	}}
	svc := &PaymentService{entClient: client, redeemService: &RedeemService{redeemRepo: redeemRepo}}
	require.NoError(t, svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		TradeNo: "alipay-trade-exact",
		OrderID: order.OutTradeNo,
		Amount:  decimal.RequireFromString("0.30"),
		Status:  payment.NotificationStatusSuccess,
	}, payment.TypeAlipay))

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status, "等额支付必须照常履约")
}

// TestPaymentOverpayToleranceIsUnchangedForMinorUnits 多付方向保留原有的按币种容差：
// 拒绝给多付的用户入账是更糟的失败模式，而且这一侧本来就不是「白送」。
func TestPaymentOverpayToleranceIsUnchangedForMinorUnits(t *testing.T) {
	t.Parallel()

	require.True(t, decimal.RequireFromString("0.01").Equal(paymentOverpayToleranceForCurrency("CNY")))
	require.True(t, decimal.RequireFromString("0.01").Equal(paymentOverpayToleranceForCurrency("JPY")))
	// 3 位小数币种：半个最小货币单位。
	require.True(t, decimal.RequireFromString("0.0005").Equal(paymentOverpayToleranceForCurrency("BHD")))
}

// TestMinorUnitToDecimalAmountStaysExact 最小货币单位换算不得再经过 float64。
func TestMinorUnitToDecimalAmountStaysExact(t *testing.T) {
	t.Parallel()

	require.Equal(t, "0.07", payment.MinorUnitToDecimalAmount(7, "CNY").StringFixed(2))
	require.Equal(t, "1234.56", payment.MinorUnitToDecimalAmount(123456, "USD").StringFixed(2))
	require.Equal(t, "700", payment.MinorUnitToDecimalAmount(700, "JPY").String())
}
