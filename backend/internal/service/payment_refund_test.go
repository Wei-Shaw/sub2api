//go:build unit

package service

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestValidateRefundRequestRejectsLegacyGuessedProviderInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-legacy@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-legacy-user").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("REFUND-LEGACY-ORDER").
		SetOutTradeNo("sub2_refund_legacy_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-legacy-refund").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
	}

	_, err = svc.validateRefundRequest(ctx, order.ID, user.ID)
	require.Error(t, err)
	require.Equal(t, "USER_REFUND_DISABLED", infraerrors.Reason(err))
}

func TestPrepareRefundRejectsLegacyGuessedProviderInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-legacy-admin@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-legacy-admin-user").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-admin-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(188).
		SetPayAmount(188).
		SetFeeRate(0).
		SetRechargeCode("REFUND-LEGACY-ADMIN-ORDER").
		SetOutTradeNo("sub2_refund_legacy_admin_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-legacy-admin-refund").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_DISABLED", infraerrors.Reason(err))
}

func TestRefundEntrypointsUseEnabledImplementation(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := &PaymentService{entClient: client}

	err := svc.RequestRefund(ctx, 1, 2, "reason")
	require.Error(t, err)
	require.Equal(t, "NOT_FOUND", infraerrors.Reason(err))

	plan, result, err := svc.PrepareRefund(ctx, 1, 0, "reason", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "NOT_FOUND", infraerrors.Reason(err))
}

func TestPrepareRefundAllowsAdditionalPartialRefund(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-partial@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-partial-user").
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-partial-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(10).
		SetPayAmount(10).
		SetFeeRate(0).
		SetRechargeCode("REFUND-PARTIAL-ORDER").
		SetOutTradeNo("sub2_refund_partial_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-partial").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPartiallyRefunded).
		SetRefundAmount(4).
		SetRefundReason("partial-1").
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	plan, result, err := svc.PrepareRefund(ctx, order.ID, 3, "partial-2", false, false)
	require.NoError(t, err)
	require.Nil(t, result)
	require.NotNil(t, plan)
	require.Equal(t, 3.0, plan.RefundAmount)
	require.Equal(t, 7.0, plan.TotalRefundAmount)

	_, err = svc.markRefundOk(ctx, plan)
	require.NoError(t, err)
	refreshed, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPartiallyRefunded, refreshed.Status)
	require.Equal(t, 7.0, refreshed.RefundAmount)
}

func TestPrepareRefundUsesPaidAmountAsRefundCapForBalanceOrders(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-paid-cap@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-paid-cap-user").
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-paid-cap-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(75).
		SetPayAmount(50).
		SetFeeRate(0).
		SetCommissionBaseAmount(50).
		SetRechargeCode("REFUND-PAID-CAP-ORDER").
		SetOutTradeNo("sub2_refund_paid_cap_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-paid-cap").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 51, "too-much", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, "REFUND_AMOUNT_EXCEEDED", infraerrors.Reason(err))

	plan, result, err = svc.PrepareRefund(ctx, order.ID, 50, "full", false, false)
	require.NoError(t, err)
	require.Nil(t, result)
	require.NotNil(t, plan)
	require.Equal(t, 50.0, plan.RefundAmount)
	require.Equal(t, 50.0, plan.TotalRefundAmount)
	require.Equal(t, 33.33, plan.GatewayAmount)

	_, err = svc.markRefundOk(ctx, plan)
	require.NoError(t, err)

	refreshed, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefunded, refreshed.Status)
	require.Equal(t, 50.0, refreshed.RefundAmount)
}

func TestPrepareRefundCalculatesBalanceDeductionFromCreditedAmount(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-balance-deduction@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-balance-deduction-user").
		SetBalance(100).
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-balance-deduction-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(75).
		SetPayAmount(50).
		SetFeeRate(0).
		SetRechargeCode("REFUND-BALANCE-DEDUCTION-ORDER").
		SetOutTradeNo("sub2_refund_balance_deduction_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-balance-deduction").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
		userRepo:  &mockUserRepo{getByIDUser: &User{Balance: 100}},
	}

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 25, "half", false, true)
	require.NoError(t, err)
	require.Nil(t, result)
	require.NotNil(t, plan)
	require.Equal(t, payment.DeductionTypeBalance, plan.DeductionType)
	require.Equal(t, 37.5, plan.BalanceToDeduct)
}

func TestRequestRefundStoresPaidAmountForBalanceOrders(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-request-paid@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-request-paid-user").
		SetBalance(75).
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-request-paid-instance").
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
		SetAmount(75).
		SetPayAmount(50).
		SetFeeRate(0).
		SetRechargeCode("REFUND-REQUEST-PAID-ORDER").
		SetOutTradeNo("sub2_refund_request_paid_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-request-paid").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient: client,
		userRepo:  &mockUserRepo{getByIDUser: &User{Balance: 75}},
	}

	err = svc.RequestRefund(ctx, order.ID, user.ID, "full")
	require.NoError(t, err)

	refreshed, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRefundRequested, refreshed.Status)
	require.Equal(t, 50.0, refreshed.RefundAmount)
}

