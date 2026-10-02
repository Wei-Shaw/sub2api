//go:build unit

package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

// A refund that went to the gateway's pending state must settle with the
// administrator's original deduction choice, not a hard-coded "deduct".
func TestRefundPendingConfirmationHonorsNoDeductChoice(t *testing.T) {
	t.Run("balance", func(t *testing.T) {
		ctx := context.Background()
		client := newPaymentConfigServiceTestClient(t)
		order := createLedgerRefundOrder(t, ctx, client, "ledger-no-deduct-balance", payment.OrderTypeBalance, 0)
		var deducted float64
		repo := &mockUserRepo{
			getByIDUser: &User{ID: order.UserID, Balance: 100},
			deductAvailableBalanceFn: func(_ context.Context, _ int64, amount float64) (float64, error) {
				deducted += amount
				return amount, nil
			},
		}
		svc := &PaymentService{entClient: client, userRepo: repo, loadBalancer: &captureLoadBalancer{}}
		prov := &ledgerRefundProvider{
			refundResp: &payment.RefundResponse{RefundID: "rf_ledger", Status: payment.ProviderStatusPending},
			queryResp:  &payment.RefundResponse{RefundID: "rf_ledger", Status: payment.ProviderStatusSuccess},
		}
		defer replacePaymentProviderFactoryForTest(t, prov)()

		plan, early, err := svc.PrepareRefund(ctx, order.ID, 0, "keep balance", false, false)
		require.NoError(t, err)
		require.Nil(t, early)
		pending, err := svc.ExecuteRefund(ctx, plan)
		require.NoError(t, err)
		require.False(t, pending.Success)
		requireLedgerOrderStatus(t, ctx, client, order.ID, OrderStatusRefundPending)

		final, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
		require.NoError(t, err)
		require.True(t, final.Success)
		require.Zero(t, deducted, "the administrator chose not to deduct the balance")
		require.Zero(t, final.BalanceDeducted)
		requireLedgerOrderStatus(t, ctx, client, order.ID, OrderStatusRefunded)
	})

	t.Run("subscription", func(t *testing.T) {
		ctx := context.Background()
		client := newPaymentConfigServiceTestClient(t)
		order := createLedgerRefundOrder(t, ctx, client, "ledger-no-deduct-sub", payment.OrderTypeSubscription, 30)
		now := time.Now().UTC().Truncate(time.Second)
		expiry := now.AddDate(0, 0, 40)
		subRepo := newLedgerSubRepo(UserSubscription{ID: 501, UserID: order.UserID, GroupID: *order.SubscriptionGroupID, ExpiresAt: expiry, Status: SubscriptionStatusActive})
		svc := &PaymentService{
			entClient: client, userRepo: &mockUserRepo{}, loadBalancer: &captureLoadBalancer{},
			subscriptionSvc: &SubscriptionService{userSubRepo: subRepo, now: func() time.Time { return now }},
		}
		prov := &ledgerRefundProvider{
			refundResp: &payment.RefundResponse{RefundID: "rf_ledger_sub", Status: payment.ProviderStatusPending},
			queryResp:  &payment.RefundResponse{RefundID: "rf_ledger_sub", Status: payment.ProviderStatusSuccess},
		}
		defer replacePaymentProviderFactoryForTest(t, prov)()

		plan, early, err := svc.PrepareRefund(ctx, order.ID, 0, "keep days", false, false)
		require.NoError(t, err)
		require.Nil(t, early)
		_, err = svc.ExecuteRefund(ctx, plan)
		require.NoError(t, err)
		requireLedgerOrderStatus(t, ctx, client, order.ID, OrderStatusRefundPending)

		final, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
		require.NoError(t, err)
		require.True(t, final.Success)
		require.Zero(t, final.SubDaysDeducted)
		sub := subRepo.current(501)
		require.True(t, sub.ExpiresAt.Equal(expiry), "the administrator chose not to deduct subscription days")
		require.Equal(t, SubscriptionStatusActive, sub.Status)
	})
}

