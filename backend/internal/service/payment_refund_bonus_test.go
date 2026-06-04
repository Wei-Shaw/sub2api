//go:build unit

package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

// refundCallTrackingProvider is a payment.Provider stub that records every
// Refund invocation. Used to *prove* that PrepareRefund's bonus guards
// short-circuit before any gateway call ever happens.
type refundCallTrackingProvider struct {
	key         string
	refundCalls int
}

func (p *refundCallTrackingProvider) Name() string        { return p.key }
func (p *refundCallTrackingProvider) ProviderKey() string { return p.key }
func (p *refundCallTrackingProvider) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeAlipay}
}
func (p *refundCallTrackingProvider) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	panic("unexpected CreatePayment call")
}
func (p *refundCallTrackingProvider) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	panic("unexpected QueryOrder call")
}
func (p *refundCallTrackingProvider) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	panic("unexpected VerifyNotification call")
}
func (p *refundCallTrackingProvider) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	p.refundCalls++
	return &payment.RefundResponse{Status: payment.ProviderStatusSuccess}, nil
}

// createBonusOrderForRefund seeds a completed balance order carrying
// bonus_amount/bonus_rate, attached to a real provider instance row so that
// `getRefundOrderProviderInstance` resolves successfully.
func createBonusOrderForRefund(
	t *testing.T,
	ctx context.Context,
	client *dbent.Client,
	user *dbent.User,
	amount, payAmount, bonusAmount, bonusRate float64,
	uniqueSuffix string,
) (*dbent.PaymentOrder, *dbent.PaymentProviderInstance) {
	t.Helper()
	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-" + uniqueSuffix).
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(amount).
		SetPayAmount(payAmount).
		SetFeeRate(0).
		SetRechargeCode("REFUND-BONUS-" + uniqueSuffix).
		SetOutTradeNo("sub2_refund_bonus_" + uniqueSuffix).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-bonus-" + uniqueSuffix).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		SetBonusAmount(bonusAmount).
		SetBonusRate(bonusRate).
		Save(ctx)
	require.NoError(t, err)
	return order, inst
}

// TestPrepareRefund_BonusOrder_SufficientBalance_ProducesFullClawbackPlan
// covers the happy path: when the user's balance covers `amount + bonus`,
// PrepareRefund must build a deduction plan whose BalanceToDeduct equals
// the *full* clawback (amount + bonus_amount), not just `amount`. This is
// the contract behind task 3.2 (deduct amount + bonus on success).
func TestPrepareRefund_BonusOrder_SufficientBalance_ProducesFullClawbackPlan(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-bonus-ok@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-bonus-ok").
		Save(ctx)
	require.NoError(t, err)

	const (
		amount      = 100.0
		payAmount   = 100.0
		bonusAmount = 5.0
		bonusRate   = 0.05
	)
	order, _ := createBonusOrderForRefund(t, ctx, client, user, amount, payAmount, bonusAmount, bonusRate, "ok")

	gwProvider := &refundCallTrackingProvider{key: payment.TypeAlipay}
	registry := payment.NewRegistry()
	registry.Register(gwProvider)

	repo := &userRepoStub{user: &User{ID: user.ID, Balance: amount + bonusAmount}}
	svc := &PaymentService{
		entClient: client,
		registry:  registry,
		userRepo:  repo,
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "user requested", false, true)
	require.NoError(t, err)
	require.Nil(t, result)
	require.NotNil(t, plan)
	require.Equal(t, payment.DeductionTypeBalance, plan.DeductionType)
	// 关键断言：BalanceToDeduct = amount + bonus_amount，确保退款时一并回收赠送。
	require.InDelta(t, amount+bonusAmount, plan.BalanceToDeduct, 1e-9)
	require.InDelta(t, amount, plan.RefundAmount, 1e-9)
	// 网关从未被调用：PrepareRefund 是规划阶段，调用 gateway 的是 ExecuteRefund。
	require.Equal(t, 0, gwProvider.refundCalls, "gateway must not be called during PrepareRefund")
}

