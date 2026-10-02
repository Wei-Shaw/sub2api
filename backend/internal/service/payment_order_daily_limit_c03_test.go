//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// TestCheckDailyLimitCountsSubscriptionOrdersByPayAmount 钉住 C-03：
// 订阅单的 amount 是 USD 套餐价，实际扣款 pay_amount 是换算后的网关币种（VND）。
// 日限额按网关币种配置，已支付的订阅单必须按 pay_amount 计入，
// 否则一笔 250000 VND 的订阅只占 10 的额度，日限额对订阅形同虚设。
func TestCheckDailyLimitCountsSubscriptionOrdersByPayAmount(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("daily-limit-c03@example.com").
		SetPasswordHash("hash").
		SetUsername("daily-limit-c03-user").
		Save(ctx)
	require.NoError(t, err)

	// 10 USD 的套餐按 25000 汇率扣了 250000 VND。
	mustCreatePaidPaymentOrder(t, ctx, client, user.ID, payment.OrderTypeSubscription, 10, 250000, OrderStatusCompleted, time.Now().UTC())

	svc := &PaymentService{entClient: client}
	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	// 250000 + 100000 > 300000：必须拒绝。
	err = svc.checkDailyLimit(ctx, tx, user.ID, 100000, 300000)
	require.NoError(t, tx.Rollback())
	require.Error(t, err)
	require.Equal(t, "DAILY_LIMIT_EXCEEDED", infraerrors.Reason(err))
}

// TestCreateOrderInTxSubscriptionConsumesDailyLimitInGatewayCurrency 端到端复现 C-03：
// 日限额 300000（网关币种）。两笔 10 USD 的订阅单各扣 250000 VND，
// 修复前新订单按 USD 套餐价 10 计入额度、挂起订单也按 10 计入，两笔都能建出来；
// 修复后第二笔必须被拒绝。
func TestCreateOrderInTxSubscriptionConsumesDailyLimitInGatewayCurrency(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("daily-limit-c03-sub@example.com").
		SetPasswordHash("hash").
		SetUsername("daily-limit-c03-sub-user").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	cfg := &PaymentConfig{MaxPendingOrders: 10, OrderTimeoutMin: 30, DailyLimit: 300000}
	req := CreateOrderRequest{
		UserID:      user.ID,
		PaymentType: payment.TypeSePayBankTransfer,
		OrderType:   payment.OrderTypeSubscription,
	}
	svcUser := &User{ID: user.ID, Email: user.Email, Username: user.Username}

	first, err := svc.createOrderInTx(ctx, req, svcUser, nil, cfg, 10, 0, 250000, 0, nil)
	require.NoError(t, err)
	require.Equal(t, 250000.0, first.PayAmount)

	_, err = svc.createOrderInTx(ctx, req, svcUser, nil, cfg, 10, 0, 250000, 0, nil)
	require.Error(t, err)
	require.Equal(t, "DAILY_LIMIT_PENDING_HOLD", infraerrors.Reason(err))
}