func TestPrepareRefundTreatsRefundRequestedAmountAsRequestedNotExecuted(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-requested@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-requested-user").
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-requested-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	requestedAt := time.Now()
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(1000).
		SetPayAmount(1000).
		SetFeeRate(0).
		SetRechargeCode("REFUND-REQUESTED-ORDER").
		SetOutTradeNo("sub2_refund_requested_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-requested").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefundRequested).
		SetRefundAmount(1000).
		SetRefundRequestReason("user requested full refund").
		SetRefundRequestedAt(requestedAt).
		SetRefundRequestedBy("42").
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetCompletedAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	plan, result, err := svc.PrepareRefund(ctx, order.ID, 400, "partial-after-request", false, false)
	require.NoError(t, err)
	require.Nil(t, result)
	require.NotNil(t, plan)
	require.Equal(t, 400.0, plan.RefundAmount)
	require.Equal(t, 400.0, plan.TotalRefundAmount)
}

func TestCustomReferralRefundBaseAmountUsesPaidAmountDenominator(t *testing.T) {
	order := &dbent.PaymentOrder{
		Amount:               75,
		PayAmount:            50,
		CommissionBaseAmount: 50,
	}

	require.Equal(t, 25.0, customReferralRefundBaseAmount(order, 25, 25))
	require.Equal(t, 50.0, customReferralRefundBaseAmount(order, 50, 50))
}

func TestMarkRefundOkCreatesDistinctAuditLogsForPartialRefunds(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-audit@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-audit-user").
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-audit-instance").
		SetConfig("{}").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(10).
		SetPayAmount(10).
		SetFeeRate(0).
		SetRechargeCode("REFUND-AUDIT-ORDER").
		SetOutTradeNo("sub2_refund_audit_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-audit").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}

	plan1, result1, err := svc.PrepareRefund(ctx, order.ID, 4, "partial-1", false, false)
	require.NoError(t, err)
	require.Nil(t, result1)
	_, err = svc.markRefundOk(ctx, plan1)
	require.NoError(t, err)

	plan2, result2, err := svc.PrepareRefund(ctx, order.ID, 3, "partial-2", false, false)
	require.NoError(t, err)
	require.Nil(t, result2)
	_, err = svc.markRefundOk(ctx, plan2)
	require.NoError(t, err)

	logs, err := svc.GetOrderAuditLogs(ctx, order.ID)
	require.NoError(t, err)
	require.Len(t, logs, 2)
	require.NotEqual(t, logs[0].Action, logs[1].Action)
	require.True(t, strings.HasPrefix(logs[0].Action, "REFUND_SUCCESS@"))
	require.True(t, strings.HasPrefix(logs[1].Action, "REFUND_SUCCESS@"))
}

func TestGwRefundRejectsAlipayMerchantIdentitySnapshotMismatch(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-snapshot-mismatch@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-snapshot-mismatch-user").
		Save(ctx)
	require.NoError(t, err)

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-mismatch-instance").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{
			"appId":      "runtime-alipay-app",
			"privateKey": "runtime-private-key",
		})).
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	instID := strconv.FormatInt(inst.ID, 10)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("REFUND-SNAPSHOT-MISMATCH-ORDER").
		SetOutTradeNo("sub2_refund_snapshot_mismatch_order").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-refund-snapshot-mismatch").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(instID).
		SetProviderKey(payment.TypeAlipay).
		SetProviderSnapshot(map[string]any{
			"schema_version":       2,
			"provider_instance_id": instID,
			"provider_key":         payment.TypeAlipay,
			"merchant_app_id":      "expected-alipay-app",
		}).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
	}

	err = svc.gwRefund(ctx, &RefundPlan{
		OrderID:       order.ID,
		Order:         order,
		RefundAmount:  order.Amount,
		GatewayAmount: order.Amount,
		Reason:        "snapshot mismatch",
	})
	require.ErrorContains(t, err, "alipay app_id mismatch")
}

func TestCalculateGatewayRefundAmountUsesCurrencyPrecision(t *testing.T) {
	require.InDelta(t, 6.173, calculateGatewayRefundAmount(100, 12.345, 50, "KWD"), 1e-12)
	require.InDelta(t, 12.345, calculateGatewayRefundAmount(100, 12.345, 100, "KWD"), 1e-12)
	require.InDelta(t, 52, calculateGatewayRefundAmount(100, 103, 50, "JPY"), 1e-12)
}

func TestFormatGatewayRefundAmountUsesOrderCurrency(t *testing.T) {
	order := &dbent.PaymentOrder{
		ProviderSnapshot: map[string]any{
			"currency": "KWD",
		},
	}

	require.Equal(t, "12.345", formatGatewayRefundAmount(12.345, order))
}

func TestValidateRefundProviderResponseAcceptsPending(t *testing.T) {
	require.NoError(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusPending}))
	require.NoError(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusSuccess}))
	require.Error(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusFailed}))
	require.Error(t, validateRefundProviderResponse(nil))
}
