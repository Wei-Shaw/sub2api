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
REDACTED

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-instance").
		SetConfig("{REDACTED").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
REDACTED

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
REDACTED

	svc := &PaymentService{
		entClient: client,
REDACTED

	_, err = svc.validateRefundRequest(ctx, order.ID, user.ID)
REDACTED
	require.Equal(t, "USER_REFUND_DISABLED", infraerrors.Reason(err))
REDACTED

func TestPrepareRefundRejectsLegacyGuessedProviderInstance(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-legacy-admin@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-legacy-admin-user").
		Save(ctx)
REDACTED

	_, err = client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-admin-instance").
		SetConfig("{REDACTED").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetAllowUserRefund(true).
		SetRefundEnabled(true).
		Save(ctx)
REDACTED

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
REDACTED

	svc := &PaymentService{
		entClient: client,
REDACTED

	plan, result, err := svc.PrepareRefund(ctx, order.ID, 0, "", false, false)
	require.Nil(t, plan)
	require.Nil(t, result)
REDACTED
	require.Equal(t, "REFUND_DISABLED", infraerrors.Reason(err))
REDACTED

func TestGwRefundRejectsAlipayMerchantIdentitySnapshotMismatch(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-snapshot-mismatch@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-snapshot-mismatch-user").
		Save(ctx)
REDACTED

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeAlipay).
		SetName("alipay-refund-mismatch-instance").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{
			"appId":      "runtime-alipay-app",
			"privateKey": "runtime-private-key",
	REDACTED)).
		SetSupportedTypes("alipay").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
REDACTED

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
	REDACTED).
		Save(ctx)
REDACTED

	svc := &PaymentService{
		entClient:    client,
		loadBalancer: newWebhookProviderTestLoadBalancer(client),
REDACTED

	_, err = svc.gwRefund(ctx, &RefundPlan{
		OrderID:       order.ID,
		Order:         order,
		RefundAmount:  order.Amount,
		GatewayAmount: order.Amount,
		Reason:        "snapshot mismatch",
REDACTED)
	require.ErrorContains(t, err, "alipay app_id mismatch")
REDACTED

func TestCalculateGatewayRefundAmountUsesCurrencyPrecision(t *testing.T) {
	require.InDelta(t, 6.173, calculateGatewayRefundAmount(100, 12.345, 50, "KWD"), 1e-12)
	require.InDelta(t, 12.345, calculateGatewayRefundAmount(100, 12.345, 100, "KWD"), 1e-12)
	require.InDelta(t, 52, calculateGatewayRefundAmount(100, 103, 50, "JPY"), 1e-12)
REDACTED

func TestFormatGatewayRefundAmountUsesOrderCurrency(t *testing.T) {
	order := &dbent.PaymentOrder{
		ProviderSnapshot: map[string]any{
			"currency": "KWD",
	REDACTED,
REDACTED

	require.Equal(t, "12.345", formatGatewayRefundAmount(12.345, order))
REDACTED

func TestValidateRefundProviderResponseAcceptsPending(t *testing.T) {
	require.NoError(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusPendingREDACTED))
	require.NoError(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusSuccessREDACTED))
	require.Error(t, validateRefundProviderResponse(&payment.RefundResponse{Status: payment.ProviderStatusFailedREDACTED))
	require.Error(t, validateRefundProviderResponse(nil))
REDACTED

func TestFinishRefundPendingMarksOrderPendingAndRollsBackDeduction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("refund-pending@example.com").
		SetPasswordHash("hash").
		SetUsername("refund-pending-user").
		Save(ctx)
REDACTED

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode("REFUND-PENDING-ORDER").
		SetOutTradeNo("sub2_refund_pending_order").
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("pi_refund_pending").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefunding).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
REDACTED

	var rolledBack float64
	userRepo := &mockUserRepo{REDACTED
	userRepo.updateBalanceFn = func(ctx context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		rolledBack += amount
		return nil
REDACTED
	svc := &PaymentService{
		entClient: client,
		userRepo:  userRepo,
REDACTED
	plan := &RefundPlan{
		OrderID:         order.ID,
		Order:           order,
		RefundAmount:    40,
		GatewayAmount:   40,
		Reason:          "gateway accepted but not final",
		Force:           true,
		DeductionType:   payment.DeductionTypeBalance,
		BalanceToDeduct: 40,
REDACTED

	result, err := svc.finishRefund(ctx, plan, &payment.RefundResponse{Status: payment.ProviderStatusPendingREDACTED)
REDACTED
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Contains(t, result.Warning, "pending confirmation")
	require.Equal(t, 40.0, rolledBack)
	require.Zero(t, plan.BalanceToDeduct)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
REDACTED
	require.Equal(t, OrderStatusRefundPending, reloaded.Status)
	require.Equal(t, 40.0, reloaded.RefundAmount)
	require.NotNil(t, reloaded.RefundReason)
	require.Equal(t, "gateway accepted but not final", *reloaded.RefundReason)
	require.Nil(t, reloaded.RefundAt)

	pendingAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
		Count(ctx)
REDACTED
	require.Equal(t, 1, pendingAudits)
	successAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
		Count(ctx)
REDACTED
	require.Zero(t, successAudits)
REDACTED

func TestFinishRefundSuccessStatusesFinalize(t *testing.T) {
	for _, status := range []string{payment.ProviderStatusSuccess, payment.ProviderStatusRefundedREDACTED {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)

			user, err := client.User.Create().
				SetEmail("refund-success-" + status + "@example.com").
				SetPasswordHash("hash").
				SetUsername("refund-success-" + status).
				Save(ctx)
		REDACTED

			order, err := client.PaymentOrder.Create().
				SetUserID(user.ID).
				SetUserEmail(user.Email).
				SetUserName(user.Username).
				SetAmount(100).
				SetPayAmount(100).
				SetFeeRate(0).
				SetRechargeCode("REFUND-SUCCESS-" + status).
				SetOutTradeNo("sub2_refund_success_" + status).
				SetPaymentType(payment.TypeStripe).
				SetPaymentTradeNo("pi_refund_success_" + status).
				SetOrderType(payment.OrderTypeBalance).
				SetStatus(OrderStatusRefunding).
				SetExpiresAt(time.Now().Add(time.Hour)).
				SetPaidAt(time.Now()).
				SetClientIP("127.0.0.1").
				SetSrcHost("api.example.com").
				Save(ctx)
		REDACTED

			svc := &PaymentService{entClient: clientREDACTED
			plan := &RefundPlan{
				OrderID:         order.ID,
				Order:           order,
				RefundAmount:    100,
				GatewayAmount:   100,
				Reason:          "final success",
				DeductionType:   payment.DeductionTypeBalance,
				BalanceToDeduct: 100,
		REDACTED

			result, err := svc.finishRefund(ctx, plan, &payment.RefundResponse{Status: statusREDACTED)
		REDACTED
			require.NotNil(t, result)
			require.True(t, result.Success)
			require.Equal(t, 100.0, result.BalanceDeducted)

			reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
		REDACTED
			require.Equal(t, OrderStatusRefunded, reloaded.Status)
			require.NotNil(t, reloaded.RefundAt)

			successAudits, err := client.PaymentAuditLog.Query().
				Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_SUCCESS")).
				Count(ctx)
		REDACTED
			require.Equal(t, 1, successAudits)
			pendingAudits, err := client.PaymentAuditLog.Query().
				Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_PENDING")).
				Count(ctx)
		REDACTED
			require.Zero(t, pendingAudits)
	REDACTED)
REDACTED
REDACTED

func TestQueryAndFinalizeRefundFinalizesProviderStatuses(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     string
		wantStatus string
		wantDeduct float64
REDACTED{
		{name: "success", status: payment.ProviderStatusSuccess, wantStatus: OrderStatusRefunded, wantDeduct: 100REDACTED,
		{name: "failed", status: payment.ProviderStatusFailed, wantStatus: OrderStatusRefundFailedREDACTED,
		{name: "pending", status: payment.ProviderStatusPending, wantStatus: OrderStatusRefundPendingREDACTED,
REDACTED {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-"+tc.name)

			var deducted float64
			svc := &PaymentService{
				entClient:    client,
				loadBalancer: &captureLoadBalancer{REDACTED,
				userRepo: &mockUserRepo{deductBalanceFn: func(ctx context.Context, id int64, amount float64) error {
					deducted += amount
					return nil
		REDACTED
		REDACTED
			restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
				refundResponse: &payment.RefundResponse{RefundID: "rf_test", Status: tc.statusREDACTED,
		REDACTED)
			defer restore()

			result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
		REDACTED
			require.NotNil(t, result)
			require.Equal(t, tc.status == payment.ProviderStatusSuccess, result.Success)
			require.Equal(t, tc.wantDeduct, deducted)

			reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
		REDACTED
			require.Equal(t, tc.wantStatus, reloaded.Status)
	REDACTED)
REDACTED
REDACTED

func TestQueryAndFinalizeRefundUnsupportedProviderReturnsClearError(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-finalize-unsupported")
	svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{REDACTEDREDACTED
	restore := replacePaymentProviderFactoryForTest(t, refundProviderTestDouble{REDACTED)
	defer restore()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.Nil(t, result)
REDACTED
	require.Equal(t, "REFUND_QUERY_UNSUPPORTED", infraerrors.Reason(err))
REDACTED

func createPendingRefundOrderForTest(t *testing.T, ctx context.Context, client *dbent.Client, suffix string) *dbent.PaymentOrder {
REDACTED

	user, err := client.User.Create().
		SetEmail(suffix + "@example.com").
		SetPasswordHash("hash").
		SetUsername(suffix).
		Save(ctx)
REDACTED

	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).
		SetName(suffix + "-provider").
		SetConfig("{REDACTED").
		SetSupportedTypes("stripe").
		SetEnabled(true).
		SetRefundEnabled(true).
		Save(ctx)
REDACTED

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(100).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode("REFUND-" + suffix).
		SetOutTradeNo("sub2_" + suffix).
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo("pi_" + suffix).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusRefundPending).
		SetRefundAmount(100).
		SetRefundReason("pending refund").
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).
		Save(ctx)
REDACTED

	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("REFUND_PENDING").
		SetOperator("admin").
		SetDetail(`{"refundID":"rf_test","deductionRollbackOK":trueREDACTED`).
		Save(ctx)
REDACTED
	return order
REDACTED

func replacePaymentProviderFactoryForTest(t *testing.T, prov payment.Provider) func() {
REDACTED
	original := createPaymentProviderFromInstance
	createPaymentProviderFromInstance = func(providerKey, instanceID string, config map[string]string) (payment.Provider, error) {
		return prov, nil
REDACTED
	return func() { createPaymentProviderFromInstance = original REDACTED
REDACTED

type refundProviderTestDouble struct{REDACTED

func (refundProviderTestDouble) Name() string { return "refund-test" REDACTED
func (refundProviderTestDouble) ProviderKey() string {
	return payment.TypeStripe
REDACTED
func (refundProviderTestDouble) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeStripeREDACTED
REDACTED
func (refundProviderTestDouble) CreatePayment(context.Context, payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return nil, nil
REDACTED
func (refundProviderTestDouble) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	return nil, nil
REDACTED
func (refundProviderTestDouble) VerifyNotification(context.Context, string, map[string]string) (*payment.PaymentNotification, error) {
	return nil, nil
REDACTED
func (refundProviderTestDouble) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, nil
REDACTED

type refundQueryProviderTestDouble struct {
	refundProviderTestDouble
	refundResponse *payment.RefundResponse
REDACTED

func (p *refundQueryProviderTestDouble) QueryRefund(context.Context, payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	return p.refundResponse, nil
REDACTED