// TestPrepareRefund_BonusOrder_InsufficientBalance_RejectsAndSkipsGateway
// nails task 3.4's hardest invariant: when balance < amount + bonus the
// service must hard-reject *before* any gateway side effect, so the user's
// balance can never go negative as a result of refund-time clawback.
func TestPrepareRefund_BonusOrder_InsufficientBalance_RejectsAndSkipsGateway(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-bonus-poor@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-bonus-poor").
		Save(ctx)
	require.NoError(t, err)

	const (
		amount      = 100.0
		payAmount   = 100.0
		bonusAmount = 5.0
		bonusRate   = 0.05
	)
	order, _ := createBonusOrderForRefund(t, ctx, client, user, amount, payAmount, bonusAmount, bonusRate, "poor")

	gwProvider := &refundCallTrackingProvider{key: payment.TypeAlipay}
	registry := payment.NewRegistry()
	registry.Register(gwProvider)

	// 余额 = 102 < 100 + 5 = 105，不足以同时回收 credited + bonus。
	repo := &userRepoStub{user: &User{ID: user.ID, Balance: 102.0}}
	svc := &PaymentService{
		entClient: client,
		registry:  registry,
		userRepo:  repo,
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "user requested", false, true)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "BALANCE_INSUFFICIENT_FOR_REFUND", infraerrors.Reason(err))
	// 即便加上 force 也不应允许绕过这个守卫——再跑一次 force=true 验证。
	plan2, result2, err2 := svc.PrepareRefund(ctx, order.ID, 0, "force", true, true)
	require.Nil(t, plan2)
	require.Nil(t, result2)
	require.Error(t, err2)
	require.Equal(t, "BALANCE_INSUFFICIENT_FOR_REFUND", infraerrors.Reason(err2),
		"force flag must NOT bypass the bonus clawback guard")

	// 网关 mock 验证：PrepareRefund 失败的全程 zero gateway calls。
	require.Equal(t, 0, gwProvider.refundCalls, "gateway must not be called when balance guard rejects refund")
}

// TestPrepareRefund_BonusOrder_PartialRefund_Rejected covers task 3.3:
// orders carrying bonus_amount > 0 must reject any partial refund (refund
// amount strictly less than the original recharge amount), with reason
// PARTIAL_REFUND_NOT_SUPPORTED_FOR_BONUS_ORDER. This pre-empts ambiguous
// pro-rata bonus splits that v1 explicitly does not support.
func TestPrepareRefund_BonusOrder_PartialRefund_Rejected(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-bonus-partial@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-bonus-partial").
		Save(ctx)
	require.NoError(t, err)

	const (
		amount      = 100.0
		payAmount   = 100.0
		bonusAmount = 5.0
		bonusRate   = 0.05
	)
	order, _ := createBonusOrderForRefund(t, ctx, client, user, amount, payAmount, bonusAmount, bonusRate, "partial")

	gwProvider := &refundCallTrackingProvider{key: payment.TypeAlipay}
	registry := payment.NewRegistry()
	registry.Register(gwProvider)

	repo := &userRepoStub{user: &User{ID: user.ID, Balance: 1000.0}}
	svc := &PaymentService{
		entClient: client,
		registry:  registry,
		userRepo:  repo,
	}

	// 部分退款（50 < 100）— 命中 PARTIAL_REFUND_NOT_SUPPORTED_FOR_BONUS_ORDER。
	plan, result, err := svc.PrepareRefund(ctx, order.ID, 50, "partial", false, true)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "PARTIAL_REFUND_NOT_SUPPORTED_FOR_BONUS_ORDER", infraerrors.Reason(err))
	require.Equal(t, 0, gwProvider.refundCalls, "gateway must not be called when partial refund is rejected")

	// 整额退款（amt = order.Amount）应放行，验证守卫只针对部分退款。
	planFull, resultFull, errFull := svc.PrepareRefund(ctx, order.ID, amount, "full", false, true)
	require.NoError(t, errFull)
	require.Nil(t, resultFull)
	require.NotNil(t, planFull)
	require.InDelta(t, amount+bonusAmount, planFull.BalanceToDeduct, 1e-9)
}

// TestPrepareRefund_NonBonusOrder_PartialRefund_Allowed sanity-checks the
// inverse: orders without a bonus should *still* permit partial refunds, so
// the new guard is strictly scoped to bonus-bearing orders.
func TestPrepareRefund_NonBonusOrder_PartialRefund_Allowed(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-no-bonus-partial@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-no-bonus-partial").
		Save(ctx)
	require.NoError(t, err)

	order, _ := createBonusOrderForRefund(t, ctx, client, user, 100, 100, 0, 0, "no-bonus-partial")

	gwProvider := &refundCallTrackingProvider{key: payment.TypeAlipay}
	registry := payment.NewRegistry()
	registry.Register(gwProvider)

	repo := &userRepoStub{user: &User{ID: user.ID, Balance: 1000.0}}
	svc := &PaymentService{
		entClient: client,
		registry:  registry,
		userRepo:  repo,
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 50, "partial", false, true)
	require.NoError(t, err)
	require.Nil(t, result)
	require.NotNil(t, plan)
	require.InDelta(t, 50.0, plan.RefundAmount, 1e-9)
	// 非 bonus 订单：BalanceToDeduct = min(refundAmount, balance) = 50。
	require.InDelta(t, 50.0, plan.BalanceToDeduct, 1e-9)
	require.Equal(t, 0, gwProvider.refundCalls)
}
