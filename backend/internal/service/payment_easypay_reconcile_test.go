//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type easyPayReconcileFixture struct {
	client     *dbent.Client
	service    *PaymentService
	order      *dbent.PaymentOrder
	userRepo   *mockUserRepo
	redeemRepo *paymentOrderLifecycleRedeemRepo
}

func newEasyPayReconcileFixture(t *testing.T, handler http.HandlerFunc) *easyPayReconcileFixture {
	t.Helper()
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	user, err := client.User.Create().
		SetEmail("easypay-safety@example.com").
		SetPasswordHash("hash").
		SetUsername("easypay-safety-user").
		Save(ctx)
	require.NoError(t, err)
	instance, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeEasyPay).
		SetName("EasyPay safety test").
		SetConfig(encryptWebhookProviderConfig(t, map[string]string{
			"pid": "pid-safety", "pkey": "pkey-safety", "apiBase": server.URL,
			"notifyUrl": "https://example.com/notify", "returnUrl": "https://example.com/return",
		})).
		SetSupportedTypes("alipay").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(50).
		SetPayAmount(50).
		SetFeeRate(0).
		SetRechargeCode("EASYPAY-SAFETY").
		SetOutTradeNo("sub2_easypay_safety").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		SetProviderKey(payment.TypeEasyPay).
		SetProviderInstanceID(strconv.FormatInt(instance.ID, 10)).
		Save(ctx)
	require.NoError(t, err)

	userRepo := &mockUserRepo{getByIDUser: &User{
		ID: user.ID, Email: user.Email, Username: user.Username,
	}}
	userRepo.updateBalanceFn = func(_ context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		userRepo.getByIDUser.Balance += amount
		return nil
	}
	redeemRepo := &paymentOrderLifecycleRedeemRepo{codesByCode: map[string]*RedeemCode{
		order.RechargeCode: {
			ID: 1, Code: order.RechargeCode, Type: RedeemTypeBalance,
			Value: order.Amount, Status: StatusUnused,
		},
	}}
	redeemService := NewRedeemService(redeemRepo, userRepo, nil, nil, nil, client, nil, nil)
	service := &PaymentService{
		entClient:       client,
		loadBalancer:    newWebhookProviderTestLoadBalancer(client),
		redeemService:   redeemService,
		userRepo:        userRepo,
		providersLoaded: true,
	}
	return &easyPayReconcileFixture{
		client: client, service: service, order: order,
		userRepo: userRepo, redeemRepo: redeemRepo,
	}
}

func TestReconcilePendingEasyPayOrderOverlappingWebhookCreditsOnce(t *testing.T) {
	queryStarted := make(chan struct{})
	releaseQuery := make(chan struct{})
	var queryOnce sync.Once
	fixture := newEasyPayReconcileFixture(t, func(w http.ResponseWriter, r *http.Request) {
		queryOnce.Do(func() { close(queryStarted) })
		select {
		case <-releaseQuery:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"trade_status":"TRADE_SUCCESS","money":"50.00","trade_no":"easypay-overlap","out_trade_no":"sub2_easypay_safety","pid":"pid-safety"}`))
	})

	type reconcileResult struct {
		recovered int
		err       error
	}
	resultCh := make(chan reconcileResult, 1)
	go func() {
		recovered, err := fixture.service.ReconcilePendingEasyPayOrders(context.Background())
		resultCh <- reconcileResult{recovered: recovered, err: err}
	}()
	<-queryStarted

	require.NoError(t, fixture.service.HandlePaymentNotification(context.Background(), &payment.PaymentNotification{
		TradeNo: "easypay-overlap",
		OrderID: fixture.order.OutTradeNo,
		Amount:  50,
		Status:  payment.NotificationStatusSuccess,
		Metadata: map[string]string{
			"pid": "pid-safety",
		},
	}, payment.TypeEasyPay))
	close(releaseQuery)
	result := <-resultCh
	require.NoError(t, result.err)
	require.Zero(t, result.recovered, "the webhook won the race, so reconcile did not recover a second time")

	reloaded, err := fixture.client.PaymentOrder.Get(context.Background(), fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.Equal(t, 50.0, fixture.userRepo.getByIDUser.Balance)
	require.Len(t, fixture.redeemRepo.useCalls, 1)
}

func TestReconcilePendingEasyPayOrderLeavesExplicitUnpaidPending(t *testing.T) {
	fixture := newEasyPayReconcileFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"trade_status":"WAITING","money":"50.00"}`))
	})

	recovered, err := fixture.service.ReconcilePendingEasyPayOrders(context.Background())
	require.NoError(t, err)
	require.Zero(t, recovered)
	reloaded, err := fixture.client.PaymentOrder.Get(context.Background(), fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, reloaded.Status)
	require.Zero(t, fixture.userRepo.getByIDUser.Balance)
	require.Empty(t, fixture.redeemRepo.useCalls)
}

