//go:build unit

package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

// 过期 / 取消订单的支付恢复窗口。
//
// 订单在 expires_at 之后被标记为 EXPIRED，或者被用户/管理员主动取消为 CANCELLED。
// 这两种状态下如果上游仍然确认收到钱，必须继续入账（否则用户付了钱拿不到额度）；
// 但恢复必须受时间约束，否则一张很久以前锁价（充值倍率/赠送档位/套餐价格都按
// 下单时快照）的订单可以被无限期地“复活”，按旧价格拿到额度。
//
// 触发无限恢复的现实路径：VerifyOrderByOutTradeNo 先落库上游 trade_no，这次写入会把
// updated_at 刷新为当下时间，而 toPaid 过去用 updated_at 判断是否处于宽限期，于是
// 几小时前就该死亡的订单在查单后照样履约。

const recoveryWindowTestPayAmount = 88.0

type paymentRecoveryWindowFixture struct {
	client   *dbent.Client
	svc      *PaymentService
	userRepo *mockUserRepo
	provider *paymentOrderLifecycleQueryProvider
	order    *dbent.PaymentOrder
}

func newPaymentRecoveryWindowFixture(t *testing.T, status string, expiresAt, updatedAt time.Time) *paymentRecoveryWindowFixture {
	t.Helper()
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("recovery-window@example.com").
		SetPasswordHash("hash").
		SetUsername("recovery-window-user").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(recoveryWindowTestPayAmount).
		SetPayAmount(recoveryWindowTestPayAmount).
		SetFeeRate(0).
		SetRechargeCode("RECOVERY-WINDOW-" + status).
		SetOutTradeNo("sub2_recovery_window_" + status).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(status).
		SetExpiresAt(expiresAt).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	order, err = client.PaymentOrder.UpdateOneID(order.ID).SetUpdatedAt(updatedAt).Save(ctx)
	require.NoError(t, err)

	userRepo := &mockUserRepo{
		getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username},
	}
	userRepo.updateBalanceFn = func(_ context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		userRepo.getByIDUser.Balance += amount
		return nil
	}
	redeemRepo := &paymentOrderLifecycleRedeemRepo{
		codesByCode: map[string]*RedeemCode{
			order.RechargeCode: {
				ID:     1,
				Code:   order.RechargeCode,
				Type:   RedeemTypeBalance,
				Value:  order.Amount,
				Status: StatusUnused,
			},
		},
	}
	redeemService := NewRedeemService(redeemRepo, userRepo, nil, nil, nil, client, nil, nil)

	registry := payment.NewRegistry()
	provider := &paymentOrderLifecycleQueryProvider{
		resp: &payment.QueryOrderResponse{
			TradeNo: "upstream-recovered-trade",
			Status:  payment.ProviderStatusPaid,
			Amount:  recoveryWindowTestPayAmount,
		},
	}
	registry.Register(provider)

	return &paymentRecoveryWindowFixture{
		client:   client,
		userRepo: userRepo,
		provider: provider,
		order:    order,
		svc: &PaymentService{
			entClient:       client,
			registry:        registry,
			redeemService:   redeemService,
			userRepo:        userRepo,
			providersLoaded: true,
		},
	}
}

func (f *paymentRecoveryWindowFixture) requireNotFulfilled(t *testing.T, wantStatus, wantAudit string) {
	t.Helper()
	ctx := context.Background()
	reloaded, err := f.client.PaymentOrder.Get(ctx, f.order.ID)
	require.NoError(t, err)
	require.Equal(t, wantStatus, reloaded.Status, "关闭超过恢复窗口的订单不得被履约")
	require.Zero(t, f.userRepo.getByIDUser.Balance, "不得按订单里的旧金额入账")

	count, err := f.client.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(f.order.ID, 10)),
			paymentauditlog.ActionEQ(wantAudit),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count, "必须留下 %s 审计，供人工对账", wantAudit)
}

func (f *paymentRecoveryWindowFixture) requireFulfilled(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	reloaded, err := f.client.PaymentOrder.Get(ctx, f.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.Equal(t, recoveryWindowTestPayAmount, f.userRepo.getByIDUser.Balance)
}

// 过期 3 小时的订单：用户点一次“查询支付状态”，上游确认已支付。
// 查单会先写 payment_trade_no（把 updated_at 刷新为当下），旧实现因此通过宽限期检查，
// 按 3 小时前锁定的金额履约。
func TestVerifyOrderByOutTradeNoDoesNotReviveOrderLongAfterExpiry(t *testing.T) {
	expiredAt := time.Now().Add(-3 * time.Hour)
	f := newPaymentRecoveryWindowFixture(t, OrderStatusExpired, expiredAt, expiredAt)

	got, err := f.svc.VerifyOrderByOutTradeNo(context.Background(), f.order.OutTradeNo, f.order.UserID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusExpired, got.Status)
	f.requireNotFulfilled(t, OrderStatusExpired, "PAYMENT_AFTER_EXPIRY")
}

// 取消同样有时间约束：取消 3 小时后才到达的支付回调不得按旧金额履约。
func TestWebhookPaymentForLongCancelledOrderIsNotFulfilled(t *testing.T) {
	cancelledAt := time.Now().Add(-3 * time.Hour)
	f := newPaymentRecoveryWindowFixture(t, OrderStatusCancelled, cancelledAt, cancelledAt)

	err := f.svc.HandlePaymentNotification(context.Background(), &payment.PaymentNotification{
		OrderID: f.order.OutTradeNo,
		TradeNo: "upstream-recovered-trade",
		Status:  payment.NotificationStatusSuccess,
		Amount:  recoveryWindowTestPayAmount,
	}, payment.TypeAlipay)
	require.NoError(t, err, "关闭后的迟到支付必须 ack，避免渠道无限重试")
	f.requireNotFulfilled(t, OrderStatusCancelled, "PAYMENT_AFTER_CANCEL")
}

// 刚过期（仍在恢复窗口内）的支付必须照常入账：这是“到期瞬间付款、回调稍晚到达”的正常情况。
func TestExpiredOrderPaidWithinRecoveryWindowIsFulfilled(t *testing.T) {
	expiredAt := time.Now().Add(-time.Minute)
	f := newPaymentRecoveryWindowFixture(t, OrderStatusExpired, expiredAt, expiredAt)

	_, err := f.svc.VerifyOrderByOutTradeNo(context.Background(), f.order.OutTradeNo, f.order.UserID)
	require.NoError(t, err)
	f.requireFulfilled(t)
}

// 取消发生在订单有效期内（用户点完取消后另一台设备完成支付）时仍要入账。
func TestCancelledOrderPaidWithinOrderWindowIsRecovered(t *testing.T) {
	now := time.Now()
	f := newPaymentRecoveryWindowFixture(t, OrderStatusCancelled, now.Add(20*time.Minute), now.Add(-time.Minute))

	err := f.svc.HandlePaymentNotification(context.Background(), &payment.PaymentNotification{
		OrderID: f.order.OutTradeNo,
		TradeNo: "upstream-recovered-trade",
		Status:  payment.NotificationStatusSuccess,
		Amount:  recoveryWindowTestPayAmount,
	}, payment.TypeAlipay)
	require.NoError(t, err)
	f.requireFulfilled(t)
}