// A deduction that would end the subscription must stay reversible: when the
// gateway then rejects the refund the whole subscription comes back.
func TestSubscriptionRefundGatewayFailureKeepsSubscription(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createLedgerRefundOrder(t, ctx, client, "ledger-sub-gateway-fail", payment.OrderTypeSubscription, 30)
	now := time.Now().UTC().Truncate(time.Second)
	expiry := now.AddDate(0, 0, 10)
	subRepo := newLedgerSubRepo(UserSubscription{ID: 601, UserID: order.UserID, GroupID: *order.SubscriptionGroupID, ExpiresAt: expiry, Status: SubscriptionStatusActive})
	svc := &PaymentService{
		entClient: client, userRepo: &mockUserRepo{}, loadBalancer: &captureLoadBalancer{},
		subscriptionSvc: &SubscriptionService{userSubRepo: subRepo, now: func() time.Time { return now }},
	}
	defer replacePaymentProviderFactoryForTest(t, &ledgerRefundProvider{refundErr: errors.New("gateway rejected refund")})()

	plan, early, err := svc.PrepareRefund(ctx, order.ID, 0, "refund subscription", false, true)
	require.NoError(t, err)
	require.Nil(t, early)
	require.Equal(t, int64(601), plan.SubscriptionID)

	result, err := svc.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Warning, "rolled back")

	require.False(t, subRepo.wasDeleted(601), "a refund must never delete the subscription row")
	sub := subRepo.current(601)
	require.True(t, sub.ExpiresAt.Equal(expiry), "rollback must give back the removed days")
	require.Equal(t, SubscriptionStatusActive, sub.Status)
	requireLedgerOrderStatus(t, ctx, client, order.ID, OrderStatusCompleted)
}

// When the pending-time compensation failed the balance is still deducted; a
// final gateway failure must return it, and a later retry must deduct again.
func TestPendingRefundFinalFailureReturnsOutstandingDeduction(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createLedgerRefundOrder(t, ctx, client, "ledger-final-fail", payment.OrderTypeBalance, 0)
	var mu sync.Mutex
	balance := 100.0
	failRestore := true
	repo := &mockUserRepo{
		getByIDUser: &User{ID: order.UserID, Balance: 100},
		deductAvailableBalanceFn: func(_ context.Context, _ int64, amount float64) (float64, error) {
			mu.Lock()
			defer mu.Unlock()
			balance -= amount
			return amount, nil
		},
		updateBalanceFn: func(_ context.Context, _ int64, amount float64) error {
			mu.Lock()
			defer mu.Unlock()
			if failRestore {
				return errors.New("balance restore unavailable")
			}
			balance += amount
			return nil
		},
	}
	svc := &PaymentService{entClient: client, userRepo: repo, loadBalancer: &captureLoadBalancer{}}
	prov := &ledgerRefundProvider{refundResp: &payment.RefundResponse{RefundID: "rf_ledger_fail", Status: payment.ProviderStatusPending}}
	defer replacePaymentProviderFactoryForTest(t, prov)()

	plan, early, err := svc.PrepareRefund(ctx, order.ID, 0, "final failure", false, true)
	require.NoError(t, err)
	require.Nil(t, early)
	pending, err := svc.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	require.Contains(t, pending.Warning, "rollback failed")
	require.Equal(t, 0.0, balance)
	requireLedgerOrderStatus(t, ctx, client, order.ID, OrderStatusRefundPending)

	failRestore = false
	prov.queryResp = &payment.RefundResponse{RefundID: "rf_ledger_fail", Status: payment.ProviderStatusFailed}
	failed, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.False(t, failed.Success)
	require.Equal(t, 100.0, balance, "the still-deducted balance must be returned on final failure")
	requireLedgerOrderStatus(t, ctx, client, order.ID, OrderStatusRefundFailed)

	prov.refundResp = &payment.RefundResponse{RefundID: "rf_ledger_retry", Status: payment.ProviderStatusSuccess}
	retryPlan, early, err := svc.PrepareRefund(ctx, order.ID, 0, "retry", false, true)
	require.NoError(t, err)
	require.Nil(t, early)
	retried, err := svc.ExecuteRefund(ctx, retryPlan)
	require.NoError(t, err)
	require.True(t, retried.Success)
	require.Equal(t, 0.0, balance, "the returned balance must be deducted again by the retry")
}