func TestReconcilePendingEasyPayOrderRejectsAmountMismatch(t *testing.T) {
	fixture := newEasyPayReconcileFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"trade_status":"TRADE_SUCCESS","money":"49.00","trade_no":"easypay-wrong-amount","out_trade_no":"sub2_easypay_safety","pid":"pid-safety"}`))
	})

	recovered, err := fixture.service.ReconcilePendingEasyPayOrders(context.Background())
	require.NoError(t, err)
	require.Zero(t, recovered)
	reloaded, err := fixture.client.PaymentOrder.Get(context.Background(), fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, reloaded.Status)
	require.Zero(t, fixture.userRepo.getByIDUser.Balance)
	require.Empty(t, fixture.redeemRepo.useCalls)

	logs, err := fixture.client.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(fixture.order.ID, 10)),
			paymentauditlog.ActionEQ("PAYMENT_AMOUNT_MISMATCH"),
		).
		All(context.Background())
	require.NoError(t, err)
	require.Len(t, logs, 1)
}

func TestReconcilePendingEasyPayOrderRejectsMismatchedOrderIdentity(t *testing.T) {
	fixture := newEasyPayReconcileFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"trade_status":"TRADE_SUCCESS","money":"50.00","trade_no":"easypay-wrong-order","out_trade_no":"another-order","pid":"pid-safety"}`))
	})

	recovered, err := fixture.service.ReconcilePendingEasyPayOrders(context.Background())
	require.NoError(t, err)
	require.Zero(t, recovered)
	reloaded, err := fixture.client.PaymentOrder.Get(context.Background(), fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, reloaded.Status)
	require.Zero(t, fixture.userRepo.getByIDUser.Balance)
	require.Empty(t, fixture.redeemRepo.useCalls)

	logs, err := fixture.client.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(fixture.order.ID, 10)),
			paymentauditlog.ActionEQ("PAYMENT_QUERY_IDENTITY_MISMATCH"),
		).
		All(context.Background())
	require.NoError(t, err)
	require.Len(t, logs, 1)
}

func TestReconcilePendingEasyPayOrderDoesNotClaimRecoveryBeforeFulfillment(t *testing.T) {
	fixture := newEasyPayReconcileFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"trade_status":"TRADE_SUCCESS","money":"50.00","trade_no":"easypay-fulfillment-failure","out_trade_no":"sub2_easypay_safety","pid":"pid-safety"}`))
	})
	fixture.userRepo.updateBalanceFn = func(context.Context, int64, float64) error {
		return errors.New("balance store unavailable")
	}

	recovered, err := fixture.service.ReconcilePendingEasyPayOrders(context.Background())
	require.NoError(t, err)
	require.Zero(t, recovered)

	reloaded, err := fixture.client.PaymentOrder.Get(context.Background(), fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusFailed, reloaded.Status)

	detected, err := fixture.client.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(fixture.order.ID, 10)),
			paymentauditlog.ActionEQ(paymentDetectedByReconcileAction),
		).
		Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, detected)
	completed, err := fixture.client.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(fixture.order.ID, 10)),
			paymentauditlog.ActionEQ(orderRecoveredByReconcileAction),
		).
		Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, completed)
}

func TestReconcilePendingEasyPayOrderRollsBackWhenDetectionAuditFails(t *testing.T) {
	fixture := newEasyPayReconcileFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"trade_status":"TRADE_SUCCESS","money":"50.00","trade_no":"easypay-audit-failure","out_trade_no":"sub2_easypay_safety","pid":"pid-safety"}`))
	})
	exec, ok := fixture.client.Driver().(refundNotificationSQLExecutor)
	require.True(t, ok)
	_, err := exec.ExecContext(context.Background(), `
		CREATE TRIGGER fail_recovery_audit
		BEFORE INSERT ON payment_audit_logs
			WHEN NEW.action = 'PAYMENT_DETECTED_BY_RECONCILE'
		BEGIN
			SELECT RAISE(FAIL, 'recovery audit unavailable');
		END;
	`)
	require.NoError(t, err)

	recovered, err := fixture.service.ReconcilePendingEasyPayOrders(context.Background())
	require.NoError(t, err)
	require.Zero(t, recovered)
	reloaded, err := fixture.client.PaymentOrder.Get(context.Background(), fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, reloaded.Status)
	require.Zero(t, fixture.userRepo.getByIDUser.Balance)
	require.Empty(t, fixture.redeemRepo.useCalls)
}

func TestReconcilePendingEasyPayOrderRetriesCompletionAuditWithoutDoubleCredit(t *testing.T) {
	fixture := newEasyPayReconcileFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"trade_status":"TRADE_SUCCESS","money":"50.00","trade_no":"easypay-completion-audit","out_trade_no":"sub2_easypay_safety","pid":"pid-safety"}`))
	})
	exec, ok := fixture.client.Driver().(refundNotificationSQLExecutor)
	require.True(t, ok)
	_, err := exec.ExecContext(context.Background(), `
		CREATE TRIGGER fail_recovery_completion_audit
		BEFORE INSERT ON payment_audit_logs
		WHEN NEW.action = 'ORDER_RECOVERED_BY_RECONCILE'
		BEGIN
			SELECT RAISE(FAIL, 'recovery completion audit unavailable');
		END;
	`)
	require.NoError(t, err)

	recovered, err := fixture.service.ReconcilePendingEasyPayOrders(context.Background())
	require.NoError(t, err)
	require.Zero(t, recovered)
	reloaded, err := fixture.client.PaymentOrder.Get(context.Background(), fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusFailed, reloaded.Status)
	require.Equal(t, 50.0, fixture.userRepo.getByIDUser.Balance)
	require.Len(t, fixture.redeemRepo.useCalls, 1)

	_, err = exec.ExecContext(context.Background(), `DROP TRIGGER fail_recovery_completion_audit`)
	require.NoError(t, err)
	require.NoError(t, fixture.service.ExecuteBalanceFulfillment(context.Background(), fixture.order.ID))
	reloaded, err = fixture.client.PaymentOrder.Get(context.Background(), fixture.order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.Equal(t, 50.0, fixture.userRepo.getByIDUser.Balance)
	require.Len(t, fixture.redeemRepo.useCalls, 1)

	logs, err := fixture.client.PaymentAuditLog.Query().
		Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(fixture.order.ID, 10)),
			paymentauditlog.ActionEQ(orderRecoveredByReconcileAction),
		).
		All(context.Background())
	require.NoError(t, err)
	require.Len(t, logs, 1)
}
