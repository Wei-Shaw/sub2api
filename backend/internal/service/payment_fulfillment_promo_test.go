//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

// userRepoBalanceStub embeds the panic-on-unexpected userRepoStub but tracks
// UpdateBalance invocations so we can assert how much bonus credit was applied.
type userRepoBalanceStub struct {
	userRepoStub
	balances    map[int64]float64
	updateCalls int
}

func (s *userRepoBalanceStub) UpdateBalance(_ context.Context, id int64, amount float64) error {
	if s.balances == nil {
		s.balances = map[int64]float64{}
	}
	s.balances[id] += amount
	s.updateCalls++
	return nil
}

// newPromoFulfillmentTestService wires a PaymentService instance with the
// minimum dependencies needed to drive applyRechargeBonusOnOrder /
// creditRechargeBonus end-to-end against a real sqlite ent client.
//
// 该 helper 故意避开 NewPaymentService（不需要 registry / loadBalancer / 各种
// 真实下游服务），直接组装裸结构体。RechargePromoActivityService 只用到
// entClient，符合 GetCurrent / GetByID 的真实实现。
func newPromoFulfillmentTestService(
	t *testing.T,
	client *dbent.Client,
	userRepo UserRepository,
) *PaymentService {
	t.Helper()

	settingRepo := &paymentConfigSettingRepoStub{
		values: map[string]string{
			SettingBalanceRechargeMult: "1",
		},
	}
	activitySvc := NewRechargePromoActivityService(client)
	configSvc := NewPaymentConfigService(client, settingRepo, []byte("0123456789abcdef0123456789abcdef"), activitySvc)

	return &PaymentService{
		entClient:     client,
		configService: configSvc,
		userRepo:      userRepo,
	}
}