// The final-failure write must not overwrite a refund that another request
// finalized in the meantime, and must report the conflict.
func TestPendingRefundFinalFailureDoesNotOverwriteConcurrentFinalization(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createLedgerPendingOrder(t, ctx, client, "ledger-final-race", `{"refundID":"rf_test","deductionRollbackOK":true}`)
	svc := &PaymentService{entClient: client, userRepo: &mockUserRepo{}, loadBalancer: &captureLoadBalancer{}}
	prov := &ledgerRefundProvider{
		queryResp: &payment.RefundResponse{RefundID: "rf_test", Status: payment.ProviderStatusFailed},
		onQuery: func() {
			_, err := client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusRefunded).Save(ctx)
			require.NoError(t, err)
		},
	}
	defer replacePaymentProviderFactoryForTest(t, prov)()

	_, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.Error(t, err)
	requireLedgerOrderStatus(t, ctx, client, order.ID, OrderStatusRefunded)
	failedAudits, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("REFUND_FAILED")).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, failedAudits)
}

// An unreadable pending audit must stop settlement instead of being treated
// as "compensation succeeded", which would deduct the balance a second time.
func TestQueryAndFinalizeRefundRejectsUnreadablePendingAudit(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createLedgerPendingOrder(t, ctx, client, "ledger-bad-audit", `{"refundID":"rf_test","deductionRollbackOK":false,`)
	deductions := 0
	svc := &PaymentService{
		entClient: client, loadBalancer: &captureLoadBalancer{},
		userRepo: &mockUserRepo{deductAvailableBalanceFn: func(_ context.Context, _ int64, amount float64) (float64, error) {
			deductions++
			return amount, nil
		}},
	}
	defer replacePaymentProviderFactoryForTest(t, &ledgerRefundProvider{
		queryResp: &payment.RefundResponse{RefundID: "rf_test", Status: payment.ProviderStatusSuccess},
	})()

	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.Error(t, err)
	require.Nil(t, result)
	require.Zero(t, deductions)
	requireLedgerOrderStatus(t, ctx, client, order.ID, OrderStatusRefundPending)
}

// A subscription refund that went pending gives the days back while waiting
// and takes the same interval again on confirmation, keeping a renewal bought
// in between.
func TestSubscriptionRefundPendingConfirmationReplaysRemovedInterval(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createLedgerRefundOrder(t, ctx, client, "ledger-sub-pending", payment.OrderTypeSubscription, 30)
	now := time.Now().UTC().Truncate(time.Second)
	subRepo := newLedgerSubRepo(UserSubscription{ID: 701, UserID: order.UserID, GroupID: *order.SubscriptionGroupID, ExpiresAt: now.AddDate(0, 0, 10), Status: SubscriptionStatusActive})
	svc := &PaymentService{
		entClient: client, userRepo: &mockUserRepo{}, loadBalancer: &captureLoadBalancer{},
		subscriptionSvc: &SubscriptionService{userSubRepo: subRepo, now: func() time.Time { return now }},
	}
	prov := &ledgerRefundProvider{
		refundResp: &payment.RefundResponse{RefundID: "rf_sub_pending", Status: payment.ProviderStatusPending},
		queryResp:  &payment.RefundResponse{RefundID: "rf_sub_pending", Status: payment.ProviderStatusSuccess},
	}
	defer replacePaymentProviderFactoryForTest(t, prov)()

	plan, early, err := svc.PrepareRefund(ctx, order.ID, 0, "refund subscription", false, true)
	require.NoError(t, err)
	require.Nil(t, early)
	_, err = svc.ExecuteRefund(ctx, plan)
	require.NoError(t, err)
	waiting := subRepo.current(701)
	require.True(t, waiting.ExpiresAt.Equal(now.AddDate(0, 0, 10)), "pending refund gives the days back while waiting")
	require.Equal(t, SubscriptionStatusActive, waiting.Status)

	require.NoError(t, subRepo.ExtendExpiry(ctx, 701, now.AddDate(0, 0, 40)))
	final, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.True(t, final.Success)
	settled := subRepo.current(701)
	require.True(t, settled.ExpiresAt.Equal(now.AddDate(0, 0, 30)), "the renewal survives settlement: %s", settled.ExpiresAt)
	require.Equal(t, SubscriptionStatusActive, settled.Status)
	require.False(t, subRepo.wasDeleted(701))
}