// TestApplyRechargeBonusOnOrder_ActivePromo_PersistsBonusFields covers the
// fulfillment-time bonus persistence (task 2.5 in the recharge-bonus-promo
// change). The active activity row is shaped like the production payload:
// tiers ascending by min_amount, enabled = TRUE, valid window straddling now.
func TestApplyRechargeBonusOnOrder_ActivePromo_PersistsBonusFields(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("promo-fulfillment@example.com").
		SetPasswordHash("hash").
		SetUsername("promo-fulfillment").
		Save(ctx)
	require.NoError(t, err)

	now := time.Now()
	activity, err := client.RechargePromoActivity.Create().
		SetName("test-activity").
		SetEnabled(true).
		SetValidFrom(now.Add(-1 * time.Hour)).
		SetValidUntil(now.Add(24 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{
			{MinAmount: 100, BonusRate: 0.05},
		}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	// PayAmount = 200 命中 100 档（rate 0.05），multiplier = 1.0 →
	// gross = 200 × 1.0 × 0.05 = 10.0；ceil2(10.0) = 10.0。
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(200).
		SetPayAmount(200).
		SetFeeRate(0).
		SetRechargeCode("BONUS-FULFILL-1").
		SetOutTradeNo("sub2_bonus_fulfill_1").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-bonus-1").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRecharging).
		SetExpiresAt(now.Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	balanceStub := &userRepoBalanceStub{}
	svc := newPromoFulfillmentTestService(t, client, balanceStub)

	require.NoError(t, svc.applyRechargeBonusOnOrder(ctx, order))

	// 校验内存对象与 DB 持久化值一致。
	require.InDelta(t, 0.05, order.BonusRate, 1e-9)
	require.InDelta(t, 10.0, order.BonusAmount, 1e-9)
	require.NotNil(t, order.ActivityID)
	require.Equal(t, activity.ID, *order.ActivityID)

	persisted, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.InDelta(t, 0.05, persisted.BonusRate, 1e-9)
	require.InDelta(t, 10.0, persisted.BonusAmount, 1e-9)
	require.NotNil(t, persisted.ActivityID)
	require.Equal(t, activity.ID, *persisted.ActivityID)

	// 二次 apply 应是 no-op（订单已携带 bonus 字段）。
	require.NoError(t, svc.applyRechargeBonusOnOrder(ctx, order))
	persistedAgain, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.InDelta(t, 10.0, persistedAgain.BonusAmount, 1e-9)

	// 首次 credit 应把 10.0 加到余额上。
	require.NoError(t, svc.creditRechargeBonus(ctx, order))
	require.Equal(t, 1, balanceStub.updateCalls)
	require.InDelta(t, 10.0, balanceStub.balances[user.ID], 1e-9)

	// 第二次 credit 必须幂等：BONUS_CREDITED 审计已写入，不能再次扣加。
	require.NoError(t, svc.creditRechargeBonus(ctx, order))
	require.Equal(t, 1, balanceStub.updateCalls, "creditRechargeBonus must be idempotent on retry")
	require.InDelta(t, 10.0, balanceStub.balances[user.ID], 1e-9)
}

// TestApplyRechargeBonusOnOrder_SubscriptionOrder_NoOp ensures bonus only
// applies to balance recharges, never to subscription purchases — even when an
// active promo would otherwise match the pay amount.
func TestApplyRechargeBonusOnOrder_SubscriptionOrder_NoOp(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("sub-no-bonus@example.com").
		SetPasswordHash("hash").
		SetUsername("sub-no-bonus").
		Save(ctx)
	require.NoError(t, err)

	now := time.Now()
	_, err = client.RechargePromoActivity.Create().
		SetName("test-activity-sub").
		SetEnabled(true).
		SetValidFrom(now.Add(-1 * time.Hour)).
		SetValidUntil(now.Add(24 * time.Hour)).
		SetTiers([]domain.RechargePromoTier{
			{MinAmount: 100, BonusRate: 0.05},
		}).
		SetOperator("system").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(200).
		SetPayAmount(200).
		SetFeeRate(0).
		SetRechargeCode("BONUS-SUB-1").
		SetOutTradeNo("sub2_bonus_sub_1").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-bonus-sub-1").
		SetOrderType(payment.OrderTypeSubscription).
		SetStatus(OrderStatusRecharging).
		SetExpiresAt(now.Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	balanceStub := &userRepoBalanceStub{}
	svc := newPromoFulfillmentTestService(t, client, balanceStub)

	require.NoError(t, svc.applyRechargeBonusOnOrder(ctx, order))
	require.Equal(t, float64(0), order.BonusRate)
	require.Equal(t, float64(0), order.BonusAmount)
	require.Nil(t, order.ActivityID)

	persisted, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, float64(0), persisted.BonusRate)
	require.Equal(t, float64(0), persisted.BonusAmount)
	require.Nil(t, persisted.ActivityID)

	// creditRechargeBonus 也应直接 short-circuit（OrderType != balance）。
	require.NoError(t, svc.creditRechargeBonus(ctx, order))
	require.Equal(t, 0, balanceStub.updateCalls)
	require.Empty(t, balanceStub.balances)
}

// TestApplyRechargeBonusOnOrder_NoActivePromo_LeavesOrderUntouched covers the
// "no activity row" path: GetCurrent returns nil → ResolveRechargeBonus
// returns (0, 0) → order keeps its zero defaults.
func TestApplyRechargeBonusOnOrder_NoActivePromo_LeavesOrderUntouched(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("no-promo@example.com").
		SetPasswordHash("hash").
		SetUsername("no-promo").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(200).
		SetPayAmount(200).
		SetFeeRate(0).
		SetRechargeCode("BONUS-NONE-1").
		SetOutTradeNo("sub2_bonus_none_1").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-bonus-none-1").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRecharging).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	balanceStub := &userRepoBalanceStub{}
	svc := newPromoFulfillmentTestService(t, client, balanceStub)

	require.NoError(t, svc.applyRechargeBonusOnOrder(ctx, order))
	require.Equal(t, float64(0), order.BonusRate)
	require.Equal(t, float64(0), order.BonusAmount)
	require.Nil(t, order.ActivityID)

	require.NoError(t, svc.creditRechargeBonus(ctx, order))
	require.Equal(t, 0, balanceStub.updateCalls)
}