// Audits written before the choice was recorded settle from the amount they
// actually rolled back, so a pending "no deduction" refund stays undeducted.
func TestRefundPendingLegacyAuditSettlesRolledBackAmount(t *testing.T) {
	for _, tc := range []struct {
		name   string
		detail string
		want   float64
	}{
		{name: "nothing was deducted", detail: `{"refundID":"rf_test","balanceDeducted":0,"balanceRolledBack":0,"subDaysRolledBack":0,"deductionRollbackOK":true}`, want: 0},
		{name: "a clamped deduction", detail: `{"refundID":"rf_test","balanceDeducted":0,"balanceRolledBack":35,"subDaysRolledBack":0,"deductionRollbackOK":true}`, want: 35},
		{name: "no rolled back amount", detail: `{"refundID":"rf_test","deductionRollbackOK":true}`, want: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			order := createLedgerPendingOrder(t, ctx, client, "ledger-legacy-"+strconv.Itoa(int(tc.want)), tc.detail)
			var deducted float64
			svc := &PaymentService{
				entClient: client, loadBalancer: &captureLoadBalancer{},
				userRepo: &mockUserRepo{deductAvailableBalanceFn: func(_ context.Context, _ int64, amount float64) (float64, error) {
					deducted += amount
					return amount, nil
				}},
			}
			defer replacePaymentProviderFactoryForTest(t, &ledgerRefundProvider{
				queryResp: &payment.RefundResponse{RefundID: "rf_test", Status: payment.ProviderStatusSuccess},
			})()

			result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
			require.NoError(t, err)
			require.True(t, result.Success)
			require.Equal(t, tc.want, deducted)
		})
	}
}

func createLedgerRefundOrder(t *testing.T, ctx context.Context, client *dbent.Client, suffix, orderType string, subscriptionDays int) *dbent.PaymentOrder {
	t.Helper()
	user, err := client.User.Create().SetEmail(suffix + "@example.com").SetPasswordHash("hash").SetUsername(suffix).Save(ctx)
	require.NoError(t, err)
	inst, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeStripe).SetName(suffix + "-provider").SetConfig("{}").
		SetSupportedTypes("stripe").SetEnabled(true).SetRefundEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	create := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(100).SetPayAmount(100).SetFeeRate(0).
		SetRechargeCode("REFUND-" + suffix).SetOutTradeNo("sub2_" + suffix).
		SetPaymentType(payment.TypeStripe).SetPaymentTradeNo("pi_" + suffix).
		SetOrderType(orderType).SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).SetPaidAt(time.Now()).
		SetClientIP("127.0.0.1").SetSrcHost("api.example.com").
		SetProviderInstanceID(strconv.FormatInt(inst.ID, 10)).SetProviderKey(payment.TypeStripe)
	if orderType == payment.OrderTypeSubscription {
		create = create.SetSubscriptionGroupID(77).SetSubscriptionDays(subscriptionDays)
	}
	order, err := create.Save(ctx)
	require.NoError(t, err)
	return order
}

func createLedgerPendingOrder(t *testing.T, ctx context.Context, client *dbent.Client, suffix, pendingDetail string) *dbent.PaymentOrder {
	t.Helper()
	order := createLedgerRefundOrder(t, ctx, client, suffix, payment.OrderTypeBalance, 0)
	order, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetStatus(OrderStatusRefundPending).SetRefundAmount(100).SetRefundReason("pending refund").
		Save(ctx)
	require.NoError(t, err)
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).SetAction("REFUND_PENDING").SetOperator("admin").
		SetDetail(pendingDetail).
		Save(ctx)
	require.NoError(t, err)
	return order
}

func requireLedgerOrderStatus(t *testing.T, ctx context.Context, client *dbent.Client, orderID int64, status string) {
	t.Helper()
	reloaded, err := client.PaymentOrder.Get(ctx, orderID)
	require.NoError(t, err)
	require.Equal(t, status, reloaded.Status)
}

type ledgerRefundProvider struct {
	refundProviderTestDouble
	refundResp *payment.RefundResponse
	refundErr  error
	queryResp  *payment.RefundResponse
	onQuery    func()
}

func (p *ledgerRefundProvider) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	if p.refundErr != nil {
		return nil, p.refundErr
	}
	return p.refundResp, nil
}

func (p *ledgerRefundProvider) QueryRefund(context.Context, payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	if p.onQuery != nil {
		p.onQuery()
	}
	return p.queryResp, nil
}

// ledgerSubRepo is an in-memory subscription table with soft deletion, enough
// to observe whether a refund shortened, expired, restored or deleted a row.
type ledgerSubRepo struct {
	userSubRepoNoop
	mu      sync.Mutex
	rows    map[int64]UserSubscription
	deleted map[int64]bool
}

func newLedgerSubRepo(subs ...UserSubscription) *ledgerSubRepo {
	r := &ledgerSubRepo{rows: map[int64]UserSubscription{}, deleted: map[int64]bool{}}
	for _, sub := range subs {
		r.rows[sub.ID] = sub
	}
	return r
}

func (r *ledgerSubRepo) current(id int64) UserSubscription {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rows[id]
}

func (r *ledgerSubRepo) wasDeleted(id int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.deleted[id]
}

func (r *ledgerSubRepo) live(id int64) (*UserSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sub, ok := r.rows[id]
	if !ok || r.deleted[id] {
		return nil, ErrSubscriptionNotFound
	}
	return &sub, nil
}

func (r *ledgerSubRepo) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	return r.live(id)
}

func (r *ledgerSubRepo) GetByIDForUpdate(_ context.Context, id int64) (*UserSubscription, error) {
	return r.live(id)
}

func (r *ledgerSubRepo) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, sub := range r.rows {
		if !r.deleted[id] && sub.UserID == userID && sub.GroupID == groupID && sub.Status == SubscriptionStatusActive {
			cp := sub
			return &cp, nil
		}
	}
	return nil, ErrSubscriptionNotFound
}

func (r *ledgerSubRepo) ExtendExpiry(_ context.Context, id int64, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	sub, ok := r.rows[id]
	if !ok || r.deleted[id] {
		return ErrSubscriptionNotFound
	}
	sub.ExpiresAt = expiresAt
	r.rows[id] = sub
	return nil
}

func (r *ledgerSubRepo) UpdateStatus(_ context.Context, id int64, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	sub, ok := r.rows[id]
	if !ok || r.deleted[id] {
		return ErrSubscriptionNotFound
	}
	sub.Status = status
	r.rows[id] = sub
	return nil
}

func (r *ledgerSubRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleted[id] = true
	return nil
}
